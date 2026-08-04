package smodel

type OctoManifest struct {
	Revision  int32  `bson:"revision"`
	UrlFormat string `bson:"urlFormat"`
}

type Qseed struct {
	Device string `bson:"device"`
	Qseed  string `bson:"qseed"`
}

type ServerProfile struct {
	AppVersion         string `bson:"appVersion"`
	Platform           string `bson:"platform"`
	MasterVersion      string `bson:"masterVersion"`
	FirebaseProductKey string `bson:"firebaseProductKey"`
	FirebaseProjectId  string `bson:"firebaseProjectId"`
	PackageName        string `bson:"packageName"`
	OctoApiKey         string `bson:"octoApiKey"`
	OctoManifestKey    string `bson:"octoManifestKey"`
}

type Migration struct {
	Migration int32 `bson:"migration"`
}
