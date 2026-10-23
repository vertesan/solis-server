package makino

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db"
	"github.com/vertesan/solis-server/internal/makino/catalog"
)

func runServer(ctx context.Context, args []string, stdout, stderr io.Writer) (retErr error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		serverUsage(stdout)
		return nil
	}
	kind := args[0]
	switch kind {
	case "notice", "shop", "exchange", "gacha", "gacha-rules":
	default:
		return fmt.Errorf("unknown server catalog %q", kind)
	}
	flags := flag.NewFlagSet("makino server "+kind, flag.ContinueOnError)
	flags.SetOutput(stderr)
	overwrite := flags.Bool("overwrite", false, "replace the complete catalog")
	add := flags.Bool("add", false, "upsert imported entries by ID")
	clear := flags.Bool("clear", false, "clear the catalog")
	export := flags.Bool("export", false, "print current catalog as editable YAML")
	file := flags.String("file", "", "input YAML file")
	dir := flags.String("dir", "", "input directory (all .yaml/.yml files, recursively)")
	flags.Usage = func() { serverUsage(stderr) }
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	mode, count := "", 0
	for name, enabled := range map[string]bool{"overwrite": *overwrite, "add": *add, "clear": *clear, "export": *export} {
		if enabled {
			mode, count = name, count+1
		}
	}
	if count != 1 {
		return errors.New("choose exactly one of --overwrite, --add, --clear, or --export")
	}
	if kind == "gacha-rules" && mode != "overwrite" && mode != "export" {
		return errors.New("gacha-rules supports only --overwrite and --export; use gacha --clear to disable draws")
	}
	var documents [][]byte
	if mode == "clear" || mode == "export" {
		if *file != "" || *dir != "" {
			return errors.New("--clear and --export do not accept --file or --dir")
		}
	} else {
		if *file != "" && *dir != "" {
			return errors.New("--file and --dir cannot be combined")
		}
		inputDir := *dir
		if *file == "" && inputDir == "" {
			inputDir = filepath.Join("config", "catalogs", kind)
		}
		var err error
		documents, err = readCatalogFiles(*file, inputDir)
		if err != nil {
			return err
		}
	}
	// Do not run Initialize: management commands must neither migrate/reset
	// catalogs nor download master data. Connect once and always release it.
	if err := db.Connect(); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Disconnect()) }()
	if mode == "export" {
		var data []byte
		var err error
		if kind == "gacha" || kind == "gacha-rules" {
			data, err = catalog.ExportGacha(ctx, kind)
		} else {
			data, err = catalog.Export(ctx, kind)
		}
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	}
	var err error
	if kind == "gacha" || kind == "gacha-rules" {
		err = catalog.ApplyGacha(ctx, kind, mode, documents)
	} else {
		err = catalog.Apply(ctx, kind, mode, documents)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s catalog: %s complete\n", kind, mode)
	return err
}

func readCatalogFiles(file, dir string) ([][]byte, error) {
	paths := []string{}
	if file != "" {
		paths = append(paths, file)
	} else {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("--dir must name a directory")
		}
		if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			ext := strings.ToLower(filepath.Ext(path))
			if entry.Type().IsRegular() && (ext == ".yaml" || ext == ".yml") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, errors.New("no YAML files found; use --clear to empty a catalog")
	}
	documents := make([][]byte, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("input %q must be a regular file", path)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		// A single input cannot represent a MongoDB document above 16 MiB.
		const maxInputSize = 16 << 20
		data, readErr := io.ReadAll(io.LimitReader(file, maxInputSize+1))
		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, fmt.Errorf("read %q: %w", path, err)
		}
		if len(data) > maxInputSize {
			return nil, fmt.Errorf("input %q exceeds 16 MiB", path)
		}
		documents = append(documents, data)
	}
	return documents, nil
}

func serverUsage(out io.Writer) {
	fmt.Fprintln(out, `Usage: makino server <notice|shop|exchange|gacha|gacha-rules> <operation>

Operations (exactly one):
  --export                       Print current editable YAML to stdout.
  --overwrite                    Import config/catalogs/<catalog> by default.
  --overwrite --file FILE        Replace the whole catalog atomically.
  --overwrite --dir DIRECTORY    Import all YAML files, sorted by path.
  --add --file FILE              Upsert entries by ID; keep other entries.
  --add --dir DIRECTORY          Upsert all entries in the YAML directory.
  --clear                        Clear the catalog (no input files).

Requires SOLIS_DB_USERNAME/PASSWORD. Changes are visible on the next game request; the server
does not need to restart. Invalid input rolls back the complete operation.
Empty imports are rejected; clearing requires the explicit --clear flag.
When --file and --dir are omitted for an import, the default input directory is
config/catalogs/<catalog>, relative to the working directory. --export prints
to stdout; it never writes files implicitly.

YAML shapes (use --export for a complete, editable example):
  notice: a NoticeInfo or {notices: [...], malfunctionNotices: [...],
          prNotices: [...], showcaseShopNotice: {...}}
  shop: {shopId: ..., info: {...}, items: [...], loginBonusPackageItem: ...}
        or {shops: [...]}; --add replaces an entire shop's entry, not one item.
  exchange: an ExchangeBooth or {booths: [...]}; --add replaces matching booths.
  gacha: {gachaIds: [gacha-normal-...], ignoreOfficialDates: true}
         --overwrite selects that exact roster; --add appends IDs.
         useMasterCatalog: true retains automatic selection alongside explicit IDs.
         Explicit IDs can reopen old gachas; resource/unlock requirements remain.
  gacha-rules: rarity probabilities, reward pools, guarantees, and notice aliases.
               Only --export and --overwrite are supported.

First-start seeds are bundled from internal/db/internal/migration/catalogdata.
They are separate from config/catalogs, which is the CLI's working directory.
Editing seed files does not change an already-initialized server.`)
}
