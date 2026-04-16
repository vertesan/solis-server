package mstruct

// master data version struct for `_version` database
type Version struct {
	Version string `bson:"version"`
}

// wrapped raw protobuf bytes for each master data document
type DataHolder struct {
	Data []byte `bson:"data"`
}

func (d *DataHolder) GetData() []byte {
	return d.Data
}

// ==============================================================
// AI generated MongoDB structs for `master` database collections

type Accessory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type AccessoryEnhancement struct {
	Rarity     int64
	DataHolder `bson:",inline"`
}

type ActivityAbility struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ActivityFanEvent struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ActivityPerformance struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ActivityPromotion struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ActivityRefresh struct {
	Id         string
	DataHolder `bson:",inline"`
}

type AdvLiveMovie struct {
	Id         string
	DataHolder `bson:",inline"`
}

type AppCapacityLimit struct {
	AppCapacityLimitKey int64
	DataHolder          `bson:",inline"`
}

type Area struct {
	Id         string
	DataHolder `bson:",inline"`
}

type AreaGroup struct {
	Id         string
	DataHolder `bson:",inline"`
}

type AssetDownload struct {
	AssetId    string
	DataHolder `bson:",inline"`
}

type BacksideAnimation struct {
	PanelType          int64
	PanelAttributeType int64
	PanelRank          int64
	DataHolder         `bson:",inline"`
}

type BacksidePanelGoalSetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type BacksideQuestSetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type BoxGacha struct {
	Id         string
	DataHolder `bson:",inline"`
}

type BoxGachaGroup struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Card struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CardDuplicateExp struct {
	InitialRarity int64
	DataHolder    `bson:",inline"`
}

type CardDuplicateReward struct {
	InitialRarity int64
	CurrentRarity int64
	DataHolder    `bson:",inline"`
}

type CardEvolution struct {
	EvolutionLevel int64
	CardId         string
	DataHolder     `bson:",inline"`
}

type CardEvolutionLevel struct {
	GroupId    string
	Level      int64
	DataHolder `bson:",inline"`
}

type CardEvolutionLevelRelease struct {
	GroupId    string
	Level      int64
	DataHolder `bson:",inline"`
}

type CardEvolutionMessage struct {
	CardId         string
	EvolutionLevel int64
	Number         int64
	DataHolder     `bson:",inline"`
}

type CardImage struct {
	CardId      string
	DisplayType int64
	DataHolder  `bson:",inline"`
}

type CardLevel struct {
	Level      int64
	DataHolder `bson:",inline"`
}

type CardLevelRelease struct {
	Id         string
	Level      int64
	DataHolder `bson:",inline"`
}

type CardParameter struct {
	Id         string
	Level      int64
	DataHolder `bson:",inline"`
}

type CardRank struct {
	Rank       int64
	DataHolder `bson:",inline"`
}

type CardRarity struct {
	Rarity     int64
	DataHolder `bson:",inline"`
}

type CardSupportRelease struct {
	Count      int64
	DataHolder `bson:",inline"`
}

type Character struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CharacterActivityLevel struct {
	Level      int64
	DataHolder `bson:",inline"`
}

type CharacterCostumeGroup struct {
	CostumeId  string
	GroupId    string
	DataHolder `bson:",inline"`
}

type CharacterGroup struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ComboAdvantage struct {
	Id         string
	ComboCount int64
	DataHolder `bson:",inline"`
}

type CompanyEnjoyCharacterLevel struct {
	LevelKey   int64
	DataHolder `bson:",inline"`
}

type CompanyEnjoyCharacterLevelSetting struct {
	CharacterId string
	LevelKey    int64
	DataHolder  `bson:",inline"`
}

type CompanyEnjoyHomeAction struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CompanyEnjoyLevel struct {
	LevelKey   int64
	DataHolder `bson:",inline"`
}

type CompanyEnjoyObtainPointItemSetting struct {
	CharacterId string
	ItemId      string
	DataHolder  `bson:",inline"`
}

type CompanyGrade struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CompanyPoint struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CompanySalarySetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CompanyTrustLevel struct {
	LevelKey   int64
	DataHolder `bson:",inline"`
}

type Condition struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ConditionDescription struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Consumption struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Costume struct {
	Id         string
	DataHolder `bson:",inline"`
}

type CostumeType struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Decoration struct {
	Id         string
	DataHolder `bson:",inline"`
}

type DeckEntrustCoefficient struct {
	Position   int64
	DataHolder `bson:",inline"`
}

type Division struct {
	Id         string
	DataHolder `bson:",inline"`
}

type DutyPoint struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Emblem struct {
	Id         string
	DataHolder `bson:",inline"`
}

type EventMission struct {
	Id         string
	DataHolder `bson:",inline"`
}

type EventStory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Exercise struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ExerciseAccessoryGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type ExerciseCardGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type ExerciseCostume struct {
	CharacterId string
	DataHolder  `bson:",inline"`
}

type ExerciseDeck struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ExerciseHint struct {
	ExerciseId string
	DataHolder `bson:",inline"`
}

type ExercisePhotoGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type ExerciseUser struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ExtraStory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ExtraStoryPart struct {
	Id         string
	DataHolder `bson:",inline"`
}

type FunctionLock struct {
	Type       int64
	DataHolder `bson:",inline"`
}

type Gacha struct {
	Id         string
	DataHolder `bson:",inline"`
}

type GachaButton struct {
	Id         string
	DataHolder `bson:",inline"`
}

type GachaExchange struct {
	Id         string
	DataHolder `bson:",inline"`
}

type GachaStamp struct {
	GroupId     string
	SheetNumber int64
	StampNumber int64
	DataHolder  `bson:",inline"`
}

type GachaStampSheet struct {
	GroupId     string
	SheetNumber int64
	DataHolder  `bson:",inline"`
}

type GachaStampSheetGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type Guild struct {
	Id         string
	DataHolder `bson:",inline"`
}

type GuildTopMovie struct {
	AssetId    string
	DataHolder `bson:",inline"`
}

type Hair struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HelpCategory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HierarchyDetailGrade struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HierarchyDivisionReward struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HierarchyGrade struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HierarchyReward struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HomeAction struct {
	HomeActionId string
	DataHolder   `bson:",inline"`
}

type HomeBackground struct {
	HomeBackgroundId string
	DataHolder       `bson:",inline"`
}

type HomeBackgroundCategory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HomeDrama struct {
	HomeDramaId string
	DataHolder  `bson:",inline"`
}

type HomeDramaFree struct {
	HomeDramaPositionGroupId string
	HomeDramaPositionId      string
	Number                   int64
	DataHolder               `bson:",inline"`
}

type HomeDramaPosition struct {
	HomeDramaPositionId string
	DataHolder          `bson:",inline"`
}

type HomeDramaPositionGroup struct {
	HomeDramaPositionId      string
	HomeDramaPositionGroupId string
	DataHolder               `bson:",inline"`
}

type HomeMainCharacterStay struct {
	Id         string
	DataHolder `bson:",inline"`
}

type HomeSubCharacterStayGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type HomeTalk struct {
	HomeTalkId string
	DataHolder `bson:",inline"`
}

type HomeTalkCallPattern struct {
	CharacterId string
	PatternId   string
	DataHolder  `bson:",inline"`
}

type InviteHostReward struct {
	Id         string
	DataHolder `bson:",inline"`
}

type InviteSetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Item struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LiveAbility struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LiveBonus struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LiveBonusGroup struct {
	GroupId    string
	DataHolder `bson:",inline"`
}

type LiveResultAnimation struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LiveStartAnimation struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LiveStartVoiceAssetGroup struct {
	Id          string
	CharacterId string
	DataHolder  `bson:",inline"`
}

type LiveTip struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Loading struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LoginBonus struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LoginBonusText struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LoveColor struct {
	LoveId     string
	DataHolder `bson:",inline"`
}

type LoveHomeAction struct {
	Id         string
	DataHolder `bson:",inline"`
}

type LoveLevel struct {
	LoveId     string
	Level      int64
	DataHolder `bson:",inline"`
}

type LoveStoryEpisode struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ManagerLevel struct {
	Level      int64
	DataHolder `bson:",inline"`
}

type MarathonBoxGachaSetting struct {
	MarathonId string
	BoxGachaId string
	DataHolder `bson:",inline"`
}

type MarathonLiveBonus struct {
	Id         string
	DataHolder `bson:",inline"`
}

type MarathonLiveBonusGroup struct {
	GroupId     string
	LiveBonusId string
	DataHolder  `bson:",inline"`
}

type MarathonQuest struct {
	Id         string
	DataHolder `bson:",inline"`
}

type MarathonQuestDifficulty struct {
	DifficultyNumber int64
	MarathonQuestId  string
	DataHolder       `bson:",inline"`
}

type Message struct {
	Id         string
	DataHolder `bson:",inline"`
}

type MessageGroup struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Mission struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Music struct {
	Id         string
	DataHolder `bson:",inline"`
}

type MusicChartPattern struct {
	Id         string
	Number     int64
	DataHolder `bson:",inline"`
}

type MusicMasteryLevel struct {
	Level      int64
	DataHolder `bson:",inline"`
}

type MusicMasteryReward struct {
	MusicId     string
	CharacterId string
	DataHolder  `bson:",inline"`
}

type MusicMasteryRewardSet struct {
	Id         string
	Level      int64
	DataHolder `bson:",inline"`
}

type OrnamentHair struct {
	BaseHairId string
	DataHolder `bson:",inline"`
}

type PhotoAbility struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoAbilitySet struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoAbilityTarget struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoActivity struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoAllInOne struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoContestActivity struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoContestQuestMusic struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoContestQuestStage struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoContestSection struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoContestSectionReward struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoExpression struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoExpressionStagePosition struct {
	StageId    string
	Number     int64
	DataHolder `bson:",inline"`
}

type PhotoFacial struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoFacialMotion struct {
	PhotoFacialId string
	CharacterId   string
	DataHolder    `bson:",inline"`
}

type PhotoPose struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoPoseMotion struct {
	PhotoPoseId string
	CharacterId string
	DataHolder  `bson:",inline"`
}

type PhotoQuestMusic struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoQuestStage struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoRarityLevel struct {
	Rarity     int64
	Level      int64
	DataHolder `bson:",inline"`
}

type PhotoRecipe struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoRetouchEnhance struct {
	GroupId    string
	PhotoLevel int64
	DataHolder `bson:",inline"`
}

type PhotoShootingMotionSet struct {
	Id         string
	DataHolder `bson:",inline"`
}

type PhotoShootingReplaceCharacter struct {
	TargetCharacterId string
	DataHolder        `bson:",inline"`
}

type PhotoShootingSimplePersonalityMotion struct {
	Id              string
	PersonalityType int64
	DataHolder      `bson:",inline"`
}

type Quest struct {
	Id         string
	DataHolder `bson:",inline"`
}

type QuestAudienceAdvantage struct {
	Id             string
	AudienceAmount int64
	DataHolder     `bson:",inline"`
}

type QuestCharacterAdvantage struct {
	Id         string
	DataHolder `bson:",inline"`
}

type QuestContestRankingSeason struct {
	Id         string
	DataHolder `bson:",inline"`
}

type QuestPressure struct {
	Id         string
	DataHolder `bson:",inline"`
}

type QuestStaminaConsumption struct {
	RemainStaminaRatioPercent int64
	DataHolder                `bson:",inline"`
}

type RaceLiveBonus struct {
	Id         string
	Level      int64
	DataHolder `bson:",inline"`
}

type RaceLiveBonusGroup struct {
	GroupId     string
	LiveBonusId string
	DataHolder  `bson:",inline"`
}

type RacePanel struct {
	RaceAreaId string
	Number     int64
	DataHolder `bson:",inline"`
}

type RacePanelReward struct {
	Id         string
	RaceId     string
	DataHolder `bson:",inline"`
}

type RaceQuest struct {
	Id         string
	DataHolder `bson:",inline"`
}

type RaceQuestDifficulty struct {
	RaceQuestId      string
	DifficultyNumber int64
	DataHolder       `bson:",inline"`
}

type RaceRankingReward struct {
	RankGradeId string
	Type        int64
	RankFrom    int64
	DataHolder  `bson:",inline"`
}

type Reward struct {
	Id         string
	DataHolder `bson:",inline"`
}

type RewardSort struct {
	RewardSortTargetType   int64
	RewardSortResourceType int64
	DataHolder             `bson:",inline"`
}

type Salary struct {
	Type       int64
	Threshold  int64
	DataHolder `bson:",inline"`
}

type Setting struct {
	Id         int64
	DataHolder `bson:",inline"`
}

type ShowcaseFrame struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseHashtag struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseMusic struct {
	MusicId    string
	DataHolder `bson:",inline"`
}

type ShowcaseMusicFilter struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseSetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseToy struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseToyCategory struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ShowcaseToyShopFilter struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Skill struct {
	Id         string
	DataHolder `bson:",inline"`
}

type SkillEfficacy struct {
	Id         string
	DataHolder `bson:",inline"`
}

type SkillTarget struct {
	Id         string
	DataHolder `bson:",inline"`
}

type SkillTrigger struct {
	Id         string
	DataHolder `bson:",inline"`
}

type StaffLevel struct {
	ParameterType int64
	Level         int64
	DataHolder    `bson:",inline"`
}

type StaffLevelLimitBreakRank struct {
	ParameterType int64
	Rank          int64
	DataHolder    `bson:",inline"`
}

type StaffTraining struct {
	ParameterType int64
	DataHolder    `bson:",inline"`
}

type StaffTrainingReset struct {
	StaffTrainingResetGroupId string
	ResetCount                int64
	DataHolder                `bson:",inline"`
}

type StaffTrainingResetGroup struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Stage struct {
	Id         string
	DataHolder `bson:",inline"`
}

type StatusEffectName struct {
	StatusEffectType int64
	DataHolder       `bson:",inline"`
}

type StoreGppProduct struct {
	ProductId  string
	DataHolder `bson:",inline"`
}

type StoreProduct struct {
	ProductId  string
	DataHolder `bson:",inline"`
}

type Story struct {
	Id         string
	DataHolder `bson:",inline"`
}

type StoryEpisodeConsumption struct {
	Id           string
	ReleaseCount int64
	DataHolder   `bson:",inline"`
}

type StoryPart struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Telephone struct {
	Id         string
	DataHolder `bson:",inline"`
}

type TitleBackground struct {
	Id         string
	DataHolder `bson:",inline"`
}

type TotalStaffEnhanceAdvantage struct {
	EnhanceCount int64
	DataHolder   `bson:",inline"`
}

type TourQuestSetting struct {
	Id         string
	DataHolder `bson:",inline"`
}

type Tutorial struct {
	Type       int64
	DataHolder `bson:",inline"`
}

type Wording struct {
	Key        string
	DataHolder `bson:",inline"`
}
