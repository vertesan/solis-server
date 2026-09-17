package umodel

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
)

// UserQuestLiveHistory keeps at most three distinct cold replay payloads per quest.
type UserQuestLiveHistory struct {
	Key           string `bson:"_id"`
	SUID          string `bson:"SUID"`
	QuestID       string `bson:"questId"`
	LatestID      string `bson:"latestId"`
	BestID        string `bson:"bestId"`
	LatestClearID string `bson:"latestClearId"`
}

// UserQuestLiveReplay is small metadata; ranking reads never decode the trace.
type UserQuestLiveReplay struct {
	ID             string                           `bson:"_id"`
	SUID           string                           `bson:"SUID"`
	QuestID        string                           `bson:"questId"`
	CreatedAt      int64                            `bson:"createdAt"`
	Seed           string                           `bson:"seed"`
	FormatVersion  int32                            `bson:"formatVersion"`
	RulesetVersion string                           `bson:"rulesetVersion"`
	MasterVersion  string                           `bson:"masterVersion"`
	EncodedSize    int32                            `bson:"encodedSize"`
	CompressedSize int32                            `bson:"compressedSize"`
	ChunkCount     int32                            `bson:"chunkCount"`
	SHA256         string                           `bson:"sha256"`
	Score          int64                            `bson:"score"`
	Cleared        bool                             `bson:"cleared"`
	DeckNumber     int32                            `bson:"deckNumber,omitempty"`
	DeckPositions  []*ptransaction.UserDeckPosition `bson:"deckPositions,omitempty"`
	DeckName       string                           `bson:"deckName"`
	Cards          []*papi.LiveCardInfo             `bson:"cards"`
}

// UserQuestLiveChunk holds a bounded piece of compressed protobuf, not BSON
// arrays of chart structs. Chunks publish/prune in the gameplay transaction.
type UserQuestLiveChunk struct {
	Key      string `bson:"_id"`
	SUID     string `bson:"SUID"`
	QuestID  string `bson:"questId"`
	ReplayID string `bson:"replayId"`
	Number   int32  `bson:"number"`
	Data     []byte `bson:"data"`
}
