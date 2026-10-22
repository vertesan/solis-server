package smodel

// StagedMasterPack identifies an immutable encrypted file, not an arbitrary path.
// Keys and SQL definitions never cross the administrator HTTP API.
type StagedMasterPack struct {
	Type         string `json:"type" bson:"type"`
	BaseFileName string `json:"baseFileName" bson:"baseFileName"`
	FileName     string `json:"fileName" bson:"fileName"`
	FileSize     int64  `json:"fileSize" bson:"fileSize"`
	RowCount     int64  `json:"rowCount" bson:"rowCount"`
}

type MasterImportRequest struct {
	ID              string             `json:"id" bson:"id"`
	ExpectedVersion string             `json:"expectedVersion" bson:"expectedVersion"`
	Packs           []StagedMasterPack `json:"packs" bson:"packs"`
	AllowEmpty      bool               `json:"allowEmpty" bson:"allowEmpty"`
	// Absent on older jobs: those retain ordinary staged-import semantics.
	Reset bool `json:"reset,omitempty" bson:"reset,omitempty"`
}

// Jobs contain only a bounded manifest-sized description, never master rows.
// Committed is set in the same transaction as the master collections/version.
type MasterImportJob struct {
	ID        string              `json:"id" bson:"_id"`
	Request   MasterImportRequest `json:"request" bson:"request"`
	State     string              `json:"state" bson:"state"`
	Version   string              `json:"version,omitempty" bson:"version"`
	Committed bool                `json:"committed" bson:"committed"`
	Error     string              `json:"error,omitempty" bson:"error"`
	CreatedAt int64               `json:"createdAt" bson:"createdAt"`
	UpdatedAt int64               `json:"updatedAt" bson:"updatedAt"`
}
