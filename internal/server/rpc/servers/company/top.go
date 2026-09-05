package company

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *CompanyServer) Top(ctx context.Context, _ *papi.CompanyTopRequest) (*papi.CompanyTopResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyTopResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
			return nil, err
		}
		company, err := opuser.FindUserCompany(ctx, suid)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		totalPoint, err := companyTotalPoint(company)
		if err != nil {
			return nil, err
		}

		grades, err := opmaster.FindAllCompanyGrades(ctx)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(grades, func(i, j int) bool {
			if grades[i].GetRequiredPoint() == grades[j].GetRequiredPoint() {
				return grades[i].GetId() > grades[j].GetId()
			}
			return grades[i].GetRequiredPoint() > grades[j].GetRequiredPoint()
		})
		receivedGrades := stringSet(state.ReceivedGradeIds)
		gradeInfos := make([]*papi.CompanyGradeInfo, 0, len(grades))
		for _, grade := range grades {
			_, received := receivedGrades[grade.GetId()]
			received = received && grade.GetAchieveRewardId() != ""
			gradeInfos = append(gradeInfos, &papi.CompanyGradeInfo{
				Id:                grade.GetId(),
				IsReceivable:      grade.GetAchieveRewardId() != "" && !received && totalPoint >= grade.GetRequiredPoint(),
				IsAlreadyReceived: received,
			})
		}

		trustLevels, err := opmaster.FindAllCompanyTrustLevels(ctx)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(trustLevels, func(i, j int) bool { return trustLevels[i].GetLevelKey() > trustLevels[j].GetLevelKey() })
		receivedTrustLevels := int32Set(state.ReceivedTrustLevelKeys)
		trustLevelInfos := make([]*papi.CompanyTrustLevelInfo, 0, len(trustLevels))
		for _, level := range trustLevels {
			_, received := receivedTrustLevels[level.GetLevelKey()]
			received = received && level.GetAchieveRewardId() != ""
			trustLevelInfos = append(trustLevelInfos, &papi.CompanyTrustLevelInfo{
				LevelKey:          level.GetLevelKey(),
				IsReceivable:      level.GetAchieveRewardId() != "" && !received && company.GetTrustPoint() >= level.GetRequiredPoint(),
				IsAlreadyReceived: received,
			})
		}

		trustMissionInfos, err := companyTrustMissionInfos(ctx, suid)
		if err != nil {
			return nil, err
		}
		receivableCharacterIDs, err := receivableEnjoyCharacterIDs(ctx, suid, state)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyTopResponse{
			CompanyGradeInfos:                      gradeInfos,
			TrustMissionInfos:                      trustMissionInfos,
			TrustLevelInfos:                        trustLevelInfos,
			CurrentRank:                            1,
			ReceivableLevelRewardEnjoyCharacterIds: receivableCharacterIDs,
			ThisMonthSalaryPoint:                   0,
			DokanInfos:                             []*papi.DokanInfo{},
			CommonResponse:                         viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func companyTrustMissionInfos(ctx context.Context, suid string) ([]*papi.CompanyTrustMissionInfo, error) {
	masters, users, err := companyMissionData(ctx, suid)
	if err != nil {
		return nil, err
	}
	groups := make(map[penum.CompanyTrustMissionCategoryType][]*ptransaction.UserMission)
	for _, userMission := range users {
		master := masters[userMission.GetMissionId()]
		if master == nil || master.GetCompanyTrustMissionCategoryType() == penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Unknown {
			continue
		}
		category := master.GetCompanyTrustMissionCategoryType()
		groups[category] = append(groups[category], userMission)
	}
	infos := make([]*papi.CompanyTrustMissionInfo, 0, len(groups))
	for category := penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Area; category <= penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Growth; category++ {
		missions := groups[category]
		if len(missions) == 0 {
			continue
		}
		sortUserCompanyMissions(missions)
		infos = append(infos, &papi.CompanyTrustMissionInfo{CategoryType: category, UserMissions: missions})
	}
	return infos, nil
}

func receivableEnjoyCharacterIDs(
	ctx context.Context,
	suid string,
	state *umodel.UserCompanyRewardState,
) ([]string, error) {
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	for _, character := range characters {
		keys, err := receivableCompanyEnjoyLevelKeys(ctx, character, state)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			result = append(result, character.GetCharacterId())
		}
	}
	sort.Strings(result)
	return result, nil
}
