package umodel

import (
	"time"

	"google.golang.org/protobuf/proto"
)

type UserStruct[T proto.Message] struct {
	SUID string `bson:"SUID"`
	Data T      `bson:",inline"`
}

type UserAuth struct {
	PublicUserId  string    `bson:"publicUserId"`
	ServerUserId  string    `bson:"serverUserId"`
	OnetimeToken  string    `bson:"oneTimeToken"`
	Password      string    `bson:"password"`
	FirebaseId    string    `bson:"firebaseId"`
	RefreshTokens []string  `bson:"refreshTokens"`
	ValidSince    int64     `bson:"validSince"`
	CreatedAt     int64     `bson:"createdAt"`
	LastLoginAt   int64     `bson:"lastLoginAt"`
	LastRefreshAt time.Time `bson:"lastRefreshAt"`
}

type UserShopId struct {
	SUID    string   `bson:"SUID"`
	ShopIds []string `bson:"shopIds"`
}

type UserExchangeBooth struct {
	SUID     string   `bson:"SUID"`
	BoothIds []string `bson:"boothIds"`
}

type UserExchangePurchase struct {
	SUID           string           `bson:"SUID"`
	PurchaseCounts map[string]int32 `bson:"purchaseCounts"`
	ResetTimes     map[string]int64 `bson:"resetTimes"`
}

type UserShopPurchase struct {
	SUID                     string           `bson:"SUID"`
	PurchaseCounts           map[string]int32 `bson:"purchaseCounts"`
	ResetTimes               map[string]int64 `bson:"resetTimes"`
	PurchasedItemIds         []string         `bson:"purchasedItemIds"`
	LoginBonusProgress       map[string]int32 `bson:"loginBonusProgress"`
	LoginBonusLastGrantTimes map[string]int64 `bson:"loginBonusLastGrantTimes"`
}

type UserGodState struct {
	SUID                string `bson:"SUID"`
	UnlockCurrentEvents bool   `bson:"unlockCurrentEvents"`
}

type UserShowcaseToyShopId struct {
	SUID    string   `bson:"SUID"`
	ShopIds []string `bson:"shopIds"`
}

type UserUnlockedShowcaseNumber struct {
	SUID    string  `bson:"SUID"`
	Numbers []int32 `bson:"numbers"`
}

type UserCurrentQuestArea struct {
	SUID                string `bson:"SUID"`
	CurrentMainAreaId   string `bson:"currentMainAreaId"`
	CurrentTowerQuestId string `bson:"currentTowerQuestId"`
}

type UserDiaryState struct {
	SUID    string   `bson:"SUID"`
	ReadIds []string `bson:"readIds"`
	AllRead bool     `bson:"allRead"`
}

type UserMessageProgress struct {
	SUID                  string `bson:"SUID"`
	MessageId             string `bson:"messageId"`
	LastMessageDetailId   string `bson:"lastMessageDetailId"`
	LastMessageDetailTime int64  `bson:"lastMessageDetailTime"`
}

// UserActivityLessonDailyState records the JST game day for which
// ActivityLessonProgress.DailyPromoteCount applies.  The original progress
// protobuf contains the count but not its day, so keeping the day separately
// prevents repeated resets before the next lesson reward is received.
type UserActivityLessonDailyState struct {
	SUID string `bson:"SUID"`
	Day  string `bson:"day"`
}

// UserFanEventRanking stores a user's best score for one fan-event type.  The
// private server deliberately exposes a self-only ranking, but the score still
// needs to survive completed runs.
type UserFanEventRanking struct {
	SUID               string   `bson:"SUID"`
	ActivityFanEventId string   `bson:"activityFanEventId"`
	BestScore          int64    `bson:"bestScore"`
	CharacterIds       []string `bson:"characterIds"`
}

// UserPromotionRunState stores accounting state that is not represented in
// PromotionProgress. It lives alongside the inline protobuf fields in the
// userPromotionRun document so repeated reward receipts cannot discard a
// partial multi-step fan reward.
type UserPromotionRunState struct {
	SUID                     string `bson:"SUID"`
	AccountingInitialized    bool   `bson:"accountingInitialized"`
	LastSettledCompletedStep int64  `bson:"lastSettledCompletedStep"`
	FanRewardStepRemainder   int64  `bson:"fanRewardStepRemainder"`
}
