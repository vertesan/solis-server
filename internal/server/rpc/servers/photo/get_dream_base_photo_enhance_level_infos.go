package photo

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) GetDreamBasePhotoEnhanceLevelInfos(
	ctx context.Context,
	req *papi.GetDreamBasePhotoEnhanceLevelInfosRequest,
) (*papi.GetDreamBasePhotoEnhanceLevelInfosResponse, error) {
	if req.GetBasePhotoId() == "" {
		return nil, viber.InvalidArgumentf("basePhotoId is required")
	}
	suid := viber.GetSuid(ctx).String()
	photo, err := opuser.FindUserPhoto(ctx, suid, req.GetBasePhotoId())
	if err != nil {
		return nil, viber.PhotoOwnershipError("photo", req.GetBasePhotoId(), err)
	}
	if !photo.GetIsDreamBase() || len(photo.GetAbilities()) == 0 {
		return nil, viber.FailedPreconditionf("photo %q is not an enhanced dream base photo", req.GetBasePhotoId())
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	managerLevel, err := opmaster.FindManagerLevel(ctx, user.GetManagerLevel())
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	abilityIDs := make([]string, 0, len(photo.GetAbilities()))
	for _, ability := range photo.GetAbilities() {
		abilityIDs = append(abilityIDs, ability.GetPhotoAbilityId())
	}
	masterAbilities, err := opmaster.FindPhotoAbilities(ctx, abilityIDs)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	masterByID := make(map[string]*pmaster.PhotoAbility, len(masterAbilities))
	for _, ability := range masterAbilities {
		masterByID[ability.GetId()] = ability
	}

	levels := []int32{photo.GetLevel()}
	if photo.GetLevel() <= managerLevel.GetMaxPhotoEnhanceLevel()-5 {
		levels = append(levels, photo.GetLevel()+5)
	}
	levelInfos := make([]*papi.DreamBasePhotoEnhanceLevelInfo, 0, len(levels))
	for _, level := range levels {
		effects, ok := dreamPhotoEffectsAtLevel(photo.GetAbilities(), masterByID, level)
		if !ok {
			if level == photo.GetLevel() {
				return nil, viber.FailedPreconditionf("photo %q has abilities without level data", req.GetBasePhotoId())
			}
			break
		}
		levelInfos = append(levelInfos, &papi.DreamBasePhotoEnhanceLevelInfo{
			Level:            level,
			EffectValueInfos: effects,
		})
	}
	return &papi.GetDreamBasePhotoEnhanceLevelInfosResponse{
		LevelInfos:     levelInfos,
		CommonResponse: viber.NewCommonResponse(nil),
	}, nil
}

func dreamPhotoEffectsAtLevel(
	abilities []*ptransaction.UserPhotoAbility,
	master map[string]*pmaster.PhotoAbility,
	level int32,
) ([]*papi.DreamBasePhotoEnhanceLevelEffectValueInfo, bool) {
	effects := make([]*papi.DreamBasePhotoEnhanceLevelEffectValueInfo, 0, len(abilities))
	for _, ability := range abilities {
		masterAbility, exists := master[ability.GetPhotoAbilityId()]
		if !exists {
			return nil, false
		}
		var baseValue int32
		levelFound := false
		for _, candidate := range masterAbility.GetPhotoAbilityLevels() {
			if candidate.GetLevel() == level {
				baseValue = candidate.GetValue()
				levelFound = true
				break
			}
		}
		if !levelFound {
			return nil, false
		}
		value := int64(baseValue)
		gradeFound := false
		for _, grade := range masterAbility.GetPhotoAbilityGrades() {
			if grade.GetGrade() == ability.GetGrade() {
				value = value*int64(grade.GetBonusPermil())/1000 + int64(grade.GetBonusValue())
				gradeFound = true
				break
			}
		}
		if !gradeFound {
			return nil, false
		}
		effects = append(effects, &papi.DreamBasePhotoEnhanceLevelEffectValueInfo{
			PhotoAbilityId: ability.GetPhotoAbilityId(),
			EffectValue:    value,
		})
	}
	return effects, true
}
