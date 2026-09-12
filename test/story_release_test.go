package test

import (
	"context"
	"errors"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
)

func TestStoryReleaseEpisodeScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewStoryClient(testConfig.GrpcConn)
	// Coordinates are from feature-0912. Cost and story IDs come from master.
	part, err := opmaster.FindStoryPart(ctx, "st-part-group-05")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for _, chapter := range part.GetChapters() {
		if chapter.GetChapter() == 5 && chapter.GetRoute() == 1 {
			for _, episode := range chapter.GetEpisodes() {
				if episode.GetEpisode() == 1 || episode.GetEpisode() == 2 {
					ids = append(ids, episode.GetStoryId())
				}
			}
		}
	}
	if len(ids) != 2 {
		t.Fatal("captured story episodes are not in master")
	}
	for _, id := range ids {
		if err := opuser.DeleteUserStory(ctx, testConfig.TestUserSuid, id); err != nil {
			t.Fatal(err)
		}
	}
	cost, err := opmaster.FindStoryEpisodeConsumption(ctx, part.GetStoryEpisodeConsumptionId(), 1)
	if err != nil {
		t.Fatal(err)
	}
	request := &papi.StoryReleaseEpisodeRequest{StoryPartId: part.GetId(), Chapter: 5, Route: 1, Episode: 1}
	if err := opuser.UpsertUserPoint(ctx, testConfig.TestUserSuid, &ptransaction.UserPoint{PointType: part.GetPointType(), Amount: int64(cost.GetRequiredStoryPoint()) - 1}); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.ResourceExhausted, client.ReleaseEpisode, request)
	if err := opuser.UpsertUserPoint(ctx, testConfig.TestUserSuid, &ptransaction.UserPoint{PointType: part.GetPointType(), Amount: 1000}); err != nil {
		t.Fatal(err)
	}
	first := profileCall(t, client.ReleaseEpisode, request)
	if len(first.GetCommonResponse().GetUpdatedData().GetStories()) != 1 || first.GetCommonResponse().GetUpdatedData().GetStories()[0].GetStoryId() != ids[0] ||
		first.GetCommonResponse().GetUpdatedData().GetStories()[0].GetStatusType() != penum.StoryStatusType_StoryStatusType_Unread {
		t.Fatalf("release did not unlock the captured story as unread: %v", first)
	}
	point, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, part.GetPointType())
	if err != nil || point.GetAmount() != 1000-int64(cost.GetRequiredStoryPoint()) {
		t.Fatalf("release charged an incorrect master cost: %v", err)
	}
	profileReject(t, codes.FailedPrecondition, client.ReleaseEpisode, request)
	request.Episode = 2
	profileReject(t, codes.FailedPrecondition, client.ReleaseEpisode, request)
	profileCall(t, client.Read, &papi.StoryReadRequest{StoryId: ids[0]})
	second := profileCall(t, client.ReleaseEpisode, request)
	if len(second.GetCommonResponse().GetUpdatedData().GetStories()) != 1 || second.GetCommonResponse().GetUpdatedData().GetStories()[0].GetStoryId() != ids[1] {
		t.Fatalf("reading the first episode did not permit unlocking the next: %v", second)
	}
	get := profileCall(t, papi.NewUserClient(testConfig.GrpcConn).Get, &papi.Empty{})
	for i, id := range ids {
		found := false
		for _, story := range get.GetStories() {
			if story.GetStoryId() == id {
				found = true
				want := penum.StoryStatusType_StoryStatusType_Unread
				if i == 0 {
					want = penum.StoryStatusType_StoryStatusType_Read
				}
				if story.GetStatusType() != want {
					t.Fatalf("User.Get story state = %s, want %s", story.GetStatusType(), want)
				}
			}
		}
		if !found {
			t.Fatalf("User.Get omitted unlocked story %s", id)
		}
	}
}

func TestStoryReleaseEscalatingCostScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	client := papi.NewStoryClient(testConfig.GrpcConn)
	type paidEpisode struct {
		request *papi.StoryReleaseEpisodeRequest
		storyID string
	}
	var part *pmaster.StoryPart
	var episodes []paidEpisode
	var previousCost, increasedCost int64
	boundary := -1
	// Find the first real increasing-price boundary instead of assuming the
	// captured five-point group catalog is representative of all story parts.
	for _, partID := range []string{"st-part-group-01", "st-part-group-02"} {
		candidate, err := opmaster.FindStoryPart(ctx, partID)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if candidate.GetStoryEpisodeConsumptionId() == "" {
			continue
		}
		part, episodes = candidate, nil
		seen := make(map[string]bool)
		for _, chapter := range part.GetChapters() {
			for _, episode := range chapter.GetEpisodes() {
				if episode.GetIsReleased() || episode.GetStoryId() == "" || seen[episode.GetStoryId()] {
					continue
				}
				seen[episode.GetStoryId()] = true
				episodes = append(episodes, paidEpisode{
					request: &papi.StoryReleaseEpisodeRequest{StoryPartId: part.GetId(), Chapter: chapter.GetChapter(), Route: chapter.GetRoute(), Episode: episode.GetEpisode()},
					storyID: episode.GetStoryId(),
				})
			}
		}
		previousCost = 0
		for i := range episodes {
			cost, err := opmaster.FindStoryEpisodeConsumption(ctx, part.GetStoryEpisodeConsumptionId(), int32(i+1))
			if err != nil {
				t.Fatal(err)
			}
			current := int64(cost.GetRequiredStoryPoint())
			if previousCost > 0 && current > previousCost {
				boundary, increasedCost = i, current
				break
			}
			previousCost = current
		}
		if boundary >= 0 {
			break
		}
	}
	if boundary < 0 {
		t.Skip("current group-01 and group-02 master catalogs contain no increasing story-point cost boundary")
	}
	t.Logf("testing %s release counts %d -> %d, master costs %d -> %d", part.GetId(), boundary, boundary+1, previousCost, increasedCost)
	// Set the configured user's progression just before this boundary. Earlier
	// paid stories are read so their real next-episode conditions are satisfied.
	for i, episode := range episodes {
		if i < boundary-1 {
			if err := opuser.UpsertUserStory(ctx, testConfig.TestUserSuid, &ptransaction.UserStory{
				StoryId: episode.storyID, StatusType: penum.StoryStatusType_StoryStatusType_Read,
			}); err != nil {
				t.Fatal(err)
			}
		} else if err := opuser.DeleteUserStory(ctx, testConfig.TestUserSuid, episode.storyID); err != nil {
			t.Fatal(err)
		}
	}
	setPoints := func(amount int64) {
		t.Helper()
		if err := opuser.UpsertUserPoint(ctx, testConfig.TestUserSuid, &ptransaction.UserPoint{PointType: part.GetPointType(), Amount: amount}); err != nil {
			t.Fatal(err)
		}
	}
	assertPoints := func(expected int64) {
		t.Helper()
		point, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, part.GetPointType())
		if err != nil || point.GetAmount() != expected {
			t.Fatalf("story point balance = %d, want %d: %v", point.GetAmount(), expected, err)
		}
	}
	setPoints(previousCost)
	profileCall(t, client.ReleaseEpisode, episodes[boundary-1].request)
	assertPoints(0)
	profileCall(t, client.Read, &papi.StoryReadRequest{StoryId: episodes[boundary-1].storyID})
	setPoints(previousCost)
	profileReject(t, codes.ResourceExhausted, client.ReleaseEpisode, episodes[boundary].request)
	assertPoints(previousCost)
	if _, err := opuser.FindUserStory(ctx, testConfig.TestUserSuid, episodes[boundary].storyID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("failed increased-price unlock created a story: %v", err)
	}
	setPoints(increasedCost)
	released := profileCall(t, client.ReleaseEpisode, episodes[boundary].request)
	assertPoints(0)
	stories := released.GetCommonResponse().GetUpdatedData().GetStories()
	if len(stories) != 1 || stories[0].GetStoryId() != episodes[boundary].storyID || stories[0].GetStatusType() != penum.StoryStatusType_StoryStatusType_Unread {
		t.Fatalf("increased-price unlock returned an incorrect story delta: %v", stories)
	}
	profileReject(t, codes.FailedPrecondition, client.ReleaseEpisode, episodes[boundary].request)
	assertPoints(0)
}
