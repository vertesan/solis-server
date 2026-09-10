package moshikoi

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func LoadState(ctx context.Context, suid, loveID string) (*umodel.UserLoveState, error) {
	state, err := opuser.FindUserLoveState(ctx, suid, loveID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &umodel.UserLoveState{SUID: suid, LoveID: loveID, StoryEpisodeIDs: []string{FirstEpisodeID(loveID)}}, nil
	}
	if err == nil && len(state.StoryEpisodeIDs) == 0 {
		state.StoryEpisodeIDs = []string{FirstEpisodeID(loveID)}
	}
	return state, err
}

func UserLove(ctx context.Context, suid, loveID string) (*ptransaction.UserLove, error) {
	loves, err := opuser.FindAllUserLoves(ctx, suid)
	if err != nil {
		return nil, err
	}
	for _, love := range loves {
		if love.GetLoveId() == loveID {
			return love, nil
		}
	}
	return &ptransaction.UserLove{LoveId: loveID}, nil
}

func StoreState(ctx context.Context, state *umodel.UserLoveState, love *ptransaction.UserLove) error {
	if err := opuser.UpsertUserLoveState(ctx, state); err != nil {
		return err
	}
	return opuser.UpsertUserLove(ctx, state.SUID, love)
}
