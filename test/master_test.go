package test

import (
	"fmt"
	"strings"
	"testing"

	// "github.com/vertesan/solis-server/internal/db"
	// "github.com/vertesan/solis-server/internal/db/store/opsolis"
	pb "github.com/vertesan/solis-server/internal/proto/papi"
)

func TestDownloadRawMasterDatabase(t *testing.T) {
	// if err := db.Connect(); err != nil {
	// 	t.Fatalf("failed connecting db: %v", err)
	// }
	// serverProfile, err := opsolis.GetServerProfile(context.Background())
	// if err != nil {
	// 	t.Fatalf("failed getting serverProfile: %v", err)
	// }
	// new client
	client := pb.NewMasterClient(testConfig.GrpcConn)
	// prepare context
	ctx, cancel, err := getTokenedGrpcCtx()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer cancel()
	// invoke RPC
	resp1, err := client.Get(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("could not send request: %v", err)
	}
	// if resp1.MasterTag.Version != serverProfile.MasterVersion {
	// 	t.Error("master db version mismatches")
	// }
	if len(resp1.MasterTag.MasterTagPacks) < 1 {
		t.Error("masterTagPacks has 0 length")
	}
	t.Logf("%v", resp1)

	url := resp1.MasterTag.MasterTagPacks[0].DownloadUrl
	t.Logf("original url: %s", url)
	url = strings.Replace(url, "https://climaster.game-idolypride.jp", testConfig.HttpUrl, 1)
	t.Logf("replaced url: %s", url)
	resp2, err := testConfig.HttpClient.Get(fmt.Sprintf("%s", url))
	if err != nil {
		t.Fatalf("failed downloading db file: %v", err)
	}
	if resp2.StatusCode() != 200 {
		t.Fatalf("failed downloading db file, status code: %d", resp2.StatusCode())
	}
	bodyLen := len(resp2.Body())
	if bodyLen == 0 {
		t.Fatalf("empty db file, length: %d", bodyLen)
	}
}

// func TestUnmarshalMasterBinary(t *testing.T) {
// 	resp, err := testConfig.HttpClient.Get(fmt.Sprintf("%s/getSetting", testConfig.HttpUrl))
// 	if err != nil {
// 		t.Fatalf("failed to getSetting: %v", err)
// 	}
// 	t.Logf("resp: %v", resp.String())
// }
