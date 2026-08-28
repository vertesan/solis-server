package game

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image/jpeg"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/jwto"
	"github.com/vertesan/solis-server/internal/utils/mcontext"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

const (
	firebaseStorageBucket             = "udc-idolypride"
	firebaseStorageContentType        = "image/jpeg"
	firebaseStorageProject            = "solis-firebase"
	firebaseStorageChunkSize          = "262144"
	firebaseStorageUploadActive       = "active"
	firebaseStorageUploadFinal        = "final"
	firebaseStorageTimeFormat         = "2006-01-02T15:04:05.000Z"
	photoUploadSessionDuration        = 24 * time.Hour
	photoUploadLeaseDuration          = time.Hour
	maximumPhotoPixels          int64 = 20_000_000
)

type firebaseStorageMetadataRequest struct {
	ContentType string `json:"contentType"`
}

type photoObjectName struct {
	Name         string
	PublicUserId string
	Filename     string
}

var PostPhoto fiber.Handler = func(c fiber.Ctx) error {
	name, err := parsePhotoObjectName(c.Query("name"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid photo name")
	}
	if c.Query("uploadType") != "resumable" || c.Get("X-Goog-Upload-Protocol") != "resumable" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload protocol")
	}

	userAuth, err := authenticatePhotoOwner(c, name.PublicUserId)
	if err != nil {
		return err
	}

	uploadId := c.Query("upload_id")
	if uploadId == "" {
		return startPhotoUpload(c, name, userAuth)
	}
	return finalizePhotoUpload(c, name, userAuth, uploadId)
}

func startPhotoUpload(c fiber.Ctx, name *photoObjectName, userAuth *umodel.UserAuth) error {
	if c.Get("X-Goog-Upload-Command") != "start" || c.Query("upload_protocol") != "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload command")
	}
	if c.Get("X-Goog-Upload-Header-Content-Type") != firebaseStorageContentType {
		return fiber.NewError(fiber.StatusBadRequest, "invalid photo content type")
	}

	request := &firebaseStorageMetadataRequest{}
	if err := json.Unmarshal(c.Body(), request); err != nil || request.ContentType != firebaseStorageContentType {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload metadata")
	}
	now := time.Now().UTC()
	if err := opuser.DeleteExpiredUserPhotoImageUploads(c, now); err != nil {
		mcontext.GetLogger(c).Error("failed to clean up expired photo upload sessions", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	uploadId, err := newFirebaseUploadID()
	if err != nil {
		mcontext.GetLogger(c).Error("failed to create photo upload ID", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	expiresAt := now.Add(photoUploadSessionDuration)
	metadata := &umodel.UserPhotoImageMetadata{
		ID:                   name.Name,
		SUID:                 userAuth.ServerUserId,
		PublicUserId:         name.PublicUserId,
		UploadId:             uploadId,
		UploadState:          umodel.UserPhotoImageUploadActive,
		UploadStateUpdatedAt: now,
		UploadExpiresAt:      &expiresAt,
		Name:                 name.Name,
		Bucket:               firebaseStorageBucket,
		ContentType:          firebaseStorageContentType,
	}
	started, err := opuser.StartUserPhotoImageUpload(c, metadata, now.Add(-photoUploadLeaseDuration))
	if err != nil {
		mcontext.GetLogger(c).Error("failed to persist photo upload session", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	if !started {
		existing, findErr := opuser.FindUserPhotoImageMetadata(c, name.Name)
		if findErr == nil && existing.SUID == userAuth.ServerUserId &&
			existing.PublicUserId == name.PublicUserId &&
			existing.UploadState == umodel.UserPhotoImageUploadActive {
			return sendPhotoUploadStart(c, name.Name, existing.UploadId)
		}
		return fiber.NewError(fiber.StatusConflict, "photo already exists or is being finalized")
	}
	return sendPhotoUploadStart(c, name.Name, uploadId)
}

func sendPhotoUploadStart(c fiber.Ctx, name, uploadId string) error {
	uploadURL := firebaseStorageUploadURL(name, uploadId)
	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	c.Set("X-GUploader-UploadID", uploadId)
	c.Set("X-Goog-Upload-Status", firebaseStorageUploadActive)
	c.Set("X-Goog-Upload-URL", uploadURL)
	c.Set("X-Goog-Upload-Control-URL", uploadURL)
	c.Set("X-Goog-Upload-Chunk-Granularity", firebaseStorageChunkSize)
	return c.Send(nil)
}

func finalizePhotoUpload(c fiber.Ctx, name *photoObjectName, userAuth *umodel.UserAuth, uploadId string) error {
	if c.Get("X-Goog-Upload-Command") != "upload, finalize" ||
		c.Get("X-Goog-Upload-Offset") != "0" ||
		c.Query("upload_protocol") != "resumable" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload finalization")
	}

	session, err := opuser.FindUserPhotoImageMetadata(c, name.Name)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(fiber.StatusBadRequest, "unknown upload session")
		}
		mcontext.GetLogger(c).Error("failed to retrieve photo upload session", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	if session.SUID != userAuth.ServerUserId || session.PublicUserId != name.PublicUserId || session.UploadId != uploadId {
		return fiber.NewError(fiber.StatusForbidden, "invalid upload session")
	}
	if session.UploadState == umodel.UserPhotoImageUploadFinalized {
		setFirebaseMetadataHeaders(c, uploadId, firebaseStorageUploadFinal)
		return c.JSON(session)
	}
	if session.UploadState != umodel.UserPhotoImageUploadActive {
		return fiber.NewError(fiber.StatusConflict, "upload is being finalized")
	}
	if session.UploadExpiresAt != nil && !session.UploadExpiresAt.After(time.Now()) {
		return fiber.NewError(fiber.StatusBadRequest, "upload session expired")
	}

	body := c.Body()
	if len(body) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "empty photo")
	}
	jpegConfig, err := jpeg.DecodeConfig(bytes.NewReader(body))
	if err != nil || jpegConfig.Width <= 0 || jpegConfig.Height <= 0 ||
		int64(jpegConfig.Width)*int64(jpegConfig.Height) > maximumPhotoPixels {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JPEG photo")
	}

	finalizationId := uuid.NewString()
	claimTime := time.Now().UTC()
	claimExpiresAt := claimTime.Add(photoUploadSessionDuration)
	claimed, err := opuser.ClaimUserPhotoImageUpload(
		c,
		name.Name,
		userAuth.ServerUserId,
		uploadId,
		finalizationId,
		claimTime,
		claimExpiresAt,
	)
	if err != nil {
		current, findErr := opuser.FindUserPhotoImageMetadata(c, name.Name)
		if findErr == nil && current.SUID == userAuth.ServerUserId &&
			current.UploadId == uploadId && current.FinalizationId == finalizationId &&
			current.UploadState == umodel.UserPhotoImageUploadFinalizing {
			session = current
			claimed = true
		} else {
			mcontext.GetLogger(c).Error("failed to claim photo upload session", zap.Error(err), zap.Error(findErr))
			return fiber.NewError(fiber.StatusInternalServerError, "internal error")
		}
	}
	if !claimed {
		current, findErr := opuser.FindUserPhotoImageMetadata(c, name.Name)
		if findErr == nil && current.SUID == userAuth.ServerUserId && current.UploadId == uploadId &&
			current.UploadState == umodel.UserPhotoImageUploadFinalized {
			setFirebaseMetadataHeaders(c, uploadId, firebaseStorageUploadFinal)
			return c.JSON(current)
		}
		return fiber.NewError(fiber.StatusConflict, "upload is no longer active")
	}
	session.FinalizationId = finalizationId
	session.UploadState = umodel.UserPhotoImageUploadFinalizing
	session.UploadStateUpdatedAt = claimTime
	session.UploadExpiresAt = &claimExpiresAt

	createdAt := time.Now().UTC().Truncate(time.Millisecond)
	metadata := completePhotoMetadata(session, name.Filename, body, createdAt)
	photoPath := photoImagePath(name)
	if err := writePhotoImageAtomically(photoPath, body, createdAt); err != nil {
		removeErr := os.Remove(photoPath)
		releasePhotoUpload(c, session)
		mcontext.GetLogger(c).Error("failed to store photo image", zap.Error(err), zap.Error(removeErr))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	completed, err := opuser.CompleteUserPhotoImageUpload(c, metadata)
	if err != nil || !completed {
		current, findErr := opuser.FindUserPhotoImageMetadata(c, name.Name)
		if findErr == nil && samePublishedPhoto(current, metadata) {
			setFirebaseMetadataHeaders(c, uploadId, firebaseStorageUploadFinal)
			return c.JSON(current)
		}
		if findErr == nil && current.SUID == session.SUID && current.UploadId == session.UploadId &&
			current.FinalizationId == session.FinalizationId &&
			current.UploadState == umodel.UserPhotoImageUploadFinalizing {
			removeErr := os.Remove(photoPath)
			releasePhotoUpload(c, session)
			mcontext.GetLogger(c).Error("photo metadata remained uncommitted", zap.Error(err), zap.Error(removeErr))
		} else if errors.Is(findErr, mongo.ErrNoDocuments) {
			removeErr := os.Remove(photoPath)
			mcontext.GetLogger(c).Error("photo metadata disappeared during finalization", zap.Error(err), zap.Error(removeErr))
		} else {
			// A failed MongoDB write can have committed remotely. Preserve the
			// published bytes whenever the outcome cannot be proven uncommitted.
			mcontext.GetLogger(c).Error("photo metadata finalization outcome is ambiguous", zap.Error(err), zap.Error(findErr))
		}
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	setFirebaseMetadataHeaders(c, uploadId, firebaseStorageUploadFinal)
	return c.JSON(metadata)
}

var GetPhoto fiber.Handler = func(c fiber.Ctx) error {
	decodedName, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid photo name")
	}
	name, err := parsePhotoObjectName(decodedName)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid photo name")
	}
	responseType := c.Query("alt")
	if responseType != "" && responseType != "media" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid alternate response type")
	}
	var userAuth *umodel.UserAuth
	if responseType == "" {
		userAuth, err = authenticatePhotoOwner(c, name.PublicUserId)
		if err != nil {
			return err
		}
	}

	metadata, err := opuser.FindUserPhotoImageMetadata(c, name.Name)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fiber.NewError(fiber.StatusNotFound, "photo not found")
		}
		mcontext.GetLogger(c).Error("failed to retrieve photo metadata", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	if metadata.UploadState != umodel.UserPhotoImageUploadFinalized {
		return fiber.NewError(fiber.StatusNotFound, "photo not found")
	}

	if responseType == "media" {
		if token := c.Query("token"); token == "" || token != metadata.DownloadTokens {
			return fiber.NewError(fiber.StatusForbidden, "invalid download token")
		}
		return sendPhotoImage(c, name, metadata)
	}
	if metadata.SUID != userAuth.ServerUserId || metadata.PublicUserId != userAuth.PublicUserId {
		return fiber.NewError(fiber.StatusForbidden, "photo owner mismatch")
	}

	uploadId, err := newFirebaseUploadID()
	if err != nil {
		mcontext.GetLogger(c).Error("failed to create metadata request ID", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	setFirebaseMetadataHeaders(c, uploadId, "")
	return c.JSON(metadata)
}

func authenticatePhotoOwner(c fiber.Ctx, publicUserId string) (*umodel.UserAuth, error) {
	parts := strings.Fields(c.Get(fiber.HeaderAuthorization))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Firebase") || parts[1] == "" {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid Firebase authorization")
	}
	_, claims, err := jwto.ParseRS256Token(parts[1])
	if err != nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid Firebase token")
	}

	issuer, issuerErr := claims.GetIssuer()
	audience, audienceErr := claims.GetAudience()
	subject, subjectErr := claims.GetSubject()
	userId, userIdOK := (*claims)["user_id"].(string)
	tokenPublicUserId, publicUserIdOK := (*claims)["puid"].(string)
	if issuerErr != nil || issuer != "https://securetoken.google.com/"+firebaseStorageProject ||
		audienceErr != nil || !containsString(audience, firebaseStorageProject) ||
		subjectErr != nil || subject == "" || subject != userId || !userIdOK ||
		!publicUserIdOK || tokenPublicUserId != publicUserId {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid Firebase token claims")
	}

	userAuth, err := opuser.GetUserAuthByFirebaseId(c, userId)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			mcontext.GetLogger(c).Error("failed to retrieve photo owner", zap.Error(err))
		}
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid Firebase user")
	}
	if userAuth.PublicUserId != publicUserId || userAuth.PublicUserId != tokenPublicUserId {
		return nil, fiber.NewError(fiber.StatusForbidden, "photo owner mismatch")
	}
	return userAuth, nil
}

func parsePhotoObjectName(name string) (*photoObjectName, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] != "Photo" || !isPublicUserId(parts[1]) || !isPhotoFilename(parts[2]) {
		return nil, errors.New("invalid photo object name")
	}
	return &photoObjectName{Name: name, PublicUserId: parts[1], Filename: parts[2]}, nil
}

func isPublicUserId(value string) bool {
	if len(value) != 8 {
		return false
	}
	for i := range len(value) {
		if (value[i] < 'A' || value[i] > 'Z') && (value[i] < '0' || value[i] > '9') {
			return false
		}
	}
	return true
}

func isPhotoFilename(value string) bool {
	// Keep room for the .jpg suffix on filesystems with a 255-byte segment limit.
	if value == "" || value == "." || value == ".." || len(value) > 251 {
		return false
	}
	for i := range len(value) {
		character := value[i]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' || character == '~' {
			continue
		}
		return false
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firebaseStorageUploadURL(name, uploadId string) string {
	query := url.Values{
		"name":            []string{name},
		"uploadType":      []string{"resumable"},
		"upload_id":       []string{uploadId},
		"upload_protocol": []string{"resumable"},
	}
	return "https://firebasestorage.googleapis.com/v0/b/" + firebaseStorageBucket + "/o?" + query.Encode()
}

func newFirebaseUploadID() (string, error) {
	buffer := make([]byte, 72)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "AJjja9" + base64.RawURLEncoding.EncodeToString(buffer), nil
}

func completePhotoMetadata(
	session *umodel.UserPhotoImageMetadata,
	filename string,
	body []byte,
	createdAt time.Time,
) *umodel.UserPhotoImageMetadata {
	md5Digest := md5.Sum(body)
	crc32Digest := crc32.Checksum(body, crc32.MakeTable(crc32.Castagnoli))
	crc32Bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crc32Bytes, crc32Digest)

	metadata := *session
	metadata.UploadState = umodel.UserPhotoImageUploadFinalized
	metadata.UploadStateUpdatedAt = createdAt
	metadata.UploadExpiresAt = nil
	metadata.Generation = strconv.FormatInt(createdAt.UnixMicro(), 10)
	metadata.Metageneration = "1"
	metadata.TimeCreated = createdAt.Format(firebaseStorageTimeFormat)
	metadata.Updated = metadata.TimeCreated
	metadata.StorageClass = "REGIONAL"
	metadata.Size = strconv.Itoa(len(body))
	metadata.MD5Hash = base64.StdEncoding.EncodeToString(md5Digest[:])
	metadata.ContentEncoding = "identity"
	metadata.ContentDisposition = "inline; filename*=utf-8''" + filename
	metadata.CRC32C = base64.StdEncoding.EncodeToString(crc32Bytes)
	metadata.ETag = base64.StdEncoding.EncodeToString(md5Digest[:8])
	metadata.DownloadTokens = uuid.NewString()
	return &metadata
}

func photoImagePath(name *photoObjectName) string {
	return filepath.Join(config.GetResourceDir(), "Photo", name.PublicUserId, name.Filename+".jpg")
}

func writePhotoImageAtomically(destination string, body []byte, modifiedAt time.Time) (retErr error) {
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("failed to create photo directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".photo-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary photo: %w", err)
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		if temporary != nil {
			retErr = errors.Join(retErr, temporary.Close())
		}
		if retErr != nil && !published {
			retErr = errors.Join(retErr, os.Remove(temporaryPath))
		}
	}()

	if _, err := temporary.Write(body); err != nil {
		return fmt.Errorf("failed to write temporary photo: %w", err)
	}
	if err := temporary.Chmod(0o640); err != nil {
		return fmt.Errorf("failed to set photo permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary photo: %w", err)
	}
	if err := temporary.Close(); err != nil {
		temporary = nil
		return fmt.Errorf("failed to close temporary photo: %w", err)
	}
	temporary = nil
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("failed to publish photo: %w", err)
	}
	published = true
	if err := os.Chtimes(destination, modifiedAt, modifiedAt); err != nil {
		return fmt.Errorf("failed to set photo timestamps: %w", err)
	}

	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("failed to open photo directory: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, directoryHandle.Close()) }()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("failed to sync photo directory: %w", err)
	}
	return nil
}

func releasePhotoUpload(c fiber.Ctx, session *umodel.UserPhotoImageMetadata) {
	now := time.Now().UTC()
	expiresAt := now.Add(photoUploadSessionDuration)
	released, err := opuser.ReleaseUserPhotoImageUpload(
		c,
		session.Name,
		session.SUID,
		session.UploadId,
		session.FinalizationId,
		now,
		expiresAt,
	)
	if err != nil {
		mcontext.GetLogger(c).Error("failed to release photo upload session", zap.Error(err))
	} else if !released {
		mcontext.GetLogger(c).Warn("photo upload session was no longer owned while releasing it")
	}
}

func samePublishedPhoto(current, expected *umodel.UserPhotoImageMetadata) bool {
	return current.UploadState == umodel.UserPhotoImageUploadFinalized &&
		current.ID == expected.ID && current.SUID == expected.SUID &&
		current.UploadId == expected.UploadId && current.FinalizationId == expected.FinalizationId &&
		current.Generation == expected.Generation && current.Size == expected.Size &&
		current.MD5Hash == expected.MD5Hash && current.CRC32C == expected.CRC32C &&
		current.DownloadTokens == expected.DownloadTokens
}

func setFirebaseMetadataHeaders(c fiber.Ctx, uploadId, status string) {
	c.Set(fiber.HeaderContentType, "application/json; charset=UTF-8")
	c.Set("X-GUploader-UploadID", uploadId)
	if status != "" {
		c.Set("X-Goog-Upload-Status", status)
	}
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Access-Control-Expose-Headers", "Content-Range, X-Firebase-Storage-XSRF")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set(fiber.HeaderCacheControl, "private, max-age=0")
	c.Set(fiber.HeaderExpires, time.Now().UTC().Format(http.TimeFormat))
}

func sendPhotoImage(c fiber.Ctx, name *photoObjectName, metadata *umodel.UserPhotoImageMetadata) error {
	md5Digest, err := base64.StdEncoding.DecodeString(metadata.MD5Hash)
	if err != nil || len(md5Digest) != md5.Size {
		mcontext.GetLogger(c).Error("invalid stored photo MD5 metadata", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	createdAt, err := time.Parse(firebaseStorageTimeFormat, metadata.TimeCreated)
	if err != nil {
		mcontext.GetLogger(c).Error("invalid stored photo creation time", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	c.Set(fiber.HeaderContentType, metadata.ContentType)
	requestId, err := newFirebaseUploadID()
	if err != nil {
		mcontext.GetLogger(c).Error("failed to create photo download request ID", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	c.Set("X-GUploader-UploadID", requestId)
	c.Set(fiber.HeaderExpires, time.Now().UTC().Format(http.TimeFormat))
	c.Set(fiber.HeaderCacheControl, "private, max-age=0")
	c.Set(fiber.HeaderLastModified, createdAt.Format(http.TimeFormat))
	c.Set(fiber.HeaderETag, `"`+hex.EncodeToString(md5Digest)+`"`)
	c.Set("x-goog-generation", metadata.Generation)
	c.Set("x-goog-metageneration", metadata.Metageneration)
	c.Set("x-goog-stored-content-encoding", metadata.ContentEncoding)
	c.Set("x-goog-stored-content-length", metadata.Size)
	c.Set("x-goog-meta-firebaseStorageDownloadTokens", metadata.DownloadTokens)
	c.Set(fiber.HeaderContentDisposition, metadata.ContentDisposition)
	c.Append("x-goog-hash", "crc32c="+metadata.CRC32C)
	c.Append("x-goog-hash", "md5="+metadata.MD5Hash)
	c.Set("x-goog-storage-class", metadata.StorageClass)
	return c.SendFile(photoImagePath(name), fiber.SendFile{ByteRange: true, CacheDuration: -1})
}
