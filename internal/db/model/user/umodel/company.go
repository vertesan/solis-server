package umodel

// UserCompanyRewardState stores reward claims that are not represented by the
// game's UserCompany protobuf. Character-level claims are keyed by character ID.
type UserCompanyRewardState struct {
	SUID                            string             `bson:"SUID"`
	ReceivedGradeIds                []string           `bson:"receivedGradeIds"`
	ReceivedTrustLevelKeys          []int32            `bson:"receivedTrustLevelKeys"`
	ReceivedEnjoyCharacterLevelKeys map[string][]int32 `bson:"receivedEnjoyCharacterLevelKeys"`
}
