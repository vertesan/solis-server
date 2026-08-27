package test

import (
	"context"
	"strings"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

func TestCompanyScenario(t *testing.T) {
	setupTestUser(t)

	dbCtx := context.Background()
	client := papi.NewCompanyClient(testConfig.GrpcConn)
	companyBefore, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) < 2 {
		t.Fatal("test user needs at least two characters for the Company scenario")
	}

	grades, err := opmaster.FindAllCompanyGrades(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	trustLevels, err := opmaster.FindAllCompanyTrustLevels(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	top := companyCall(t, func(ctx context.Context) (*papi.CompanyTopResponse, error) {
		return client.Top(ctx, &papi.CompanyTopRequest{})
	})
	requireCompanyCommonResponse(t, top.GetCommonResponse())
	if top.GetCurrentRank() != 1 {
		t.Fatalf("Company.Top current rank = %d, want 1", top.GetCurrentRank())
	}
	assertCompanyGradeInfos(t, top.GetCompanyGradeInfos(), grades, companyPointTotalForTest(companyBefore))
	assertCompanyTrustLevelInfos(t, top.GetTrustLevelInfos(), trustLevels, companyBefore.GetTrustPoint())

	itemSetting := selectCompanyItemSetting(t, characters)
	characterID := itemSetting.GetCharacterId()
	characterBefore, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterID)
	if err != nil {
		t.Fatal(err)
	}
	levelSettings, err := opmaster.FindCompanyEnjoyCharacterLevelSettings(dbCtx, characterID)
	if err != nil {
		t.Fatal(err)
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(dbCtx)
	if err != nil {
		t.Fatal(err)
	}
	enjoyTop := companyCall(t, func(ctx context.Context) (*papi.CompanyEnjoyCharacterTopResponse, error) {
		return client.EnjoyCharacterTop(ctx, &papi.CompanyEnjoyCharacterTopRequest{CharacterId: characterID})
	})
	requireCompanyCommonResponse(t, enjoyTop.GetCommonResponse())
	if enjoyTop.GetCurrentRank() != 1 {
		t.Fatalf("Company.EnjoyCharacterTop current rank = %d, want 1", enjoyTop.GetCurrentRank())
	}
	assertCompanyEnjoyLevelInfos(t, enjoyTop.GetEnjoyLevelInfos(), levelSettings, levels, characterBefore.GetCompanyEnjoyPoint())

	const suppliedItemAmount int64 = 3
	const consumedItemAmount int32 = 2
	if err := opuser.UpsertUserItem(dbCtx, testConfig.TestUserSuid, &ptransaction.UserItem{
		ItemId: itemSetting.GetItemId(),
		Amount: suppliedItemAmount,
	}); err != nil {
		t.Fatal(err)
	}
	useItemMission := selectCompanyMissionByType(
		t,
		characterID,
		penum.MissionType_MissionType_CompanyCharacterUseObtainEnjoyPointItemCount,
	)
	if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, &ptransaction.UserMission{
		MissionId:        useItemMission.GetId(),
		CurrentThreshold: useItemMission.GetProgresses()[0].GetThreshold(),
		StatusType:       penum.MissionStatusType_MissionStatusType_Progress,
	}); err != nil {
		t.Fatal(err)
	}
	useItem := companyCall(t, func(ctx context.Context) (*papi.CompanyUseObtainEnjoyPointItemResponse, error) {
		return client.UseEnjoyObtainPointItem(ctx, &papi.CompanyUseObtainEnjoyPointItemRequest{
			CharacterId:           characterID,
			ItemId:                itemSetting.GetItemId(),
			ConsumptionItemAmount: consumedItemAmount,
		})
	})
	requireCompanyCommonResponse(t, useItem.GetCommonResponse())
	wantPointIncrease := int64(itemSetting.GetObtainPoint()) * int64(consumedItemAmount)
	characterAfterItem, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, characterID)
	if err != nil {
		t.Fatal(err)
	}
	companyAfterItem, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if characterAfterItem.GetCompanyEnjoyPoint() != characterBefore.GetCompanyEnjoyPoint()+wantPointIncrease {
		t.Fatalf("UseEnjoyObtainPointItem character points = %d, want %d",
			characterAfterItem.GetCompanyEnjoyPoint(), characterBefore.GetCompanyEnjoyPoint()+wantPointIncrease)
	}
	if companyAfterItem.GetEnjoyPoint() != companyBefore.GetEnjoyPoint()+wantPointIncrease {
		t.Fatalf("UseEnjoyObtainPointItem company enjoy points = %d, want %d",
			companyAfterItem.GetEnjoyPoint(), companyBefore.GetEnjoyPoint()+wantPointIncrease)
	}
	if got := userItemAmount(t, itemSetting.GetItemId()); got != suppliedItemAmount-int64(consumedItemAmount) {
		t.Fatalf("UseEnjoyObtainPointItem item amount = %d, want %d", got, suppliedItemAmount-int64(consumedItemAmount))
	}
	assertCompanyCharacterBonuses(t, characterAfterItem, levels)
	updatedAfterItem := useItem.GetCommonResponse().GetUpdatedData()
	if findCompanyCharacter(updatedAfterItem.GetCharacters(), characterID) == nil ||
		updatedAfterItem.GetCompany().GetEnjoyPoint() != companyAfterItem.GetEnjoyPoint() {
		t.Fatalf("UseEnjoyObtainPointItem omitted its character or company post-image: %v", updatedAfterItem)
	}
	updatedUseItemMission := findUserMissionByID(updatedAfterItem.GetMissions(), useItemMission.GetId())
	if updatedUseItemMission == nil ||
		updatedUseItemMission.GetStatusType() != penum.MissionStatusType_MissionStatusType_Clear {
		t.Fatalf("UseEnjoyObtainPointItem did not preserve the newly-cleared mission report state: %v", updatedAfterItem.GetMissions())
	}

	rewardLevelKey := selectReachedCompanyEnjoyReward(t, levelSettings, levels, characterAfterItem.GetCompanyEnjoyPoint())
	receiveLevel := companyCall(t, func(ctx context.Context) (*papi.CompanyReceiveEnjoyCharacterLevelRewardsResponse, error) {
		return client.ReceiveEnjoyCharacterLevelRewards(ctx, &papi.CompanyReceiveEnjoyCharacterLevelRewardsRequest{
			CharacterId: characterID,
			LevelKeys:   []int32{rewardLevelKey},
		})
	})
	requireCompanyCommonResponse(t, receiveLevel.GetCommonResponse())
	if len(receiveLevel.GetResults()) == 0 {
		t.Fatalf("ReceiveEnjoyCharacterLevelRewards returned no rewards for reached level %d", rewardLevelKey)
	}
	rewardState, err := opuser.FindUserCompanyRewardState(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt32(rewardState.ReceivedEnjoyCharacterLevelKeys[characterID], rewardLevelKey) {
		t.Fatalf("ReceiveEnjoyCharacterLevelRewards did not persist level %d: %v", rewardLevelKey, rewardState)
	}
	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	_, duplicateErr := client.ReceiveEnjoyCharacterLevelRewards(grpcCtx, &papi.CompanyReceiveEnjoyCharacterLevelRewardsRequest{
		CharacterId: characterID,
		LevelKeys:   []int32{rewardLevelKey},
	})
	cancel()
	if duplicateErr == nil {
		t.Fatalf("ReceiveEnjoyCharacterLevelRewards accepted already-received level %d", rewardLevelKey)
	}

	trustMission := selectCompanyMission(t, "", true)
	trustProgress := trustMission.GetProgresses()[0]
	if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, &ptransaction.UserMission{
		MissionId:        trustMission.GetId(),
		CurrentThreshold: trustProgress.GetThreshold(),
		Progress:         trustProgress.GetThreshold(),
		StatusType:       penum.MissionStatusType_MissionStatusType_NotReceived,
	}); err != nil {
		t.Fatal(err)
	}
	companyBeforeTrust, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	bulkMission := companyCall(t, func(ctx context.Context) (*papi.CompanyBulkReceiveMissionResponse, error) {
		return client.BulkReceiveMission(ctx, &papi.CompanyBulkReceiveMissionRequest{MissionIds: []string{trustMission.GetId()}})
	})
	requireCompanyCommonResponse(t, bulkMission.GetCommonResponse())
	if len(bulkMission.GetResults()) == 0 {
		t.Fatal("Company.BulkReceiveMission returned no reward results")
	}
	companyAfterTrust, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if companyAfterTrust.GetTrustPoint() <= companyBeforeTrust.GetTrustPoint() {
		t.Fatalf("Company.BulkReceiveMission did not add trust points: before=%d after=%d",
			companyBeforeTrust.GetTrustPoint(), companyAfterTrust.GetTrustPoint())
	}
	storedTrustMission, err := findUserMission(dbCtx, trustMission.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if storedTrustMission.GetStatusType() == penum.MissionStatusType_MissionStatusType_NotReceived ||
		findUserMissionByID(bulkMission.GetCommonResponse().GetUpdatedData().GetMissions(), trustMission.GetId()) == nil {
		t.Fatalf("Company.BulkReceiveMission did not advance and return mission %q: db=%v response=%v",
			trustMission.GetId(), storedTrustMission, bulkMission)
	}

	bulkCharacterID := selectOtherCompanyCharacter(t, characters, characterID)
	enjoyMission := selectCompanyMission(t, bulkCharacterID, false)
	enjoyProgress := enjoyMission.GetProgresses()[0]
	bulkCharacterBefore, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, bulkCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	nextRewardLevel := selectNextCompanyEnjoyRewardLevel(t, bulkCharacterID, bulkCharacterBefore.GetCompanyEnjoyPoint())
	companyBeforeBulk, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	preparedPoint := nextRewardLevel.GetRequiredPoint() - 1
	companyBeforeBulk.EnjoyPoint += preparedPoint - bulkCharacterBefore.GetCompanyEnjoyPoint()
	bulkCharacterBefore.CompanyEnjoyPoint = preparedPoint
	bulkLevelSettings, err := opmaster.FindCompanyEnjoyCharacterLevelSettings(dbCtx, bulkCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	pendingBeforeBulkLevelKey := selectReachedCompanyEnjoyReward(t, bulkLevelSettings, levels, preparedPoint)
	if err := opuser.UpsertUserCharacter(dbCtx, testConfig.TestUserSuid, bulkCharacterBefore); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserCompany(dbCtx, testConfig.TestUserSuid, companyBeforeBulk); err != nil {
		t.Fatal(err)
	}
	if err := opuser.UpsertUserMission(dbCtx, testConfig.TestUserSuid, &ptransaction.UserMission{
		MissionId:        enjoyMission.GetId(),
		CurrentThreshold: enjoyProgress.GetThreshold(),
		Progress:         enjoyProgress.GetThreshold(),
		StatusType:       penum.MissionStatusType_MissionStatusType_NotReceived,
	}); err != nil {
		t.Fatal(err)
	}
	bulkEnjoy := companyCall(t, func(ctx context.Context) (*papi.CompanyBulkReceiveEnjoyResponse, error) {
		return client.BulkReceiveEnjoy(ctx, &papi.CompanyBulkReceiveEnjoyRequest{MissionIds: []string{enjoyMission.GetId()}})
	})
	requireCompanyCommonResponse(t, bulkEnjoy.GetCommonResponse())
	if len(bulkEnjoy.GetResults()) != 1 || bulkEnjoy.GetResults()[0].GetCharacterId() != bulkCharacterID {
		t.Fatalf("Company.BulkReceiveEnjoy returned the wrong character results: %v", bulkEnjoy.GetResults())
	}
	bulkResult := bulkEnjoy.GetResults()[0]
	if bulkResult.GetBeforeCharacterEnjoyPoint() != bulkCharacterBefore.GetCompanyEnjoyPoint() {
		t.Fatalf("Company.BulkReceiveEnjoy before point = %d, want %d",
			bulkResult.GetBeforeCharacterEnjoyPoint(), bulkCharacterBefore.GetCompanyEnjoyPoint())
	}
	bulkCharacterAfter, err := opuser.FindUserCharacter(dbCtx, testConfig.TestUserSuid, bulkCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	if bulkCharacterAfter.GetCompanyEnjoyPoint() <= bulkCharacterBefore.GetCompanyEnjoyPoint() {
		t.Fatalf("Company.BulkReceiveEnjoy did not add character points: before=%d after=%d",
			bulkCharacterBefore.GetCompanyEnjoyPoint(), bulkCharacterAfter.GetCompanyEnjoyPoint())
	}
	if len(bulkResult.GetEnjoyCharacterLevelRewardResults()) == 0 {
		t.Fatal("Company.BulkReceiveEnjoy did not auto-receive newly reached character-level rewards")
	}
	storedEnjoyMission, err := findUserMission(dbCtx, enjoyMission.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if storedEnjoyMission.GetStatusType() == penum.MissionStatusType_MissionStatusType_NotReceived {
		t.Fatalf("Company.BulkReceiveEnjoy did not advance mission %q: %v", enjoyMission.GetId(), storedEnjoyMission)
	}
	rewardState, err = opuser.FindUserCompanyRewardState(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt32(rewardState.ReceivedEnjoyCharacterLevelKeys[bulkCharacterID], pendingBeforeBulkLevelKey) {
		t.Fatalf("Company.BulkReceiveEnjoy did not claim already-pending level %d for %q: %v",
			pendingBeforeBulkLevelKey, bulkCharacterID, rewardState)
	}

	topBeforeBulkLevels := companyCall(t, func(ctx context.Context) (*papi.CompanyTopResponse, error) {
		return client.Top(ctx, &papi.CompanyTopRequest{})
	})
	receivableCharacterIDs := topBeforeBulkLevels.GetReceivableLevelRewardEnjoyCharacterIds()
	if len(receivableCharacterIDs) == 0 {
		t.Fatal("Company scenario has no remaining character-level rewards for the bulk receive API")
	}
	charactersBeforeBulkLevels, err := opuser.FindAllUserCharacters(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	pointByCharacterID := make(map[string]int64, len(charactersBeforeBulkLevels))
	for _, character := range charactersBeforeBulkLevels {
		pointByCharacterID[character.GetCharacterId()] = character.GetCompanyEnjoyPoint()
	}
	bulkLevels := companyCall(t, func(ctx context.Context) (*papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse, error) {
		return client.BulkReceiveEnjoyCharacterLevelRewards(ctx, &papi.Empty{})
	})
	requireCompanyCommonResponse(t, bulkLevels.GetCommonResponse())
	if len(bulkLevels.GetResults()) != len(receivableCharacterIDs) {
		t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards result count = %d, want %d from Company.Top: %v",
			len(bulkLevels.GetResults()), len(receivableCharacterIDs), bulkLevels.GetResults())
	}
	for index, result := range bulkLevels.GetResults() {
		if result.GetCharacterId() != receivableCharacterIDs[index] {
			t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards result %d character = %q, want %q",
				index, result.GetCharacterId(), receivableCharacterIDs[index])
		}
		if result.GetBeforeCharacterEnjoyPoint() != pointByCharacterID[result.GetCharacterId()] {
			t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards before point for %q = %d, want %d",
				result.GetCharacterId(), result.GetBeforeCharacterEnjoyPoint(), pointByCharacterID[result.GetCharacterId()])
		}
		if findCompanyCharacter(bulkLevels.GetCommonResponse().GetUpdatedData().GetCharacters(), result.GetCharacterId()) == nil {
			t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards omitted character %q post-image", result.GetCharacterId())
		}
	}
	topAfterBulkLevels := companyCall(t, func(ctx context.Context) (*papi.CompanyTopResponse, error) {
		return client.Top(ctx, &papi.CompanyTopRequest{})
	})
	if len(topAfterBulkLevels.GetReceivableLevelRewardEnjoyCharacterIds()) != 0 {
		t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards left receivable characters: %v",
			topAfterBulkLevels.GetReceivableLevelRewardEnjoyCharacterIds())
	}
	emptyBulkLevels := companyCall(t, func(ctx context.Context) (*papi.CompanyBulkReceiveEnjoyCharacterLevelRewardsResponse, error) {
		return client.BulkReceiveEnjoyCharacterLevelRewards(ctx, &papi.Empty{})
	})
	requireCompanyCommonResponse(t, emptyBulkLevels.GetCommonResponse())
	if len(emptyBulkLevels.GetResults()) != 0 {
		t.Fatalf("Company.BulkReceiveEnjoyCharacterLevelRewards repeated claims: %v", emptyBulkLevels.GetResults())
	}

	trustRewardLevel := selectCompanyTrustRewardLevel(t, trustLevels)
	companyBeforeTrustLevel, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if companyBeforeTrustLevel.GetTrustPoint() < trustRewardLevel.GetRequiredPoint() {
		companyBeforeTrustLevel.TrustPoint = trustRewardLevel.GetRequiredPoint()
		if err := opuser.UpsertUserCompany(dbCtx, testConfig.TestUserSuid, companyBeforeTrustLevel); err != nil {
			t.Fatal(err)
		}
	}
	receiveTrustLevel := companyCall(t, func(ctx context.Context) (*papi.CompanyReceiveTrustLevelRewardsResponse, error) {
		return client.ReceiveTrustLevelRewards(ctx, &papi.CompanyReceiveTrustLevelRewardsRequest{
			LevelKeys: []int32{trustRewardLevel.GetLevelKey()},
		})
	})
	requireCompanyCommonResponse(t, receiveTrustLevel.GetCommonResponse())
	if len(receiveTrustLevel.GetResults()) == 0 {
		t.Fatalf("Company.ReceiveTrustLevelRewards returned no rewards for level %d", trustRewardLevel.GetLevelKey())
	}
	companyAfterTrustLevel, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if companyAfterTrustLevel.GetTrustPoint() != companyBeforeTrustLevel.GetTrustPoint() ||
		receiveTrustLevel.GetCommonResponse().GetUpdatedData().GetCompany().GetTrustPoint() != companyAfterTrustLevel.GetTrustPoint() {
		t.Fatalf("Company.ReceiveTrustLevelRewards changed or omitted Company points: before=%v after=%v response=%v",
			companyBeforeTrustLevel, companyAfterTrustLevel, receiveTrustLevel)
	}
	rewardState, err = opuser.FindUserCompanyRewardState(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt32(rewardState.ReceivedTrustLevelKeys, trustRewardLevel.GetLevelKey()) {
		t.Fatalf("Company.ReceiveTrustLevelRewards did not persist level %d: %v",
			trustRewardLevel.GetLevelKey(), rewardState)
	}
	assertCompanyTrustLevelReceived(t, client, trustRewardLevel.GetLevelKey())
	assertCompanyTrustLevelClaimRejected(t, client, trustRewardLevel.GetLevelKey())

	gradeReward := selectCompanyGradeReward(t, grades)
	companyBeforeGrade, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	currentCompanyPoint := companyPointTotalForTest(companyBeforeGrade)
	if currentCompanyPoint < gradeReward.GetRequiredPoint() {
		companyBeforeGrade.SalaryPoint += gradeReward.GetRequiredPoint() - currentCompanyPoint
		if err := opuser.UpsertUserCompany(dbCtx, testConfig.TestUserSuid, companyBeforeGrade); err != nil {
			t.Fatal(err)
		}
	}
	receiveGrade := companyCall(t, func(ctx context.Context) (*papi.CompanyReceiveGradeRewardsResponse, error) {
		return client.ReceiveGradeRewards(ctx, &papi.CompanyReceiveGradeRewardsRequest{
			GradeIds: []string{gradeReward.GetId()},
		})
	})
	requireCompanyCommonResponse(t, receiveGrade.GetCommonResponse())
	if len(receiveGrade.GetResults()) == 0 {
		t.Fatalf("Company.ReceiveGradeRewards returned no rewards for %q", gradeReward.GetId())
	}
	companyAfterGrade, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	updatedGradeCompany := receiveGrade.GetCommonResponse().GetUpdatedData().GetCompany()
	if companyPointTotalForTest(companyAfterGrade) != companyPointTotalForTest(companyBeforeGrade) ||
		updatedGradeCompany == nil || companyPointTotalForTest(updatedGradeCompany) != companyPointTotalForTest(companyAfterGrade) {
		t.Fatalf("Company.ReceiveGradeRewards changed or omitted Company points: before=%v after=%v response=%v",
			companyBeforeGrade, companyAfterGrade, receiveGrade)
	}
	rewardState, err = opuser.FindUserCompanyRewardState(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(rewardState.ReceivedGradeIds, gradeReward.GetId()) {
		t.Fatalf("Company.ReceiveGradeRewards did not persist grade %q: %v", gradeReward.GetId(), rewardState)
	}
	assertCompanyGradeReceived(t, client, gradeReward.GetId())
	assertCompanyGradeClaimRejected(t, client, gradeReward.GetId())

	companyAfter, err := opuser.FindUserCompany(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	user, err := opuser.FindUser(dbCtx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	ranking := companyCall(t, func(ctx context.Context) (*papi.CompanyListRankingResponse, error) {
		return client.ListRanking(ctx, &papi.Empty{})
	})
	requireCompanyCommonResponse(t, ranking.GetCommonResponse())
	if ranking.GetSelfRank() != 1 || len(ranking.GetRankingInfos()) != 1 {
		t.Fatalf("Company.ListRanking was not self-only: %v", ranking)
	}
	rankInfo := ranking.GetRankingInfos()[0]
	wantCompanyPoint := companyPointTotalForTest(companyAfter)
	if ranking.GetSelfPoint() != wantCompanyPoint || rankInfo.GetCurrentRank() != 1 ||
		rankInfo.GetUserId() != testConfig.TestUserPublicId || rankInfo.GetManagerName() != user.GetName() ||
		rankInfo.GetEmblemId() != user.GetEmblemId() || rankInfo.GetCurrentPoint() != wantCompanyPoint ||
		rankInfo.GetCurrentEnjoyPoint() != companyAfter.GetEnjoyPoint() ||
		rankInfo.GetCurrentTrustPoint() != companyAfter.GetTrustPoint() ||
		rankInfo.GetCurrentSalaryPoint() != companyAfter.GetSalaryPoint() {
		t.Fatalf("Company.ListRanking self row is inconsistent with current user data: %v", ranking)
	}

	enjoyRanking := companyCall(t, func(ctx context.Context) (*papi.CompanyListEnjoyCharacterRankingResponse, error) {
		return client.ListEnjoyCharacterRanking(ctx, &papi.CompanyListEnjoyCharacterRankingRequest{CharacterId: characterID})
	})
	requireCompanyCommonResponse(t, enjoyRanking.GetCommonResponse())
	if enjoyRanking.GetSelfRank() != 1 || len(enjoyRanking.GetRankingInfos()) != 1 {
		t.Fatalf("Company.ListEnjoyCharacterRanking was not self-only: %v", enjoyRanking)
	}
	enjoyRankInfo := enjoyRanking.GetRankingInfos()[0]
	if enjoyRanking.GetSelfPoint() != characterAfterItem.GetCompanyEnjoyPoint() || enjoyRankInfo.GetCurrentRank() != 1 ||
		enjoyRankInfo.GetUserId() != testConfig.TestUserPublicId || enjoyRankInfo.GetManagerName() != user.GetName() ||
		enjoyRankInfo.GetEmblemId() != user.GetEmblemId() ||
		enjoyRankInfo.GetCurrentEnjoyPoint() != characterAfterItem.GetCompanyEnjoyPoint() {
		t.Fatalf("Company.ListEnjoyCharacterRanking self row is inconsistent with current user data: %v", enjoyRanking)
	}
}

func companyCall[T any](t *testing.T, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		var zero T
		t.Fatal(err)
		return zero
	}
	return response
}

func requireCompanyCommonResponse(t *testing.T, response *papi.CommonResponse) {
	t.Helper()
	if response == nil || response.GetUpdatedData() == nil || response.GetDeletedData() == nil {
		t.Fatalf("Company API returned an incomplete common response: %v", response)
	}
}

func assertCompanyGradeInfos(t *testing.T, got []*papi.CompanyGradeInfo, masters []*pmaster.CompanyGrade, point int64) {
	t.Helper()
	masterByID := make(map[string]*pmaster.CompanyGrade, len(masters))
	for _, master := range masters {
		masterByID[master.GetId()] = master
	}
	if len(got) != len(masterByID) {
		t.Fatalf("Company.Top returned %d grade infos, want %d from master data", len(got), len(masterByID))
	}
	for _, info := range got {
		master := masterByID[info.GetId()]
		if master == nil {
			t.Fatalf("Company.Top returned unknown grade %q", info.GetId())
		}
		wantReceivable := master.GetAchieveRewardId() != "" && master.GetRequiredPoint() <= point
		if info.GetIsAlreadyReceived() || info.GetIsReceivable() != wantReceivable {
			t.Fatalf("Company.Top grade %q flags = %v, want receivable=%t received=false",
				info.GetId(), info, wantReceivable)
		}
		delete(masterByID, info.GetId())
	}
	if len(masterByID) != 0 {
		t.Fatalf("Company.Top omitted grade master rows: %v", masterByID)
	}
}

func assertCompanyTrustLevelInfos(t *testing.T, got []*papi.CompanyTrustLevelInfo, masters []*pmaster.CompanyTrustLevel, point int64) {
	t.Helper()
	masterByKey := make(map[int32]*pmaster.CompanyTrustLevel, len(masters))
	for _, master := range masters {
		masterByKey[master.GetLevelKey()] = master
	}
	if len(got) != len(masterByKey) {
		t.Fatalf("Company.Top returned %d trust levels, want %d from master data", len(got), len(masterByKey))
	}
	for _, info := range got {
		master := masterByKey[info.GetLevelKey()]
		if master == nil {
			t.Fatalf("Company.Top returned unknown trust level %d", info.GetLevelKey())
		}
		wantReceivable := master.GetAchieveRewardId() != "" && master.GetRequiredPoint() <= point
		if info.GetIsAlreadyReceived() || info.GetIsReceivable() != wantReceivable {
			t.Fatalf("Company.Top trust level %d flags = %v, want receivable=%t received=false",
				info.GetLevelKey(), info, wantReceivable)
		}
		delete(masterByKey, info.GetLevelKey())
	}
}

func assertCompanyEnjoyLevelInfos(
	t *testing.T,
	got []*papi.CompanyEnjoyCharacterLevelInfo,
	settings []*pmaster.CompanyEnjoyCharacterLevelSetting,
	levels []*pmaster.CompanyEnjoyCharacterLevel,
	point int64,
) {
	t.Helper()
	settingByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevelSetting, len(settings))
	for _, setting := range settings {
		settingByKey[setting.GetLevelKey()] = setting
	}
	levelByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevel, len(levels))
	for _, level := range levels {
		levelByKey[level.GetLevelKey()] = level
	}
	if len(got) != len(settingByKey) {
		t.Fatalf("Company.EnjoyCharacterTop returned %d levels, want %d from master data", len(got), len(settingByKey))
	}
	for _, info := range got {
		setting := settingByKey[info.GetLevelKey()]
		level := levelByKey[info.GetLevelKey()]
		if setting == nil || level == nil || info.GetCharacterId() != setting.GetCharacterId() {
			t.Fatalf("Company.EnjoyCharacterTop returned an unknown or mismatched level: %v", info)
		}
		wantReceivable := setting.GetAchieveRewardId() != "" && level.GetRequiredPoint() <= point
		if info.GetIsAlreadyReceived() || info.GetIsReceivable() != wantReceivable {
			t.Fatalf("Company.EnjoyCharacterTop level %d flags = %v, want receivable=%t received=false",
				info.GetLevelKey(), info, wantReceivable)
		}
		delete(settingByKey, info.GetLevelKey())
	}
}

func selectCompanyItemSetting(t *testing.T, characters []*ptransaction.UserCharacter) *pmaster.CompanyEnjoyObtainPointItemSetting {
	t.Helper()
	settings, err := opmaster.FindAllCompanyEnjoyObtainPointItemSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owned := make(map[string]struct{}, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = struct{}{}
	}
	for _, setting := range settings {
		if _, ok := owned[setting.GetCharacterId()]; !ok || setting.GetItemId() == "" || setting.GetObtainPoint() <= 0 {
			continue
		}
		items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, setting.GetItemId())
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 0 {
			return setting
		}
	}
	t.Fatal("master data has no usable Company enjoy-point item setting for an owned character")
	return nil
}

func selectReachedCompanyEnjoyReward(
	t *testing.T,
	settings []*pmaster.CompanyEnjoyCharacterLevelSetting,
	levels []*pmaster.CompanyEnjoyCharacterLevel,
	point int64,
) int32 {
	t.Helper()
	levelByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevel, len(levels))
	for _, level := range levels {
		levelByKey[level.GetLevelKey()] = level
	}
	for _, setting := range settings {
		level := levelByKey[setting.GetLevelKey()]
		if setting.GetAchieveRewardId() == "" || level == nil || level.GetRequiredPoint() > point {
			continue
		}
		reward, err := opmaster.FindReward(context.Background(), setting.GetAchieveRewardId())
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range reward.GetResources() {
			// The story-bond marker is claimed and persisted, but intentionally
			// has no inventory result because it is not a real user item.
			if resource.GetResourceType() == penum.ResourceType_ResourceType_Item &&
				strings.HasPrefix(resource.GetResourceId(), "item-enjoy-story-bond") {
				continue
			}
			return setting.GetLevelKey()
		}
	}
	t.Fatalf("master data has no reward-bearing Company enjoy level reached at %d points", point)
	return 0
}

func selectNextCompanyEnjoyRewardLevel(
	t *testing.T,
	characterID string,
	point int64,
) *pmaster.CompanyEnjoyCharacterLevel {
	t.Helper()
	settings, err := opmaster.FindCompanyEnjoyCharacterLevelSettings(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settingByKey := make(map[int32]*pmaster.CompanyEnjoyCharacterLevelSetting, len(settings))
	for _, setting := range settings {
		settingByKey[setting.GetLevelKey()] = setting
	}
	for _, level := range levels {
		setting := settingByKey[level.GetLevelKey()]
		if level.GetRequiredPoint() <= point || setting == nil || setting.GetAchieveRewardId() == "" {
			continue
		}
		reward, err := opmaster.FindReward(context.Background(), setting.GetAchieveRewardId())
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range reward.GetResources() {
			if resource.GetResourceType() == penum.ResourceType_ResourceType_Item &&
				strings.HasPrefix(resource.GetResourceId(), "item-enjoy-story-bond") {
				continue
			}
			return level
		}
	}
	t.Fatalf("master data has no later reward-bearing Company enjoy level for %q after %d points", characterID, point)
	return nil
}

func selectCompanyTrustRewardLevel(
	t *testing.T,
	levels []*pmaster.CompanyTrustLevel,
) *pmaster.CompanyTrustLevel {
	t.Helper()
	for _, level := range levels {
		if level.GetLevelKey() <= 0 || level.GetAchieveRewardId() == "" {
			continue
		}
		reward, err := opmaster.FindReward(context.Background(), level.GetAchieveRewardId())
		if err != nil {
			t.Fatal(err)
		}
		if len(reward.GetResources()) > 0 {
			return level
		}
	}
	t.Fatal("master data has no reward-bearing Company trust level")
	return nil
}

func selectCompanyGradeReward(t *testing.T, grades []*pmaster.CompanyGrade) *pmaster.CompanyGrade {
	t.Helper()
	for _, grade := range grades {
		if grade.GetId() == "" || grade.GetAchieveRewardId() == "" {
			continue
		}
		reward, err := opmaster.FindReward(context.Background(), grade.GetAchieveRewardId())
		if err != nil {
			t.Fatal(err)
		}
		if len(reward.GetResources()) > 0 {
			return grade
		}
	}
	t.Fatal("master data has no reward-bearing Company grade")
	return nil
}

func assertCompanyTrustLevelReceived(t *testing.T, client papi.CompanyClient, levelKey int32) {
	t.Helper()
	top := companyCall(t, func(ctx context.Context) (*papi.CompanyTopResponse, error) {
		return client.Top(ctx, &papi.CompanyTopRequest{})
	})
	for _, info := range top.GetTrustLevelInfos() {
		if info.GetLevelKey() != levelKey {
			continue
		}
		if !info.GetIsAlreadyReceived() || info.GetIsReceivable() {
			t.Fatalf("Company.Top trust level %d flags after receipt = %v", levelKey, info)
		}
		return
	}
	t.Fatalf("Company.Top omitted trust level %d after receipt", levelKey)
}

func assertCompanyTrustLevelClaimRejected(t *testing.T, client papi.CompanyClient, levelKey int32) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := client.ReceiveTrustLevelRewards(ctx, &papi.CompanyReceiveTrustLevelRewardsRequest{
		LevelKeys: []int32{levelKey},
	}); err == nil {
		t.Fatalf("Company.ReceiveTrustLevelRewards accepted already-received level %d", levelKey)
	}
}

func assertCompanyGradeReceived(t *testing.T, client papi.CompanyClient, gradeID string) {
	t.Helper()
	top := companyCall(t, func(ctx context.Context) (*papi.CompanyTopResponse, error) {
		return client.Top(ctx, &papi.CompanyTopRequest{})
	})
	for _, info := range top.GetCompanyGradeInfos() {
		if info.GetId() != gradeID {
			continue
		}
		if !info.GetIsAlreadyReceived() || info.GetIsReceivable() {
			t.Fatalf("Company.Top grade %q flags after receipt = %v", gradeID, info)
		}
		return
	}
	t.Fatalf("Company.Top omitted grade %q after receipt", gradeID)
}

func assertCompanyGradeClaimRejected(t *testing.T, client papi.CompanyClient, gradeID string) {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := client.ReceiveGradeRewards(ctx, &papi.CompanyReceiveGradeRewardsRequest{
		GradeIds: []string{gradeID},
	}); err == nil {
		t.Fatalf("Company.ReceiveGradeRewards accepted already-received grade %q", gradeID)
	}
}

func selectCompanyMission(t *testing.T, characterID string, trust bool) *pmaster.Mission {
	t.Helper()
	missions, err := opmaster.FindAllMissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantResourceID := "company_grade_point-trust"
	if !trust {
		wantResourceID = "company_grade_point-enjoy-" + strings.TrimPrefix(characterID, "char-")
	}
	for _, mission := range missions {
		if trust {
			if mission.GetCompanyTrustMissionCategoryType() == 0 || mission.GetCompanyEnjoyCharacterId() != "" {
				continue
			}
		} else if mission.GetCompanyEnjoyCharacterId() != characterID || mission.GetCompanyEnjoyCharacterMissionCategoryType() == 0 {
			continue
		}
		if len(mission.GetProgresses()) == 0 || mission.GetProgresses()[0].GetThreshold() <= 0 ||
			!strings.Contains(mission.GetProgresses()[0].GetRewardSetId(), ";company_point;"+wantResourceID+";") {
			continue
		}
		return mission
	}
	t.Fatalf("master data has no suitable Company mission for character=%q trust=%t", characterID, trust)
	return nil
}

func selectCompanyMissionByType(
	t *testing.T,
	characterID string,
	missionType penum.MissionType,
) *pmaster.Mission {
	t.Helper()
	missions, err := opmaster.FindAllMissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, mission := range missions {
		if mission.GetMissionType() == missionType && mission.GetCompanyEnjoyCharacterId() == characterID &&
			len(mission.GetProgresses()) > 0 && mission.GetProgresses()[0].GetThreshold() > 0 {
			return mission
		}
	}
	t.Fatalf("master data has no Company mission type %s for %q", missionType, characterID)
	return nil
}

func selectOtherCompanyCharacter(t *testing.T, characters []*ptransaction.UserCharacter, excluded string) string {
	t.Helper()
	for _, character := range characters {
		if character.GetCharacterId() == excluded {
			continue
		}
		missions, err := opmaster.FindAllMissions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		resourceID := "company_grade_point-enjoy-" + strings.TrimPrefix(character.GetCharacterId(), "char-")
		for _, mission := range missions {
			if mission.GetCompanyEnjoyCharacterId() == character.GetCharacterId() && len(mission.GetProgresses()) > 0 &&
				strings.Contains(mission.GetProgresses()[0].GetRewardSetId(), ";company_point;"+resourceID+";") {
				return character.GetCharacterId()
			}
		}
	}
	t.Fatal("master data has no second owned character with a Company enjoy mission")
	return ""
}

func assertCompanyCharacterBonuses(t *testing.T, character *ptransaction.UserCharacter, levels []*pmaster.CompanyEnjoyCharacterLevel) {
	t.Helper()
	want := map[penum.ParameterType]int64{
		penum.ParameterType_ParameterType_Dance:     0,
		penum.ParameterType_ParameterType_Vocal:     0,
		penum.ParameterType_ParameterType_Visual:    0,
		penum.ParameterType_ParameterType_Stamina:   0,
		penum.ParameterType_ParameterType_Mental:    0,
		penum.ParameterType_ParameterType_Technique: 0,
	}
	for _, level := range levels {
		if level.GetRequiredPoint() <= character.GetCompanyEnjoyPoint() &&
			level.GetBonusParameterType() != penum.ParameterType_ParameterType_Unknown {
			want[level.GetBonusParameterType()] += int64(level.GetBonusParameterUpPermil())
		}
	}
	got := make(map[penum.ParameterType]int64, len(character.GetCompanyBonusParameterInfos()))
	for _, bonus := range character.GetCompanyBonusParameterInfos() {
		if _, duplicate := got[bonus.GetBonusParameterType()]; duplicate {
			t.Fatalf("character has duplicate Company bonus parameter type %s: %v", bonus.GetBonusParameterType(), character)
		}
		got[bonus.GetBonusParameterType()] = bonus.GetBonusParameterUpPermil()
	}
	if len(got) != len(want) {
		t.Fatalf("character Company bonuses = %v, want %v", got, want)
	}
	for parameterType, amount := range want {
		if got[parameterType] != amount {
			t.Fatalf("character Company bonus %s = %d, want %d", parameterType, got[parameterType], amount)
		}
	}
}

func findCompanyCharacter(characters []*ptransaction.UserCharacter, characterID string) *ptransaction.UserCharacter {
	for _, character := range characters {
		if character.GetCharacterId() == characterID {
			return character
		}
	}
	return nil
}

func companyPointTotalForTest(company *ptransaction.UserCompany) int64 {
	return company.GetEnjoyPoint() + company.GetTrustPoint() + company.GetSalaryPoint()
}

func containsInt32(values []int32, want int32) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
