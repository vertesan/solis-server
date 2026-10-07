package exercise

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	exerciselogic "github.com/vertesan/solis-server/internal/makino/exercise"
	"github.com/vertesan/solis-server/internal/makino/live"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExerciseServer) Start(ctx context.Context, req *papi.ExerciseStartRequest) (*papi.ExerciseStartResponse, error) {
	if req.GetExerciseId() == "" {
		return nil, viber.InvalidArgumentf("exerciseId is required")
	}
	suid, now := viber.GetSuid(ctx).String(), time.Now().UnixMilli()
	var response *papi.ExerciseStartResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		// Serialize first-clear rewards with other quest starts. An identical
		// formation replays the same simulation, but never awards items twice.
		if _, err := opuser.LockUserQuestStart(ctx, suid); err != nil {
			return nil, err
		}
		catalog, err := loadAvailableCatalog(ctx, suid, req.GetExerciseId())
		if err != nil {
			return nil, err
		}
		if catalog.Quest.GetClearScore() == 0 {
			return nil, status.Error(codes.Unimplemented, "Exercise battle opponent data is not available")
		}
		saved, err := loadSavedPositions(ctx, suid, req.GetExerciseId())
		if err != nil {
			return nil, err
		}
		positions, err := catalog.ResolvePositions(saved, true)
		if err != nil {
			return nil, err
		}
		input, err := prepareExerciseInput(ctx, suid, catalog, positions)
		if err != nil {
			return nil, err
		}
		result, err := live.Simulate(ctx, input)
		if err != nil {
			var unsupported *live.UnsupportedError
			if errors.As(err, &unsupported) {
				return nil, status.Error(codes.Unimplemented, unsupported.Error())
			}
			return nil, err
		}
		result.DetailLogId = catalog.Quest.GetId()
		updated := &papi.UpdatedData{}
		progress, err := opuser.FindUserQuest(ctx, suid, catalog.Quest.GetId())
		if errors.Is(err, mongo.ErrNoDocuments) {
			progress = &ptransaction.UserQuest{QuestId: catalog.Quest.GetId()}
		} else if err != nil {
			return nil, err
		}
		if !viber.SameJSTDay(progress.GetLastClearedTime(), now) {
			progress.DailyClearCount = 0
		}
		var rewards []*ptransaction.RewardResult
		if result.GetCleared() {
			if progress.GetLastClearedTime() == 0 && catalog.Exercise.GetRewardId() != "" {
				resources, err := viber.ResolveRewardResources(ctx, catalog.Exercise.GetRewardId(), 1)
				if err != nil {
					return nil, err
				}
				for _, resource := range resources {
					granted, err := viber.GrantExchangeRewards(ctx, suid, resource, updated)
					if err != nil {
						return nil, err
					}
					rewards = append(rewards, granted...)
				}
			}
			if progress.GetDailyClearCount() == math.MaxInt32 {
				return nil, viber.FailedPreconditionf("Exercise clear count overflow")
			}
			progress.DailyClearCount++
			progress.LastClearedTime = now
			score := result.GetUserInfos()[0].GetUserResult().GetScore()
			if score > progress.GetHighestScore() {
				progress.HighestScore, progress.HighestScoreTime = score, now
			}
			if err := viber.UnlockLiveMusic(ctx, suid, catalog.Quest.GetMusicId(), updated); err != nil {
				return nil, err
			}
		}
		if err := opuser.UpsertUserQuest(ctx, suid, progress); err != nil {
			return nil, err
		}
		updated.Quests = []*ptransaction.UserQuest{progress}
		// Captures initialize the Exercise area, but do not advance its ordinary
		// main-live clear counters. Per-lesson conditions use UserQuest instead.
		areas, err := opuser.FindAllUserAreas(ctx, suid)
		if err != nil {
			return nil, err
		}
		found := false
		for _, area := range areas {
			found = found || area.GetAreaId() == catalog.Quest.GetAreaId()
		}
		if !found {
			area := &ptransaction.UserArea{AreaId: catalog.Quest.GetAreaId()}
			if err := opuser.UpsertUserArea(ctx, suid, area); err != nil {
				return nil, err
			}
			updated.Areas = []*ptransaction.UserArea{area}
		}
		response = &papi.ExerciseStartResponse{QuestId: catalog.Quest.GetId(), Result: result,
			RankPatterns: viber.LiveClearRankPatterns(catalog.Quest.GetClearScore()), ClearRewards: rewards, CommonResponse: viber.NewCommonResponse(updated)}
		return nil, nil
	})
	return response, viber.GameplayError(err)
}

func prepareExerciseInput(ctx context.Context, suid string, catalog *exerciselogic.Catalog, positions []*papi.ExerciseDeckPosition) (*live.Input, error) {
	// A fixed seed, not request/user/time entropy: changing a warehouse
	// or re-running this deck cannot reroll the result. Jitter and buff crits
	// are present in official captures; only the captured first seven lessons
	// suppress natural criticals. Later lessons can teach technique equipment.
	seed, coefficient, disableBaseCritical, err := exerciselogic.ScoringParameters(catalog.Exercise.GetId(), catalog.Quest.GetMusicId())
	if err != nil {
		return nil, err
	}
	input, _, err := viber.NewLiveInput(ctx, catalog.Quest, seed)
	if err != nil {
		return nil, err
	}
	input.DisableBaseCriticalRate = disableBaseCritical
	input.BeatScoreCoefficientPermil = coefficient
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	self := &papi.LiveUserInfo{Index: 1, Name: user.GetName(), EmblemId: user.GetEmblemId(), UserDeck: &papi.LiveUserDeck{}}
	input.Users = []*papi.LiveUserInfo{self}
	assets, err := exerciselogic.CharacterAssets(ctx, catalog, positions)
	if err != nil {
		return nil, err
	}
	var photos []viber.PreparedPhotoBonus
	u := catalog.User
	for i, position := range positions {
		master, err := opmaster.FindCard(ctx, position.GetCardId())
		if err != nil {
			return nil, err
		}
		character, err := opmaster.FindCharacter(ctx, master.GetCharacterId())
		if err != nil {
			return nil, err
		}
		base, err := usercard.CalcUserCardBaseAttr(ctx, master, u.GetCardLevel(), u.GetCardRarity())
		if err != nil {
			return nil, err
		}
		var fixed *pmaster.ExerciseDeckPosition
		for _, candidate := range catalog.Fixed.GetPositions() {
			if candidate.GetPosition() == position.GetPosition() {
				fixed = candidate
				break
			}
		}
		c := &papi.LiveDeckCard{Index: position.GetPosition(), CardId: master.GetId(), Level: u.GetCardLevel(), Rarity: u.GetCardRarity(),
			Vocal: base.Vocal + int64(u.GetAddVocal()) + int64(fixed.GetAddVocal()), Dance: base.Dance + int64(u.GetAddDance()) + int64(fixed.GetAddDance()),
			Visual: base.Visual + int64(u.GetAddVisual()) + int64(fixed.GetAddVisual()), Stamina: base.Stamina + int64(u.GetAddStamina()) + int64(fixed.GetAddStamina()),
			Mental:      int64(input.Setting.GetCardDefaultMental()) + int64(u.GetAddMental()) + int64(fixed.GetAddMental()),
			Technique:   int64(input.Setting.GetCardDefaultTechnique()) + int64(u.GetAddTechnique()) + int64(fixed.GetAddTechnique()),
			SkillLevel1: u.GetSkillLevel1(), SkillLevel2: u.GetSkillLevel2(), SkillLevel3: u.GetSkillLevel3(),
			DisplayCharacterId: assets[i].GetCharacterId(), DisplayCostumeId: assets[i].GetCostumeId(), DisplayHairId: assets[i].GetHairId()}
		// ExerciseUser explicitly controls which native skills are learned;
		// later lessons can deliberately leave skill3 locked at high card levels.
		for index, id := range []string{master.GetSkillId1(), master.GetSkillId2(), master.GetSkillId3()} {
			level := []int32{u.GetSkillLevel1(), u.GetSkillLevel2(), u.GetSkillLevel3()}[index]
			if id != "" && level > 0 {
				c.Skills = append(c.Skills, &papi.LiveDeckCardSkill{Index: int32(index + 1), SkillId: id, SkillLevel: level, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card})
			}
		}
		bonuses := map[penum.PhotoAbilityType]int64{}
		for _, id := range []string{position.GetPart1AccessoryId(), position.GetPart2AccessoryId()} {
			if id == "" {
				continue
			}
			accessory, err := opmaster.FindAccessory(ctx, id)
			if err != nil {
				return nil, err
			}
			for _, pair := range []struct {
				typ         penum.AccessoryParameterType
				flat, ratio int32
			}{
				{accessory.GetParam1Type(), accessory.GetParam1Value(), accessory.GetParam1Permil()},
				{accessory.GetParam2Type(), accessory.GetParam2Value(), accessory.GetParam2Permil()},
			} {
				if pair.typ == penum.AccessoryParameterType_AccessoryParameterType_Unknown {
					continue
				}
				flat, ratio, ok := viber.AccessoryPhotoTypes(pair.typ)
				if !ok || pair.flat < 0 || pair.ratio < 0 {
					return nil, viber.FailedPreconditionf("invalid Exercise accessory parameters")
				}
				bonuses[flat] += int64(pair.flat)
				bonuses[ratio] += int64(pair.ratio)
			}
		}
		for _, id := range position.GetPhotoAllInOneIds() {
			photo, err := opmaster.FindPhotoAllInOne(ctx, id)
			if err != nil {
				return nil, err
			}
			for _, ability := range photo.GetAbilities() {
				if ability.GetMissionId() != "" {
					continue
				}
				definitions, err := opmaster.FindPhotoAbilities(ctx, []string{ability.GetPhotoAbilityId()})
				if err != nil {
					return nil, err
				}
				if len(definitions) != 1 {
					return nil, viber.FailedPreconditionf("Exercise photo ability is missing")
				}
				d := definitions[0]
				if d.GetMoodType() != penum.MoodType_MoodType_Unknown && d.GetMoodType() != catalog.Quest.GetMoodType() ||
					d.GetConditionCardType() != penum.CardType_CardType_Unknown && d.GetConditionCardType() != master.GetType() ||
					d.GetConditionLiveType() != penum.LiveType_LiveType_Unknown && d.GetConditionLiveType() != penum.LiveType_LiveType_Score {
					continue
				}
				photos = append(photos, viber.PreparedPhotoBonus{Source: c.GetIndex(), Definition: d, Amount: int64(ability.GetEffectValue())})
			}
		}
		input.Cards[c.Index] = live.CardData{Bonuses: bonuses, CharacterID: master.GetCharacterId(), GroupID: character.GetCharacterGroupId(),
			Type: master.GetType(), Attribute: viber.LiveCardAttribute(master), LightFans: int64(u.GetLightFanAmount()), MiddleFans: int64(u.GetMiddleFanAmount()), HeavyFans: int64(u.GetHeavyFanAmount())}
		self.UserDeck.Cards = append(self.UserDeck.Cards, c)
	}
	if err := viber.ApplyPreparedPhotoBonuses(ctx, input, self, photos); err != nil {
		return nil, err
	}
	var actors []live.CardData
	for _, c := range self.UserDeck.Cards {
		if err := viber.ApplyLiveCardStats(c, input.Cards[c.Index].Bonuses); err != nil {
			return nil, err
		}
		c.StartStamina = c.GetStamina()
		actors = append(actors, input.Cards[c.Index])
	}
	audiences, err := live.AllocateAudience(actors, catalog.Quest.GetMaxCapacity(), input.Setting)
	if err != nil {
		return nil, viber.FailedPreconditionf("%v", err)
	}
	for i, c := range self.UserDeck.Cards {
		c.AudienceAmount = audiences[i]
	}
	if err := viber.PrepareLiveQuestBonus(ctx, input); err != nil {
		return nil, err
	}
	if err := viber.ResolveLiveSkills(ctx, input); err != nil {
		return nil, err
	}
	return input, nil
}
