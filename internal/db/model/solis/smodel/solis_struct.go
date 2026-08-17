package smodel

import "github.com/vertesan/solis-server/internal/proto/papi"

type OctoManifest struct {
	Revision  int32  `bson:"revision"`
	UrlFormat string `bson:"urlFormat"`
}

type Qseed struct {
	Device string `bson:"device"`
	Qseed  string `bson:"qseed"`
}

type ServerProfile struct {
	AppVersion         string `bson:"appVersion"`
	Platform           string `bson:"platform"`
	MasterVersion      string `bson:"masterVersion"`
	FirebaseProductKey string `bson:"firebaseProductKey"`
	FirebaseProjectId  string `bson:"firebaseProjectId"`
	PackageName        string `bson:"packageName"`
	OctoApiKey         string `bson:"octoApiKey"`
	OctoManifestKey    string `bson:"octoManifestKey"`
}

type Migration struct {
	Migration int32 `bson:"migration"`
}

type NoticeCatalog struct {
	Notices            []*papi.NoticeInfo `bson:"notices"`
	MalfunctionNotices []*papi.NoticeInfo `bson:"malfunctionNotices"`
	PrNotices          []*papi.NoticeInfo `bson:"prNotices"`
}

// ShopCatalogEntry groups the global definition of a shop with the items
// exposed by the shop-specific APIs. User purchase/unlock state belongs in the
// user database and is deliberately not stored here.
type ShopCatalogEntry struct {
	ShopId                string                      `bson:"shopId"`
	Info                  *papi.ShopInfo              `bson:"info"`
	Items                 []*papi.ShopItem            `bson:"items"`
	LoginBonusPackageItem *papi.LoginBonusPackageItem `bson:"loginBonusPackageItem,omitempty"`
}
