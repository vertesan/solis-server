package story

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *StoryServer) ReadCharacterCompanyEnjoyStory(
	ctx context.Context,
	req *papi.StoryReadCharacterCompanyEnjoyStoryRequest,
) (*papi.StoryReadCharacterCompanyEnjoyStoryResponse, error) {
	if req.GetCharacterId() == "" || req.GetNumber() <= 0 {
		return nil, viber.InvalidArgumentf("characterId and a positive number are required")
	}
	masterCharacter, err := opmaster.FindCharacter(ctx, req.CharacterId)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	storyId := ""
	for _, definition := range masterCharacter.GetCompanyEnjoyStories() {
		if definition.GetNumber() == req.GetNumber() {
			storyId = definition.GetStoryId()
			break
		}
	}
	if storyId == "" {
		return nil, viber.InvalidArgumentf("character company enjoy story does not exist")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.StoryReadCharacterCompanyEnjoyStoryResponse
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := viber.EnsureUnlockedCompanyEnjoyStories(ctx, suid, req.GetCharacterId(), nil); err != nil {
			return nil, err
		}
		rewards, updated, err := readUserStory(ctx, suid, storyId, false)
		if err != nil {
			return nil, err
		}
		response = &papi.StoryReadCharacterCompanyEnjoyStoryResponse{
			Rewards: rewards, CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
