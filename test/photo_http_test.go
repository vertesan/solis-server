package test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/server/http/game"
	"github.com/vertesan/solis-server/internal/utils/config"
	"github.com/vertesan/solis-server/internal/utils/jwto"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3/client"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestPhotoImageStorageScenario(t *testing.T) {
	setupTestUser(t)

	userAuth, err := opuser.GetUserAuthByServerId(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatalf("failed to retrieve configured test user: %v", err)
	}
	firebaseIdToken := getPhotoFirebaseIdToken(t, userAuth)
	authorization := "Firebase " + firebaseIdToken

	filename := "photo-test-non-uuid"
	objectName := "Photo/" + testConfig.TestUserPublicId + "/" + filename
	imagePath := filepath.Join(config.GetResourceDir(), "Photo", testConfig.TestUserPublicId, filename+".jpg")
	if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove a stale test photo image: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
			t.Errorf("failed to clean up test photo image: %v", err)
		}
	})
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("test photo path unexpectedly exists before upload: %v", err)
	}

	startURL := fmt.Sprintf("%s/v0/b/udc-idolypride/o", testConfig.HttpUrl)
	startHeaders := map[string]string{
		"Authorization":                     authorization,
		"X-Goog-Upload-Command":             "start",
		"X-Goog-Upload-Protocol":            "resumable",
		"X-Goog-Upload-Header-Content-Type": "image/jpeg",
	}
	unauthenticatedStart, err := testConfig.HttpClient.Post(startURL, client.Config{
		Param: map[string]string{
			"name":       objectName,
			"uploadType": "resumable",
		},
		Header: map[string]string{
			"X-Goog-Upload-Command":             "start",
			"X-Goog-Upload-Protocol":            "resumable",
			"X-Goog-Upload-Header-Content-Type": "image/jpeg",
		},
		Body: map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("unauthenticated photo upload request failed: %v", err)
	}
	if unauthenticatedStart.StatusCode() != 401 {
		t.Fatalf("unauthenticated photo upload returned status %d, want 401", unauthenticatedStart.StatusCode())
	}
	invalidNameResponse, err := testConfig.HttpClient.Post(startURL, client.Config{
		Param: map[string]string{
			"name":       "Photo/" + testConfig.TestUserPublicId + "/..",
			"uploadType": "resumable",
		},
		Header: startHeaders,
		Body:   map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("malformed photo upload request failed: %v", err)
	}
	if invalidNameResponse.StatusCode() != 400 {
		t.Fatalf("malformed photo name returned status %d, want 400: %s", invalidNameResponse.StatusCode(), invalidNameResponse.Body())
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("malformed photo request wrote the valid target path: %v", err)
	}

	wrongOwnerResponse, err := testConfig.HttpClient.Post(startURL, client.Config{
		Param: map[string]string{
			"name":       "Photo/ZZZZZZZZ/" + filename,
			"uploadType": "resumable",
		},
		Header: startHeaders,
		Body:   map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("wrong-owner photo upload request failed: %v", err)
	}
	if wrongOwnerResponse.StatusCode() != 401 && wrongOwnerResponse.StatusCode() != 403 {
		t.Fatalf("wrong-owner photo upload returned status %d, want 401 or 403", wrongOwnerResponse.StatusCode())
	}

	startResponse, err := testConfig.HttpClient.Post(startURL, client.Config{
		Param: map[string]string{
			"name":       objectName,
			"uploadType": "resumable",
		},
		Header: startHeaders,
		Body:   map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("photo upload start failed: %v", err)
	}
	if startResponse.StatusCode() != 200 {
		t.Fatalf("photo upload start returned status %d: %s", startResponse.StatusCode(), startResponse.Body())
	}
	uploadId := startResponse.Header("X-GUploader-UploadID")
	uploadURL := startResponse.Header("X-Goog-Upload-URL")
	if uploadId == "" || uploadURL == "" || startResponse.Header("X-Goog-Upload-Status") != "active" {
		t.Fatalf("photo upload start returned incomplete session headers: id=%q url=%q status=%q", uploadId, uploadURL, startResponse.Header("X-Goog-Upload-Status"))
	}
	if startResponse.Header("X-Goog-Upload-Control-URL") != uploadURL ||
		startResponse.Header("X-Goog-Upload-Chunk-Granularity") != "262144" {
		t.Fatalf("photo upload start returned incompatible resumable headers")
	}
	repeatedStart, err := testConfig.HttpClient.Post(startURL, client.Config{
		Param:  map[string]string{"name": objectName, "uploadType": "resumable"},
		Header: startHeaders,
		Body:   map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatalf("repeated photo upload start failed: %v", err)
	}
	if repeatedStart.StatusCode() != 200 || repeatedStart.Header("X-GUploader-UploadID") != uploadId {
		t.Fatalf("repeated photo upload start invalidated the live session")
	}

	objectURL := fmt.Sprintf("%s/v0/b/udc-idolypride/o/%s", testConfig.HttpUrl, url.PathEscape(objectName))
	beforeFinalize, err := testConfig.HttpClient.Get(objectURL, client.Config{
		Header:                 map[string]string{"Authorization": authorization},
		DisablePathNormalizing: true,
	})
	if err != nil {
		t.Fatalf("pre-finalize photo metadata request failed: %v", err)
	}
	if beforeFinalize.StatusCode() != 404 {
		t.Fatalf("active upload was visible before finalization: status %d", beforeFinalize.StatusCode())
	}

	parsedUploadURL, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("photo upload URL is invalid: %v", err)
	}
	if parsedUploadURL.Scheme != "https" || parsedUploadURL.Host != "firebasestorage.googleapis.com" || parsedUploadURL.Query().Get("upload_id") != uploadId {
		t.Fatalf("photo upload URL is not Firebase-compatible: %s", uploadURL)
	}
	finalizeURL := testConfig.HttpUrl + parsedUploadURL.Path + "?" + parsedUploadURL.RawQuery
	finalizeHeaders := map[string]string{
		"Authorization":          authorization,
		"Content-Type":           "application/x-www-form-urlencoded",
		"X-Goog-Upload-Command":  "upload, finalize",
		"X-Goog-Upload-Protocol": "resumable",
		"X-Goog-Upload-Offset":   "0",
	}
	wrongUploadURL, err := url.Parse(finalizeURL)
	if err != nil {
		t.Fatalf("failed to parse local photo finalization URL: %v", err)
	}
	wrongUploadQuery := wrongUploadURL.Query()
	wrongUploadQuery.Set("upload_id", "wrong-upload-id")
	wrongUploadURL.RawQuery = wrongUploadQuery.Encode()
	wrongUploadResponse, err := testConfig.HttpClient.Post(wrongUploadURL.String(), client.Config{
		Header: finalizeHeaders,
		Body:   makeTestJPEG(t),
	})
	if err != nil {
		t.Fatalf("wrong-session photo finalization request failed: %v", err)
	}
	if wrongUploadResponse.StatusCode() != 403 {
		t.Fatalf("wrong-session photo finalization returned status %d, want 403", wrongUploadResponse.StatusCode())
	}

	invalidJPEGResponse, err := testConfig.HttpClient.Post(finalizeURL, client.Config{
		Header: finalizeHeaders,
		Body:   []byte("not a JPEG"),
	})
	if err != nil {
		t.Fatalf("invalid JPEG finalization request failed: %v", err)
	}
	if invalidJPEGResponse.StatusCode() != 400 {
		t.Fatalf("invalid JPEG finalization returned status %d, want 400", invalidJPEGResponse.StatusCode())
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("invalid JPEG finalization created a file: %v", err)
	}

	jpegBody := makeTestJPEG(t)
	finalizeResponse, err := testConfig.HttpClient.Post(finalizeURL, client.Config{
		Header: finalizeHeaders,
		Body:   jpegBody,
	})
	if err != nil {
		t.Fatalf("photo upload finalization failed: %v", err)
	}
	if finalizeResponse.StatusCode() != 200 {
		t.Fatalf("photo upload finalization returned status %d: %s", finalizeResponse.StatusCode(), finalizeResponse.Body())
	}
	if finalizeResponse.Header("X-Goog-Upload-Status") != "final" || finalizeResponse.Header("X-GUploader-UploadID") != uploadId {
		t.Fatalf("photo finalization returned incompatible upload headers")
	}
	metadata := decodePhotoMetadata(t, finalizeResponse.Body())
	assertPhotoMetadata(t, metadata, objectName, filename, jpegBody)

	storedBody, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("failed to read stored photo: %v", err)
	}
	if !bytes.Equal(storedBody, jpegBody) {
		t.Fatalf("stored photo differs from uploaded JPEG")
	}
	storedMetadata, err := opuser.FindUserPhotoImageMetadata(context.Background(), objectName)
	if err != nil {
		t.Fatalf("failed to retrieve stored photo metadata: %v", err)
	}
	if storedMetadata.SUID != testConfig.TestUserSuid || storedMetadata.UploadId != uploadId ||
		storedMetadata.UploadState != umodel.UserPhotoImageUploadFinalized || storedMetadata.DownloadTokens != metadata.DownloadTokens {
		t.Fatalf("stored photo session metadata is inconsistent: %+v", storedMetadata)
	}

	repeatedFinalize, err := testConfig.HttpClient.Post(finalizeURL, client.Config{Header: finalizeHeaders, Body: jpegBody})
	if err != nil {
		t.Fatalf("repeated photo finalization failed: %v", err)
	}
	if repeatedFinalize.StatusCode() != 200 {
		t.Fatalf("repeated photo finalization returned status %d: %s", repeatedFinalize.StatusCode(), repeatedFinalize.Body())
	}
	repeatedMetadata := decodePhotoMetadata(t, repeatedFinalize.Body())
	if repeatedMetadata.DownloadTokens != metadata.DownloadTokens || repeatedMetadata.Generation != metadata.Generation {
		t.Fatalf("repeated photo finalization changed stable metadata")
	}

	metadataResponse, err := testConfig.HttpClient.Get(objectURL, client.Config{
		Header:                 map[string]string{"Authorization": authorization},
		DisablePathNormalizing: true,
	})
	if err != nil {
		t.Fatalf("photo metadata GET failed: %v", err)
	}
	if metadataResponse.StatusCode() != 200 {
		t.Fatalf("photo metadata GET returned status %d: %s", metadataResponse.StatusCode(), metadataResponse.Body())
	}
	getMetadata := decodePhotoMetadata(t, metadataResponse.Body())
	if getMetadata.DownloadTokens != metadata.DownloadTokens || getMetadata.MD5Hash != metadata.MD5Hash {
		t.Fatalf("photo metadata GET differs from finalization metadata")
	}
	unauthenticatedMetadata, err := testConfig.HttpClient.Get(objectURL, client.Config{DisablePathNormalizing: true})
	if err != nil {
		t.Fatalf("unauthenticated photo metadata request failed: %v", err)
	}
	if unauthenticatedMetadata.StatusCode() != 401 {
		t.Fatalf("unauthenticated photo metadata returned status %d, want 401", unauthenticatedMetadata.StatusCode())
	}

	wrongTokenResponse, err := testConfig.HttpClient.Get(objectURL, client.Config{
		Param:                  map[string]string{"alt": "media", "token": uuid.NewString()},
		DisablePathNormalizing: true,
	})
	if err != nil {
		t.Fatalf("wrong-token photo request failed: %v", err)
	}
	if wrongTokenResponse.StatusCode() != 403 {
		t.Fatalf("wrong-token photo request returned status %d, want 403", wrongTokenResponse.StatusCode())
	}

	mediaResponse, err := testConfig.HttpClient.Get(objectURL, client.Config{
		Param:                  map[string]string{"alt": "media", "token": metadata.DownloadTokens},
		DisablePathNormalizing: true,
	})
	if err != nil {
		t.Fatalf("photo media GET failed: %v", err)
	}
	if mediaResponse.StatusCode() != 200 {
		t.Fatalf("photo media GET returned status %d: %s", mediaResponse.StatusCode(), mediaResponse.Body())
	}
	if !bytes.Equal(mediaResponse.Body(), jpegBody) {
		t.Fatalf("downloaded photo differs from uploaded JPEG")
	}
	md5Digest := md5.Sum(jpegBody)
	if mediaResponse.Header("Content-Type") != "image/jpeg" ||
		mediaResponse.Header("Accept-Ranges") != "bytes" ||
		mediaResponse.Header("x-goog-generation") != metadata.Generation ||
		mediaResponse.Header("x-goog-meta-firebaseStorageDownloadTokens") != metadata.DownloadTokens ||
		mediaResponse.Header("ETag") != `"`+hex.EncodeToString(md5Digest[:])+`"` ||
		mediaResponse.Header("Content-Disposition") != metadata.ContentDisposition ||
		!strings.Contains(mediaResponse.Header("x-goog-hash"), "crc32c="+metadata.CRC32C) ||
		!strings.Contains(mediaResponse.Header("x-goog-hash"), "md5="+metadata.MD5Hash) {
		t.Fatalf("photo media GET returned incompatible Firebase metadata headers")
	}
	rangeResponse, err := testConfig.HttpClient.Get(objectURL, client.Config{
		Param:                  map[string]string{"alt": "media", "token": metadata.DownloadTokens},
		Header:                 map[string]string{"Range": "bytes=0-9"},
		DisablePathNormalizing: true,
	})
	if err != nil {
		t.Fatalf("photo byte-range GET failed: %v", err)
	}
	if rangeResponse.StatusCode() != 206 || !bytes.Equal(rangeResponse.Body(), jpegBody[:10]) ||
		rangeResponse.Header("Content-Range") != fmt.Sprintf("bytes 0-9/%d", len(jpegBody)) {
		t.Fatalf("photo byte-range GET returned an incompatible response")
	}

	if err := deleteConfiguredTestUser(); err != nil {
		t.Fatalf("failed to delete configured test user after photo scenario: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(imagePath)); !os.IsNotExist(err) {
		t.Fatalf("deleting the test user did not remove its photo directory: %v", err)
	}
	if _, err := opuser.FindUserPhotoImageMetadata(context.Background(), objectName); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("deleting the test user did not remove photo metadata: %v", err)
	}
}

func TestPhotoImageReplacementScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	userAuth, err := opuser.GetUserAuthByServerId(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	authorization := "Firebase " + getPhotoFirebaseIdToken(t, userAuth)
	filename := "photo-replacement-" + uuid.NewString()
	name := "Photo/" + testConfig.TestUserPublicId + "/" + filename
	objectURL := testConfig.HttpUrl + "/v0/b/udc-idolypride/o/" + url.PathEscape(name)
	imagePath := filepath.Join(config.GetResourceDir(), "Photo", testConfig.TestUserPublicId, filename+".jpg")
	start := func() (string, string) {
		t.Helper()
		response, err := testConfig.HttpClient.Post(testConfig.HttpUrl+"/v0/b/udc-idolypride/o", client.Config{
			Param: map[string]string{"name": name, "uploadType": "resumable"},
			Header: map[string]string{
				"Authorization": authorization, "X-Goog-Upload-Command": "start",
				"X-Goog-Upload-Protocol": "resumable", "X-Goog-Upload-Header-Content-Type": "image/jpeg",
			},
			Body: map[string]string{"contentType": "image/jpeg"},
		})
		if err != nil || response.StatusCode() != 200 {
			t.Fatalf("replacement upload start failed: response=%v error=%v", response, err)
		}
		uploadURL, err := url.Parse(response.Header("X-Goog-Upload-URL"))
		if err != nil {
			t.Fatal(err)
		}
		return testConfig.HttpUrl + uploadURL.RequestURI(), response.Header("X-GUploader-UploadID")
	}
	finalizeHeaders := map[string]string{
		"Authorization": authorization, "X-Goog-Upload-Command": "upload, finalize",
		"X-Goog-Upload-Protocol": "resumable", "X-Goog-Upload-Offset": "0",
		"Content-Type": "application/x-www-form-urlencoded",
	}
	assertPublished := func(expected *umodel.UserPhotoImageMetadata, body []byte) {
		t.Helper()
		response, err := testConfig.HttpClient.Get(objectURL, client.Config{
			Header: map[string]string{"Authorization": authorization}, DisablePathNormalizing: true,
		})
		if err != nil || response.StatusCode() != 200 {
			t.Fatalf("published metadata became unavailable: response=%v error=%v", response, err)
		}
		metadata := decodePhotoMetadata(t, response.Body())
		if metadata.Generation != expected.Generation || metadata.DownloadTokens != expected.DownloadTokens || metadata.MD5Hash != expected.MD5Hash {
			t.Fatalf("published metadata changed before a successful finalize: got %+v, want %+v", metadata, expected)
		}
		response, err = testConfig.HttpClient.Get(objectURL, client.Config{
			Param: map[string]string{"alt": "media", "token": expected.DownloadTokens}, DisablePathNormalizing: true,
		})
		if err != nil || response.StatusCode() != 200 || !bytes.Equal(response.Body(), body) ||
			response.Header("x-goog-generation") != expected.Generation {
			t.Fatalf("published photo/metadata mismatch: response=%v error=%v", response, err)
		}
		stored, err := os.ReadFile(imagePath)
		if err != nil || !bytes.Equal(stored, body) {
			t.Fatalf("published file changed unexpectedly: %v", err)
		}
		metadataList, err := opuser.FindFinalizedUserPhotoImageMetadataByNames(ctx, testConfig.TestUserSuid, []string{name})
		if err != nil || len(metadataList) != 1 || metadataList[0].Generation != expected.Generation {
			t.Fatalf("published photo missing from gRPC image lookup: metadata=%v error=%v", metadataList, err)
		}
	}

	originalBody := makeTestJPEG(t)
	replacementImage := image.NewRGBA(image.Rect(0, 0, 4, 3))
	replacementImage.Set(0, 0, color.RGBA{G: 0xff, A: 0xff})
	var replacementBuffer bytes.Buffer
	if err := jpeg.Encode(&replacementBuffer, replacementImage, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	replacementBody := replacementBuffer.Bytes()
	// JPEG decoders ignore padding after EOI. Use different images of equal
	// length so a stale response cannot hide behind a changed Content-Length.
	size := max(len(originalBody), len(replacementBody))
	originalBody = append(originalBody, make([]byte, size-len(originalBody))...)
	replacementBody = append(replacementBody, make([]byte, size-len(replacementBody))...)
	originalURL, originalID := start()
	response, err := testConfig.HttpClient.Post(originalURL, client.Config{Header: finalizeHeaders, Body: originalBody})
	if err != nil || response.StatusCode() != 200 {
		t.Fatalf("initial image finalize failed: response=%v error=%v", response, err)
	}
	original := decodePhotoMetadata(t, response.Body())
	assertPublished(original, originalBody)

	replacementURL, replacementID := start()
	if replacementID == originalID {
		t.Fatal("replacement reused the finalized upload session")
	}
	if _, repeatedID := start(); repeatedID != replacementID {
		t.Fatal("duplicate start invalidated the active replacement")
	}
	assertPublished(original, originalBody)
	response, err = testConfig.HttpClient.Post(replacementURL, client.Config{Header: finalizeHeaders, Body: []byte("not a JPEG")})
	if err != nil || response.StatusCode() != 400 {
		t.Fatalf("invalid replacement was not rejected: response=%v error=%v", response, err)
	}
	assertPublished(original, originalBody)
	response, err = testConfig.HttpClient.Post(originalURL, client.Config{Header: finalizeHeaders, Body: originalBody})
	if err != nil || response.StatusCode() != 403 {
		t.Fatalf("old upload session can overwrite replacement: response=%v error=%v", response, err)
	}
	assertPublished(original, originalBody)

	response, err = testConfig.HttpClient.Post(replacementURL, client.Config{Header: finalizeHeaders, Body: replacementBody})
	if err != nil || response.StatusCode() != 200 {
		t.Fatalf("replacement finalize failed: response=%v error=%v", response, err)
	}
	replacement := decodePhotoMetadata(t, response.Body())
	assertPhotoMetadata(t, replacement, name, filename, replacementBody)
	if replacement.Generation == original.Generation || replacement.MD5Hash == original.MD5Hash ||
		replacement.ETag == original.ETag || replacement.DownloadTokens == original.DownloadTokens {
		t.Fatalf("replacement did not revise metadata and its Firebase download URL: original=%+v replacement=%+v", original, replacement)
	}
	assertPublished(replacement, replacementBody)
	response, err = testConfig.HttpClient.Get(objectURL, client.Config{
		Param: map[string]string{"alt": "media", "token": original.DownloadTokens}, DisablePathNormalizing: true,
	})
	if err != nil || response.StatusCode() != 403 {
		t.Fatalf("old download token survived replacement: response=%v error=%v", response, err)
	}

	// HTTP dates have only whole-second precision. Model a same-second
	// replacement without a timing-dependent sleep or an upload race.
	modifiedAt, err := time.Parse("2006-01-02T15:04:05.000Z", original.TimeCreated)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(imagePath, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
	originalDigest := md5.Sum(originalBody)
	replacementDigest := md5.Sum(replacementBody)
	oldETag := `"` + hex.EncodeToString(originalDigest[:]) + `"`
	newETag := `"` + hex.EncodeToString(replacementDigest[:]) + `"`
	for _, headers := range []map[string]string{
		{"If-None-Match": oldETag, "If-Modified-Since": modifiedAt.Format(http.TimeFormat)},
		{"If-Modified-Since": modifiedAt.Format(http.TimeFormat)},
	} {
		response, err := testConfig.HttpClient.Get(objectURL, client.Config{
			Param:  map[string]string{"alt": "media", "token": replacement.DownloadTokens},
			Header: headers, DisablePathNormalizing: true,
		})
		if err != nil || response.StatusCode() != 200 || !bytes.Equal(response.Body(), replacementBody) || response.Header("ETag") != newETag {
			t.Fatalf("old cache validators hid the replacement: response=%v error=%v", response, err)
		}
	}
	response, err = testConfig.HttpClient.Get(objectURL, client.Config{
		Param:  map[string]string{"alt": "media", "token": replacement.DownloadTokens},
		Header: map[string]string{"If-None-Match": newETag}, DisablePathNormalizing: true,
	})
	if err != nil || response.StatusCode() != 304 || len(response.Body()) != 0 || response.Header("ETag") != newETag {
		t.Fatalf("current photo ETag did not revalidate: response=%v error=%v", response, err)
	}
	response, err = testConfig.HttpClient.Get(objectURL, client.Config{
		Param:                  map[string]string{"alt": "media", "token": replacement.DownloadTokens},
		Header:                 map[string]string{"Range": "bytes=0-199", "If-None-Match": oldETag, "If-Modified-Since": modifiedAt.Format(http.TimeFormat)},
		DisablePathNormalizing: true,
	})
	if err != nil || response.StatusCode() != 206 || !bytes.Equal(response.Body(), replacementBody[:200]) ||
		response.Header("Content-Range") != fmt.Sprintf("bytes 0-199/%d", len(replacementBody)) {
		t.Fatalf("replacement range download was stale: response=%v error=%v", response, err)
	}
	response, err = testConfig.HttpClient.Post(replacementURL, client.Config{Header: finalizeHeaders, Body: originalBody})
	if err != nil || response.StatusCode() != 200 || decodePhotoMetadata(t, response.Body()).Generation != replacement.Generation ||
		decodePhotoMetadata(t, response.Body()).DownloadTokens != replacement.DownloadTokens {
		t.Fatalf("replacement finalize retry is not idempotent: response=%v error=%v", response, err)
	}
	assertPublished(replacement, replacementBody)

	// Two finalizers for one session must publish one generation and agree on
	// its metadata, even when their submitted bytes differ.
	concurrentURL, _ := start()
	type finalizeResult struct {
		status int
		body   []byte
		err    error
	}
	results := make(chan finalizeResult, 2)
	for _, body := range [][]byte{originalBody, replacementBody} {
		go func() {
			response, err := testConfig.HttpClient.Post(concurrentURL, client.Config{Header: finalizeHeaders, Body: body})
			if err != nil {
				results <- finalizeResult{err: err}
				return
			}
			results <- finalizeResult{status: response.StatusCode(), body: append([]byte(nil), response.Body()...)}
		}()
	}
	var concurrentMetadata *umodel.UserPhotoImageMetadata
	for range 2 {
		result := <-results
		if result.err != nil || result.status != 200 {
			t.Fatalf("concurrent finalization failed: status=%d error=%v body=%s", result.status, result.err, result.body)
		}
		metadata := decodePhotoMetadata(t, result.body)
		if concurrentMetadata != nil && (metadata.Generation != concurrentMetadata.Generation || metadata.MD5Hash != concurrentMetadata.MD5Hash || metadata.DownloadTokens != concurrentMetadata.DownloadTokens) {
			t.Fatal("concurrent finalizers published conflicting generations")
		}
		concurrentMetadata = metadata
	}
	if concurrentMetadata.MD5Hash == original.MD5Hash {
		replacementBody = originalBody
	} else if concurrentMetadata.MD5Hash != replacement.MD5Hash {
		t.Fatal("concurrent finalization published bytes from neither request")
	}
	replacement = concurrentMetadata
	assertPublished(replacement, replacementBody)

	// Expired replacement sessions must not delete the last published image.
	expiredAt := time.Now().Add(-25 * time.Hour)
	expiredSession := &umodel.UserPhotoImageMetadata{
		ID: name, SUID: testConfig.TestUserSuid, PublicUserId: testConfig.TestUserPublicId,
		Name: name, Bucket: "udc-idolypride", ContentType: "image/jpeg", UploadId: uuid.NewString(),
		UploadState: umodel.UserPhotoImageUploadActive, UploadStateUpdatedAt: expiredAt, UploadExpiresAt: &expiredAt,
	}
	if started, err := opuser.StartUserPhotoImageUpload(ctx, expiredSession, time.Now().Add(-time.Hour)); err != nil || !started {
		t.Fatalf("failed to prepare expired replacement: started=%v error=%v", started, err)
	}
	if err := opuser.DeleteExpiredUserPhotoImageUploads(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	assertPublished(replacement, replacementBody)
	if _, nextID := start(); nextID == expiredSession.UploadId || nextID == replacementID {
		t.Fatal("expired replacement session was not renewed")
	}
	assertPublished(replacement, replacementBody)
}

func getPhotoFirebaseIdToken(t *testing.T, userAuth *umodel.UserAuth) string {
	t.Helper()
	customToken, err := jwto.CreateToken(jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"aud": "https://identitytoolkit.googleapis.com/google.identity.identitytoolkit.v1.IdentityToolkit",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"sub": "firebase-adminsdk-hvn46@solis-firebase.iam.gserviceaccount.com",
		"uid": userAuth.FirebaseId,
	}))
	if err != nil {
		t.Fatalf("failed to create Firebase custom token: %v", err)
	}
	response, err := testConfig.HttpClient.Post(
		fmt.Sprintf("%s/identitytoolkit/v3/relyingparty/verifyCustomToken", testConfig.HttpUrl),
		client.Config{
			Param:  map[string]string{"key": testConfig.FirebaseProductKey},
			Header: map[string]string{"X-Android-Package": "game.qualiarts.idolypride"},
			Body: &game.VerifyCustomTokenRequest{
				Token:             customToken,
				ReturnSecureToken: true,
			},
		},
	)
	if err != nil {
		t.Fatalf("Firebase custom-token verification failed: %v", err)
	}
	if response.StatusCode() != 200 {
		t.Fatalf("Firebase custom-token verification returned status %d: %s", response.StatusCode(), response.Body())
	}
	verifyResponse := &game.VerifyCustomTokenResponse{}
	if err := json.Unmarshal(response.Body(), verifyResponse); err != nil {
		t.Fatalf("failed to decode Firebase token response: %v", err)
	}
	if verifyResponse.IdToken == "" {
		t.Fatal("Firebase token response omitted idToken")
	}
	return verifyResponse.IdToken
}

func makeTestJPEG(t *testing.T) []byte {
	t.Helper()
	photo := image.NewRGBA(image.Rect(0, 0, 3, 2))
	photo.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	photo.Set(1, 0, color.RGBA{G: 0xff, A: 0xff})
	photo.Set(2, 0, color.RGBA{B: 0xff, A: 0xff})
	var body bytes.Buffer
	if err := jpeg.Encode(&body, photo, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("failed to create test JPEG: %v", err)
	}
	return body.Bytes()
}

func decodePhotoMetadata(t *testing.T, body []byte) *umodel.UserPhotoImageMetadata {
	t.Helper()
	metadata := &umodel.UserPhotoImageMetadata{}
	if err := json.Unmarshal(body, metadata); err != nil {
		t.Fatalf("failed to decode photo metadata: %v", err)
	}
	return metadata
}

func assertPhotoMetadata(t *testing.T, metadata *umodel.UserPhotoImageMetadata, objectName, filename string, body []byte) {
	t.Helper()
	md5Digest := md5.Sum(body)
	crc32Digest := crc32.Checksum(body, crc32.MakeTable(crc32.Castagnoli))
	crc32Bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crc32Bytes, crc32Digest)
	if metadata.Name != objectName || metadata.Bucket != "udc-idolypride" || metadata.ContentType != "image/jpeg" ||
		metadata.Metageneration != "1" || metadata.StorageClass != "REGIONAL" || metadata.ContentEncoding != "identity" ||
		metadata.Size != strconv.Itoa(len(body)) || metadata.MD5Hash != base64.StdEncoding.EncodeToString(md5Digest[:]) ||
		metadata.CRC32C != base64.StdEncoding.EncodeToString(crc32Bytes) || metadata.DownloadTokens == "" ||
		metadata.ContentDisposition != "inline; filename*=utf-8''"+filename || metadata.Generation == "" ||
		metadata.TimeCreated == "" || metadata.Updated != metadata.TimeCreated || metadata.ETag == "" {
		t.Fatalf("photo metadata is incompatible with uploaded image: %+v", metadata)
	}
}
