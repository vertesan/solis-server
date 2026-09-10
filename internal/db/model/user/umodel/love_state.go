package umodel

import "github.com/vertesan/solis-server/internal/proto/papi"

// UserLoveState supplements the client-facing UserLove with server-owned
// progression. Saves deliberately contain only the current story route.
type UserLoveState struct {
	SUID                 string               `bson:"SUID"`
	LoveID               string               `bson:"loveId"`
	Registered           bool                 `bson:"registered"`
	PlayerName           string               `bson:"playerName"`
	Exp                  int64                `bson:"exp"`
	LastLessonRewardTime int64                `bson:"lastLessonRewardTime"`
	LastPlayTime         int64                `bson:"lastPlayTime"`
	StoryEpisodeIDs      []string             `bson:"storyEpisodeIds"`
	Selections           []LoveStorySelection `bson:"selections"`
	PastSelections       []LoveStorySelection `bson:"pastSelections"`
	ActiveStoryEpisodeID string               `bson:"activeStoryEpisodeId"`
	Saves                []UserLoveSave       `bson:"saves"`
}

type LoveStorySelection struct {
	StoryEpisodeID string `bson:"storyEpisodeId"`
	GroupIndex     int32  `bson:"groupIndex"`
	Index          int32  `bson:"index"`
}

type UserLoveSave struct {
	Info            *papi.LoveSaveDataInfo `bson:"info"`
	StoryEpisodeIDs []string               `bson:"storyEpisodeIds"`
	Selections      []LoveStorySelection   `bson:"selections"`
}
