package viber

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type commerceConditionState uint8

const (
	commerceConditionIndeterminate commerceConditionState = iota
	commerceConditionUnsatisfied
	commerceConditionSatisfied
)

var supportedCommerceConditionFields = map[string]struct{}{
	"satisfyCondition":              {},
	"notSatisfyCondition":           {},
	"startHours":                    {},
	"managerLevel":                  {},
	"deckMaxOverallValue":           {},
	"joinedGuild":                   {},
	"clearQuest":                    {},
	"clearMission":                  {},
	"companyGradeGte":               {},
	"companyPointGte":               {},
	"companyEnjoyLevelGte":          {},
	"companyEnjoyPointGte":          {},
	"companyEnjoyCharacterLevelGte": {},
	"companyTrustLevelGte":          {},
	"companyTrustPointGte":          {},
	"companySalaryPointGte":         {},
	"companyEnjoyCharacterPointGte": {},
	"clearArea":                     {},
	"obtainCard":                    {},
	"obtainCostume":                 {},
	"cardRarity":                    {},
	"term":                          {},
	"gameStartTerm":                 {},
	"itemAmount":                    {},
	"purchaseShop":                  {},
	"readStory":                     {},
	"finishedMessage":               {},
	"reliability":                   {},
	"obtainCharacter":               {},
	"obtainEmblem":                  {},
	"staffTrainTotalCount":          {},
	"hierarchyDetailGradeId":        {},
	"hierarchyPointGte":             {},
}

type CommerceConditionEvaluator struct {
	ctx  context.Context
	suid string
	now  int64

	conditions map[string]*pmaster.Condition
	results    map[string]commerceConditionState
	evaluating map[string]struct{}

	user                   *ptransaction.User
	userLoaded             bool
	cards                  map[string]*ptransaction.UserCard
	cardsLoaded            bool
	costumes               map[string]struct{}
	costumesLoaded         bool
	areas                  map[string]*ptransaction.UserArea
	areasLoaded            bool
	masterAreas            map[string]*pmaster.Area
	items                  map[string]int64
	itemsLoaded            bool
	purchasedShopItems     map[string]struct{}
	shopPurchasesLoaded    bool
	stories                map[string]*ptransaction.UserStory
	storiesLoaded          bool
	messages               map[string]*ptransaction.UserMessage
	messagesLoaded         bool
	characters             map[string]*ptransaction.UserCharacter
	charactersLoaded       bool
	emblems                map[string]struct{}
	emblemsLoaded          bool
	quests                 map[string]*ptransaction.UserQuest
	questsLoaded           bool
	missions               map[string]*ptransaction.UserMission
	missionsLoaded         bool
	totalCount             *ptransaction.UserTotalCount
	totalCountLoaded       bool
	company                *ptransaction.UserCompany
	companyLoaded          bool
	hierarchy              *ptransaction.UserHierarchy
	hierarchyLoaded        bool
	companyEnjoyLevels     []*pmaster.CompanyEnjoyLevel
	companyTrustLevels     []*pmaster.CompanyTrustLevel
	companyCharacterLevels []*pmaster.CompanyEnjoyCharacterLevel
	hierarchyDetailGrades  []*pmaster.HierarchyDetailGrade
}

func NewCommerceConditionEvaluator(ctx context.Context, suid string, now int64) *CommerceConditionEvaluator {
	return &CommerceConditionEvaluator{
		ctx:         ctx,
		suid:        suid,
		now:         now,
		conditions:  make(map[string]*pmaster.Condition),
		results:     make(map[string]commerceConditionState),
		evaluating:  make(map[string]struct{}),
		masterAreas: make(map[string]*pmaster.Area),
	}
}

func (e *CommerceConditionEvaluator) primeConditions(conditions []*pmaster.Condition) {
	for _, condition := range conditions {
		if condition.GetId() != "" {
			e.conditions[condition.GetId()] = condition
		}
	}
}

func (e *CommerceConditionEvaluator) Satisfied(conditionID string) (bool, error) {
	state, err := e.evaluate(conditionID)
	return state == commerceConditionSatisfied, err
}

func (e *CommerceConditionEvaluator) evaluate(conditionID string) (commerceConditionState, error) {
	if conditionID == "" {
		return commerceConditionSatisfied, nil
	}
	if result, ok := e.results[conditionID]; ok {
		return result, nil
	}
	if _, ok := e.evaluating[conditionID]; ok {
		return commerceConditionIndeterminate, nil
	}

	condition, ok := e.conditions[conditionID]
	if !ok {
		var err error
		condition, err = opmaster.FindCondition(e.ctx, conditionID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				e.results[conditionID] = commerceConditionIndeterminate
				return commerceConditionIndeterminate, nil
			}
			return commerceConditionIndeterminate, err
		}
		e.conditions[conditionID] = condition
	}

	e.evaluating[conditionID] = struct{}{}
	defer delete(e.evaluating, conditionID)
	if hasUnknownProtoFields(condition.ProtoReflect()) || hasUnsupportedConditionFields(condition.ProtoReflect()) {
		e.results[conditionID] = commerceConditionIndeterminate
		return commerceConditionIndeterminate, nil
	}

	// Empty master conditions are unconditional. This is also used as the
	// positive side of several explicit not-satisfied commerce conditions.
	if len(condition.GetSettings()) == 0 {
		e.results[conditionID] = commerceConditionSatisfied
		return commerceConditionSatisfied, nil
	}

	result := commerceConditionUnsatisfied
	for _, setting := range condition.GetSettings() {
		settingResult, err := e.evaluateSetting(setting)
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if settingResult == commerceConditionSatisfied {
			e.results[conditionID] = commerceConditionSatisfied
			return commerceConditionSatisfied, nil
		}
		if settingResult == commerceConditionIndeterminate {
			result = commerceConditionIndeterminate
		}
	}
	e.results[conditionID] = result
	return result, nil
}

func (e *CommerceConditionEvaluator) evaluateSetting(setting *pmaster.ConditionSetting) (commerceConditionState, error) {
	if setting == nil {
		return commerceConditionIndeterminate, nil
	}
	if hasUnknownProtoFields(setting.ProtoReflect()) {
		return commerceConditionIndeterminate, nil
	}

	result := commerceConditionSatisfied
	hasKnownField := false
	merge := func(next commerceConditionState) {
		if result == commerceConditionUnsatisfied || next == commerceConditionSatisfied {
			return
		}
		if next == commerceConditionUnsatisfied {
			result = commerceConditionUnsatisfied
			return
		}
		result = commerceConditionIndeterminate
	}

	if value := setting.GetSatisfyCondition(); value != nil {
		hasKnownField = true
		if value.GetConditionId() == "" {
			return commerceConditionIndeterminate, nil
		}
		state, err := e.evaluate(value.GetConditionId())
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		merge(state)
	}
	if value := setting.GetNotSatisfyCondition(); value != nil {
		hasKnownField = true
		if value.GetConditionId() == "" {
			return commerceConditionIndeterminate, nil
		}
		state, err := e.evaluate(value.GetConditionId())
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		switch state {
		case commerceConditionSatisfied:
			merge(commerceConditionUnsatisfied)
		case commerceConditionUnsatisfied:
			merge(commerceConditionSatisfied)
		default:
			merge(commerceConditionIndeterminate)
		}
	}
	if value := setting.GetStartHours(); value != nil {
		hasKnownField = true
		if value.GetHoursLte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		user, err := e.loadUser()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		gameStartTime := user.GetGameStartTime()
		duration := int64(value.GetHoursLte()) * time.Hour.Milliseconds()
		if gameStartTime <= 0 || duration <= 0 || gameStartTime > math.MaxInt64-duration {
			return commerceConditionIndeterminate, nil
		}
		if e.now > gameStartTime+duration {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetTerm(); value != nil {
		hasKnownField = true
		if value.GetNowAfter() <= 0 && value.GetNowBefore() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		if value.GetNowAfter() > 0 && e.now < value.GetNowAfter() ||
			value.GetNowBefore() > 0 && e.now > value.GetNowBefore() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetGameStartTerm(); value != nil {
		hasKnownField = true
		if value.GetGameStartDatetimeAfter() <= 0 && value.GetGameStartDatetimeBefore() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		user, err := e.loadUser()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		gameStartTime := user.GetGameStartTime()
		if gameStartTime <= 0 {
			return commerceConditionIndeterminate, nil
		}
		if value.GetGameStartDatetimeAfter() > 0 && gameStartTime < value.GetGameStartDatetimeAfter() ||
			value.GetGameStartDatetimeBefore() > 0 && gameStartTime > value.GetGameStartDatetimeBefore() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetManagerLevel(); value != nil {
		hasKnownField = true
		if value.GetLevelGte() <= 0 && value.GetLevelLte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		user, err := e.loadUser()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if value.GetLevelGte() > 0 && user.GetManagerLevel() < value.GetLevelGte() ||
			value.GetLevelLte() > 0 && user.GetManagerLevel() > value.GetLevelLte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetDeckMaxOverallValue(); value != nil {
		hasKnownField = true
		if value.GetValueGte() <= 0 && value.GetValueLte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		user, err := e.loadUser()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if value.GetValueGte() > 0 && user.GetDeckMaxOverallValue() < int64(value.GetValueGte()) ||
			value.GetValueLte() > 0 && user.GetDeckMaxOverallValue() > int64(value.GetValueLte()) {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetJoinedGuild(); value != nil {
		hasKnownField = true
		user, err := e.loadUser()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		switch value.GetJoinedGuildType() {
		case penum.JoinedGuildType_JoinedGuildType_Join:
			if user.GetGuildId() == "" {
				merge(commerceConditionUnsatisfied)
			}
		case penum.JoinedGuildType_JoinedGuildType_NotJoin:
			if user.GetGuildId() != "" {
				merge(commerceConditionUnsatisfied)
			}
		default:
			return commerceConditionIndeterminate, nil
		}
	}
	if value := setting.GetClearQuest(); value != nil {
		hasKnownField = true
		if value.GetQuestId() == "" {
			return commerceConditionIndeterminate, nil
		}
		quests, err := e.loadQuests()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		quest := quests[value.GetQuestId()]
		if quest == nil || quest.GetLastClearedTime() <= 0 {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetClearMission(); value != nil {
		hasKnownField = true
		if value.GetMissionId() == "" || value.GetThreshold() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		missions, err := e.loadMissions()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		mission := missions[value.GetMissionId()]
		if mission == nil || mission.GetProgress() < value.GetThreshold() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyGradeGte(); value != nil {
		hasKnownField = true
		if value.GetCompanyGradeId() == "" {
			return commerceConditionIndeterminate, nil
		}
		grade, err := opmaster.FindCompanyGrade(e.ctx, value.GetCompanyGradeId())
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return commerceConditionIndeterminate, nil
			}
			return commerceConditionIndeterminate, err
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		total, err := companyPointTotal(company)
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if total < grade.GetRequiredPoint() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyPointGte(); value != nil {
		hasKnownField = true
		if value.GetPointGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		total, err := companyPointTotal(company)
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if total < value.GetPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyEnjoyLevelGte(); value != nil {
		hasKnownField = true
		if value.GetLevelGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		levels, err := e.loadCompanyEnjoyLevels()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if companyEnjoyLevelAtPoint(levels, company.GetEnjoyPoint()) < value.GetLevelGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyEnjoyPointGte(); value != nil {
		hasKnownField = true
		if value.GetPointGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if company.GetEnjoyPoint() < value.GetPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyEnjoyCharacterLevelGte(); value != nil {
		hasKnownField = true
		if value.GetCharacterId() == "" || value.GetLevelGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		characters, err := e.loadCharacters()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		levels, err := e.loadCompanyCharacterLevels()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		character := characters[value.GetCharacterId()]
		if character == nil || companyCharacterLevelAtPoint(levels, character.GetCompanyEnjoyPoint()) < value.GetLevelGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyTrustLevelGte(); value != nil {
		hasKnownField = true
		if value.GetLevelGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		levels, err := e.loadCompanyTrustLevels()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if companyTrustLevelAtPoint(levels, company.GetTrustPoint()) < value.GetLevelGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyTrustPointGte(); value != nil {
		hasKnownField = true
		if value.GetPointGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if company.GetTrustPoint() < value.GetPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanySalaryPointGte(); value != nil {
		hasKnownField = true
		if value.GetPointGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		company, err := e.loadCompany()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if company.GetSalaryPoint() < value.GetPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCompanyEnjoyCharacterPointGte(); value != nil {
		hasKnownField = true
		if value.GetCharacterId() == "" || value.GetPointGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		characters, err := e.loadCharacters()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		character := characters[value.GetCharacterId()]
		if character == nil || character.GetCompanyEnjoyPoint() < value.GetPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetHierarchyPointGte(); value != nil {
		hasKnownField = true
		if value.GetHierarchyPointGte() < 0 {
			return commerceConditionIndeterminate, nil
		}
		hierarchy, err := e.loadHierarchy()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		point, err := HierarchyTotalPoint(hierarchy)
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if point < value.GetHierarchyPointGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetHierarchyDetailGradeId(); value != nil {
		hasKnownField = true
		if value.GetHierarchyDetailGradeId() == "" {
			return commerceConditionIndeterminate, nil
		}
		hierarchy, err := e.loadHierarchy()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		grades, err := e.loadHierarchyDetailGrades()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		point, err := HierarchyTotalPoint(hierarchy)
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		currentGrade, err := ResolveHierarchyDetailGrade(grades, point, 1)
		if err != nil {
			return commerceConditionIndeterminate, nil
		}
		achievedID, err := StrongestHierarchyDetailGradeID(
			grades, currentGrade.GetId(), hierarchy.GetBestDetailGradeId(),
		)
		if err != nil {
			return commerceConditionIndeterminate, nil
		}
		reached, err := HierarchyDetailGradeReached(grades, achievedID, value.GetHierarchyDetailGradeId())
		if err != nil {
			return commerceConditionIndeterminate, nil
		}
		if !reached {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetClearArea(); value != nil {
		hasKnownField = true
		if value.GetAreaId() == "" {
			return commerceConditionIndeterminate, nil
		}
		state, err := e.hasClearedArea(value.GetAreaId())
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		merge(state)
	}
	if value := setting.GetObtainCard(); value != nil {
		hasKnownField = true
		if value.GetCardId() == "" {
			return commerceConditionIndeterminate, nil
		}
		cards, err := e.loadCards()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if _, ok := cards[value.GetCardId()]; !ok {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetObtainCostume(); value != nil {
		hasKnownField = true
		if value.GetCostumeId() == "" {
			return commerceConditionIndeterminate, nil
		}
		costumes, err := e.loadCostumes()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if _, ok := costumes[value.GetCostumeId()]; !ok {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetCardRarity(); value != nil {
		hasKnownField = true
		if value.GetCardId() == "" || value.GetRarityGte() <= 0 && value.GetRarityLte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		cards, err := e.loadCards()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		card, ok := cards[value.GetCardId()]
		if !ok || value.GetRarityGte() > 0 && card.GetRarity() < value.GetRarityGte() ||
			value.GetRarityLte() > 0 && card.GetRarity() > value.GetRarityLte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetItemAmount(); value != nil {
		hasKnownField = true
		if value.GetItemId() == "" || value.GetAmountGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		items, err := e.loadItems()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if items[value.GetItemId()] < value.GetAmountGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetPurchaseShop(); value != nil {
		hasKnownField = true
		if value.GetShopItemId() == "" {
			return commerceConditionIndeterminate, nil
		}
		purchasedItems, err := e.loadPurchasedShopItems()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if _, ok := purchasedItems[value.GetShopItemId()]; !ok {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetReadStory(); value != nil {
		hasKnownField = true
		if value.GetStoryId() == "" {
			return commerceConditionIndeterminate, nil
		}
		stories, err := e.loadStories()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		story := stories[value.GetStoryId()]
		if story == nil || story.GetStatusType() != penum.StoryStatusType_StoryStatusType_Read {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetFinishedMessage(); value != nil {
		hasKnownField = true
		if value.GetMessageId() == "" {
			return commerceConditionIndeterminate, nil
		}
		messages, err := e.loadMessages()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		message := messages[value.GetMessageId()]
		if message == nil || message.GetMessageStatusType() != penum.MessageStatusType_MessageStatusType_Finished {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetReliability(); value != nil {
		hasKnownField = true
		if value.GetCharacterId() == "" || value.GetReliabilityGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		characters, err := e.loadCharacters()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		character := characters[value.GetCharacterId()]
		if character == nil || character.GetReliability() < value.GetReliabilityGte() {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetObtainCharacter(); value != nil {
		hasKnownField = true
		if value.GetCharacterId() == "" {
			return commerceConditionIndeterminate, nil
		}
		characters, err := e.loadCharacters()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if _, ok := characters[value.GetCharacterId()]; !ok {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetObtainEmblem(); value != nil {
		hasKnownField = true
		if value.GetEmblemId() == "" {
			return commerceConditionIndeterminate, nil
		}
		emblems, err := e.loadEmblems()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if _, ok := emblems[value.GetEmblemId()]; !ok {
			merge(commerceConditionUnsatisfied)
		}
	}
	if value := setting.GetStaffTrainTotalCount(); value != nil {
		hasKnownField = true
		if value.GetCountGte() <= 0 {
			return commerceConditionIndeterminate, nil
		}
		totalCount, err := e.loadTotalCount()
		if err != nil {
			return commerceConditionIndeterminate, err
		}
		if totalCount.GetStaffTrainCount() < int64(value.GetCountGte()) {
			merge(commerceConditionUnsatisfied)
		}
	}

	setting.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if _, ok := supportedCommerceConditionFields[field.JSONName()]; !ok {
			merge(commerceConditionIndeterminate)
		}
		return true
	})
	if !hasKnownField {
		return commerceConditionIndeterminate, nil
	}
	return result, nil
}

func hasUnsupportedConditionFields(message protoreflect.Message) bool {
	unsupported := false
	message.Range(func(field protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		switch field.JSONName() {
		case "id", "settings":
			return true
		default:
			unsupported = true
			return false
		}
	})
	return unsupported
}

func hasUnknownProtoFields(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return true
	}
	found := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsMap() {
			if field.MapValue().Message() == nil {
				return true
			}
			value.Map().Range(func(_ protoreflect.MapKey, mapValue protoreflect.Value) bool {
				if hasUnknownProtoFields(mapValue.Message()) {
					found = true
					return false
				}
				return true
			})
			return !found
		}
		if field.IsList() {
			if field.Message() == nil {
				return true
			}
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				if hasUnknownProtoFields(list.Get(index).Message()) {
					found = true
					return false
				}
			}
			return true
		}
		if field.Message() != nil && hasUnknownProtoFields(value.Message()) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (e *CommerceConditionEvaluator) loadUser() (*ptransaction.User, error) {
	if !e.userLoaded {
		var err error
		e.user, err = opuser.FindUser(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.userLoaded = true
	}
	return e.user, nil
}

func (e *CommerceConditionEvaluator) loadCards() (map[string]*ptransaction.UserCard, error) {
	if !e.cardsLoaded {
		values, err := opuser.FindAllUserCards(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.cards = make(map[string]*ptransaction.UserCard, len(values))
		for _, value := range values {
			e.cards[value.GetCardId()] = value
		}
		e.cardsLoaded = true
	}
	return e.cards, nil
}

func (e *CommerceConditionEvaluator) loadCostumes() (map[string]struct{}, error) {
	if !e.costumesLoaded {
		values, err := opuser.FindAllUserCostumes(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.costumes = make(map[string]struct{}, len(values))
		for _, value := range values {
			e.costumes[value.GetCostumeId()] = struct{}{}
		}
		e.costumesLoaded = true
	}
	return e.costumes, nil
}

func (e *CommerceConditionEvaluator) loadAreas() (map[string]*ptransaction.UserArea, error) {
	if !e.areasLoaded {
		values, err := opuser.FindAllUserAreas(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.areas = make(map[string]*ptransaction.UserArea, len(values))
		for _, value := range values {
			e.areas[value.GetAreaId()] = value
		}
		e.areasLoaded = true
	}
	return e.areas, nil
}

func (e *CommerceConditionEvaluator) hasClearedArea(areaID string) (commerceConditionState, error) {
	areas, err := e.loadAreas()
	if err != nil {
		return commerceConditionIndeterminate, err
	}
	area, ok := areas[areaID]
	if !ok {
		return commerceConditionUnsatisfied, nil
	}

	master, ok := e.masterAreas[areaID]
	if !ok {
		master, err = opmaster.FindArea(e.ctx, areaID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return commerceConditionIndeterminate, nil
			}
			return commerceConditionIndeterminate, err
		}
		e.masterAreas[areaID] = master
	}
	if len(master.GetQuestIds()) == 0 {
		if area.GetLastClearedTime() <= 0 {
			return commerceConditionUnsatisfied, nil
		}
		return commerceConditionSatisfied, nil
	}
	if int64(area.GetClearQuestCount()) < int64(len(master.GetQuestIds())) {
		return commerceConditionUnsatisfied, nil
	}
	return commerceConditionSatisfied, nil
}

func (e *CommerceConditionEvaluator) loadItems() (map[string]int64, error) {
	if !e.itemsLoaded {
		values, err := opuser.FindAllUserItems(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.items = make(map[string]int64)
		for _, value := range values {
			if value.GetAmount() <= 0 || value.GetExpiredTime() > 0 && value.GetExpiredTime() <= e.now {
				continue
			}
			amount := e.items[value.GetItemId()]
			if value.GetAmount() > math.MaxInt64-amount {
				e.items[value.GetItemId()] = math.MaxInt64
				continue
			}
			e.items[value.GetItemId()] = amount + value.GetAmount()
		}
		e.itemsLoaded = true
	}
	return e.items, nil
}

func (e *CommerceConditionEvaluator) loadPurchasedShopItems() (map[string]struct{}, error) {
	if !e.shopPurchasesLoaded {
		purchase, err := opuser.FindUserShopPurchase(e.ctx, e.suid)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		e.purchasedShopItems = make(map[string]struct{})
		if purchase != nil {
			for _, itemID := range purchase.PurchasedItemIds {
				if itemID != "" {
					e.purchasedShopItems[itemID] = struct{}{}
				}
			}
		}
		e.shopPurchasesLoaded = true
	}
	return e.purchasedShopItems, nil
}

func (e *CommerceConditionEvaluator) loadStories() (map[string]*ptransaction.UserStory, error) {
	if !e.storiesLoaded {
		values, err := opuser.FindAllUserStories(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.stories = make(map[string]*ptransaction.UserStory, len(values))
		for _, value := range values {
			e.stories[value.GetStoryId()] = value
		}
		e.storiesLoaded = true
	}
	return e.stories, nil
}

func (e *CommerceConditionEvaluator) loadMessages() (map[string]*ptransaction.UserMessage, error) {
	if !e.messagesLoaded {
		values, err := opuser.FindAllUserMessages(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.messages = make(map[string]*ptransaction.UserMessage, len(values))
		for _, value := range values {
			e.messages[value.GetMessageId()] = value
		}
		e.messagesLoaded = true
	}
	return e.messages, nil
}

func (e *CommerceConditionEvaluator) loadCharacters() (map[string]*ptransaction.UserCharacter, error) {
	if !e.charactersLoaded {
		values, err := opuser.FindAllUserCharacters(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.characters = make(map[string]*ptransaction.UserCharacter, len(values))
		for _, value := range values {
			e.characters[value.GetCharacterId()] = value
		}
		e.charactersLoaded = true
	}
	return e.characters, nil
}

func (e *CommerceConditionEvaluator) loadEmblems() (map[string]struct{}, error) {
	if !e.emblemsLoaded {
		values, err := opuser.FindAllUserEmblems(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.emblems = make(map[string]struct{}, len(values))
		for _, value := range values {
			e.emblems[value.GetEmblemId()] = struct{}{}
		}
		e.emblemsLoaded = true
	}
	return e.emblems, nil
}

func (e *CommerceConditionEvaluator) loadQuests() (map[string]*ptransaction.UserQuest, error) {
	if !e.questsLoaded {
		values, err := opuser.FindAllUserQuests(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.quests = make(map[string]*ptransaction.UserQuest, len(values))
		for _, value := range values {
			e.quests[value.GetQuestId()] = value
		}
		e.questsLoaded = true
	}
	return e.quests, nil
}

func (e *CommerceConditionEvaluator) loadMissions() (map[string]*ptransaction.UserMission, error) {
	if !e.missionsLoaded {
		values, err := opuser.FindAllUserMissions(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.missions = make(map[string]*ptransaction.UserMission, len(values))
		for _, value := range values {
			e.missions[value.GetMissionId()] = value
		}
		e.missionsLoaded = true
	}
	return e.missions, nil
}

func (e *CommerceConditionEvaluator) loadTotalCount() (*ptransaction.UserTotalCount, error) {
	if !e.totalCountLoaded {
		var err error
		e.totalCount, err = opuser.FindUserTotalCount(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.totalCountLoaded = true
	}
	return e.totalCount, nil
}

func (e *CommerceConditionEvaluator) loadCompany() (*ptransaction.UserCompany, error) {
	if !e.companyLoaded {
		var err error
		e.company, err = opuser.FindUserCompany(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.companyLoaded = true
	}
	return e.company, nil
}

func (e *CommerceConditionEvaluator) loadHierarchy() (*ptransaction.UserHierarchy, error) {
	if !e.hierarchyLoaded {
		var err error
		e.hierarchy, err = opuser.FindUserHierarchy(e.ctx, e.suid)
		if err != nil {
			return nil, err
		}
		e.hierarchyLoaded = true
	}
	return e.hierarchy, nil
}

func (e *CommerceConditionEvaluator) loadCompanyEnjoyLevels() ([]*pmaster.CompanyEnjoyLevel, error) {
	if e.companyEnjoyLevels == nil {
		var err error
		e.companyEnjoyLevels, err = opmaster.FindAllCompanyEnjoyLevels(e.ctx)
		if err != nil {
			return nil, err
		}
	}
	return e.companyEnjoyLevels, nil
}

func (e *CommerceConditionEvaluator) loadCompanyTrustLevels() ([]*pmaster.CompanyTrustLevel, error) {
	if e.companyTrustLevels == nil {
		var err error
		e.companyTrustLevels, err = opmaster.FindAllCompanyTrustLevels(e.ctx)
		if err != nil {
			return nil, err
		}
	}
	return e.companyTrustLevels, nil
}

func (e *CommerceConditionEvaluator) loadCompanyCharacterLevels() ([]*pmaster.CompanyEnjoyCharacterLevel, error) {
	if e.companyCharacterLevels == nil {
		var err error
		e.companyCharacterLevels, err = opmaster.FindAllCompanyEnjoyCharacterLevels(e.ctx)
		if err != nil {
			return nil, err
		}
	}
	return e.companyCharacterLevels, nil
}

func (e *CommerceConditionEvaluator) loadHierarchyDetailGrades() ([]*pmaster.HierarchyDetailGrade, error) {
	if e.hierarchyDetailGrades == nil {
		var err error
		e.hierarchyDetailGrades, err = opmaster.FindAllHierarchyDetailGrades(e.ctx)
		if err != nil {
			return nil, err
		}
	}
	return e.hierarchyDetailGrades, nil
}

func companyEnjoyLevelAtPoint(levels []*pmaster.CompanyEnjoyLevel, point int64) int32 {
	var result int32
	for _, level := range levels {
		if level.GetRequiredPoint() <= point && level.GetLevelKey() > result {
			result = level.GetLevelKey()
		}
	}
	return result
}

func companyTrustLevelAtPoint(levels []*pmaster.CompanyTrustLevel, point int64) int32 {
	var result int32
	for _, level := range levels {
		if level.GetRequiredPoint() <= point && level.GetLevelKey() > result {
			result = level.GetLevelKey()
		}
	}
	return result
}

func companyCharacterLevelAtPoint(levels []*pmaster.CompanyEnjoyCharacterLevel, point int64) int32 {
	var result int32
	for _, level := range levels {
		if level.GetRequiredPoint() <= point && level.GetLevelKey() > result {
			result = level.GetLevelKey()
		}
	}
	return result
}

func FilterHomeCommerceIDs(
	ctx context.Context,
	suid string,
	shopIDs []string,
	_ []string,
	boothIDs []string,
) ([]string, []string, []string, []string, []string, []string, error) {
	now := time.Now().UnixMilli()
	evaluator := NewCommerceConditionEvaluator(ctx, suid, now)
	shops, err := opsolis.FindAllShopCatalogEntries(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	legacyShowcaseShopIDs := make(map[string]struct{})
	for _, shop := range shops {
		if shop.Info != nil && IsShowcaseShopType(shop.Info.GetType()) {
			legacyShowcaseShopIDs[shop.ShopId] = struct{}{}
		}
	}
	allShopIDs := make([]string, 0, len(shopIDs)+len(shops))
	shopSet := make(map[string]struct{}, len(shopIDs)+len(shops))
	for _, shopID := range shopIDs {
		_, legacyShowcase := legacyShowcaseShopIDs[shopID]
		if IsShowcaseCommerceShopID(shopID) || legacyShowcase || strings.HasPrefix(shopID, "shop_top_shop-showcase-") {
			continue
		}
		if _, exists := shopSet[shopID]; !exists {
			shopSet[shopID] = struct{}{}
			allShopIDs = append(allShopIDs, shopID)
		}
	}
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		if IsShowcaseShopType(shop.Info.GetType()) {
			continue
		}
		if _, ok := shopSet[shop.ShopId]; !ok {
			shopSet[shop.ShopId] = struct{}{}
			allShopIDs = append(allShopIDs, shop.ShopId)
		}
	}
	visibleShops := make([]string, 0, len(shopIDs))
	for _, shop := range shops {
		if shop.Info == nil {
			continue
		}
		if IsShowcaseShopType(shop.Info.GetType()) {
			continue
		}
		visible, err := evaluator.Satisfied(shop.Info.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if !visible {
			continue
		}
		hasContent, err := ShopCatalogHasVisibleContent(evaluator, shop)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if !hasContent {
			continue
		}
		if _, ok := shopSet[shop.ShopId]; ok {
			visibleShops = append(visibleShops, shop.ShopId)
		}
	}
	showcaseCatalog, err := BuildShowcaseCommerceCatalog(ctx, suid)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	allShowcaseShopIDs := showcaseCatalog.ShopIDs()
	visibleShowcaseShops := make([]string, 0, len(allShowcaseShopIDs))
	for _, shop := range showcaseCatalog.Shops() {
		visible, err := evaluator.Satisfied(shop.Info.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if visible {
			visibleShowcaseShops = append(visibleShowcaseShops, shop.ShopId)
		}
	}

	booths, err := opsolis.FindAllExchangeBooths(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	allBoothIDs := append([]string(nil), boothIDs...)
	boothSet := StringSet(allBoothIDs)
	for _, booth := range booths {
		if _, ok := boothSet[booth.GetId()]; !ok {
			boothSet[booth.GetId()] = struct{}{}
			allBoothIDs = append(allBoothIDs, booth.GetId())
		}
	}
	visibleBooths := make([]string, 0, len(boothIDs))
	for _, booth := range booths {
		if _, ok := boothSet[booth.GetId()]; !ok || booth.GetEndTime() > 0 && booth.GetEndTime() <= now {
			continue
		}
		visible, err := evaluator.Satisfied(booth.GetViewConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		unlocked, err := evaluator.Satisfied(booth.GetUnlockConditionId())
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if visible && unlocked {
			visibleBooths = append(visibleBooths, booth.GetId())
		}
	}
	return visibleShops, visibleShowcaseShops, visibleBooths,
		allShopIDs, allShowcaseShopIDs, allBoothIDs, nil
}

func IsShowcaseShopType(shopType penum.ShopType) bool {
	switch shopType {
	case penum.ShopType_ShopType_ShowcaseToyShop,
		penum.ShopType_ShopType_DefaultShowcaseToyShop,
		penum.ShopType_ShopType_OnlyShowcaseShop:
		return true
	default:
		return false
	}
}

func StringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
