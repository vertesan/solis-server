package ustruct

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
	OnetimeToken  string    `bson:"onetimeToken"`
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
