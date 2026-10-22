package makino

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/makino/masterdata"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func runMaster(ctx context.Context, serverURL string, args []string, stdout, stderr io.Writer) (retErr error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		masterUsage(stdout)
		return nil
	}
	command := args[0]
	if command != "export" && command != "import" && command != "reset" && command != "status" {
		return errors.New("unknown master command; expected export, import, reset, or status")
	}
	flags := flag.NewFlagSet("makino master "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { masterUsage(stderr) }
	var file, directory, masterType, id string
	var all, force, dryRun, allowEmpty bool
	if command == "status" {
		flags.StringVar(&id, "id", "", "master publication job ID")
	} else if command == "reset" {
		flags.StringVar(&masterType, "type", "", "active master type to restore to the official baseline")
		flags.BoolVar(&all, "all", false, "restore every active master type to the official baseline")
		flags.BoolVar(&dryRun, "dry-run", false, "validate original master files without publishing")
	} else {
		flags.StringVar(&file, "file", "", "YAML input/output file")
		flags.StringVar(&directory, "dir", "", "directory containing master YAML files")
		flags.BoolVar(&all, "all", false, "require every active master type")
		if command == "export" {
			flags.StringVar(&masterType, "type", "", "active master type")
			flags.BoolVar(&force, "force", false, "overwrite existing YAML files")
		} else {
			flags.BoolVar(&dryRun, "dry-run", false, "validate and encrypt without publishing")
			flags.BoolVar(&allowEmpty, "allow-empty", false, "allow emptying currently nonempty master collections")
		}
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if command == "status" {
		if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
			return errors.New("--id requires a publication job UUID")
		}
	}
	if command != "status" && file != "" && directory != "" {
		return errors.New("choose exactly one of --file or --dir")
	}
	if command == "import" && file == "" && directory == "" {
		return errors.New("choose exactly one of --file or --dir")
	}
	if command == "export" && all == (masterType != "") {
		return errors.New("export requires exactly one of --type or --all")
	}
	if command == "reset" && all == (masterType != "") {
		return errors.New("reset requires exactly one of --type or --all")
	}
	if command == "export" && all && directory == "" {
		return errors.New("--all exports require --dir; bulk export to stdout is not supported")
	}
	var paths []string
	if command == "import" {
		if all && file != "" {
			return errors.New("--all imports require --dir")
		}
		var err error
		paths, err = masterInputPaths(file, directory)
		if err != nil {
			return err
		}
	}
	var owner *masterdata.Ownership
	if command == "import" || command == "reset" || command == "status" {
		var err error
		owner, err = masterdata.AcquireOwnership()
		if err != nil && !errors.Is(err, masterdata.ErrOwnershipBusy) {
			return err
		}
		// A free OS lock proves no participating server can be running or
		// starting. A busy lock always chooses HTTP; network errors never grant
		// permission to bypass the live server's coordinator.
		if owner == nil {
			if command == "status" {
				return getMasterStatus(ctx, serverURL, id, stdout)
			}
			if !dryRun {
				if _, err := config.GetAdminAuth(); err != nil {
					return err
				}
			}
		}
		// Registered before Disconnect so the ownership lock remains held until
		// MongoDB sessions close, including an interrupted publication.
		defer func() { retErr = errors.Join(retErr, owner.Close()) }()
	}
	if err := db.Connect(); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Disconnect()) }()
	if command == "status" {
		job, err := opsolis.FindMasterImportJob(ctx, id)
		if err != nil {
			return fmt.Errorf("read offline master publication status: %w", err)
		}
		return json.NewEncoder(stdout).Encode(job)
	}
	if command == "export" {
		options := masterdata.ExportOptions{Type: masterType, File: file, Directory: directory, All: all, Force: force}
		if file == "" && directory == "" {
			options.Output = stdout
			return masterdata.Export(ctx, options)
		}
		if err := masterdata.Export(ctx, options); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, "master export complete")
		return err
	}
	if owner != nil && !dryRun {
		if err := owner.Recover(ctx); err != nil {
			return fmt.Errorf("recover interrupted master publication: %w", err)
		}
	}
	var request *masterdata.ImportRequest
	var err error
	if command == "reset" {
		request, err = masterdata.PrepareReset(ctx, masterdata.ResetOptions{Type: masterType, All: all})
	} else {
		request, err = masterdata.PrepareImport(ctx, masterdata.ImportOptions{Files: paths, All: all, AllowEmpty: allowEmpty})
	}
	if err != nil {
		return err
	}
	if dryRun || len(request.Packs) == 0 {
		if !request.Reset {
			if err := masterdata.RemoveStaging(request.ID); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(stdout, "master validation complete: %d changed pack(s); no publication requested\n", len(request.Packs))
		return err
	}
	if owner != nil {
		fmt.Fprintf(stderr, "server is stopped; publishing locally (job %s)\n", request.ID)
		job, publishErr := owner.Publish(ctx, *request)
		var outputErr error
		if job != nil {
			outputErr = json.NewEncoder(stdout).Encode(job)
		}
		return errors.Join(publishErr, outputErr)
	}
	// Once submitted, only the server may clean up these staged files. A lost
	// HTTP response is not proof that the asynchronous job was not accepted.
	fmt.Fprintf(stderr, "master publication job: %s (check with master status --id %s)\n", request.ID, request.ID)
	return postAdmin(ctx, serverURL, "/admin/master/import", request, "", stdout)
}

func masterInputPaths(file, directory string) ([]string, error) {
	if file != "" {
		return []string{file}, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.Type().IsRegular() && (extension == ".yaml" || extension == ".yml") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("no master YAML files found")
	}
	sort.Strings(paths)
	return paths, nil
}

func getMasterStatus(ctx context.Context, serverURL, id string, stdout io.Writer) error {
	base, err := url.Parse(serverURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("--server must be an HTTP(S) URL without credentials, a query, or a fragment")
	}
	token, err := config.GetAdminAuth()
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/admin/master/status"
	base.RawPath = ""
	base.RawQuery = url.Values{"id": []string{id}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return errors.New("cannot build admin request")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("admin status request failed: %s", strings.ReplaceAll(err.Error(), token, "[redacted]"))
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return errors.New("cannot read admin status response")
	}
	if len(data) > 1<<20 {
		return errors.New("admin status response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("admin API returned HTTP %d: %s", response.StatusCode, strings.ReplaceAll(strings.TrimSpace(string(data)), token, "[redacted]"))
	}
	if !json.Valid(data) {
		return errors.New("admin API returned invalid JSON")
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}

func masterUsage(output io.Writer) {
	fmt.Fprintln(output, `Usage:
  makino master export --type Card [--file config/master/Card.yaml] [--force]
  makino master export --all --dir config/master [--force]
  makino master import --file config/master/Card.yaml [--dry-run] [--allow-empty]
  makino master import --dir config/master [--all] [--dry-run] [--allow-empty]
  makino master reset --type Card [--dry-run]
  makino master reset --all [--dry-run]
  makino master status --id JOB_ID

Single-type exports write YAML only to stdout when --file is omitted.
Export --all requires --dir and never writes bulk YAML to stdout.
Exports require --force before overwriting files. Directory imports replace only
supplied collections; --all requires every active master type. Emptying a
nonempty collection requires --allow-empty. Unchanged tables are skipped.
Reset restores the selected type or all types to the recorded official baseline;
it does not alter user inventories or progression. Missing original files are
downloaded from their recorded official URLs, including during reset --dry-run.

Master commands default to a 10-minute timeout; the global --timeout override
must appear before master. Import stages fresh encrypted files locally; reset
validates the original files. Both submit an asynchronous publication job to the
authenticated running server.
When the shared resource lock is free, Makino automatically holds it and
publishes locally and synchronously using the same transaction/recovery workflow.
When it is owned by the server, Makino uses the coordinated HTTP publication API.
MongoDB must remain running. Local publication/status does not require
SOLIS_ADMIN_AUTH; HTTP requests do. HTTP errors never trigger local publication.
Status also works with the server stopped. Upgrade both binaries before use;
old server binaries do not participate in the ownership-lock protocol.
Makino and the server must share SOLIS_RESOURCE_DIR. Exported files contain
plaintext protobuf data, never encryption keys or editable SQL definitions.
Gameplay pauses briefly during publication; outdated clients return to title.
An official baseline must have been recorded by the normal server update path.

Status "complete" means published and cleaned up. "cleanup_pending" means
publication succeeded; startup recovery will retry cleanup, not undo the data.
Superseded original files are retained for resets; obsolete customized files are
removed after publication when no active or original manifest references them.
Do not delete staging files after an ambiguous HTTP failure; check the job ID.
This workflow supports one running server instance. It does not migrate user
inventories or progression. A different supplied official release replaces
customizations and refreshes the original manifest backup.`)
}
