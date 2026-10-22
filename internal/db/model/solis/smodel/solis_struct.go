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
	MasterVersionOrig  string `bson:"masterVersionOrig"`
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
	Notices            []*papi.NoticeInfo `bson:"notices" json:"notices"`
	MalfunctionNotices []*papi.NoticeInfo `bson:"malfunctionNotices" json:"malfunctionNotices"`
	PrNotices          []*papi.NoticeInfo `bson:"prNotices" json:"prNotices"`
	ShowcaseShopNotice *papi.NoticeInfo   `bson:"showcaseShopNotice" json:"showcaseShopNotice"`
}

// ShopCatalogEntry groups the global definition of a shop with the items
// exposed by the shop-specific APIs. User purchase/unlock state belongs in the
// user database and is deliberately not stored here.
type ShopCatalogEntry struct {
	ShopId string           `bson:"shopId" json:"shopId"`
	Info   *papi.ShopInfo   `bson:"info" json:"info"`
	Items  []*papi.ShopItem `bson:"items" json:"items"`
	// Optional fields must remain in BSON: full catalog upserts use $set and
	// must clear a removed package or product mapping instead of retaining it.
	LoginBonusPackageItem *papi.LoginBonusPackageItem `bson:"loginBonusPackageItem" json:"loginBonusPackageItem,omitempty"`
	// OriginalProductIds keeps the StoreProduct link for IAP bundles converted
	// to free Shop.Purchase items. It is server-only and never enters the API payload.
	OriginalProductIds map[string]string `bson:"originalProductIds" json:"originalProductIds,omitempty"`
}
