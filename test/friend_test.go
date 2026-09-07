package test

import (
	"context"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"

	"google.golang.org/protobuf/proto"
)

func TestFriendIsolationScenario(t *testing.T) {
	setupTestUser(t)

	client := papi.NewFriendClient(testConfig.GrpcConn)
	before, err := opuser.FindUser(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}

	assertEmptyLists := func() {
		t.Helper()
		list := friendCall(t, "List", func(ctx context.Context) (*papi.FriendListResponse, error) {
			return client.List(ctx, &papi.Empty{})
		})
		applying := friendCall(t, "ListApplying", func(ctx context.Context) (*papi.FriendListApplyingResponse, error) {
			return client.ListApplying(ctx, &papi.Empty{})
		})
		applied := friendCall(t, "ListApplied", func(ctx context.Context) (*papi.FriendListAppliedResponse, error) {
			return client.ListApplied(ctx, &papi.Empty{})
		})
		search := friendCall(t, "Search", func(ctx context.Context) (*papi.FriendSearchResponse, error) {
			return client.Search(ctx, &papi.Empty{})
		})
		for name, profiles := range map[string][]*papi.ProfileInfo{
			"List": list.GetProfiles(), "ListApplying": applying.GetProfiles(),
			"ListApplied": applied.GetProfiles(), "Search": search.GetProfiles(),
		} {
			if len(profiles) != 0 {
				t.Fatalf("Friend.%s exposed %d profiles in the isolated server", name, len(profiles))
			}
		}
	}
	assertEmptyLists()

	// Exercise stale/crafted client requests without creating any other account.
	for _, userID := range []string{"", testConfig.TestUserPublicId, "TPUEAH4L"} {
		apply := friendCall(t, "Apply", func(ctx context.Context) (*papi.FriendApplyResponse, error) {
			return client.Apply(ctx, &papi.FriendApplyRequest{UserId: userID})
		})
		if apply.GetApplied() || !apply.GetIsAccountDeleted() || apply.GetIsAlreadyFriend() ||
			apply.GetIsAlreadyOffering() || apply.GetIsAlreadyOffered() {
			t.Fatalf("Friend.Apply reported an available target or friendship: %v", apply)
		}
		approve := friendCall(t, "Approve", func(ctx context.Context) (*papi.FriendApproveResponse, error) {
			return client.Approve(ctx, &papi.FriendApproveRequest{UserId: userID})
		})
		if approve.GetApproved() || !approve.GetIsOfferDeleted() {
			t.Fatalf("Friend.Approve reported a phantom friendship: %v", approve)
		}
		reject := friendCall(t, "Reject", func(ctx context.Context) (*papi.FriendRejectResponse, error) {
			return client.Reject(ctx, &papi.FriendRejectRequest{UserId: userID})
		})
		if !reject.GetIsOfferDeleted() {
			t.Fatalf("Friend.Reject reported an existing offer: %v", reject)
		}
		friendCall(t, "Cancel", func(ctx context.Context) (*papi.FriendCancelResponse, error) {
			return client.Cancel(ctx, &papi.FriendCancelRequest{UserId: userID})
		})
		friendCall(t, "Delete", func(ctx context.Context) (*papi.FriendDeleteResponse, error) {
			return client.Delete(ctx, &papi.FriendDeleteRequest{UserIds: []string{userID}})
		})
	}
	friendCall(t, "CancelAll", func(ctx context.Context) (*papi.FriendCancelAllResponse, error) {
		return client.CancelAll(ctx, &papi.Empty{})
	})
	assertEmptyLists()

	after, err := opuser.FindUser(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, after) {
		t.Fatal("no-op Friend RPCs changed the persisted user")
	}
}

func friendCall[T interface{ GetCommonResponse() *papi.CommonResponse }](t *testing.T, method string, call func(context.Context) (T, error)) T {
	t.Helper()
	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	response, err := call(ctx)
	if err != nil {
		t.Fatalf("Friend.%s failed: %v", method, err)
	}
	common := response.GetCommonResponse()
	if common == nil || !proto.Equal(common.GetUpdatedData(), &papi.UpdatedData{}) ||
		!proto.Equal(common.GetDeletedData(), &papi.DeletedData{}) {
		t.Fatalf("Friend.%s returned missing common data or unexpected state changes: %v", method, common)
	}
	return response
}
