package test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestQuestLiveReplayRetentionScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	areas, err := opmaster.FindAllAreas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var questID string
	for _, area := range areas {
		if area.GetType() == penum.AreaType_AreaType_Contest && len(area.GetQuestIds()) > 0 {
			questID = area.GetQuestIds()[0]
			break
		}
	}
	if questID == "" {
		t.Fatal("master has no contest quest")
	}
	cards, err := opuser.FindAllUserCards(ctx, testConfig.TestUserSuid)
	if err != nil || len(cards) == 0 {
		t.Fatalf("configured user cards: %v", err)
	}
	deck := &papi.LiveUserDeck{DeckName: "replay unit"}
	for i, card := range cards[:min(5, len(cards))] {
		deck.Cards = append(deck.Cards, &papi.LiveDeckCard{Index: int32(i + 1), CardId: card.GetCardId(), Level: card.GetLevel(), Rarity: card.GetRarity()})
	}
	makeResult := func(score int64, cleared bool) *papi.LiveResult {
		return &papi.LiveResult{Cleared: cleared, TotalAudienceAmount: 50,
			UserInfos: []*papi.LiveUserInfo{{Index: 1, UserId: testConfig.TestUserPublicId, UserDeck: deck, UserResult: &papi.LiveUserResult{Score: score}}},
			Charts:    []*papi.LiveChart{{Number: 1, Beats: []*papi.LiveBeat{{CardIndex: 1, Score: score}}}},
		}
	}
	patterns := []*papi.QuestRankPattern{{RankType: penum.ResultRankType_ResultRankType_C, ThresholdScore: 50}}
	info := opuser.QuestLiveResultInfo{RankPatterns: patterns, RulesetVersion: "replay-test-v1", MasterVersion: "test-master"}
	save := func(result *papi.LiveResult, best bool) {
		t.Helper()
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			if err := opuser.SaveQuestLiveResult(ctx, testConfig.TestUserSuid, questID, result, best, info); err != nil {
				return nil, err
			}
			if best {
				return nil, opuser.UpsertUserQuest(ctx, testConfig.TestUserSuid, &ptransaction.UserQuest{QuestId: questID, HighestScore: result.GetUserInfos()[0].GetUserResult().GetScore()})
			}
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	best := makeResult(100, true)
	// Unknown protobuf fields exercise lossless format evolution and a genuinely
	// multi-chunk payload without fabricating additional users or game cards.
	rng := rand.New(rand.NewPCG(1, 2))
	opaque := make([]byte, 2<<20)
	for i := range opaque {
		opaque[i] = byte(rng.Uint64())
	}
	unknown := protowire.AppendTag(nil, 50000, protowire.BytesType)
	best.ProtoReflect().SetUnknown(protowire.AppendBytes(unknown, opaque))
	save(best, true)
	metadata, err := opuser.FindQuestLiveReplayInfo(ctx, testConfig.TestUserSuid, questID, opuser.QuestLiveReplayBest)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ChunkCount < 2 || metadata.RulesetVersion != info.RulesetVersion || metadata.MasterVersion != info.MasterVersion || metadata.Seed == "" {
		t.Fatalf("replay metadata lost provenance/chunking: %#v", metadata)
	}
	client := papi.NewQuestClient(testConfig.GrpcConn)
	latest := profileCall(t, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: questID, UserId: testConfig.TestUserPublicId, IsDetail: true})
	if !proto.Equal(latest.GetResult(), best) || len(latest.GetRankPatterns()) != 1 || !proto.Equal(latest.GetRankPatterns()[0], patterns[0]) {
		t.Fatal("latest clear replay did not preserve the complete trace and rank patterns")
	}
	ranked := profileCall(t, client.GetRankingLiveResult, &papi.QuestGetRankingLiveResultRequest{QuestId: questID, UserId: testConfig.TestUserPublicId})
	if !proto.Equal(ranked.GetResult(), best) {
		t.Fatal("best replay differs from saved snapshot")
	}
	profileReject(t, codes.NotFound, client.GetLatestClearLiveResult, &papi.QuestGetLatestClearLiveResultRequest{QuestId: questID, UserId: "NORESULT"})
	profileReject(t, codes.NotFound, client.GetRankingLiveResult, &papi.QuestGetRankingLiveResultRequest{QuestId: questID, UserId: "NORESULT"})
	if len(profileCall(t, client.ListLatestClear, &papi.QuestListLatestClearRequest{QuestId: questID}).GetClears()) != 0 {
		t.Fatal("latest-clear public list must remain empty")
	}
	second := makeResult(90, true)
	save(second, false)
	save(makeResult(80, false), false)
	last := makeResult(70, false)
	save(last, false)
	history, err := opuser.FindUserQuestLiveHistory(ctx, testConfig.TestUserSuid, questID)
	if err != nil {
		t.Fatal(err)
	}
	if history.LatestID != last.GetDetailLogId() || history.BestID != best.GetDetailLogId() || history.LatestClearID != second.GetDetailLogId() {
		t.Fatalf("unexpected retained references: %#v", history)
	}
	assertQuestReplayStorage(t, questID, 3)
	for selection, expected := range map[opuser.QuestLiveReplaySelection]*papi.LiveResult{
		opuser.QuestLiveReplayLatest: last, opuser.QuestLiveReplayBest: best, opuser.QuestLiveReplayLatestClear: second,
	} {
		replay, err := opuser.FindQuestLiveResult(ctx, testConfig.TestUserSuid, questID, selection)
		if err != nil || !proto.Equal(replay.GetResult(), expected) {
			t.Fatalf("selection %d returned wrong snapshot: %v", selection, err)
		}
	}
	abort := errors.New("abort replay transaction")
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := opuser.SaveQuestLiveResult(ctx, testConfig.TestUserSuid, questID, makeResult(1000, true), true, info); err != nil {
			return nil, err
		}
		return nil, abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("rollback failed: %v", err)
	}
	after, err := opuser.FindUserQuestLiveHistory(ctx, testConfig.TestUserSuid, questID)
	if err != nil || *after != *history {
		t.Fatalf("aborted replay replaced history: %v", err)
	}
	assertQuestReplayStorage(t, questID, 3)
	// Each malformed-storage case is rolled back: fail closed before decoding or
	// returning partial images of the historical live.
	for _, corruption := range []string{"oversized", "missing-chunk", "bad-digest"} {
		err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			database := mongo.SessionFromContext(ctx).Client().Database("user")
			filter := bson.M{"SUID": testConfig.TestUserSuid, "questId": questID, "_id": best.GetDetailLogId()}
			var err error
			switch corruption {
			case "oversized":
				_, err = database.Collection(uname.UserQuestLiveReplay).UpdateOne(ctx, filter, bson.M{"$set": bson.M{"encodedSize": int32(opuser.QuestLiveReplayMaxBytes + 1)}})
			case "missing-chunk":
				_, err = database.Collection(uname.UserQuestLiveChunk).DeleteOne(ctx, bson.M{"SUID": testConfig.TestUserSuid, "questId": questID, "replayId": best.GetDetailLogId(), "number": 0})
			case "bad-digest":
				_, err = database.Collection(uname.UserQuestLiveReplay).UpdateOne(ctx, filter, bson.M{"$set": bson.M{"sha256": fmt.Sprintf("%064d", 0)}})
			}
			if err != nil {
				return nil, err
			}
			_, err = opuser.FindQuestLiveResult(ctx, testConfig.TestUserSuid, questID, opuser.QuestLiveReplayBest)
			if !errors.Is(err, opuser.ErrQuestLiveReplayCorrupt) {
				return nil, fmt.Errorf("%s was not rejected: %v", corruption, err)
			}
			return nil, abort
		})
		if !errors.Is(err, abort) {
			t.Fatal(err)
		}
	}
	newBest := makeResult(110, true)
	save(newBest, true)
	assertQuestReplayStorage(t, questID, 1)
	ranking := profileCall(t, client.ListContestRanking, &papi.QuestListContestRankingRequest{QuestId: questID}).GetRanks()[0]
	if !ranking.GetIsLiveResultDetailAvailable() || ranking.GetScore() != 110 || ranking.GetDeckName() != deck.GetDeckName() || len(ranking.GetCards()) != len(deck.GetCards()) {
		t.Fatalf("ranking is not wired to persisted replay metadata: %v", ranking)
	}
	if err := deleteConfiguredTestUser(); err != nil {
		t.Fatal(err)
	}
	assertQuestReplayStorage(t, questID, 0)
	if _, err := opuser.FindUserQuestLiveHistory(ctx, testConfig.TestUserSuid, questID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("user deletion left replay history: %v", err)
	}
}

func assertQuestReplayStorage(t *testing.T, questID string, expected int64) {
	t.Helper()
	err := transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		database := mongo.SessionFromContext(ctx).Client().Database("user")
		filter := bson.M{"SUID": testConfig.TestUserSuid, "questId": questID}
		count, err := database.Collection(uname.UserQuestLiveReplay).CountDocuments(ctx, filter)
		if err != nil {
			return nil, err
		}
		if count != expected {
			return nil, fmt.Errorf("retained %d replay payloads, expected %d", count, expected)
		}
		cursor, err := database.Collection(uname.UserQuestLiveChunk).Find(ctx, filter)
		if err != nil {
			return nil, err
		}
		defer cursor.Close(ctx)
		var chunkCount int
		for cursor.Next(ctx) {
			if len(cursor.Current) >= 1<<20 {
				return nil, errors.New("replay chunk BSON exceeded 1 MiB")
			}
			var chunk umodel.UserQuestLiveChunk
			if err := cursor.Decode(&chunk); err != nil {
				return nil, err
			}
			if len(chunk.Data) > opuser.QuestLiveReplayChunkBytes {
				return nil, errors.New("chunk payload exceeded limit")
			}
			count, err := database.Collection(uname.UserQuestLiveReplay).CountDocuments(ctx, bson.M{"SUID": testConfig.TestUserSuid, "questId": questID, "_id": chunk.ReplayID})
			if err != nil {
				return nil, err
			}
			if count != 1 {
				return nil, errors.New("orphaned replay chunk")
			}
			chunkCount++
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		if expected == 0 && chunkCount != 0 {
			return nil, errors.New("user deletion left replay chunks")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQuestLiveRequestRetryScenario(t *testing.T) {
	setupTestUser(t)
	ctx, err := mcontext.WithLiveRequest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seed, startedAt, ok := mcontext.GetLiveRequest(ctx)
	if !ok || startedAt.IsZero() {
		t.Fatal("live request metadata missing")
	}
	attempts := 0
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		attempts++
		ctx, err := mcontext.WithLiveRequest(ctx)
		if err != nil {
			return nil, err
		}
		actualSeed, actualTime, ok := mcontext.GetLiveRequest(ctx)
		if !ok || actualSeed != seed || actualTime != startedAt {
			return nil, errors.New("transaction retry rerolled live request")
		}
		if attempts == 1 {
			return nil, mongo.CommandError{Code: 112, Message: "injected retry", Labels: []string{"TransientTransactionError"}}
		}
		return nil, nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry-stable request failed (%d attempts): %v", attempts, err)
	}
	fresh, err := mcontext.WithLiveRequest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nextSeed, nextTime, _ := mcontext.GetLiveRequest(fresh)
	if nextSeed == seed && nextTime == startedAt {
		t.Fatal("independent live request reused metadata")
	}
}
