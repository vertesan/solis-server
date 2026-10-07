package umodel

import "github.com/vertesan/solis-server/internal/proto/papi"

// UserExerciseDeck holds at most five editable positions. The training inventory
// and fixed positions are always read from master data, never user possessions.
// Completion remains in UserQuest; no unbounded replay/history is embedded here.
type UserExerciseDeck struct {
	SUID       string             `bson:"SUID"`
	ExerciseID string             `bson:"exerciseId"`
	Data       *papi.ExerciseInfo `bson:"data"`
}
