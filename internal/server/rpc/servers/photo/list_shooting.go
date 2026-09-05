package photo

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) ListShooting(ctx context.Context, _ *papi.Empty) (*papi.PhotoListShootingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	activities, err := opuser.FindAllUserPhotoActivities(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	musics, err := opuser.FindAllUserPhotoMusics(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	stages, err := opuser.FindAllUserPhotoStages(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	expressions, err := opuser.FindAllUserPhotoExpressions(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	masterActivities, err := opmaster.FindAllPhotoActivities(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterMusics, err := opmaster.FindAllPhotoQuestMusics(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterStages, err := opmaster.FindAllPhotoQuestStages(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterExpressions, err := opmaster.FindAllPhotoExpressions(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	activities = viber.FilterUserPhotoActivities(activities, masterActivities)
	musics = viber.FilterUserPhotoMusics(musics, masterMusics)
	stages = viber.FilterUserPhotoStages(stages, masterStages)
	expressions = viber.FilterUserPhotoExpressions(expressions, masterExpressions)
	sort.Slice(activities, func(i, j int) bool { return activities[i].GetPhotoActivityId() < activities[j].GetPhotoActivityId() })
	sort.Slice(musics, func(i, j int) bool { return musics[i].GetMusicId() < musics[j].GetMusicId() })
	sort.Slice(stages, func(i, j int) bool { return stages[i].GetStageId() < stages[j].GetStageId() })
	sort.Slice(expressions, func(i, j int) bool {
		return expressions[i].GetPhotoExpressionId() < expressions[j].GetPhotoExpressionId()
	})

	return &papi.PhotoListShootingResponse{
		Activities:     activities,
		Musics:         musics,
		Stages:         stages,
		Expressions:    expressions,
		DokanInfo:      &papi.PhotoDokanInfo{Expressions: []*papi.DokanInfo{}},
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
