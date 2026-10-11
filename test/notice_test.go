package test

import (
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"google.golang.org/protobuf/proto"
)

func TestNoticeFallbackScenario(t *testing.T) {
	setupTestUser(t)
	response := profileCall(t, papi.NewNoticeClient(testConfig.GrpcConn).Get, &papi.NoticeGetRequest{})
	assertDummyNoticeInfo(t, "Notice.Get", response.GetNotice())
}

func assertDummyNoticeInfo(t *testing.T, source string, notice *papi.NoticeInfo) {
	t.Helper()
	// Comparing decoded RPC messages also ensures the empty notice survives
	// protobuf serialization instead of disappearing as an absent field.
	expected := &papi.NoticeInfo{LinkType: penum.LinkType_LinkType_WebView}
	if notice == nil || !proto.Equal(notice, expected) {
		t.Fatalf("%s notice = %v, want an empty WebView notice", source, notice)
	}
}
