package umodel

// UserDreamState retains only the current monthly boundary. Generated areas
// have their own lifetime and are never cleared by the monthly reset.
type UserDreamState struct {
	SUID            string `bson:"SUID"`
	PeriodStart     int64  `bson:"periodStart"`
	EventMissionID  string `bson:"eventMissionId"`
	QuestClearCount int64  `bson:"questClearCount"`
	MaxDifficulty   int32  `bson:"maxDifficulty"`
}
