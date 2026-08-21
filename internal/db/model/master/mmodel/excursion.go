package mmodel

type ExcursionGazeReaction struct {
	CharacterId      string
	ExcursionPlaceId string
	Number           int64
	DataHolder       `bson:",inline"`
}

type ExcursionPlace struct {
	Id         string
	DataHolder `bson:",inline"`
}

type ExcursionPlaceSetting struct {
	CharacterId      string
	ExcursionPlaceId string
	DataHolder       `bson:",inline"`
}
