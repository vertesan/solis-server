package test

import (
	"testing"

	"context"
	"time"

	pb "vertesan/solis-server/proto/papi"
)

func TestSystemCheck(t *testing.T) {
	client := pb.NewSystemClient(testConfig.GrpcConn)

	// SystemCheck
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	resp, err := client.Check(ctx, &pb.SystemCheckRequest{FirebaseIDToken: "aaaac"})
	if err != nil {
		t.Fatalf("could not send request: %v", err)
	}
	var wantKeepAlive int32 = 60000
	var wantKeepAliveTimeout int32 = 5000
	if wantKeepAlive != resp.GetKeepAliveTimeMilliseconds() || wantKeepAliveTimeout != resp.GetKeepAliveTimeoutMilliseconds() {
		t.Fatalf(`SystemCheckResp = %q, want match for KeepAliveTimeMilliseconds %#q, KeepAliveTimeoutMilliseconds %#q`, resp, wantKeepAlive, wantKeepAliveTimeout)
	}
}
