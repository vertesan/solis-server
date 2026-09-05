package company

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) EnjoyCharacterTop(
	ctx context.Context,
	req *papi.CompanyEnjoyCharacterTopRequest,
) (*papi.CompanyEnjoyCharacterTopResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, viber.InvalidArgumentf("characterId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyEnjoyCharacterTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		character, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId())
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		if err := viber.EnsureUnlockedCompanyEnjoyStories(ctx, suid, character.GetCharacterId(), updated); err != nil {
			return nil, err
		}
		if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		levelInfos, err := companyEnjoyLevelInfos(ctx, character, state)
		if err != nil {
			return nil, err
		}
		missionInfos, err := companyEnjoyMissionInfos(ctx, suid, character.GetCharacterId())
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyEnjoyCharacterTopResponse{
			EnjoyLevelInfos: levelInfos,
			MissionInfos:    missionInfos,
			CurrentRank:     1,
			CommonResponse:  viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func companyEnjoyMissionInfos(
	ctx context.Context,
	suid string,
	characterID string,
) ([]*papi.CompanyEnjoyCharacterMissionInfo, error) {
	masters, users, err := companyMissionData(ctx, suid)
	if err != nil {
		return nil, err
	}
	groups := make(map[penum.CompanyEnjoyCharacterMissionCategoryType][]*ptransaction.UserMission)
	for _, userMission := range users {
		master := masters[userMission.GetMissionId()]
		if master == nil || master.GetCompanyEnjoyCharacterId() != characterID ||
			master.GetCompanyEnjoyCharacterMissionCategoryType() == penum.CompanyEnjoyCharacterMissionCategoryType_CompanyEnjoyCharacterMissionCategoryType_Unknown {
			continue
		}
		category := master.GetCompanyEnjoyCharacterMissionCategoryType()
		groups[category] = append(groups[category], userMission)
	}
	infos := make([]*papi.CompanyEnjoyCharacterMissionInfo, 0, len(groups))
	for category := penum.CompanyEnjoyCharacterMissionCategoryType_CompanyEnjoyCharacterMissionCategoryType_Enjoy; category <= penum.CompanyEnjoyCharacterMissionCategoryType_CompanyEnjoyCharacterMissionCategoryType_Action; category++ {
		missions := groups[category]
		if len(missions) == 0 {
			continue
		}
		sortUserCompanyMissions(missions)
		infos = append(infos, &papi.CompanyEnjoyCharacterMissionInfo{CategoryType: category, UserMissions: missions})
	}
	return infos, nil
}

func companyEnjoyLevelInfos(
	ctx context.Context,
	character *ptransaction.UserCharacter,
	state *umodel.UserCompanyRewardState,
) ([]*papi.CompanyEnjoyCharacterLevelInfo, error) {
	settings, err := opmaster.FindCompanyEnjoyCharacterLevelSettings(ctx, character.GetCharacterId())
	if err != nil {
		return nil, err
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return nil, err
	}
	levelByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevel, len(levels))
	for _, level := range levels {
		levelByKey[level.GetLevelKey()] = level
	}
	received := int32Set(state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()])
	infos := make([]*papi.CompanyEnjoyCharacterLevelInfo, 0, len(settings))
	for _, setting := range settings {
		level := levelByKey[setting.GetLevelKey()]
		if level == nil {
			return nil, viber.FailedPreconditionf("company enjoy level %d is missing", setting.GetLevelKey())
		}
		_, alreadyReceived := received[setting.GetLevelKey()]
		infos = append(infos, &papi.CompanyEnjoyCharacterLevelInfo{
			CharacterId:       character.GetCharacterId(),
			LevelKey:          setting.GetLevelKey(),
			IsReceivable:      !alreadyReceived && character.GetCompanyEnjoyPoint() >= level.GetRequiredPoint(),
			IsAlreadyReceived: alreadyReceived,
		})
	}
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].GetLevelKey() < infos[j].GetLevelKey() })
	return infos, nil
}
