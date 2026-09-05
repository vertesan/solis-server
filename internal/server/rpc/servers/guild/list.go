package guild

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *GuildServer) List(ctx context.Context, _ *papi.Empty) (*papi.GuildListResponse, error) {
	masterGuilds, err := opmaster.FindAllGuilds(ctx)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	guilds := make([]*papi.GuildParam, 0, len(masterGuilds))
	for index, guild := range masterGuilds {
		rank := int32(index + 1)
		guilds = append(guilds, &papi.GuildParam{
			Id:               guild.GetId(),
			WeaknessRank:     rank,
			MemberAmountRank: rank,
			PrevSeasonResult: &papi.GuildGvgSeasonResult{},
		})
	}

	return &papi.GuildListResponse{
		Guilds:         guilds,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}
