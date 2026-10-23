package makino

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/vertesan/solis-server/internal/makino/masterdata"
)

func runCustom(ctx context.Context, serverURL string, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		customUsage(stdout)
		return nil
	}
	template := args[0]
	types, err := masterdata.CustomTypes(template)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("makino custom "+template, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { customUsage(stderr) }
	reset := flags.Bool("reset", false, "restore all affected collections to their official baseline")
	dryRun := flags.Bool("dry-run", false, "validate the customization or reset without publishing")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return runMasterPublication(ctx, serverURL, *dryRun, stdout, stderr, func(ctx context.Context) (*masterdata.ImportRequest, error) {
		if *reset {
			return masterdata.PrepareReset(ctx, masterdata.ResetOptions{Types: types})
		}
		return masterdata.PrepareCustom(ctx, template)
	})
}

func customUsage(output io.Writer) {
	fmt.Fprintln(output, `Usage:
  makino custom home-action [--reset] [--dry-run]
  makino custom photo [--reset] [--dry-run]

home-action maps all LoveHomeAction records into HomeAction with an empty
conditionId and weight 100. Existing HomeAction records are preserved except
for records with the same IDs, which are replaced by the mapped records.
photo unlocks PhotoActivity, PhotoExpression, PhotoQuestMusic, and
PhotoQuestStage, and permits all costume types from Costume for activity and
expression photography.

--reset restores the whole affected collections to their official baseline,
including removing any other manual changes in those collections. It does not
reset source collections such as LoveHomeAction or Costume.
--dry-run validates and prepares the changes without publishing them.

Custom commands default to a 10-minute timeout; the global --timeout override
must appear before custom. Publication automatically coordinates with a running
server or runs locally when the server is stopped. MongoDB must be running,
and Makino must share SOLIS_RESOURCE_DIR with the server. HTTP publication
requires SOLIS_ADMIN_AUTH. See makino master --help for publication details.`)
}
