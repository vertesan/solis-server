package company

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
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
