package viber

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// UnlockLiveMusic grants first ownership on a successful Quest or Exercise live.
func UnlockLiveMusic(ctx context.Context, suid, musicID string, updated *papi.UpdatedData) error {
	if musicID == "" {
		return nil
	}
	rows, err := opuser.FindAllUserMusics(ctx, suid)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.GetMusicId() == musicID {
			return nil
		}
	}
	row := &ptransaction.UserMusic{MusicId: musicID}
	if err := opuser.UpsertUserMusic(ctx, suid, row); err != nil {
		return err
	}
	updated.Musics = append(updated.Musics, row)
	return nil
}
