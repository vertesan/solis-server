package viber

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	companyEnjoyPointPrefix = "company_grade_point-enjoy-"
	companyTrustPointID     = "company_grade_point-trust"
	CompanyUnlockCondition  = "cd-function_lock-company"
	// The spelling is part of the official master-data identifier.
	companySalaryPointID = "company_grade_point-salaly"
)

// AddCompanyEnjoyPoints applies character-specific enjoy points to both the
// character and the company aggregate, then rebuilds the character's live and
// activity bonus parameters from the current master level table.
func AddCompanyEnjoyPoints(
	ctx context.Context,
	suid string,
	characterID string,
	amount int64,
	updated *papi.UpdatedData,
) (*ptransaction.UserCharacter, *ptransaction.UserCompany, error) {
	if characterID == "" || amount <= 0 {
		return nil, nil, status.Error(codes.FailedPrecondition, "company enjoy point target and positive amount are required")
	}
	character, err := opuser.FindUserCharacter(ctx, suid, characterID)
	if err != nil {
		return nil, nil, err
	}
	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return nil, nil, err
	}
	if character.GetCompanyEnjoyPoint() < 0 || company.GetEnjoyPoint() < 0 ||
		character.GetCompanyEnjoyPoint() > math.MaxInt64-amount || company.GetEnjoyPoint() > math.MaxInt64-amount {
		return nil, nil, status.Error(codes.FailedPrecondition, "company enjoy point amount overflows")
	}
	character.CompanyEnjoyPoint += amount
	company.EnjoyPoint += amount
	if err := RecomputeCompanyCharacterBonuses(ctx, character); err != nil {
		return nil, nil, err
	}
	if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
		return nil, nil, err
	}
	if err := opuser.UpsertUserCompany(ctx, suid, company); err != nil {
		return nil, nil, err
	}
	if updated != nil {
		putUpdatedCompanyCharacter(updated, character)
		updated.Company = company
	}
	if err := EnsureUnlockedCompanyEnjoyStories(ctx, suid, characterID, updated); err != nil {
		return nil, nil, err
	}
	return character, company, nil
}

// EnsureUnlockedCompanyEnjoyStories materializes unread user-story rows for
// Company stories whose master unlock conditions are currently satisfied.
// Existing read progress is never overwritten.
func EnsureUnlockedCompanyEnjoyStories(
	ctx context.Context,
	suid string,
	characterID string,
	updated *papi.UpdatedData,
) error {
	masterCharacter, err := opmaster.FindCharacter(ctx, characterID)
	if err != nil {
		return err
	}
	evaluator := NewCommerceConditionEvaluator(ctx, suid, time.Now().UnixMilli())
	for _, definition := range masterCharacter.GetCompanyEnjoyStories() {
		if definition.GetStoryId() == "" {
			continue
		}
		unlocked, err := evaluator.Satisfied(definition.GetUnlockConditionId())
		if err != nil {
			return err
		}
		if !unlocked {
			continue
		}
		if _, err := opuser.FindUserStory(ctx, suid, definition.GetStoryId()); err == nil {
			continue
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		story := &ptransaction.UserStory{
			StoryId:    definition.GetStoryId(),
			StatusType: penum.StoryStatusType_StoryStatusType_Unread,
		}
		if err := opuser.UpsertUserStory(ctx, suid, story); err != nil {
			return err
		}
		if updated != nil {
			updated.Stories = append(updated.Stories, story)
		}
	}
	return nil
}

// RecomputeCompanyCharacterBonuses derives the cumulative bonus vector from
// the character's current enjoy points. Persisting the character remains the
// caller's responsibility.
func RecomputeCompanyCharacterBonuses(ctx context.Context, character *ptransaction.UserCharacter) error {
	if character == nil || character.GetCompanyEnjoyPoint() < 0 {
		return status.Error(codes.FailedPrecondition, "valid company character data is required")
	}
	levels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return err
	}
	bonuses := map[penum.ParameterType]int64{
		penum.ParameterType_ParameterType_Dance:     0,
		penum.ParameterType_ParameterType_Vocal:     0,
		penum.ParameterType_ParameterType_Visual:    0,
		penum.ParameterType_ParameterType_Stamina:   0,
		penum.ParameterType_ParameterType_Mental:    0,
		penum.ParameterType_ParameterType_Technique: 0,
	}
	for _, level := range levels {
		if level == nil || level.GetRequiredPoint() > character.GetCompanyEnjoyPoint() ||
			level.GetBonusParameterType() == penum.ParameterType_ParameterType_Unknown {
			continue
		}
		parameterType := level.GetBonusParameterType()
		increment := int64(level.GetBonusParameterUpPermil())
		if increment < 0 || bonuses[parameterType] > math.MaxInt64-increment {
			return status.Error(codes.FailedPrecondition, "company bonus parameter overflows")
		}
		bonuses[parameterType] += increment
	}
	types := make([]penum.ParameterType, 0, len(bonuses))
	for parameterType := range bonuses {
		types = append(types, parameterType)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	character.CompanyBonusParameterInfos = make([]*ptransaction.CompanyBonusParameterInfo, 0, len(types))
	for _, parameterType := range types {
		character.CompanyBonusParameterInfos = append(character.CompanyBonusParameterInfos, &ptransaction.CompanyBonusParameterInfo{
			BonusParameterType:     parameterType,
			BonusParameterUpPermil: bonuses[parameterType],
		})
	}
	return nil
}

// GrantCompanyPoint grants one CompanyPoint reward. RewardResult amounts are
// expressed as total company points, matching the official API, while the
// resource ID determines which component is mutated.
func GrantCompanyPoint(
	ctx context.Context,
	suid string,
	resourceID string,
	amount int64,
	updated *papi.UpdatedData,
) (*ptransaction.RewardResult, error) {
	if resourceID == "" || amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "company point resource and positive amount are required")
	}
	if _, err := opmaster.FindCompanyPoint(ctx, resourceID); err != nil {
		return nil, invalidExchangeMasterResource("company point", resourceID, err)
	}
	company, err := opuser.FindUserCompany(ctx, suid)
	if err != nil {
		return nil, err
	}
	before, err := CompanyPointTotal(company)
	if err != nil || before > math.MaxInt64-amount {
		return nil, status.Error(codes.FailedPrecondition, "company point amount overflows")
	}

	switch {
	case strings.HasPrefix(resourceID, companyEnjoyPointPrefix):
		suffix := strings.TrimPrefix(resourceID, companyEnjoyPointPrefix)
		if suffix == "" {
			return nil, status.Error(codes.FailedPrecondition, "company enjoy point resource is malformed")
		}
		_, company, err = AddCompanyEnjoyPoints(ctx, suid, "char-"+suffix, amount, updated)
		if err != nil {
			return nil, err
		}
	case resourceID == companyTrustPointID:
		if company.GetTrustPoint() < 0 || company.GetTrustPoint() > math.MaxInt64-amount {
			return nil, status.Error(codes.FailedPrecondition, "company trust point amount overflows")
		}
		company.TrustPoint += amount
		if err := opuser.UpsertUserCompany(ctx, suid, company); err != nil {
			return nil, err
		}
		if updated != nil {
			updated.Company = company
		}
	case resourceID == companySalaryPointID:
		if company.GetSalaryPoint() < 0 || company.GetSalaryPoint() > math.MaxInt64-amount {
			return nil, status.Error(codes.FailedPrecondition, "company salary point amount overflows")
		}
		company.SalaryPoint += amount
		if err := opuser.UpsertUserCompany(ctx, suid, company); err != nil {
			return nil, err
		}
		if updated != nil {
			updated.Company = company
		}
	default:
		return nil, status.Errorf(codes.FailedPrecondition, "company point resource %q has no supported destination", resourceID)
	}
	return exchangeRewardResult(
		penum.ResourceType_ResourceType_CompanyPoint,
		resourceID,
		before,
		before+amount,
		amount,
	), nil
}

func CompanyPointTotal(company *ptransaction.UserCompany) (int64, error) {
	if company == nil || company.GetEnjoyPoint() < 0 || company.GetTrustPoint() < 0 || company.GetSalaryPoint() < 0 ||
		company.GetEnjoyPoint() > math.MaxInt64-company.GetTrustPoint() ||
		company.GetEnjoyPoint()+company.GetTrustPoint() > math.MaxInt64-company.GetSalaryPoint() {
		return 0, status.Error(codes.FailedPrecondition, "company point total overflows")
	}
	return company.GetEnjoyPoint() + company.GetTrustPoint() + company.GetSalaryPoint(), nil
}

func putUpdatedCompanyCharacter(updated *papi.UpdatedData, character *ptransaction.UserCharacter) {
	for index, existing := range updated.Characters {
		if existing.GetCharacterId() == character.GetCharacterId() {
			updated.Characters[index] = character
			return
		}
	}
	updated.Characters = append(updated.Characters, character)
}
