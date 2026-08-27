package servers

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const companyEnjoyStoryItemPrefix = "item-enjoy-story-bond-"

type CompanyServer struct {
	papi.UnimplementedCompanyServer
}

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

func (s *CompanyServer) ListRanking(ctx context.Context, _ *papi.Empty) (*papi.CompanyListRankingResponse, error) {
	suid := viber.GetSuid(ctx).String()
	user, public, company, err := companyRankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	totalPoint, err := companyTotalPoint(company)
	if err != nil {
		return nil, err
	}
	info := &papi.CompanyRankingInfo{
		UserId:             public.GetPublicUserId(),
		CurrentRank:        1,
		ManagerName:        user.GetName(),
		CurrentPoint:       totalPoint,
		EmblemId:           user.GetEmblemId(),
		CurrentEnjoyPoint:  company.GetEnjoyPoint(),
		CurrentTrustPoint:  company.GetTrustPoint(),
		CurrentSalaryPoint: company.GetSalaryPoint(),
	}
	return &papi.CompanyListRankingResponse{
		RankingInfos:   []*papi.CompanyRankingInfo{info},
		SelfPoint:      totalPoint,
		SelfRank:       1,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *CompanyServer) ReceiveGradeRewards(
	ctx context.Context,
	req *papi.CompanyReceiveGradeRewardsRequest,
) (*papi.CompanyReceiveGradeRewardsResponse, error) {
	if err := validateUniqueCompanyGradeIDs(req.GetGradeIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveGradeRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		company, err := opuser.FindUserCompany(ctx, suid)
		if err != nil {
			return nil, err
		}
		totalPoint, err := companyTotalPoint(company)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		received := stringSet(state.ReceivedGradeIds)
		updated := &papi.UpdatedData{Company: company}
		results := make([]*ptransaction.RewardResult, 0)
		for _, gradeID := range req.GetGradeIds() {
			grade, err := opmaster.FindCompanyGrade(ctx, gradeID)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, viber.InvalidArgumentf("unknown Company grade %q", gradeID)
				}
				return nil, err
			}
			if grade.GetAchieveRewardId() == "" {
				return nil, viber.FailedPreconditionf("Company grade %q has no reward", gradeID)
			}
			if _, exists := received[gradeID]; exists {
				return nil, viber.FailedPreconditionf("Company grade %q reward was already received", gradeID)
			}
			if totalPoint < grade.GetRequiredPoint() {
				return nil, viber.FailedPreconditionf("Company grade %q is not reached", gradeID)
			}
			granted, err := grantCompanyRewardSet(ctx, suid, grade.GetAchieveRewardId(), updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
			received[gradeID] = struct{}{}
			state.ReceivedGradeIds = append(state.ReceivedGradeIds, gradeID)
		}
		sort.Strings(state.ReceivedGradeIds)
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveGradeRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CompanyServer) BulkReceiveMission(
	ctx context.Context,
	req *papi.CompanyBulkReceiveMissionRequest,
) (*papi.CompanyBulkReceiveMissionResponse, error) {
	if err := validateCompanyMissionIDs(req.GetMissionIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveMissionResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		results, _, _, err := receiveCompanyMissions(ctx, suid, req.GetMissionIds(), false, updated)
		if err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveMissionResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

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

func (s *CompanyServer) ListEnjoyCharacterRanking(
	ctx context.Context,
	req *papi.CompanyListEnjoyCharacterRankingRequest,
) (*papi.CompanyListEnjoyCharacterRankingResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, viber.InvalidArgumentf("characterId is required")
	}
	suid := viber.GetSuid(ctx).String()
	character, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId())
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	user, public, _, err := companyRankingUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	info := &papi.CompanyEnjoyCharacterRankingInfo{
		UserId:            public.GetPublicUserId(),
		CurrentRank:       1,
		ManagerName:       user.GetName(),
		CurrentEnjoyPoint: character.GetCompanyEnjoyPoint(),
		EmblemId:          user.GetEmblemId(),
	}
	return &papi.CompanyListEnjoyCharacterRankingResponse{
		RankingInfos:   []*papi.CompanyEnjoyCharacterRankingInfo{info},
		SelfPoint:      character.GetCompanyEnjoyPoint(),
		SelfRank:       1,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func (s *CompanyServer) ReceiveEnjoyCharacterLevelRewards(
	ctx context.Context,
	req *papi.CompanyReceiveEnjoyCharacterLevelRewardsRequest,
) (*papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse, error) {
	if req.GetCharacterId() == "" || len(req.GetLevelKeys()) == 0 {
		return nil, viber.InvalidArgumentf("characterId and levelKeys are required")
	}
	if err := validateUniqueLevelKeys(req.GetLevelKeys()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		character, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId())
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		results, err := claimCompanyEnjoyLevelRewards(
			ctx, suid, character, req.GetLevelKeys(), state, true, updated,
		)
		if err != nil {
			return nil, err
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		putUpdatedCompanyCharacter(updated, character)
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CompanyServer) UseEnjoyObtainPointItem(
	ctx context.Context,
	req *papi.CompanyUseObtainEnjoyPointItemRequest,
) (*papi.CompanyUseObtainEnjoyPointItemResponse, error) {
	if req.GetCharacterId() == "" || req.GetItemId() == "" || req.GetConsumptionItemAmount() <= 0 {
		return nil, viber.InvalidArgumentf("characterId, itemId, and a positive consumptionItemAmount are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyUseObtainEnjoyPointItemResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := requireOwnedCompanyCharacter(ctx, suid, req.GetCharacterId()); err != nil {
			return nil, err
		}
		if err := viber.ReconcileCompanyMissions(ctx, suid); err != nil {
			return nil, err
		}
		setting, err := opmaster.FindCompanyEnjoyObtainPointItemSetting(ctx, req.GetCharacterId(), req.GetItemId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, viber.InvalidArgumentf("item is unavailable for this character")
			}
			return nil, err
		}
		if setting.GetObtainPoint() <= 0 || int64(req.GetConsumptionItemAmount()) > math.MaxInt64/int64(setting.GetObtainPoint()) {
			return nil, viber.FailedPreconditionf("master enjoy point amount is invalid")
		}
		updated := &papi.UpdatedData{}
		if _, err := viber.ConsumeExchangeItemStacks(
			ctx, suid, req.GetItemId(), int64(req.GetConsumptionItemAmount()), updated,
		); err != nil {
			return nil, err
		}
		pointAmount := int64(setting.GetObtainPoint()) * int64(req.GetConsumptionItemAmount())
		if _, _, err := viber.AddCompanyEnjoyPoints(ctx, suid, req.GetCharacterId(), pointAmount, updated); err != nil {
			return nil, err
		}
		characterMissions, err := advanceCompanyMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CompanyCharacterUseObtainEnjoyPointItemCount,
			int64(req.GetConsumptionItemAmount()),
			req.GetCharacterId(),
		)
		if err != nil {
			return nil, err
		}
		totalMissions, err := advanceCompanyMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_CompanyUseObtainEnjoyPointItemTotalCount,
			int64(req.GetConsumptionItemAmount()),
		)
		if err != nil {
			return nil, err
		}
		for _, mission := range append(characterMissions, totalMissions...) {
			putUpdatedCompanyMission(updated, mission)
		}
		totalCount, err := opuser.FindUserTotalCount(ctx, suid)
		if err != nil {
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			totalCount = &ptransaction.UserTotalCount{}
			if err := opuser.UpsertUserTotalCount(ctx, suid, totalCount); err != nil {
				return nil, err
			}
		}
		updated.TotalCount = totalCount
		response = &papi.CompanyUseObtainEnjoyPointItemResponse{
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, viber.ErrExchangeInsufficientResource) {
			return nil, status.Error(codes.ResourceExhausted, "insufficient item amount")
		}
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CompanyServer) BulkReceiveEnjoy(
	ctx context.Context,
	req *papi.CompanyBulkReceiveEnjoyRequest,
) (*papi.CompanyBulkReceiveEnjoyResponse, error) {
	if err := validateCompanyMissionIDs(req.GetMissionIds()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveEnjoyResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated := &papi.UpdatedData{}
		_, beforeByCharacter, characterOrder, err := receiveCompanyMissions(
			ctx, suid, req.GetMissionIds(), true, updated,
		)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		results := make([]*papi.CompanyBulkReceiveEnjoyResult, 0, len(characterOrder))
		for _, characterID := range characterOrder {
			character, err := opuser.FindUserCharacter(ctx, suid, characterID)
			if err != nil {
				return nil, err
			}
			levelKeys, err := receivableCompanyEnjoyLevelKeys(ctx, character, state)
			if err != nil {
				return nil, err
			}
			levelResults, err := claimCompanyEnjoyLevelRewards(
				ctx, suid, character, levelKeys, state, false, updated,
			)
			if err != nil {
				return nil, err
			}
			levelResults, err = viber.AggregateRewardResults(levelResults)
			if err != nil {
				return nil, err
			}
			results = append(results, &papi.CompanyBulkReceiveEnjoyResult{
				CharacterId:                      characterID,
				BeforeCharacterEnjoyPoint:        beforeByCharacter[characterID],
				EnjoyCharacterLevelRewardResults: levelResults,
			})
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveEnjoyResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CompanyServer) BulkReceiveEnjoyCharacterLevelRewards(
	ctx context.Context,
	_ *papi.Empty,
) (*papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse, error) {
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		characters, err := opuser.FindAllUserCharacters(ctx, suid)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(characters, func(i, j int) bool {
			return characters[i].GetCharacterId() < characters[j].GetCharacterId()
		})
		updated := &papi.UpdatedData{}
		results := make([]*papi.CompanyBulkReceiveEnjoyResult, 0)
		for _, character := range characters {
			levelKeys, err := receivableCompanyEnjoyLevelKeys(ctx, character, state)
			if err != nil {
				return nil, err
			}
			if len(levelKeys) == 0 {
				continue
			}
			levelResults, err := claimCompanyEnjoyLevelRewards(
				ctx, suid, character, levelKeys, state, false, updated,
			)
			if err != nil {
				return nil, err
			}
			levelResults, err = viber.AggregateRewardResults(levelResults)
			if err != nil {
				return nil, err
			}
			putUpdatedCompanyCharacter(updated, character)
			results = append(results, &papi.CompanyBulkReceiveEnjoyResult{
				CharacterId:                      character.GetCharacterId(),
				BeforeCharacterEnjoyPoint:        character.GetCompanyEnjoyPoint(),
				EnjoyCharacterLevelRewardResults: levelResults,
			})
		}
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func (s *CompanyServer) ReceiveTrustLevelRewards(
	ctx context.Context,
	req *papi.CompanyReceiveTrustLevelRewardsRequest,
) (*papi.CompanyReceiveTrustLevelRewardsResponse, error) {
	if len(req.GetLevelKeys()) == 0 {
		return nil, viber.InvalidArgumentf("levelKeys is required")
	}
	if err := validateUniqueLevelKeys(req.GetLevelKeys()); err != nil {
		return nil, err
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.CompanyReceiveTrustLevelRewardsResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		company, err := opuser.FindUserCompany(ctx, suid)
		if err != nil {
			return nil, err
		}
		state, err := findCompanyRewardStateOrEmpty(ctx, suid)
		if err != nil {
			return nil, err
		}
		received := int32Set(state.ReceivedTrustLevelKeys)
		updated := &papi.UpdatedData{Company: company}
		results := make([]*ptransaction.RewardResult, 0)
		for _, levelKey := range req.GetLevelKeys() {
			level, err := opmaster.FindCompanyTrustLevel(ctx, levelKey)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, viber.InvalidArgumentf("unknown Company trust level %d", levelKey)
				}
				return nil, err
			}
			if level.GetAchieveRewardId() == "" {
				return nil, viber.FailedPreconditionf("Company trust level %d has no reward", levelKey)
			}
			if _, exists := received[levelKey]; exists {
				return nil, viber.FailedPreconditionf("Company trust level %d reward was already received", levelKey)
			}
			if company.GetTrustPoint() < level.GetRequiredPoint() {
				return nil, viber.FailedPreconditionf("Company trust level %d is not reached", levelKey)
			}
			granted, err := grantCompanyRewardSet(ctx, suid, level.GetAchieveRewardId(), updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
			received[levelKey] = struct{}{}
			state.ReceivedTrustLevelKeys = append(state.ReceivedTrustLevelKeys, levelKey)
		}
		sort.SliceStable(state.ReceivedTrustLevelKeys, func(i, j int) bool {
			return state.ReceivedTrustLevelKeys[i] < state.ReceivedTrustLevelKeys[j]
		})
		if err := opuser.UpsertUserCompanyRewardState(ctx, state); err != nil {
			return nil, err
		}
		results, err = viber.AggregateRewardResults(results)
		if err != nil {
			return nil, err
		}
		response = &papi.CompanyReceiveTrustLevelRewardsResponse{
			Results:        results,
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func companyRankingUser(
	ctx context.Context,
	suid string,
) (*ptransaction.User, *ptransaction.UserPublic, *ptransaction.UserCompany, error) {
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, nil, nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, nil, nil, err
	}
	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return nil, nil, nil, err
	}
	return user, public, company, nil
}

func companyTotalPoint(company *ptransaction.UserCompany) (int64, error) {
	if company.GetEnjoyPoint() < 0 || company.GetTrustPoint() < 0 || company.GetSalaryPoint() < 0 {
		return 0, viber.FailedPreconditionf("company point is invalid")
	}
	if company.GetEnjoyPoint() > math.MaxInt64-company.GetTrustPoint() ||
		company.GetEnjoyPoint()+company.GetTrustPoint() > math.MaxInt64-company.GetSalaryPoint() {
		return 0, viber.FailedPreconditionf("company point overflows")
	}
	return company.GetEnjoyPoint() + company.GetTrustPoint() + company.GetSalaryPoint(), nil
}

func findCompanyRewardStateOrEmpty(ctx context.Context, suid string) (*umodel.UserCompanyRewardState, error) {
	state, err := opuser.FindUserCompanyRewardState(ctx, suid)
	if err == nil {
		if state.ReceivedEnjoyCharacterLevelKeys == nil {
			state.ReceivedEnjoyCharacterLevelKeys = make(map[string][]int32)
		}
		return state, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return &umodel.UserCompanyRewardState{
		SUID:                            suid,
		ReceivedGradeIds:                []string{},
		ReceivedTrustLevelKeys:          []int32{},
		ReceivedEnjoyCharacterLevelKeys: make(map[string][]int32),
	}, nil
}

func requireOwnedCompanyCharacter(ctx context.Context, suid, characterID string) (*ptransaction.UserCharacter, error) {
	character, err := opuser.FindUserCharacter(ctx, suid, characterID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.PermissionDenied, "character is not owned")
		}
		return nil, err
	}
	return character, nil
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

func companyMissionData(
	ctx context.Context,
	suid string,
) (map[string]*pmaster.Mission, []*ptransaction.UserMission, error) {
	users, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, 0, len(users))
	for _, mission := range users {
		ids = append(ids, mission.GetMissionId())
	}
	masters, err := opmaster.FindMissionsByIds(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*pmaster.Mission, len(masters))
	for _, mission := range masters {
		byID[mission.GetId()] = mission
	}
	return byID, users, nil
}

func sortUserCompanyMissions(missions []*ptransaction.UserMission) {
	sort.SliceStable(missions, func(i, j int) bool {
		return missions[i].GetMissionId() < missions[j].GetMissionId()
	})
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

func receivableCompanyEnjoyLevelKeys(
	ctx context.Context,
	character *ptransaction.UserCharacter,
	state *umodel.UserCompanyRewardState,
) ([]int32, error) {
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
	keys := make([]int32, 0)
	for _, setting := range settings {
		level := levelByKey[setting.GetLevelKey()]
		if level == nil {
			return nil, viber.FailedPreconditionf("company enjoy level %d is missing", setting.GetLevelKey())
		}
		if _, exists := received[setting.GetLevelKey()]; !exists &&
			character.GetCompanyEnjoyPoint() >= level.GetRequiredPoint() {
			keys = append(keys, setting.GetLevelKey())
		}
	}
	sort.SliceStable(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys, nil
}

func validateUniqueCompanyGradeIDs(gradeIDs []string) error {
	if len(gradeIDs) == 0 {
		return viber.InvalidArgumentf("gradeIds is required")
	}
	seen := make(map[string]struct{}, len(gradeIDs))
	for _, gradeID := range gradeIDs {
		if gradeID == "" {
			return viber.InvalidArgumentf("gradeId must not be empty")
		}
		if _, exists := seen[gradeID]; exists {
			return viber.InvalidArgumentf("gradeIds must not contain duplicates")
		}
		seen[gradeID] = struct{}{}
	}
	return nil
}

func validateCompanyMissionIDs(ids []string) error {
	if len(ids) == 0 {
		return viber.InvalidArgumentf("missionIds is required")
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return viber.InvalidArgumentf("missionId must not be empty")
		}
		if _, exists := seen[id]; exists {
			return viber.InvalidArgumentf("missionIds must not contain duplicates")
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Company mission progress is first reported as Clear. A later progress event
// changes that state to NotReceived, which matches the two-step transition in
// the captured gift-use sequence.
func advanceCompanyMissions(
	ctx context.Context,
	suid string,
	missionType penum.MissionType,
	amount int64,
	targetIDs ...string,
) ([]*ptransaction.UserMission, error) {
	existing, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, err
	}
	type missionSnapshot struct {
		status    penum.MissionStatusType
		progress  int64
		threshold int64
	}
	before := make(map[string]missionSnapshot, len(existing))
	for _, mission := range existing {
		before[mission.GetMissionId()] = missionSnapshot{
			status:    mission.GetStatusType(),
			progress:  mission.GetProgress(),
			threshold: mission.GetCurrentThreshold(),
		}
	}

	changed, err := viber.AdvanceUserMissions(ctx, suid, missionType, amount, targetIDs...)
	if err != nil {
		return nil, err
	}
	for _, mission := range changed {
		previous, ok := before[mission.GetMissionId()]
		if !ok || previous.status != penum.MissionStatusType_MissionStatusType_Progress ||
			previous.progress >= previous.threshold || mission.GetProgress() < previous.threshold {
			continue
		}
		mission.StatusType = penum.MissionStatusType_MissionStatusType_Clear
		if err := opuser.UpsertUserMission(ctx, suid, mission); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

func receiveCompanyMissions(
	ctx context.Context,
	suid string,
	missionIDs []string,
	enjoyOnly bool,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, map[string]int64, []string, error) {
	masters, err := opmaster.FindMissionsByIds(ctx, missionIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	masterByID := make(map[string]*pmaster.Mission, len(masters))
	for _, mission := range masters {
		masterByID[mission.GetId()] = mission
	}
	userMissions, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, nil, nil, err
	}
	userByID := make(map[string]*ptransaction.UserMission, len(userMissions))
	for _, mission := range userMissions {
		userByID[mission.GetMissionId()] = mission
	}

	results := make([]*ptransaction.RewardResult, 0)
	beforeByCharacter := make(map[string]int64)
	characterOrder := make([]string, 0)
	for _, missionID := range missionIDs {
		master := masterByID[missionID]
		if master == nil {
			return nil, nil, nil, viber.InvalidArgumentf("unknown missionId %q", missionID)
		}
		isTrust := master.GetCompanyTrustMissionCategoryType() != penum.CompanyTrustMissionCategoryType_CompanyTrustMissionCategoryType_Unknown
		characterID := master.GetCompanyEnjoyCharacterId()
		if !isTrust && characterID == "" {
			return nil, nil, nil, viber.InvalidArgumentf("mission %q is not a Company mission", missionID)
		}
		if enjoyOnly && characterID == "" {
			return nil, nil, nil, viber.InvalidArgumentf("mission %q is not an enjoy mission", missionID)
		}
		userMission := userByID[missionID]
		if userMission == nil {
			return nil, nil, nil, viber.FailedPreconditionf("mission %q is not active", missionID)
		}
		if userMission.GetStatusType() != penum.MissionStatusType_MissionStatusType_NotReceived {
			return nil, nil, nil, viber.FailedPreconditionf("mission %q reward is not receivable", missionID)
		}
		progress, err := companyMissionProgress(master, userMission.GetCurrentThreshold())
		if err != nil {
			return nil, nil, nil, err
		}
		if characterID != "" {
			if _, exists := beforeByCharacter[characterID]; !exists {
				character, err := requireOwnedCompanyCharacter(ctx, suid, characterID)
				if err != nil {
					return nil, nil, nil, err
				}
				beforeByCharacter[characterID] = character.GetCompanyEnjoyPoint()
				characterOrder = append(characterOrder, characterID)
			}
		}
		resources, err := viber.ResolveRewardResources(ctx, progress.GetRewardSetId(), 1)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, resource := range resources {
			granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
			if err != nil {
				return nil, nil, nil, err
			}
			results = append(results, granted...)
		}
		advanceCompanyMissionAfterReceipt(userMission, master)
		if err := opuser.UpsertUserMission(ctx, suid, userMission); err != nil {
			return nil, nil, nil, err
		}
		putUpdatedCompanyMission(updated, userMission)
	}
	return results, beforeByCharacter, characterOrder, nil
}

func companyMissionProgress(master *pmaster.Mission, threshold int64) (*pmaster.MissionProgress, error) {
	var found *pmaster.MissionProgress
	for _, progress := range master.GetProgresses() {
		if progress.GetThreshold() != threshold {
			continue
		}
		if found != nil {
			return nil, viber.FailedPreconditionf("mission %q has duplicate thresholds", master.GetId())
		}
		found = progress
	}
	if found == nil || found.GetRewardSetId() == "" {
		return nil, viber.FailedPreconditionf("mission %q threshold is invalid", master.GetId())
	}
	return found, nil
}

func advanceCompanyMissionAfterReceipt(userMission *ptransaction.UserMission, master *pmaster.Mission) {
	nextThreshold := int64(0)
	for _, progress := range master.GetProgresses() {
		threshold := progress.GetThreshold()
		if threshold > userMission.GetCurrentThreshold() && (nextThreshold == 0 || threshold < nextThreshold) {
			nextThreshold = threshold
		}
	}
	if nextThreshold == 0 {
		userMission.StatusType = penum.MissionStatusType_MissionStatusType_Received
		return
	}
	userMission.CurrentThreshold = nextThreshold
	if userMission.GetProgress() >= nextThreshold {
		userMission.StatusType = penum.MissionStatusType_MissionStatusType_NotReceived
	} else {
		userMission.StatusType = penum.MissionStatusType_MissionStatusType_Progress
	}
}

func validateUniqueLevelKeys(levelKeys []int32) error {
	seen := make(map[int32]struct{}, len(levelKeys))
	for _, levelKey := range levelKeys {
		if levelKey <= 0 {
			return viber.InvalidArgumentf("levelKeys must be positive")
		}
		if _, exists := seen[levelKey]; exists {
			return viber.InvalidArgumentf("levelKeys must not contain duplicates")
		}
		seen[levelKey] = struct{}{}
	}
	return nil
}

func grantCompanyRewardSet(
	ctx context.Context,
	suid string,
	rewardID string,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	resources, err := viber.ResolveRewardResources(ctx, rewardID, 1)
	if err != nil {
		return nil, err
	}
	results := make([]*ptransaction.RewardResult, 0, len(resources))
	for _, resource := range resources {
		granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, granted...)
	}
	return results, nil
}

func claimCompanyEnjoyLevelRewards(
	ctx context.Context,
	suid string,
	character *ptransaction.UserCharacter,
	levelKeys []int32,
	state *umodel.UserCompanyRewardState,
	requireAll bool,
	updated *papi.UpdatedData,
) ([]*ptransaction.RewardResult, error) {
	if len(levelKeys) == 0 {
		return []*ptransaction.RewardResult{}, nil
	}
	settings, err := opmaster.FindCompanyEnjoyCharacterLevelSettings(ctx, character.GetCharacterId())
	if err != nil {
		return nil, err
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return nil, err
	}
	settingByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevelSetting, len(settings))
	for _, setting := range settings {
		settingByKey[setting.GetLevelKey()] = setting
	}
	levelByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevel, len(levels))
	for _, level := range levels {
		levelByKey[level.GetLevelKey()] = level
	}
	received := int32Set(state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()])
	results := make([]*ptransaction.RewardResult, 0)
	for _, levelKey := range levelKeys {
		setting := settingByKey[levelKey]
		level := levelByKey[levelKey]
		if setting == nil || level == nil {
			return nil, viber.InvalidArgumentf("unknown Company enjoy level %d", levelKey)
		}
		if _, exists := received[levelKey]; exists {
			if requireAll {
				return nil, viber.FailedPreconditionf("Company enjoy level %d reward was already received", levelKey)
			}
			continue
		}
		if character.GetCompanyEnjoyPoint() < level.GetRequiredPoint() {
			if requireAll {
				return nil, viber.FailedPreconditionf("Company enjoy level %d is not reached", levelKey)
			}
			continue
		}
		resources, err := viber.ResolveRewardResources(ctx, setting.GetAchieveRewardId(), 1)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			if resource.GetResourceType() == penum.ResourceType_ResourceType_Item &&
				strings.HasPrefix(resource.GetResourceId(), companyEnjoyStoryItemPrefix) {
				continue
			}
			granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
			if err != nil {
				return nil, err
			}
			results = append(results, granted...)
		}
		received[levelKey] = struct{}{}
		state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()] = append(
			state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()], levelKey,
		)
	}
	sort.SliceStable(state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()], func(i, j int) bool {
		return state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()][i] <
			state.ReceivedEnjoyCharacterLevelKeys[character.GetCharacterId()][j]
	})
	return results, nil
}

func putUpdatedCompanyCharacter(updated *papi.UpdatedData, character *ptransaction.UserCharacter) {
	for index, current := range updated.GetCharacters() {
		if current.GetCharacterId() == character.GetCharacterId() {
			updated.Characters[index] = character
			return
		}
	}
	updated.Characters = append(updated.Characters, character)
}

func putUpdatedCompanyMission(updated *papi.UpdatedData, mission *ptransaction.UserMission) {
	for index, current := range updated.GetMissions() {
		if current.GetMissionId() == mission.GetMissionId() {
			updated.Missions[index] = mission
			return
		}
	}
	updated.Missions = append(updated.Missions, mission)
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func int32Set(values []int32) map[int32]struct{} {
	result := make(map[int32]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
