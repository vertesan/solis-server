package viber

import (
	"context"
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/makino/live"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LiveDeckCharacterAssets is shared by asset prefetch and live preparation:
// both must choose the same owned replacement and exactly the same appearance.
// Supplied master characters avoid duplicate DB reads in the Start adapter.
func LiveDeckCharacterAssets(ctx context.Context, positions []*ptransaction.UserDeckPosition, originalIDs []string, characters []*ptransaction.UserCharacter, masters map[string]*pmaster.Character) ([]*papi.LiveCharacterAssetInfo, error) {
	if len(positions) != len(originalIDs) {
		return nil, FailedPreconditionf("live appearance inputs differ in size")
	}
	owned := make(map[string]*ptransaction.UserCharacter, len(characters))
	ordered := make([]string, 0, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = character
		ordered = append(ordered, character.GetCharacterId())
	}
	sort.Strings(ordered)
	reserved, used := map[string]bool{}, map[string]bool{}
	for _, id := range originalIDs {
		reserved[id] = true
	}
	assets := make([]*papi.LiveCharacterAssetInfo, 0, len(positions))
	for index, position := range positions {
		id, costumeID, hairID := originalIDs[index], position.GetCostumeId(), position.GetHairId()
		if used[id] {
			// Replacement selection is an authored deterministic rule. Never pick
			// an unowned idol or steal a later original idol's display identity.
			id, costumeID, hairID = "", "", ""
			for _, candidate := range ordered {
				if !reserved[candidate] && !used[candidate] {
					id = candidate
					break
				}
			}
		}
		character := owned[id]
		if character == nil {
			return nil, FailedPreconditionf("not enough owned idols for the live appearance")
		}
		asset, err := questCharacterAsset(ctx, character, masters[id], costumeID, hairID)
		if err != nil {
			return nil, err
		}
		used[id] = true
		assets = append(assets, asset)
	}
	return assets, nil
}

// Start and asset prefetch share extra performer selection. Extra performers
// have no cards and never participate in audience or score calculations.
func PrepareExtraLiveAppearances(ctx context.Context, suid string, music *pmaster.Music, characters []*ptransaction.UserCharacter, masters map[string]*pmaster.Character, displayed []*papi.LiveCharacterAssetInfo) ([]*papi.ExtraCharacterInfo, error) {
	owned := make(map[string]*ptransaction.UserCharacter, len(characters))
	ids := make([]string, 0, len(characters))
	for _, character := range characters {
		owned[character.GetCharacterId()] = character
		ids = append(ids, character.GetCharacterId())
	}
	used := make([]string, 0, len(displayed))
	for _, asset := range displayed {
		used = append(used, asset.GetCharacterId())
	}
	chosen, err := live.ExtraCharacterIDs(music.GetDisplayCharacterAmount(), ids, used)
	if err != nil {
		return nil, FailedPreconditionf("%v", err)
	}
	result := make([]*papi.ExtraCharacterInfo, 0, len(chosen))
	for i, id := range chosen {
		asset, err := questCharacterAsset(ctx, owned[id], masters[id], "", "")
		if err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserCostume(ctx, suid, asset.GetCostumeId()); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserHair(ctx, suid, asset.GetHairId()); err != nil {
			return nil, err
		}
		result = append(result, &papi.ExtraCharacterInfo{Position: int32(len(displayed) + i + 1), CharacterId: asset.GetCharacterId(), CostumeId: asset.GetCostumeId(), HairId: asset.GetHairId()})
	}
	return result, nil
}

func questCharacterAsset(ctx context.Context, character *ptransaction.UserCharacter, master *pmaster.Character, costumeID, hairID string) (*papi.LiveCharacterAssetInfo, error) {
	if character == nil {
		return nil, FailedPreconditionf("live appearance requires an owned idol")
	}
	var err error
	if master == nil {
		master, err = opmaster.FindCharacter(ctx, character.GetCharacterId())
		if err != nil {
			return nil, err
		}
	}
	if master.GetId() != character.GetCharacterId() {
		return nil, FailedPreconditionf("live appearance character does not match master")
	}
	if costumeID == "" {
		costumeID = character.GetLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultLiveCostumeId()
	}
	if costumeID == "" {
		costumeID = master.GetDefaultCostumeId()
	}
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	if costume.GetCharacterId() != character.GetCharacterId() || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
		return nil, FailedPreconditionf("deck costume has no usable live model for this idol")
	}
	if hairID == "" {
		hairID = character.GetLiveHairId()
	}
	if hairID == "" {
		hairID = costume.GetDefaultHairId()
	}
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return nil, err
	}
	fits := func(hair *pmaster.Hair) bool {
		return hair.GetCharacterId() == character.GetCharacterId() && !slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) && (len(hair.GetWearableCostumeIds()) == 0 || slices.Contains(hair.GetWearableCostumeIds(), costumeID))
	}
	if !fits(hair) {
		// Global live hair can become incompatible with a deck-specific outfit.
		// Prefetch and Start must both use the costume's compatible default.
		hairID = costume.GetDefaultHairId()
		hair, err = opmaster.FindHair(ctx, hairID)
		if err != nil {
			return nil, err
		}
		if !fits(hair) {
			return nil, FailedPreconditionf("costume default hair has no compatible live model")
		}
	}
	return &papi.LiveCharacterAssetInfo{CharacterId: character.GetCharacterId(), CostumeId: costumeID, HairId: hairID}, nil
}

// LiveFormation preserves lineup references, not frozen card/equipment stats.
// Bulk plays revalidate these IDs against current possessions without altering a
// saved user deck. Ordinary Start reads its deck fresh on each transaction retry.
type LiveFormation struct {
	Name      string
	Positions []*ptransaction.UserDeckPosition
}

func LoadLiveFormation(ctx context.Context, suid string, number int32) (*LiveFormation, error) {
	deck, err := opuser.FindUserDeck(ctx, suid, number)
	if err != nil {
		return nil, err
	}
	positions, err := opuser.FindUserDeckPositions(ctx, suid, number)
	if err != nil {
		return nil, err
	}
	return &LiveFormation{Name: deck.GetName(), Positions: positions}, nil
}

func GrantLiveResource(ctx context.Context, suid string, resourceType penum.ResourceType, id string, amount int64, updated *papi.UpdatedData) ([]*ptransaction.RewardResult, error) {
	var results []*ptransaction.RewardResult
	for amount > 0 {
		chunk := min(amount, int64(math.MaxInt32))
		if resourceType == penum.ResourceType_ResourceType_Card {
			chunk = 1
		}
		rows, err := GrantGachaReward(ctx, suid, &pmaster.RewardResource{ResourceType: resourceType, ResourceId: id, Amount: int32(chunk)}, nil, updated)
		if err != nil {
			return nil, err
		}
		results = append(results, rows...)
		last := make(map[string]int, len(updated.Missions))
		for i, mission := range updated.Missions {
			last[mission.GetMissionId()] = i
		}
		missions := updated.Missions[:0]
		for i, mission := range updated.Missions {
			if last[mission.GetMissionId()] == i {
				missions = append(missions, mission)
			}
		}
		updated.Missions = missions
		amount -= chunk
	}
	return results, nil
}

func LiveResultCharacterIDs(input *live.Input, result *papi.LiveResult) []string {
	var ids []string
	if len(result.GetUserInfos()) == 0 {
		return ids
	}
	for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
		id := input.Cards[card.GetIndex()].CharacterID
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// LiveClearRankPatterns drives the per-beat progress display for non-ranking
// lives. These are display grades only, not additional reward thresholds.
func LiveClearRankPatterns(clearScore int64) []*papi.QuestRankPattern {
	clear := max(int64(0), clearScore)
	return []*papi.QuestRankPattern{
		{RankType: penum.ResultRankType_ResultRankType_B, ThresholdScore: int64(min(float64(math.MaxInt64-1024), math.Floor(float64(clear)*1.2)))},
		{RankType: penum.ResultRankType_ResultRankType_C, ThresholdScore: clear},
		{RankType: penum.ResultRankType_ResultRankType_D},
	}
}

// GrantLiveMastery grants per-character mastery and its threshold rewards. The
// optional accumulator attributes actual diamond rewards to their character,
// for Dream's bulk-skip reward summary.
func GrantLiveMastery(ctx context.Context, suid, musicID string, characters []string, count int32, reward *papi.QuestReward, updated *papi.UpdatedData, stoneByCharacter ...map[string]int64) error {
	if musicID == "" {
		return nil
	}
	levels, err := opmaster.FindAllMusicMasteryLevels(ctx)
	if err != nil {
		return err
	}
	rows, err := opuser.FindUserMusicMasteries(ctx, suid, musicID)
	if err != nil {
		return err
	}
	byID := map[string]*ptransaction.UserCharacterMusic{}
	for _, row := range rows {
		byID[row.GetCharacterId()] = row
	}
	rules, err := opmaster.FindMusicMasteryRewards(ctx, musicID)
	if err != nil {
		return err
	}
	sets := map[string]string{}
	for _, rule := range rules {
		sets[rule.GetCharacterId()] = rule.GetMusicMasteryRewardSetId()
	}
	loaded := map[string][]*pmaster.MusicMasteryRewardSet{}
	for _, id := range characters {
		row := byID[id]
		if row == nil {
			row = &ptransaction.UserCharacterMusic{MusicId: musicID, CharacterId: id}
		}
		before := row.GetMasteryLevel()
		if row.GetMasteryPoint() > math.MaxInt32-count {
			return FailedPreconditionf("music mastery overflows")
		}
		row.MasteryPoint += count
		for _, level := range levels {
			if row.GetMasteryPoint() >= level.GetRequiredMasteryPoint() {
				row.MasteryLevel = max(row.GetMasteryLevel(), level.GetLevel())
			}
		}
		if err := opuser.UpsertUserCharacterMusic(ctx, suid, row); err != nil {
			return err
		}
		updated.CharacterMusics = append(updated.CharacterMusics, row)
		if row.GetMasteryLevel() <= before || sets[id] == "" {
			continue
		}
		rewards, ok := loaded[sets[id]]
		if !ok {
			rewards, err = opmaster.FindMusicMasteryRewardLevels(ctx, sets[id])
			if err != nil {
				return err
			}
			loaded[sets[id]] = rewards
		}
		for _, r := range rewards {
			if r.GetLevel() <= before || r.GetLevel() > row.GetMasteryLevel() {
				continue
			}
			resources, err := ResolveRewardResources(ctx, r.GetRewardId(), 1)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if _, err := GrantLiveResource(ctx, suid, resource.GetResourceType(), resource.GetResourceId(), int64(resource.GetAmount()), updated); err != nil {
					return err
				}
				if resource.GetResourceType() == penum.ResourceType_ResourceType_Stone || resource.GetResourceType() == penum.ResourceType_ResourceType_PaidStone {
					for _, amounts := range stoneByCharacter {
						if amounts != nil {
							amounts[id] += int64(resource.GetAmount())
						}
					}
				}
				reward.MusicMasteryRewards = append(reward.MusicMasteryRewards, &ptransaction.Reward{ResourceType: resource.GetResourceType(), ResourceId: resource.GetResourceId(), Amount: int64(resource.GetAmount())})
			}
		}
	}
	return nil
}

// NewLiveInput loads only shared master rules. Exercise supplies an isolated
// training roster rather than importing the requesting user's owned bonuses.
func NewLiveInput(ctx context.Context, q *pmaster.Quest, seed uint64) (*live.Input, *pmaster.Music, error) {
	settingID, err := config.GetGameSettingId()
	if err != nil {
		return nil, nil, err
	}
	setting, err := opmaster.FindSetting(ctx, settingID)
	if err != nil {
		return nil, nil, err
	}
	input := &live.Input{Quest: q, Setting: setting, Seed: seed, Cards: map[int32]live.CardData{}, Skills: map[string]*pmaster.Skill{}, Efficacies: map[string]*pmaster.SkillEfficacy{}, Triggers: map[string]*pmaster.SkillTrigger{}, Targets: map[string]*pmaster.SkillTarget{}}
	if input.Chart, err = opmaster.FindLiveChart(ctx, q.GetMusicChartPatternId()); err != nil {
		return nil, nil, err
	}
	sort.Slice(input.Chart, func(i, j int) bool { return input.Chart[i].GetNumber() < input.Chart[j].GetNumber() })
	if len(input.Chart) == 0 {
		return nil, nil, FailedPreconditionf("quest has no chart")
	}
	if input.Pressure, err = opmaster.FindLivePressure(ctx, q.GetQuestPressureId()); err != nil {
		return nil, nil, err
	}
	if input.AudienceAdvantages, err = opmaster.FindLiveAudienceAdvantages(ctx, q.GetQuestAudienceAdvantageId()); err != nil {
		return nil, nil, err
	}
	music, err := opmaster.FindMusic(ctx, q.GetMusicId())
	if err != nil {
		return nil, nil, err
	}
	// Use the song's reference chart, not the current quest's easier/harder
	// variant. Some event/360 songs have no canonical -001 chart; in that case
	// the resolver uses this quest's explicit chart with the same approximation.
	reference := input.Chart
	referenceID := "chart-" + strings.TrimPrefix(music.GetId(), "music-") + "-001"
	if referenceID != q.GetMusicChartPatternId() {
		reference, err = opmaster.FindLiveChart(ctx, referenceID)
		if err != nil {
			return nil, nil, err
		}
	}
	input.BeatScoreCoefficientPermil, err = live.BeatScoreCoefficientPermil(music.GetId(), reference, input.Chart)
	if err != nil {
		return nil, nil, err
	}
	if input.ComboAdvantages, err = opmaster.FindLiveComboAdvantages(ctx, music.GetComboAdvantageId()); err != nil {
		return nil, nil, err
	}
	if input.StaminaConsumption, err = opmaster.FindLiveStaminaConsumption(ctx); err != nil {
		return nil, nil, err
	}
	return input, music, nil
}

// PrepareLiveInput resolves all user/master references within the caller's
// snapshot transaction. It performs no writes and no simulator-time DB lookups.
func PrepareLiveInput(ctx context.Context, suid string, req *papi.QuestStartRequest, q *pmaster.Quest, seed uint64, formation *LiveFormation) (*live.Input, error) {
	if q == nil || req == nil || req.GetDeckNumber() <= 0 {
		return nil, InvalidArgumentf("a quest and saved deck are required")
	}
	if len(req.GetUserBuddyDeckPositions()) > 0 {
		return nil, status.Error(codes.Unimplemented, "friend buddy live units are not supported")
	}
	input, music, err := NewLiveInput(ctx, q, seed)
	if err != nil {
		return nil, err
	}
	setting := input.Setting
	if formation == nil {
		return nil, InvalidArgumentf("live formation is required")
	}
	deck := &ptransaction.UserDeck{Number: req.GetDeckNumber(), Name: formation.Name}
	positions := formation.Positions
	if len(positions) != 5 {
		return nil, FailedPreconditionf("a live unit requires five occupied positions")
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].GetPosition() < positions[j].GetPosition() })
	owned, err := opuser.FindAllUserCards(ctx, suid)
	if err != nil {
		return nil, err
	}
	ownedByID := map[string]*ptransaction.UserCard{}
	cardIDs := map[string]bool{}
	for _, c := range owned {
		ownedByID[c.GetCardId()] = c
	}
	for _, p := range positions {
		cardIDs[p.GetCardId()] = true
	}
	var npc *papi.QuestOpponentInfo
	if q.GetClearScore() == 0 {
		npc, err = live.NPCRoster(q.GetId())
		if err != nil {
			return nil, status.Error(codes.Unimplemented, err.Error())
		}
		for _, c := range npc.GetCardInfos() {
			cardIDs[c.GetCardId()] = true
		}
	}
	masters, err := opmaster.FindLiveCards(ctx, liveStringKeys(cardIDs))
	if err != nil {
		return nil, err
	}
	byCard := map[string]*pmaster.Card{}
	for _, c := range masters {
		byCard[c.GetId()] = c
	}
	characters, err := opuser.FindAllUserCharacters(ctx, suid)
	if err != nil {
		return nil, err
	}
	byCharacter := map[string]*ptransaction.UserCharacter{}
	for _, c := range characters {
		byCharacter[c.GetCharacterId()] = c
	}
	masterCharacters, err := opmaster.FindAllCharacters(ctx)
	if err != nil {
		return nil, err
	}
	byMasterCharacter := map[string]*pmaster.Character{}
	for _, c := range masterCharacters {
		byMasterCharacter[c.GetId()] = c
	}
	staffs, err := opuser.FindAllUserStaffs(ctx, suid)
	if err != nil {
		return nil, err
	}
	staffLevels, err := opmaster.FindAllStaffLevels(ctx)
	if err != nil {
		return nil, err
	}
	staffBonus := map[penum.ParameterType]int64{}
	for _, s := range staffs {
		found := s.GetLevel() == 0
		for _, level := range staffLevels {
			if s.GetParameterType() == level.GetParameterType() && s.GetLevel() == level.GetLevel() {
				staffBonus[s.GetParameterType()] = int64(level.GetAdvantage())
				found = true
				break
			}
		}
		if !found {
			return nil, FailedPreconditionf("staff level is missing from master")
		}
	}
	companyLevels, err := opmaster.FindAllCompanyEnjoyCharacterLevels(ctx)
	if err != nil {
		return nil, err
	}
	global, globalPassives, equipmentBoost, err := prepareOwnedLiveAbilities(ctx, owned, q)
	if err != nil {
		return nil, err
	}
	user, err := opuser.FindUser(ctx, suid)
	if err != nil {
		return nil, err
	}
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, err
	}
	self := &papi.LiveUserInfo{Index: 1, UserId: public.GetPublicUserId(), Name: user.GetName(), EmblemId: user.GetEmblemId(), UserDeck: &papi.LiveUserDeck{DeckName: deck.GetName()}}
	input.Users = append(input.Users, self)
	seenCards, seenPhotos := map[string]bool{}, map[string]bool{}
	var photoBonuses []PreparedPhotoBonus
	accessoryCounts := map[string]int64{}
	originalIDs := make([]string, len(positions))
	for i, p := range positions {
		originalIDs[i] = byCard[p.GetCardId()].GetCharacterId()
	}
	appearances, err := LiveDeckCharacterAssets(ctx, positions, originalIDs, characters, byMasterCharacter)
	if err != nil {
		return nil, err
	}
	extra, err := PrepareExtraLiveAppearances(ctx, suid, music, characters, byMasterCharacter, appearances)
	if err != nil {
		return nil, err
	}
	self.UserDeck.ExtraCharacterInfos = extra
	for i, p := range positions {
		if p.GetPosition() != int32(i+1) || seenCards[p.GetCardId()] {
			return nil, InvalidArgumentf("live positions 1..5 and cards must be unique")
		}
		seenCards[p.GetCardId()] = true
		u, m := ownedByID[p.GetCardId()], byCard[p.GetCardId()]
		if u == nil || m == nil {
			return nil, FailedPreconditionf("live card %q is not owned or not in master", p.GetCardId())
		}
		uc, mc := byCharacter[m.GetCharacterId()], byMasterCharacter[m.GetCharacterId()]
		if uc == nil || mc == nil {
			return nil, FailedPreconditionf("live character is missing")
		}
		c := &papi.LiveDeckCard{Index: int32(i + 1), CardId: u.GetCardId(), Level: u.GetLevel(), Rarity: u.GetRarity(), EvolutionLevel: u.GetEvolutionLevel(), Vocal: u.GetVocal(), Dance: u.GetDance(), Visual: u.GetVisual(), Stamina: u.GetStamina(), Mental: int64(setting.GetCardDefaultMental()), Technique: int64(setting.GetCardDefaultTechnique())}
		if c.GetLevel() <= 0 {
			return nil, FailedPreconditionf("live card has no effective level")
		}
		bonuses := map[penum.PhotoAbilityType]int64{}
		for k, v := range global {
			bonuses[k] = v
		}
		for typ, v := range staffBonus {
			flat, _, ok := parameterPhotoTypes(typ)
			if !ok {
				return nil, FailedPreconditionf("unsupported staff parameter %v", typ)
			}
			if err := addLiveBonus(bonuses, flat, v); err != nil {
				return nil, err
			}
		}
		for _, level := range companyLevels {
			if level.GetRequiredPoint() <= uc.GetCompanyEnjoyPoint() && level.GetBonusParameterType() != penum.ParameterType_ParameterType_Unknown {
				_, ratio, ok := parameterPhotoTypes(level.GetBonusParameterType())
				if !ok {
					return nil, FailedPreconditionf("unsupported company parameter %v", level.GetBonusParameterType())
				}
				if err := addLiveBonus(bonuses, ratio, int64(level.GetBonusParameterUpPermil())); err != nil {
					return nil, err
				}
			}
		}
		if err := prepareLiveEquipment(ctx, suid, q, p, u, mc, m.GetType(), c, bonuses, equipmentBoost, &photoBonuses, seenPhotos, accessoryCounts); err != nil {
			return nil, err
		}
		levels := []int32{u.GetSkillLevel1(), u.GetSkillLevel2(), u.GetSkillLevel3(), u.GetSkillLevel4()}
		ids := []string{u.GetSkillId1(), u.GetSkillId2(), u.GetSkillId3(), u.GetSkillId4()}
		if err := prepareNativeSkills(ctx, c, m, ids, levels); err != nil {
			return nil, err
		}
		for j, ref := range globalPassives {
			c.Skills = append(c.Skills, &papi.LiveDeckCardSkill{Index: int32(2001 + j), SkillId: ref.GetSkillId(), SkillLevel: ref.GetSkillLevel(), PossessionType: penum.SkillPossessionType_SkillPossessionType_Ability, LiveAbilityId: ref.GetLiveAbilityId(), LiveAbilityLevel: ref.GetLiveAbilityLevel()})
		}
		appearance := appearances[i]
		// Prefetch is a projection; Start additionally validates ownership of the
		// exact chosen outfit/hair, including a substituted idol's live settings.
		if _, err := opuser.FindUserCostume(ctx, suid, appearance.GetCostumeId()); err != nil {
			return nil, err
		}
		if _, err := opuser.FindUserHair(ctx, suid, appearance.GetHairId()); err != nil {
			return nil, err
		}
		c.DisplayCharacterId, c.DisplayCostumeId, c.DisplayHairId = appearance.GetCharacterId(), appearance.GetCostumeId(), appearance.GetHairId()
		input.Cards[c.Index] = live.CardData{CharacterID: mc.GetId(), GroupID: mc.GetCharacterGroupId(), Type: m.GetType(), Attribute: LiveCardAttribute(m), LightFans: int64(uc.GetLightFanAmount()), MiddleFans: int64(uc.GetMiddleFanAmount()), HeavyFans: int64(uc.GetHeavyFanAmount()), Bonuses: bonuses}
		self.UserDeck.Cards = append(self.UserDeck.Cards, c)
	}
	if err := ApplyPreparedPhotoBonuses(ctx, input, self, photoBonuses); err != nil {
		return nil, err
	}
	var characterAdvantage *pmaster.QuestCharacterAdvantage
	if q.GetQuestCharacterAdvantageId() != "" {
		characterAdvantage, err = opmaster.FindLiveCharacterAdvantage(ctx, q.GetQuestCharacterAdvantageId())
		if err != nil {
			return nil, err
		}
	}
	for _, c := range self.GetUserDeck().GetCards() {
		if err := ApplyLiveCardStats(c, input.Cards[c.Index].Bonuses); err != nil {
			return nil, err
		}
		c.StartStamina = c.Stamina
	}
	for id, count := range accessoryCounts {
		owned, err := opuser.FindUserAccessory(ctx, suid, id)
		if err != nil {
			return nil, err
		}
		if owned.GetAmount() < count {
			return nil, FailedPreconditionf("insufficient accessory copies")
		}
	}
	if npc != nil {
		if err := prepareNPC(ctx, input, npc, byCard, byMasterCharacter); err != nil {
			return nil, err
		}
	}
	for index, data := range input.Cards {
		data.CharacterAdvantagePermil, err = live.ResolveCharacterAdvantage(data, characterAdvantage)
		if err != nil {
			return nil, FailedPreconditionf("%v", err)
		}
		input.Cards[index] = data
	}
	if err := PrepareLiveQuestBonus(ctx, input); err != nil {
		return nil, err
	}
	if err := ResolveLiveSkills(ctx, input); err != nil {
		return nil, err
	}
	var actors []live.CardData
	var cards []*papi.LiveDeckCard
	for _, side := range input.Users {
		for _, c := range side.GetUserDeck().GetCards() {
			actors = append(actors, input.Cards[c.Index])
			cards = append(cards, c)
		}
	}
	audiences, err := live.AllocateAudience(actors, q.GetMaxCapacity(), setting)
	if err != nil {
		return nil, FailedPreconditionf("%v", err)
	}
	for i, c := range cards {
		c.AudienceAmount = audiences[i]
	}
	return input, nil
}

// Attribute is stable card metadata, not the currently largest buffed stat.
// The master has no attribute field; its unique highest ratio identifies it.
// No current master ties exist; future ties prefer Vocal, Dance, then Visual.
func LiveCardAttribute(m *pmaster.Card) penum.AttributeType {
	attribute, highest := penum.AttributeType_AttributeType_Vocal, m.GetVocalRatioPermil()
	if m.GetDanceRatioPermil() > highest {
		attribute, highest = penum.AttributeType_AttributeType_Dance, m.GetDanceRatioPermil()
	}
	if m.GetVisualRatioPermil() > highest {
		attribute = penum.AttributeType_AttributeType_Visual
	}
	return attribute
}

func liveStringKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func parameterPhotoTypes(typ penum.ParameterType) (penum.PhotoAbilityType, penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.ParameterType_ParameterType_Dance:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply, true
	case penum.ParameterType_ParameterType_Vocal:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply, true
	case penum.ParameterType_ParameterType_Visual:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply, true
	case penum.ParameterType_ParameterType_Stamina:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd, penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply, true
	case penum.ParameterType_ParameterType_Mental:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalAdd, penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply, true
	case penum.ParameterType_ParameterType_Technique:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd, penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply, true
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

func AccessoryPhotoTypes(typ penum.AccessoryParameterType) (penum.PhotoAbilityType, penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.AccessoryParameterType_AccessoryParameterType_Dance:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Dance)
	case penum.AccessoryParameterType_AccessoryParameterType_Vocal:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Vocal)
	case penum.AccessoryParameterType_AccessoryParameterType_Visual:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Visual)
	case penum.AccessoryParameterType_AccessoryParameterType_Stamina:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Stamina)
	case penum.AccessoryParameterType_AccessoryParameterType_Mental:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Mental)
	case penum.AccessoryParameterType_AccessoryParameterType_Technique:
		return parameterPhotoTypes(penum.ParameterType_ParameterType_Technique)
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

// The two enums are independent wire contracts; do not convert their numbers.
func liveAbilityPhotoType(typ penum.LiveAbilityType) (penum.PhotoAbilityType, bool) {
	switch typ {
	case penum.LiveAbilityType_LiveAbilityType_DanceAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceAdd, true
	case penum.LiveAbilityType_LiveAbilityType_DanceMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_VocalAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalAdd, true
	case penum.LiveAbilityType_LiveAbilityType_VocalMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_VisualAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualAdd, true
	case penum.LiveAbilityType_LiveAbilityType_VisualMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_StaminaAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd, true
	case penum.LiveAbilityType_LiveAbilityType_StaminaMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_MentalAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalAdd, true
	case penum.LiveAbilityType_LiveAbilityType_MentalMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_TechniqueAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd, true
	case penum.LiveAbilityType_LiveAbilityType_TechniqueMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_ManagerExp:
		return penum.PhotoAbilityType_PhotoAbilityType_ManagerExp, true
	case penum.LiveAbilityType_LiveAbilityType_Gold:
		return penum.PhotoAbilityType_PhotoAbilityType_Gold, true
	case penum.LiveAbilityType_LiveAbilityType_CardExp:
		return penum.PhotoAbilityType_PhotoAbilityType_CardExp, true
	case penum.LiveAbilityType_LiveAbilityType_BeatScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_BeatScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_ActiveSkillScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_ActiveSkillScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_SpecialSkillScoreAdd:
		return penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd, true
	case penum.LiveAbilityType_LiveAbilityType_SpecialSkillScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_CriticalScoreMultiply:
		return penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply, true
	case penum.LiveAbilityType_LiveAbilityType_AudienceAmountUp:
		return penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp, true
	default:
		return penum.PhotoAbilityType_PhotoAbilityType_Unknown, false
	}
}

func addLiveBonus(b map[penum.PhotoAbilityType]int64, k penum.PhotoAbilityType, v int64) error {
	if v < 0 || b[k] > math.MaxInt64-v {
		return FailedPreconditionf("negative or overflowing live bonus")
	}
	b[k] += v
	return nil
}

func ApplyLiveCardStats(c *papi.LiveDeckCard, b map[penum.PhotoAbilityType]int64) error {
	values := []struct {
		value     *int64
		parameter penum.ParameterType
	}{
		{&c.Dance, penum.ParameterType_ParameterType_Dance}, {&c.Vocal, penum.ParameterType_ParameterType_Vocal},
		{&c.Visual, penum.ParameterType_ParameterType_Visual}, {&c.Stamina, penum.ParameterType_ParameterType_Stamina},
		{&c.Mental, penum.ParameterType_ParameterType_Mental}, {&c.Technique, penum.ParameterType_ParameterType_Technique},
	}
	for _, entry := range values {
		value := entry.value
		if *value < 0 {
			return FailedPreconditionf("negative card base parameter")
		}
		flatType, ratioType, _ := parameterPhotoTypes(entry.parameter)
		flat, ratio := b[flatType], b[ratioType]
		n := new(big.Int).Add(big.NewInt(1000), big.NewInt(ratio))
		n.Mul(n, big.NewInt(*value))
		n.Quo(n, big.NewInt(1000))
		n.Add(n, big.NewInt(flat))
		if !n.IsInt64() || n.Sign() < 0 {
			return FailedPreconditionf("live parameter overflows")
		}
		*value = n.Int64()
	}
	return nil
}

type liveEquipmentBoost struct{ accessoryPermil, photoLevel int64 }

func prepareOwnedLiveAbilities(ctx context.Context, owned []*ptransaction.UserCard, q *pmaster.Quest) (map[penum.PhotoAbilityType]int64, []*papi.LiveDeckCardSkill, liveEquipmentBoost, error) {
	var equipment liveEquipmentBoost
	ids := map[string]bool{}
	for _, c := range owned {
		if c.GetLiveAbilityId() != "" && c.GetLiveAbilityLevel() > 0 {
			ids[c.GetLiveAbilityId()] = true
		}
	}
	masters, err := opmaster.FindLiveAbilities(ctx, liveStringKeys(ids))
	if err != nil {
		return nil, nil, equipment, err
	}
	byID := map[string]*pmaster.LiveAbility{}
	for _, m := range masters {
		byID[m.GetId()] = m
	}
	bonuses := map[penum.PhotoAbilityType]int64{}
	var passives []*papi.LiveDeckCardSkill
	for _, c := range owned {
		if c.GetLiveAbilityId() == "" || c.GetLiveAbilityLevel() <= 0 {
			continue
		}
		var chosen *pmaster.LiveAbilityLevel
		for _, level := range byID[c.GetLiveAbilityId()].GetLevels() {
			if level.GetLevel() == c.GetLiveAbilityLevel() && level.GetRequiredCardLevel() <= c.GetLevel() {
				chosen = level
				break
			}
		}
		if chosen == nil {
			return nil, nil, equipment, FailedPreconditionf("owned live ability level is invalid")
		}
		if chosen.GetMoodType() != penum.MoodType_MoodType_Unknown && chosen.GetMoodType() != q.GetMoodType() {
			continue
		}
		if chosen.GetType() == penum.LiveAbilityType_LiveAbilityType_PassiveSkill {
			passives = append(passives, &papi.LiveDeckCardSkill{SkillId: chosen.GetSkillId(), SkillLevel: chosen.GetValue(), LiveAbilityId: c.GetLiveAbilityId(), LiveAbilityLevel: chosen.GetLevel()})
			continue
		}
		var target *int64
		switch chosen.GetType() {
		case penum.LiveAbilityType_LiveAbilityType_AccessoryParameterUp:
			target = &equipment.accessoryPermil
		case penum.LiveAbilityType_LiveAbilityType_PhotoLevelUp:
			target = &equipment.photoLevel
		}
		if target != nil {
			if chosen.GetValue() < 0 || *target > math.MaxInt64-int64(chosen.GetValue()) {
				return nil, nil, equipment, FailedPreconditionf("equipment bonus overflows")
			}
			*target += int64(chosen.GetValue())
			continue
		}
		photoType, ok := liveAbilityPhotoType(chosen.GetType())
		if !ok {
			return nil, nil, equipment, status.Errorf(codes.Unimplemented, "live ability %v is unsupported", chosen.GetType())
		}
		if err := addLiveBonus(bonuses, photoType, int64(chosen.GetValue())); err != nil {
			return nil, nil, equipment, err
		}
	}
	return bonuses, passives, equipment, nil
}

func prepareLiveEquipment(ctx context.Context, suid string, q *pmaster.Quest, p *ptransaction.UserDeckPosition, u *ptransaction.UserCard, character *pmaster.Character, cardType penum.CardType, c *papi.LiveDeckCard, b map[penum.PhotoAbilityType]int64, equipment liveEquipmentBoost, photos *[]PreparedPhotoBonus, seenPhotos map[string]bool, counts map[string]int64) error {
	for part, id := range []string{p.GetPart1AccessoryId(), p.GetPart2AccessoryId()} {
		if id == "" {
			continue
		}
		a, err := opmaster.FindAccessory(ctx, id)
		if err != nil {
			return err
		}
		category := []penum.AccessoryCategoryType{penum.AccessoryCategoryType_AccessoryCategoryType_Part1, penum.AccessoryCategoryType_AccessoryCategoryType_Part2}[part]
		if a.GetCategoryType() != category || a.GetCharacterId() != "" && a.GetCharacterId() != character.GetId() || a.GetCharacterGroupId() != "" && a.GetCharacterGroupId() != character.GetCharacterGroupId() {
			return FailedPreconditionf("accessory cannot be equipped by this card")
		}
		counts[id]++
		for _, pair := range []struct {
			typ         penum.AccessoryParameterType
			flat, ratio int32
		}{{a.GetParam1Type(), a.GetParam1Value(), a.GetParam1Permil()}, {a.GetParam2Type(), a.GetParam2Value(), a.GetParam2Permil()}} {
			if pair.typ == penum.AccessoryParameterType_AccessoryParameterType_Unknown {
				continue
			}
			flat, ratio := int64(pair.flat), int64(pair.ratio)
			boost := equipment.accessoryPermil
			if boost > 0 {
				if boost > math.MaxInt64-1000 || flat > math.MaxInt64/(1000+boost) || ratio > math.MaxInt64/(1000+boost) {
					return FailedPreconditionf("accessory bonus overflows")
				}
				flat = flat * (1000 + boost) / 1000
				ratio = ratio * (1000 + boost) / 1000
			}
			flatType, ratioType, ok := AccessoryPhotoTypes(pair.typ)
			if !ok {
				return FailedPreconditionf("unsupported accessory parameter %v", pair.typ)
			}
			if err := addLiveBonus(b, flatType, flat); err != nil {
				return err
			}
			if err := addLiveBonus(b, ratioType, ratio); err != nil {
				return err
			}
		}
	}
	if len(p.GetPhotoIds()) > int(u.GetPhotoEquipableCount()) {
		return FailedPreconditionf("insufficient photo slots")
	}
	var retouched int
	for _, id := range p.GetPhotoIds() {
		if id == "" || seenPhotos[id] {
			return FailedPreconditionf("photo must not be equipped twice")
		}
		seenPhotos[id] = true
		photo, err := opuser.FindUserPhoto(ctx, suid, id)
		if err != nil {
			return err
		}
		if len(photo.GetCharacterIds()) > 0 && !slices.Contains(photo.GetCharacterIds(), character.GetId()) {
			return FailedPreconditionf("photo is not equipable by this idol")
		}
		if photo.GetPhotoRecipeId() != "" {
			retouched++
		}
		if retouched > 1 {
			return FailedPreconditionf("only one retouched photo may be equipped")
		}
		ids := map[string]bool{}
		for _, ability := range photo.GetAbilities() {
			if ability.GetIsAvailable() {
				ids[ability.GetPhotoAbilityId()] = true
			}
		}
		definitions, err := opmaster.FindPhotoAbilities(ctx, liveStringKeys(ids))
		if err != nil {
			return err
		}
		byID := map[string]*pmaster.PhotoAbility{}
		for _, d := range definitions {
			byID[d.GetId()] = d
		}
		for _, ability := range photo.GetAbilities() {
			if !ability.GetIsAvailable() {
				continue
			}
			d := byID[ability.GetPhotoAbilityId()]
			if d == nil {
				return FailedPreconditionf("photo ability master is missing")
			}
			if d.GetMoodType() != penum.MoodType_MoodType_Unknown && d.GetMoodType() != q.GetMoodType() {
				continue
			}
			if d.GetConditionCardType() != penum.CardType_CardType_Unknown {
				if cardType != d.GetConditionCardType() {
					continue
				}
			}
			kind := penum.LiveType_LiveType_Score
			if q.GetClearScore() == 0 {
				kind = penum.LiveType_LiveType_Battle
			}
			if d.GetConditionLiveType() != penum.LiveType_LiveType_Unknown && d.GetConditionLiveType() != kind {
				continue
			}
			level := int64(photo.GetLevel()) + equipment.photoLevel
			if level <= 0 || level > math.MaxInt32 {
				return FailedPreconditionf("invalid effective photo level")
			}
			effect, err := PhotoAbilityEffectValue(d, int32(level), ability.GetGrade())
			if err != nil {
				return err
			}
			*photos = append(*photos, PreparedPhotoBonus{Source: c.GetIndex(), Definition: d, Amount: effect})
		}
	}
	return nil
}

func preparedPhotoBonusType(typ penum.PhotoAbilityType) bool {
	switch typ {
	case penum.PhotoAbilityType_PhotoAbilityType_DanceAdd,
		penum.PhotoAbilityType_PhotoAbilityType_DanceMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VocalAdd,
		penum.PhotoAbilityType_PhotoAbilityType_VocalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_VisualAdd,
		penum.PhotoAbilityType_PhotoAbilityType_VisualMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_StaminaAdd,
		penum.PhotoAbilityType_PhotoAbilityType_StaminaMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_MentalAdd,
		penum.PhotoAbilityType_PhotoAbilityType_MentalMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_TechniqueAdd,
		penum.PhotoAbilityType_PhotoAbilityType_TechniqueMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ManagerExp,
		penum.PhotoAbilityType_PhotoAbilityType_Gold,
		penum.PhotoAbilityType_PhotoAbilityType_CardExp,
		penum.PhotoAbilityType_PhotoAbilityType_BeatScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_BeatScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_ActiveSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreAdd,
		penum.PhotoAbilityType_PhotoAbilityType_SpecialSkillScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_CriticalScoreMultiply,
		penum.PhotoAbilityType_PhotoAbilityType_AudienceAmountUp,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill1,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill2,
		penum.PhotoAbilityType_PhotoAbilityType_CoolTimeReductionSkill3:
		return true
	default:
		return false
	}
}

type PreparedPhotoBonus struct {
	Source     int32
	Definition *pmaster.PhotoAbility
	Amount     int64
}

// Resolve all recipients before finalizing any card's stats, so a neighbor's
// photo affects earlier and later positions identically. Definitions are loaded
// once per preparation, never from inside the per-beat engine.
func ApplyPreparedPhotoBonuses(ctx context.Context, input *live.Input, user *papi.LiveUserInfo, bonuses []PreparedPhotoBonus) error {
	ids := map[string]bool{}
	for _, bonus := range bonuses {
		if bonus.Definition.GetAbilityTargetId() != "" {
			ids[bonus.Definition.GetAbilityTargetId()] = true
		}
	}
	targets := map[string]*pmaster.PhotoAbilityTarget{}
	if len(ids) > 0 {
		rows, err := opmaster.FindLivePhotoTargets(ctx, liveStringKeys(ids))
		if err != nil {
			return err
		}
		for _, row := range rows {
			targets[row.GetId()] = row
		}
		if len(targets) != len(ids) {
			return FailedPreconditionf("photo target master is missing")
		}
	}
	cards := user.GetUserDeck().GetCards()
	byIndex := make(map[int32]*papi.LiveDeckCard, len(cards))
	for _, card := range cards {
		byIndex[card.GetIndex()] = card
	}
	for _, bonus := range bonuses {
		d := bonus.Definition
		if bonus.Amount < 0 {
			return FailedPreconditionf("negative photo effect")
		}
		recipients, err := live.PhotoTargetIndexes(cards, input.Cards, bonus.Source, targets[d.GetAbilityTargetId()])
		if err != nil {
			return FailedPreconditionf("%v", err)
		}
		for _, index := range recipients {
			data := input.Cards[index]
			switch d.GetAbilityType() {
			case penum.PhotoAbilityType_PhotoAbilityType_PassiveSkill:
				if bonus.Amount <= 0 || bonus.Amount > math.MaxInt32 {
					return FailedPreconditionf("invalid photo passive level")
				}
				card := byIndex[index]
				photoIndex := int32(1001)
				for _, skill := range card.GetSkills() {
					if skill.GetPossessionType() == penum.SkillPossessionType_SkillPossessionType_Photo {
						photoIndex++
					}
				}
				card.Skills = append(card.Skills, &papi.LiveDeckCardSkill{Index: photoIndex, SkillId: d.GetSkillId(), SkillLevel: int32(bonus.Amount), PossessionType: penum.SkillPossessionType_SkillPossessionType_Photo})
			case penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectValueIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_ReceivedStrengthEffectCountIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectValueIncrease,
				penum.PhotoAbilityType_PhotoAbilityType_AffectStrengthEffectCountIncrease:
				if bonus.Amount > math.MaxInt32 || d.GetStatusEffectType() == penum.StatusEffectType_StatusEffectType_Unknown {
					return FailedPreconditionf("invalid status-restricted photo modifier")
				}
				data.StrengthModifiers = append(data.StrengthModifiers, live.StrengthModifier{Type: d.GetAbilityType(), StatusTypes: []penum.StatusEffectType{d.GetStatusEffectType()}, Amount: int32(bonus.Amount)})
			default:
				if !preparedPhotoBonusType(d.GetAbilityType()) {
					return status.Errorf(codes.Unimplemented, "photo ability %v is unsupported", d.GetAbilityType())
				}
				if err := addLiveBonus(data.Bonuses, d.GetAbilityType(), bonus.Amount); err != nil {
					return err
				}
			}
			input.Cards[index] = data
		}
	}
	return nil
}

func prepareNativeSkills(ctx context.Context, c *papi.LiveDeckCard, m *pmaster.Card, ids []string, levels []int32) error {
	_, unlockedIDs, _, err := usercard.CalcUnlocksWithEvolution(ctx, c.GetLevel(), c.GetEvolutionLevel(), m)
	if err != nil {
		return err
	}
	native := []*papi.LiveDeckCardSkill{}
	masterIDs := []string{m.GetSkillId1(), m.GetSkillId2(), m.GetSkillId3(), m.GetSkillId4()}
	for i, id := range ids {
		if id == "" {
			id = masterIDs[i]
		}
		if unlockedIDs[i] == "" || id == "" {
			continue
		}
		level := levels[i]
		if level == 0 {
			return FailedPreconditionf("learned native skill has an invalid level")
		}
		native = append(native, &papi.LiveDeckCardSkill{Index: int32(i + 1), SkillId: id, SkillLevel: level, PossessionType: penum.SkillPossessionType_SkillPossessionType_Card})
	}
	c.Skills = append(native, c.Skills...)
	return nil
}

// NPC visual defaults are independent of the requesting user's preferences.
func prepareNPCAppearance(ctx context.Context, c *papi.LiveDeckCard, m *pmaster.Character) error {
	costumeID := m.GetDefaultCostumeId()
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return err
	}
	if costume.GetCharacterId() != m.GetId() || strings.HasPrefix(costume.GetCostumeTypeId(), "sd-") {
		return FailedPreconditionf("invalid live costume")
	}
	hairID := costume.GetDefaultHairId()
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return err
	}
	if hair.GetCharacterId() != m.GetId() || slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) || len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), costumeID) {
		return FailedPreconditionf("invalid live hair")
	}
	c.DisplayCharacterId, c.DisplayCostumeId, c.DisplayHairId = m.GetId(), costumeID, hairID
	return nil
}

func prepareNPC(ctx context.Context, input *live.Input, roster *papi.QuestOpponentInfo, masters map[string]*pmaster.Card, characters map[string]*pmaster.Character) error {
	bonus, err := live.NPCBalance(input.Quest, input.Setting)
	if err != nil {
		return err
	}
	side := &papi.LiveUserInfo{Index: 2, IsNpc: true, Name: roster.GetName(), UserDeck: &papi.LiveUserDeck{}}
	sort.Slice(roster.CardInfos, func(i, j int) bool { return roster.CardInfos[i].GetPosition() < roster.CardInfos[j].GetPosition() })
	for _, r := range roster.GetCardInfos() {
		m := masters[r.GetCardId()]
		if m == nil {
			return FailedPreconditionf("NPC card master missing")
		}
		if r.GetEvolutionLevel() != 0 {
			return status.Error(codes.Unimplemented, "NPC evolution preparation is not supported")
		}
		base, err := usercard.CalcUserCardBaseAttr(ctx, m, r.GetLevel(), r.GetRarity())
		if err != nil {
			return err
		}
		for _, v := range []int64{base.Vocal, base.Dance, base.Visual} {
			if v > math.MaxInt64-bonus.Primary {
				return FailedPreconditionf("NPC stat overflows")
			}
		}
		if base.Stamina > math.MaxInt64-bonus.Stamina {
			return FailedPreconditionf("NPC stamina overflows")
		}
		c := &papi.LiveDeckCard{Index: 5 + r.GetPosition(), CardId: r.GetCardId(), Level: r.GetLevel(), Rarity: r.GetRarity(), Vocal: base.Vocal + bonus.Primary, Dance: base.Dance + bonus.Primary, Visual: base.Visual + bonus.Primary, Stamina: base.Stamina + bonus.Stamina, Mental: bonus.Mental, Technique: bonus.Technique}
		c.StartStamina = c.Stamina
		if err := prepareNativeSkills(ctx, c, m, []string{m.GetSkillId1(), m.GetSkillId2(), m.GetSkillId3(), m.GetSkillId4()}, []int32{-1, -1, -1, -1}); err != nil {
			return err
		}
		character := characters[m.GetCharacterId()]
		if character == nil {
			return FailedPreconditionf("NPC character missing")
		}
		if err := prepareNPCAppearance(ctx, c, character); err != nil {
			return err
		}
		input.Cards[c.Index] = live.CardData{CharacterID: m.GetCharacterId(), GroupID: character.GetCharacterGroupId(), Type: m.GetType(), Attribute: LiveCardAttribute(m), LightFans: int64(r.GetLightFanAmount()), MiddleFans: int64(r.GetMiddleFanAmount()), HeavyFans: int64(r.GetHeavyFanAmount()), Bonuses: map[penum.PhotoAbilityType]int64{}}
		side.UserDeck.Cards = append(side.UserDeck.Cards, c)
	}
	input.Users = append(input.Users, side)
	return nil
}

func PrepareLiveQuestBonus(ctx context.Context, input *live.Input) error {
	if input.Quest.GetLiveBonusGroupId() == "" {
		return nil
	}
	group, err := opmaster.FindLiveBonusGroup(ctx, input.Quest.GetLiveBonusGroupId())
	if err != nil {
		return err
	}
	bonuses, err := opmaster.FindLiveBonuses(ctx, group.GetLiveBonusIds())
	if err != nil {
		return err
	}
	if len(bonuses) != len(group.GetLiveBonusIds()) {
		return FailedPreconditionf("live bonus group is incomplete")
	}
	byID := map[string]*pmaster.LiveBonus{}
	ids := map[string]bool{}
	for _, b := range bonuses {
		byID[b.GetId()] = b
		ids[b.GetLiveAbilityId()] = true
	}
	abilities, err := opmaster.FindLiveAbilities(ctx, liveStringKeys(ids))
	if err != nil {
		return err
	}
	byAbility := map[string]*pmaster.LiveAbility{}
	for _, a := range abilities {
		byAbility[a.GetId()] = a
	}
	for i, id := range group.GetLiveBonusIds() {
		b := byID[id]
		var level *pmaster.LiveAbilityLevel
		for _, l := range byAbility[b.GetLiveAbilityId()].GetLevels() {
			if l.GetLevel() == b.GetLiveAbilityLevel() {
				level = l
				break
			}
		}
		if level == nil {
			return FailedPreconditionf("quest live bonus level missing")
		}
		if level.GetType() != penum.LiveAbilityType_LiveAbilityType_PassiveSkill {
			return status.Error(codes.Unimplemented, "non-passive quest bonus is not yet supported")
		}
		for _, u := range input.Users {
			u.UserDeck.BonusSkills = append(u.UserDeck.BonusSkills, &papi.LiveDeckCardSkill{Index: 3001 + int32(i), SkillId: level.GetSkillId(), SkillLevel: level.GetValue(), PossessionType: penum.SkillPossessionType_SkillPossessionType_Quest, LiveAbilityId: b.GetLiveAbilityId(), LiveAbilityLevel: b.GetLiveAbilityLevel()})
		}
	}
	return nil
}

func ResolveLiveSkills(ctx context.Context, input *live.Input) error {
	ids := map[string]bool{}
	for _, u := range input.Users {
		for _, c := range u.GetUserDeck().GetCards() {
			for _, s := range c.GetSkills() {
				ids[s.GetSkillId()] = true
			}
		}
		for _, s := range u.GetUserDeck().GetBonusSkills() {
			ids[s.GetSkillId()] = true
		}
	}
	skills, err := opmaster.FindLiveSkills(ctx, liveStringKeys(ids))
	if err != nil {
		return err
	}
	for _, s := range skills {
		input.Skills[s.GetId()] = s
	}
	if len(skills) != len(ids) {
		return FailedPreconditionf("live references missing skill definitions")
	}
	efficacyIDs, triggerIDs, targetIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	collect := func(ref *papi.LiveDeckCardSkill, cardLevel int32) error {
		definition := input.Skills[ref.GetSkillId()]
		if ref.GetSkillLevel() < 0 {
			ref.SkillLevel = 0
			for _, l := range definition.GetLevels() {
				if l.GetRequiredCardLevel() <= cardLevel {
					ref.SkillLevel = max(ref.SkillLevel, l.GetLevel())
				}
			}
		}
		var level *pmaster.SkillLevel
		for _, l := range definition.GetLevels() {
			if l.GetLevel() == ref.GetSkillLevel() {
				level = l
				break
			}
		}
		if level == nil {
			return FailedPreconditionf("invalid skill level for %q", ref.GetSkillId())
		}
		if ref.GetPossessionType() == penum.SkillPossessionType_SkillPossessionType_Card && level.GetRequiredCardLevel() > cardLevel {
			return FailedPreconditionf("native skill level exceeds the card's effective level")
		}
		triggerIDs[level.GetTriggerId()] = true
		for _, d := range level.GetSkillDetails() {
			efficacyIDs[d.GetEfficacyId()] = true
			triggerIDs[d.GetTriggerId()] = true
		}
		for _, id := range ref.GetRewrittenEfficacyIds() {
			efficacyIDs[id] = true
		}
		return nil
	}
	for _, u := range input.Users {
		for _, c := range u.GetUserDeck().GetCards() {
			for _, s := range c.GetSkills() {
				if err := collect(s, c.GetLevel()); err != nil {
					return err
				}
				switch s.GetIndex() {
				case 1:
					c.SkillLevel1 = s.GetSkillLevel()
				case 2:
					c.SkillLevel2 = s.GetSkillLevel()
				case 3:
					c.SkillLevel3 = s.GetSkillLevel()
				case 4:
					c.SkillLevel4 = s.GetSkillLevel()
				}
			}
		}
		for _, s := range u.GetUserDeck().GetBonusSkills() {
			if err := collect(s, math.MaxInt32); err != nil {
				return err
			}
		}
	}
	efficacies, err := opmaster.FindLiveEfficacies(ctx, liveStringKeys(efficacyIDs))
	if err != nil {
		return err
	}
	for _, e := range efficacies {
		input.Efficacies[e.GetId()] = e
		targetIDs[e.GetSkillTargetId()] = true
		// Conditional score encodes an additional trigger inside the efficacy ID
		// rather than SkillLevel/SkillDetail. Resolve it before entering the engine.
		if strings.HasPrefix(e.GetId(), "ef-score_get_by_trigger-") {
			parts := strings.Split(strings.TrimPrefix(e.GetId(), "ef-"), "-")
			if len(parts) >= 6 {
				triggerIDs[strings.Join(parts[2:len(parts)-2], "-")] = true
			}
		}
	}
	if len(efficacies) != len(liveStringKeys(efficacyIDs)) {
		return FailedPreconditionf("live efficacy definition is missing")
	}
	for _, efficacy := range efficacies {
		destination, conversion, err := live.ConversionStatusEfficacyType(efficacy)
		if err != nil {
			return FailedPreconditionf("%v", err)
		}
		if !conversion {
			continue
		}
		found := false
		for _, candidate := range input.Efficacies {
			if candidate.GetType() == destination {
				found = true
				break
			}
		}
		if found {
			continue
		}
		name := efficacy.GetId()[strings.LastIndex(efficacy.GetId(), "-")+1:]
		definition, err := opmaster.FindLiveStatusEfficacy(ctx, name)
		if err != nil {
			return err
		}
		if definition.GetType() != destination {
			return FailedPreconditionf("conversion destination master mismatch")
		}
		input.Efficacies[definition.GetId()] = definition
	}
	triggers, err := opmaster.FindLiveTriggers(ctx, liveStringKeys(triggerIDs))
	if err != nil {
		return err
	}
	for _, t := range triggers {
		input.Triggers[t.GetId()] = t
	}
	if len(triggers) != len(liveStringKeys(triggerIDs)) {
		return FailedPreconditionf("live trigger definition is missing")
	}
	targets, err := opmaster.FindLiveTargets(ctx, liveStringKeys(targetIDs))
	if err != nil {
		return err
	}
	for _, t := range targets {
		input.Targets[t.GetId()] = t
	}
	if len(targets) != len(liveStringKeys(targetIDs)) {
		return FailedPreconditionf("live target definition is missing")
	}
	return nil
}
