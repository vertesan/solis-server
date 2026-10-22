package masterdata

import (
	"context"
	"errors"
	"fmt"
	"github.com/vertesan/solis-server/internal/db/store/opmaster/version"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
	"sort"
	"sync/atomic"
)

type Snapshot struct{ tag *ptransaction.MasterTag }

func (s *Snapshot) Version() string {
	if s == nil {
		return ""
	}
	return s.tag.Version
}
func (s *Snapshot) Tag() *ptransaction.MasterTag {
	if s == nil {
		return nil
	}
	return proto.Clone(s.tag).(*ptransaction.MasterTag)
}

var currentSnapshot atomic.Pointer[Snapshot]

func Current() *Snapshot { return currentSnapshot.Load() }

func RefreshSnapshot(ctx context.Context) error {
	var next *Snapshot
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		profile, err := opsolis.FindServerProfile(ctx)
		if err != nil {
			return nil, err
		}
		v, err := version.GetMasterVersion(ctx)
		if err != nil {
			return nil, err
		}
		if v == "" || v != profile.MasterVersion {
			return nil, errors.New("master version documents disagree")
		}
		packs, err := opsolis.GetAllMasterTagPack(ctx)
		if err != nil {
			return nil, err
		}
		sort.Slice(packs, func(i, j int) bool { return packs[i].Type < packs[j].Type })
		next = &Snapshot{tag: &ptransaction.MasterTag{Version: v, MasterTagPacks: packs}}
		return nil, nil
	})
	if err == nil {
		currentSnapshot.Store(next)
	}
	return err
}

func validateOriginalBaseline(ctx context.Context, active map[string]*ptransaction.MasterTagPack) error {
	originals, err := opsolis.GetAllOriginalMasterTagPack(ctx)
	if err != nil {
		return err
	}
	byType := make(map[string]*ptransaction.MasterTagPack, len(originals))
	for _, p := range originals {
		if p == nil || p.Type == "" || byType[p.Type] != nil || !hashName.MatchString(p.FileName) || p.FileSize <= 0 || p.CryptoKey == "" {
			return errors.New("original master baseline contains invalid or duplicate packs")
		}
		byType[p.Type] = p
	}
	for name, p := range active {
		original := byType[name]
		if original == nil || original.CryptoKey != p.CryptoKey {
			return fmt.Errorf("original master baseline is missing or incompatible for %s; perform an official update first", name)
		}
	}
	return nil
}
