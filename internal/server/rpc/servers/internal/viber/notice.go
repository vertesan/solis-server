package viber

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

// NewDummyNoticeInfo keeps the client's notice button usable when a page has
// no announcement. Return a fresh message so response builders never share it.
func NewDummyNoticeInfo() *papi.NoticeInfo {
	return &papi.NoticeInfo{
		Id:                  "",
		Title:               "",
		BannerAssetId:       "",
		LinkType:            penum.LinkType_LinkType_WebView,
		LinkDetail:          "",
		DisplayNotification: false,
	}
}
