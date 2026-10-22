package masterdata

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/crypto"
	"google.golang.org/protobuf/proto"
)

type ResetOptions struct {
	Type string
	All  bool
}

// PrepareReset validates the complete selection before requesting publication.
// It does not change MongoDB or remove files. Missing baseline files may be
// recovered from the trusted original manifest, but are never regenerated from
// customized rows or assigned a new content-addressed name.
func PrepareReset(ctx context.Context, options ResetOptions) (*ImportRequest, error) {
	if options.All == (options.Type != "") {
		return nil, errors.New("reset requires exactly one of --type or --all")
	}
	version, active, err := activeManifest(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkVersion(ctx, version); err != nil {
		return nil, err
	}
	originals, err := resetOriginals(ctx, active)
	if err != nil {
		return nil, err
	}
	names := []string{options.Type}
	if options.All {
		names = names[:0]
		for name := range active {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	request := &ImportRequest{ID: uuid.NewString(), ExpectedVersion: version, Reset: true}
	for _, name := range names {
		current := active[name]
		if current == nil {
			return nil, fmt.Errorf("master type %q is not active", name)
		}
		original := originals[name]
		// Also verify/restore a missing original when the selected type is
		// already at baseline. No version-only publication is needed in this case.
		count, err := originalRowCount(ctx, original)
		if err != nil {
			return nil, err
		}
		if proto.Equal(current, original) {
			continue
		}
		request.Packs = append(request.Packs, StagedPack{Type: name, BaseFileName: current.FileName,
			FileName: original.FileName, FileSize: int64(original.FileSize), RowCount: count})
	}
	if err := unchangedVersion(ctx, version); err != nil {
		return nil, err
	}
	return request, nil
}

func resetOriginals(ctx context.Context, active map[string]*ptransaction.MasterTagPack) (map[string]*ptransaction.MasterTagPack, error) {
	if err := validateOriginalBaseline(ctx, active); err != nil {
		return nil, err
	}
	packs, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*ptransaction.MasterTagPack, len(packs))
	for _, pack := range packs {
		result[pack.Type] = pack
	}
	return result, nil
}

func originalRowCount(ctx context.Context, pack *ptransaction.MasterTagPack) (int64, error) {
	if err := ensureOriginalFile(ctx, pack); err != nil {
		return 0, err
	}
	schema, err := crypto.InspectMasterDb(ctx, filepath.Join(config.GetDBDir(), pack.FileName), pack.CryptoKey, pack.Type)
	if err != nil {
		return 0, err
	}
	return schema.RowCount, nil
}

func prepareResetPublication(ctx context.Context, job *ImportJob, active map[string]*ptransaction.MasterTagPack) ([]*ptransaction.MasterTagPack, error) {
	originals, err := resetOriginals(ctx, active)
	if err != nil {
		return nil, err
	}
	result := make([]*ptransaction.MasterTagPack, 0, len(job.Request.Packs))
	for _, pack := range job.Request.Packs {
		current, original := active[pack.Type], originals[pack.Type]
		if current == nil || original == nil || current.FileName != pack.BaseFileName {
			return nil, ErrStaleMaster
		}
		// The request cannot select arbitrary files, sizes, keys, URLs or rows.
		if pack.FileName != original.FileName || pack.FileSize != int64(original.FileSize) {
			return nil, fmt.Errorf("reset target %s differs from the original manifest", pack.Type)
		}
		count, err := originalRowCount(ctx, original)
		if err != nil {
			return nil, err
		}
		if count != pack.RowCount {
			return nil, fmt.Errorf("reset target %s row count differs from the original file", pack.Type)
		}
		result = append(result, proto.Clone(original).(*ptransaction.MasterTagPack))
		active[pack.Type] = original
	}
	// A full return to the official pack set also restores its version. A
	// partial reset keeps a distinct custom version for the remaining edits.
	allOriginal := len(active) == len(originals)
	for name, current := range active {
		if !proto.Equal(current, originals[name]) {
			allOriginal = false
			break
		}
	}
	if allOriginal {
		profile, err := opsolis.FindServerProfile(ctx)
		if err != nil {
			return nil, err
		}
		job.Version = profile.MasterVersionOrig
	}
	return result, nil
}
