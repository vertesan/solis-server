package test

import (
	"testing"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func TestUser(t *testing.T) {
	setupTestUser(t)

	// test `/api.User/Get`
	{
		uClient := papi.NewUserClient(testConfig.GrpcConn)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatalf("%v", err)
		}
		defer cancel()
		resp, err := uClient.Get(ctx, &papi.Empty{})
		if err != nil {
			t.Fatalf("%v", err)
		}
		t.Logf("UserGetResponse:\n%v", resp)
	}

	// test `/api.Home/Login`
	{
		hClient := papi.NewHomeClient(testConfig.GrpcConn)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatalf("%v", err)
		}
		defer cancel()
		resp, err := hClient.Login(ctx, &papi.HomeLoginRequest{})
		if err != nil {
			t.Fatalf("%v", err)
		}
		t.Logf("HomeLoginResponse:\n%v", resp)
	}
}
