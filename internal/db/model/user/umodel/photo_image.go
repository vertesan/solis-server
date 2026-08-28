package umodel

import "time"

const (
	UserPhotoImageUploadActive     = "active"
	UserPhotoImageUploadFinalizing = "finalizing"
	UserPhotoImageUploadFinalized  = "finalized"
)

// UserPhotoImageMetadata stores the Firebase Storage-compatible metadata for
// one uploaded photo. The session fields are internal bookkeeping and must not
// be exposed in the Firebase metadata response.
type UserPhotoImageMetadata struct {
	ID                   string     `bson:"_id" json:"-"`
	SUID                 string     `bson:"SUID" json:"-"`
	PublicUserId         string     `bson:"publicUserId" json:"-"`
	UploadId             string     `bson:"uploadId" json:"-"`
	UploadState          string     `bson:"uploadState" json:"-"`
	FinalizationId       string     `bson:"finalizationId,omitempty" json:"-"`
	UploadStateUpdatedAt time.Time  `bson:"uploadStateUpdatedAt" json:"-"`
	UploadExpiresAt      *time.Time `bson:"uploadExpiresAt,omitempty" json:"-"`

	Name               string `bson:"name" json:"name"`
	Bucket             string `bson:"bucket" json:"bucket"`
	Generation         string `bson:"generation" json:"generation"`
	Metageneration     string `bson:"metageneration" json:"metageneration"`
	ContentType        string `bson:"contentType" json:"contentType"`
	TimeCreated        string `bson:"timeCreated" json:"timeCreated"`
	Updated            string `bson:"updated" json:"updated"`
	StorageClass       string `bson:"storageClass" json:"storageClass"`
	Size               string `bson:"size" json:"size"`
	MD5Hash            string `bson:"md5Hash" json:"md5Hash"`
	ContentEncoding    string `bson:"contentEncoding" json:"contentEncoding"`
	ContentDisposition string `bson:"contentDisposition" json:"contentDisposition"`
	CRC32C             string `bson:"crc32c" json:"crc32c"`
	ETag               string `bson:"etag" json:"etag"`
	DownloadTokens     string `bson:"downloadTokens" json:"downloadTokens"`
}
