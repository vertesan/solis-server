package umodel

// UserBirthdayState keeps only the current annual event. Its three participants,
// watched stories, and claimed rewards remain stable if favorites are edited.
type UserBirthdayState struct {
	SUID        string   `bson:"SUID"`
	EventYear   int      `bson:"eventYear"`
	BirthdayIDs []string `bson:"birthdayIds"`
	// Older events stored the participants in reverse favorite order.
	InFavoriteOrder bool     `bson:"inFavoriteOrder"`
	WatchedIDs      []string `bson:"watchedIds"`
	ClaimedIDs      []string `bson:"claimedIds"`
	RemindedYear    int      `bson:"remindedYear"`
}
