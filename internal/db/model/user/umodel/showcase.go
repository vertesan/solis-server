package umodel

import "github.com/vertesan/solis-server/internal/proto/papi"

type UserShowcaseLayout struct {
	SUID       string                   `bson:"SUID"`
	Number     int32                    `bson:"number"`
	LayoutInfo *papi.ShowcaseLayoutInfo `bson:"layoutInfo"`
}

type UserShowcaseMysetLayout struct {
	SUID       string                   `bson:"SUID"`
	Number     int32                    `bson:"number"`
	LayoutInfo *papi.ShowcaseLayoutInfo `bson:"layoutInfo"`
}
