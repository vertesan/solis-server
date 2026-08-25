package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestInterceptorDailyRelogin(t *testing.T) {
	setupTestUser(t)

	jst := time.FixedZone("JST", 9*60*60)
	previousDay := time.Now().In(jst).AddDate(0, 0, -1)
	if err := opuser.UpdateUserAuthLastLoginTime(context.Background(), testConfig.TestUserSuid, previousDay.UnixMilli()); err != nil {
		t.Fatalf("failed to backdate last login: %v", err)
	}

	userClient := papi.NewUserClient(testConfig.GrpcConn)
	staleCtx, staleCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	var responseHeaders metadata.MD
	staleResponse, err := userClient.Get(staleCtx, &papi.Empty{}, grpc.Header(&responseHeaders))
	staleCancel()
	if err != nil {
		t.Fatalf("daily relogin response returned a gRPC error: %v", err)
	}
	if staleResponse == nil || proto.Size(staleResponse) != 0 {
		t.Fatalf("daily relogin response was not empty: %v", staleResponse)
	}
	assertSingleMetadataValue(t, responseHeaders, "x-error-action", "title")
	assertSingleMetadataValue(t, responseHeaders, "x-error-message-bin", "日付が変わりました")
	assertSingleMetadataValue(t, responseHeaders, "x-error-code", "1003")

	staleAuth, err := opuser.GetUserAuthByServerId(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if staleAuth.LastLoginAt != previousDay.UnixMilli() {
		t.Fatalf("daily relogin response advanced lastLoginAt: got %d, want %d", staleAuth.LastLoginAt, previousDay.UnixMilli())
	}

	now := time.Now()
	firebaseToken, err := jwto.CreateToken(jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"exp":     now.Add(time.Hour).Unix(),
		"iat":     now.Unix(),
		"puid":    staleAuth.PublicUserId,
		"user_id": staleAuth.FirebaseId,
	}))
	if err != nil {
		t.Fatalf("failed to create Firebase token: %v", err)
	}
	authCtx, authCancel := getBasicGrpcCtx()
	authResponse, err := papi.NewAuthClient(testConfig.GrpcConn).Login(authCtx, &papi.AuthLoginRequest{
		FirebaseIDToken: firebaseToken,
	})
	authCancel()
	if err != nil {
		t.Fatalf("Auth.Login failed after the date-change response: %v", err)
	}
	if authResponse.GetGameAuthToken() == "" {
		t.Fatal("Auth.Login returned an empty game auth token")
	}

	currentAuth, err := opuser.GetUserAuthByServerId(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSTDay(currentAuth.LastLoginAt, time.Now().UnixMilli(), jst) {
		t.Fatalf("Auth.Login did not advance lastLoginAt to the current JST day: %d", currentAuth.LastLoginAt)
	}

	currentCtx, currentCancel := fullGrpcCtxWithToken(t, authResponse.GetGameAuthToken())
	currentResponse, err := userClient.Get(currentCtx, &papi.Empty{})
	currentCancel()
	if err != nil {
		t.Fatalf("authenticated request still failed after Auth.Login: %v", err)
	}
	if currentResponse.GetUser() == nil {
		t.Fatalf("authenticated request remained empty after Auth.Login: %v", currentResponse)
	}
}

func TestInterceptorAppRequestIdempotency(t *testing.T) {
	setupTestUser(t)

	const requestId = "639230407163253360"
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)
	failedCtx, failedCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	failedCtx = metadata.AppendToOutgoingContext(failedCtx, "x-app-request-id", requestId)
	_, err = homeClient.SetCharacterPosition(failedCtx, &papi.HomeSetCharacterPositionRequest{})
	failedCancel()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid action returned %v, want %v", err, codes.InvalidArgument)
	}

	userClient := papi.NewUserClient(testConfig.GrpcConn)
	firstCtx, firstCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	firstCtx = metadata.AppendToOutgoingContext(firstCtx, "x-app-request-id", requestId)
	firstResponse, err := userClient.Get(firstCtx, &papi.Empty{})
	firstCancel()
	if err != nil {
		t.Fatalf("request ID was consumed by a rolled-back action: %v", err)
	}
	if firstResponse.GetUser() == nil {
		t.Fatalf("first request returned no user: %v", firstResponse)
	}

	duplicateCtx, duplicateCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	duplicateCtx = metadata.AppendToOutgoingContext(duplicateCtx, "x-app-request-id", requestId)
	_, err = userClient.Get(duplicateCtx, &papi.Empty{})
	duplicateCancel()
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate request returned %v, want %v", err, codes.AlreadyExists)
	}

	malformedCtx, malformedCancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	malformedCtx = metadata.AppendToOutgoingContext(malformedCtx, "x-app-request-id", "not-a-uint64")
	_, err = userClient.Get(malformedCtx, &papi.Empty{})
	malformedCancel()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("malformed request ID returned %v, want %v", err, codes.InvalidArgument)
	}

	const concurrentRequestId = "639230409085488700"
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			ctx, cancel, err := getFullGrpcCtx()
			if err != nil {
				results <- err
				return
			}
			defer cancel()
			<-start
			ctx = metadata.AppendToOutgoingContext(ctx, "x-app-request-id", concurrentRequestId)
			_, err = userClient.Get(ctx, &papi.Empty{})
			results <- err
		}()
	}
	close(start)
	successes := 0
	duplicates := 0
	for range 2 {
		err := <-results
		switch status.Code(err) {
		case codes.OK:
			successes++
		case codes.AlreadyExists:
			duplicates++
		default:
			t.Fatalf("concurrent duplicate request returned unexpected error: %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent duplicate results: successes=%d duplicates=%d, want 1 each", successes, duplicates)
	}
}

func fullGrpcCtxWithToken(t *testing.T, token string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := getBasicGrpcCtx()
	profile, err := opsolis.FindServerProfile(context.Background())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return metadata.AppendToOutgoingContext(
		ctx,
		"x-auth-token", token,
		"x-master-version", profile.MasterVersion,
	), cancel
}

func assertSingleMetadataValue(t *testing.T, values metadata.MD, key, expected string) {
	t.Helper()
	actual := values.Get(key)
	if len(actual) != 1 || actual[0] != expected {
		t.Fatalf("response metadata %q = %q, want [%q]", key, actual, expected)
	}
}

func sameJSTDay(leftMillis, rightMillis int64, jst *time.Location) bool {
	left := time.UnixMilli(leftMillis).In(jst)
	right := time.UnixMilli(rightMillis).In(jst)
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}
