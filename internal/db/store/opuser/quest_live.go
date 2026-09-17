package opuser

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/mcontext"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	// These bounds include the whole encoded replay, not the large JSON form.
	QuestLiveReplayMaxBytes           = 64 << 20
	QuestLiveReplayMaxCompressedBytes = 32 << 20
	QuestLiveReplayChunkBytes         = 512 << 10
	questLiveReplayFormatVersion      = 1
)

var ErrQuestLiveReplayCorrupt = errors.New("quest live replay is corrupt")

type QuestLiveResultInfo struct {
	RankPatterns   []*papi.QuestRankPattern
	RulesetVersion string
	MasterVersion  string
	DeckNumber     int32
	DeckPositions  []*ptransaction.UserDeckPosition
}

type QuestLiveReplaySelection int

const (
	QuestLiveReplayLatest QuestLiveReplaySelection = iota
	QuestLiveReplayBest
	QuestLiveReplayLatestClear
)

func FindUserQuestLiveHistory(ctx context.Context, suid, questID string) (*umodel.UserQuestLiveHistory, error) {
	return ucommon.FindOne[umodel.UserQuestLiveHistory](ctx, uname.UserQuestLiveHistory,
		bson.M{"SUID": suid, "questId": questID})
}

func questLiveReplayID(history *umodel.UserQuestLiveHistory, selection QuestLiveReplaySelection) (string, error) {
	switch selection {
	case QuestLiveReplayLatest:
		return history.LatestID, nil
	case QuestLiveReplayBest:
		return history.BestID, nil
	case QuestLiveReplayLatestClear:
		return history.LatestClearID, nil
	default:
		return "", fmt.Errorf("invalid quest replay selection")
	}
}

// FindQuestLiveReplayInfo only reads small metadata, suitable for rankings.
// Call in a transaction when it is followed by dependent reads.
func FindQuestLiveReplayInfo(ctx context.Context, suid, questID string, selection QuestLiveReplaySelection) (*umodel.UserQuestLiveReplay, error) {
	history, err := FindUserQuestLiveHistory(ctx, suid, questID)
	if err != nil {
		return nil, err
	}
	id, err := questLiveReplayID(history, selection)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, mongo.ErrNoDocuments
	}
	return ucommon.FindOne[umodel.UserQuestLiveReplay](ctx, uname.UserQuestLiveReplay,
		bson.M{"SUID": suid, "questId": questID, "_id": id})
}

// FindQuestLatestClearReplayInfos resolves menu/bulk-play references without
// reading or decompressing any chart payloads. Read inside a transaction so
// concurrent replay pruning cannot invalidate the references between queries.
func FindQuestLatestClearReplayInfos(ctx context.Context, suid string, questIDs []string) (map[string]*umodel.UserQuestLiveReplay, error) {
	result := make(map[string]*umodel.UserQuestLiveReplay, len(questIDs))
	if len(questIDs) == 0 {
		return result, nil
	}
	histories, err := ucommon.FindAll[umodel.UserQuestLiveHistory](ctx, uname.UserQuestLiveHistory,
		bson.M{"SUID": suid, "questId": bson.M{"$in": questIDs}})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(histories))
	wanted := make(map[string]string, len(histories))
	for _, history := range histories {
		if history.LatestClearID != "" {
			ids = append(ids, history.LatestClearID)
			wanted[history.QuestID] = history.LatestClearID
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	replays, err := ucommon.FindAll[umodel.UserQuestLiveReplay](ctx, uname.UserQuestLiveReplay,
		bson.M{"SUID": suid, "_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	for _, replay := range replays {
		if replay.Cleared && wanted[replay.QuestID] == replay.ID {
			result[replay.QuestID] = replay
		}
	}
	return result, nil
}

// SaveQuestLiveResult atomically publishes a compressed trace with its latest,
// best and latest-clear references, pruning anything no longer referenced.
// It joins the gameplay transaction; standalone callers also get atomic writes.
func SaveQuestLiveResult(ctx context.Context, suid, questID string, result *papi.LiveResult, isBest bool, infos ...QuestLiveResultInfo) error {
	if suid == "" || len(suid) > 128 || questID == "" || len(questID) > 256 || result == nil || len(result.GetUserInfos()) == 0 || len(result.GetUserInfos()) > 2 || result.GetUserInfos()[0].GetUserResult() == nil || len(infos) > 1 {
		return status.Error(codes.InvalidArgument, "invalid quest live result")
	}
	if result.GetUserInfos()[0].GetIndex() != 1 || result.GetUserInfos()[0].GetIsNpc() {
		return status.Error(codes.InvalidArgument, "quest live result has no primary user side")
	}
	var info QuestLiveResultInfo
	if len(infos) == 1 {
		info = infos[0]
	}
	if len(info.RankPatterns) > 256 || len(info.RulesetVersion) > 256 || len(info.MasterVersion) > 256 || info.DeckNumber < 0 || len(info.DeckPositions) > 5 {
		return status.Error(codes.InvalidArgument, "invalid quest live result metadata")
	}
	ctx, err := mcontext.WithLiveRequest(ctx)
	if err != nil {
		return err
	}
	seed, startedAt, _ := mcontext.GetLiveRequest(ctx)
	// The opaque ID is stable on driver retries, but does not expose the seed.
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%s:%016x:%d", suid, questID, seed, startedAt.UnixNano()))).String()
	result.DetailLogId = id
	result.IsDisplayDetailAvailable = true
	snapshot := &papi.QuestGetLatestClearLiveResultResponse{Result: result, RankPatterns: info.RankPatterns}
	if proto.Size(snapshot) > QuestLiveReplayMaxBytes {
		return status.Error(codes.ResourceExhausted, "live replay exceeds encoded size limit")
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
	if err != nil {
		return err
	}
	compressed := &questReplayBuffer{}
	writer, err := gzip.NewWriterLevel(compressed, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if _, err := writer.Write(raw); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	metadata := &umodel.UserQuestLiveReplay{
		ID: id, SUID: suid, QuestID: questID, CreatedAt: startedAt.UnixMilli(), Seed: fmt.Sprintf("%016x", seed),
		FormatVersion: questLiveReplayFormatVersion, RulesetVersion: info.RulesetVersion, MasterVersion: info.MasterVersion,
		EncodedSize: int32(len(raw)), CompressedSize: int32(compressed.Len()),
		ChunkCount: int32((compressed.Len() + QuestLiveReplayChunkBytes - 1) / QuestLiveReplayChunkBytes),
		SHA256:     hex.EncodeToString(digest[:]), Score: result.GetUserInfos()[0].GetUserResult().GetScore(), Cleared: result.GetCleared(),
		DeckNumber: info.DeckNumber, DeckName: result.GetUserInfos()[0].GetUserDeck().GetDeckName(),
	}
	for _, position := range info.DeckPositions {
		if position == nil || proto.Size(position) > 4096 {
			return status.Error(codes.InvalidArgument, "invalid quest live deck position metadata")
		}
		metadata.DeckPositions = append(metadata.DeckPositions, proto.Clone(position).(*ptransaction.UserDeckPosition))
	}
	for _, card := range result.GetUserInfos()[0].GetUserDeck().GetCards() {
		metadata.Cards = append(metadata.Cards, &papi.LiveCardInfo{
			Position: card.GetIndex(), CardId: card.GetCardId(), Level: card.GetLevel(), Rarity: card.GetRarity(), EvolutionLevel: card.GetEvolutionLevel(),
		})
	}
	if len(metadata.Cards) > 5 || len(metadata.DeckName) > 1024 {
		return status.Error(codes.InvalidArgument, "invalid quest live deck metadata")
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		history, err := FindUserQuestLiveHistory(ctx, suid, questID)
		if errors.Is(err, mongo.ErrNoDocuments) {
			history = &umodel.UserQuestLiveHistory{Key: suid + ":" + questID, SUID: suid, QuestID: questID}
		} else if err != nil {
			return nil, err
		}
		history.LatestID = id
		if isBest {
			history.BestID = id
		}
		if result.GetCleared() {
			history.LatestClearID = id
		}
		chunks := make([]any, 0, metadata.ChunkCount)
		body := compressed.Bytes()
		for offset, number := 0, int32(0); offset < len(body); number++ {
			end := min(offset+QuestLiveReplayChunkBytes, len(body))
			chunks = append(chunks, &umodel.UserQuestLiveChunk{
				Key: fmt.Sprintf("%s:%d", id, number), SUID: suid, QuestID: questID, ReplayID: id, Number: number, Data: body[offset:end],
			})
			offset = end
		}
		if _, err := depot.User.Collection(uname.UserQuestLiveChunk).InsertMany(ctx, chunks); err != nil {
			return nil, err
		}
		if err := ucommon.InsertOne(ctx, uname.UserQuestLiveReplay, metadata); err != nil {
			return nil, err
		}
		if err := ucommon.UpsertOne(ctx, uname.UserQuestLiveHistory, bson.M{"_id": history.Key}, history); err != nil {
			return nil, err
		}
		retained := []string{history.LatestID, history.BestID, history.LatestClearID}
		if err := ucommon.Delete(ctx, uname.UserQuestLiveReplay, bson.M{"SUID": suid, "questId": questID, "_id": bson.M{"$nin": retained}}); err != nil {
			return nil, err
		}
		return nil, ucommon.Delete(ctx, uname.UserQuestLiveChunk, bson.M{"SUID": suid, "questId": questID, "replayId": bson.M{"$nin": retained}})
	})
}

type questReplayBuffer struct{ bytes.Buffer }

func (b *questReplayBuffer) Write(data []byte) (int, error) {
	if len(data) > QuestLiveReplayMaxCompressedBytes-b.Len() {
		return 0, status.Error(codes.ResourceExhausted, "live replay exceeds compressed size limit")
	}
	return b.Buffer.Write(data)
}

// FindQuestLiveResult reads references and chunks from one snapshot, so a
// concurrent clear/prune cannot turn an otherwise valid replay into missing data.
func FindQuestLiveResult(ctx context.Context, suid, questID string, selection QuestLiveReplaySelection) (*papi.QuestGetLatestClearLiveResultResponse, error) {
	var result *papi.QuestGetLatestClearLiveResultResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		metadata, err := FindQuestLiveReplayInfo(ctx, suid, questID, selection)
		if err != nil {
			return nil, err
		}
		result, err = decodeQuestLiveResult(ctx, metadata)
		return nil, err
	})
	return result, err
}

func decodeQuestLiveResult(ctx context.Context, metadata *umodel.UserQuestLiveReplay) (*papi.QuestGetLatestClearLiveResultResponse, error) {
	if metadata.FormatVersion != questLiveReplayFormatVersion || metadata.EncodedSize <= 0 || metadata.EncodedSize > QuestLiveReplayMaxBytes || metadata.CompressedSize <= 0 || metadata.CompressedSize > QuestLiveReplayMaxCompressedBytes || metadata.ChunkCount != (metadata.CompressedSize+QuestLiveReplayChunkBytes-1)/QuestLiveReplayChunkBytes || len(metadata.SHA256) != 64 {
		return nil, ErrQuestLiveReplayCorrupt
	}
	cursor, err := depot.User.Collection(uname.UserQuestLiveChunk).Find(ctx,
		bson.M{"SUID": metadata.SUID, "questId": metadata.QuestID, "replayId": metadata.ID},
		options.Find().SetSort(bson.D{{Key: "number", Value: 1}}).SetLimit(int64(metadata.ChunkCount)+1))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var compressed bytes.Buffer
	compressed.Grow(int(metadata.CompressedSize))
	var number int32
	for cursor.Next(ctx) {
		if len(cursor.Current) > QuestLiveReplayChunkBytes+1024 {
			return nil, ErrQuestLiveReplayCorrupt
		}
		var chunk umodel.UserQuestLiveChunk
		if err := cursor.Decode(&chunk); err != nil {
			return nil, err
		}
		if number >= metadata.ChunkCount || chunk.Number != number || len(chunk.Data) != min(QuestLiveReplayChunkBytes, int(metadata.CompressedSize)-compressed.Len()) {
			return nil, ErrQuestLiveReplayCorrupt
		}
		compressed.Write(chunk.Data)
		number++
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	if number != metadata.ChunkCount || compressed.Len() != int(metadata.CompressedSize) {
		return nil, ErrQuestLiveReplayCorrupt
	}
	reader, err := gzip.NewReader(&compressed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQuestLiveReplayCorrupt, err)
	}
	defer reader.Close()
	reader.Multistream(false)
	// The declared, bounded encoded size lets us allocate exactly once. Read one
	// extra byte to reject expansion beyond it and to force gzip checksum checks.
	raw := make([]byte, int(metadata.EncodedSize))
	if _, err := io.ReadFull(reader, raw); err != nil {
		return nil, ErrQuestLiveReplayCorrupt
	}
	var extra [1]byte
	if n, err := io.ReadFull(reader, extra[:]); n != 0 || err != io.EOF || compressed.Len() != 0 {
		return nil, ErrQuestLiveReplayCorrupt
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != metadata.SHA256 {
		return nil, ErrQuestLiveReplayCorrupt
	}
	result := &papi.QuestGetLatestClearLiveResultResponse{}
	if err := (proto.UnmarshalOptions{RecursionLimit: 100}).Unmarshal(raw, result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQuestLiveReplayCorrupt, err)
	}
	if result.GetResult().GetDetailLogId() != metadata.ID || result.GetResult().GetCleared() != metadata.Cleared || len(result.GetResult().GetUserInfos()) == 0 || result.GetResult().GetUserInfos()[0].GetUserResult().GetScore() != metadata.Score {
		return nil, ErrQuestLiveReplayCorrupt
	}
	return result, nil
}
