package test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3/client"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/utils/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestShowcaseThumbnailReplacementScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	showcases := papi.NewShowcaseClient(testConfig.GrpcConn)
	top := showcaseTop(t, showcases)
	var number int32
	for _, info := range top.GetShowcaseInfos() {
		if info.GetIsUnlocked() {
			number = info.GetNumber()
			break
		}
	}
	var frameID string
	for _, info := range top.GetFrameInfos() {
		if info.GetIsUnlocked() {
			frameID = info.GetId()
			break
		}
	}
	if number == 0 || frameID == "" {
		t.Fatal("new user has no usable showcase/frame")
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || len(user.GetNextPhotoImageIds()) == 0 {
		t.Fatalf("load reserved thumbnail: %v", err)
	}
	assetID := user.GetNextPhotoImageIds()[0]
	initialShowcase, err := opuser.FindUserShowcase(ctx, testConfig.TestUserSuid, number)
	if err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.InvalidArgument, showcases.Save, &papi.ShowcaseSaveRequest{
		Number: number, FrameId: frameID, LayoutInfo: &papi.ShowcaseLayoutInfo{},
	})
	afterRejectedSave, err := opuser.FindUserShowcase(ctx, testConfig.TestUserSuid, number)
	if err != nil || !proto.Equal(initialShowcase, afterRejectedSave) {
		t.Fatalf("a first save without a thumbnail changed the initial showcase: %v", err)
	}
	imagePath := filepath.Join(config.GetResourceDir(), "Photo", testConfig.TestUserPublicId, assetID+".jpg")
	t.Cleanup(func() {
		if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove configured user's test thumbnail: %v", err)
		}
	})
	auth, err := opuser.GetUserAuthByServerId(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	authorization := "Firebase " + getPhotoFirebaseIdToken(t, auth)
	profileCall(t, showcases.CheckThumbnailShooting, &papi.ShowcaseCheckThumbnailShootingRequest{Number: number})
	first := uploadShowcaseThumbnail(t, authorization, assetID, makeTestJPEG(t))
	profileCall(t, showcases.Save, &papi.ShowcaseSaveRequest{
		Number: number, FrameId: frameID, AssetId: assetID, LayoutInfo: &papi.ShowcaseLayoutInfo{},
	})
	userAfterFirst, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || slices.Contains(userAfterFirst.GetNextPhotoImageIds(), assetID) {
		t.Fatalf("first save did not consume its thumbnail reservation: %v", err)
	}

	// Add a real master-defined toy, then upload under the SAME thumbnail name.
	toys, err := opmaster.FindAllShowcaseToys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var toyID string
	var face int32
	for _, toy := range toys {
		if toy.GetCategoryId() == "toy-category-figure" {
			continue // The layout-only addition below uses a repeatable good.
		}
		if slices.Contains(toy.GetPutTypes(), penum.ShowcaseToyPutType_ShowcaseToyPutType_Floor) {
			toyID, face = toy.GetId(), 0
			break
		}
		if slices.Contains(toy.GetPutTypes(), penum.ShowcaseToyPutType_ShowcaseToyPutType_Wall) {
			toyID, face = toy.GetId(), 1
			break
		}
	}
	if toyID == "" {
		t.Fatal("no usable master toy")
	}
	if err := opuser.UpsertUserShowcaseToy(ctx, testConfig.TestUserSuid, &ptransaction.UserShowcaseToy{ShowcaseToyId: toyID, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	layout := &papi.ShowcaseLayoutInfo{
		ShowcaseToyAssetInfos:       []*papi.ShowcaseToyAssetInfo{{UniqueNumber: 1, ShowcaseToyId: toyID, Count: 1}},
		ShowcaseToyArrangementInfos: []*papi.ShowcaseToyArrangementInfo{{UniqueNumber: 1, ToyAssetInfoUniqueNumber: 1, ShowcaseFaceNumber: face, PositionX: 0.25, PositionY: 0.5}},
	}
	var encoded bytes.Buffer
	newImage := image.NewRGBA(image.Rect(0, 0, 4, 3))
	newImage.Set(0, 0, color.RGBA{G: 255, A: 255})
	if err := jpeg.Encode(&encoded, newImage, nil); err != nil {
		t.Fatal(err)
	}
	profileCall(t, showcases.CheckThumbnailShooting, &papi.ShowcaseCheckThumbnailShootingRequest{Number: number})
	second := uploadShowcaseThumbnail(t, authorization, assetID, encoded.Bytes())
	if second.Generation == first.Generation || second.MD5Hash == first.MD5Hash || second.DownloadTokens == first.DownloadTokens {
		t.Fatal("replacement did not publish a new image revision with a fresh download URL")
	}
	profileCall(t, showcases.Save, &papi.ShowcaseSaveRequest{Number: number, FrameId: frameID, AssetId: assetID, LayoutInfo: layout})
	userAfterSecond, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || !slices.Equal(userAfterFirst.GetNextPhotoImageIds(), userAfterSecond.GetNextPhotoImageIds()) {
		t.Fatalf("reusing the thumbnail consumed another reservation: %v", err)
	}
	stored, err := opuser.FindUserShowcaseLayout(ctx, testConfig.TestUserSuid, number)
	if err != nil || !proto.Equal(stored, layout) {
		t.Fatalf("second save did not persist the changed goods layout: %v", err)
	}
	objectName := "Photo/" + testConfig.TestUserPublicId + "/" + assetID
	media, err := testConfig.HttpClient.Get(testConfig.HttpUrl+"/v0/b/udc-idolypride/o/"+url.PathEscape(objectName), client.Config{
		Param: map[string]string{"alt": "media", "token": second.DownloadTokens}, DisablePathNormalizing: true,
	})
	if err != nil || media.StatusCode() != 200 || !bytes.Equal(media.Body(), encoded.Bytes()) {
		t.Fatalf("new thumbnail URL did not download the replacement image: %v", err)
	}
	profileReject(t, codes.FailedPrecondition, showcases.Save, &papi.ShowcaseSaveRequest{
		Number: number, FrameId: frameID, AssetId: "unreserved-other-photo", LayoutInfo: layout,
	})
	showcase, err := opuser.FindUserShowcase(ctx, testConfig.TestUserSuid, number)
	if err != nil || showcase.GetThumbnailAssetId() != assetID {
		t.Fatalf("failed save changed the current thumbnail: %v", err)
	}

	// The official layout-only overwrite sends assetId:"", unlike the explicit
	// thumbnail replacement above. It must retain both the image and case details.
	detail := profileCall(t, showcases.SaveDetail, &papi.ShowcaseSaveDetailRequest{
		Number: number, Name: "保存テスト", Comment: "-", IsOpen: false,
	})
	expectedShowcase := proto.Clone(detail.GetCommonResponse().GetUpdatedData().GetShowcases()[0]).(*ptransaction.UserShowcase)
	layout.ShowcaseToyArrangementInfos[0].PositionX = -0.1
	layout.ShowcaseToyArrangementInfos[0].Rotation = 15
	for iteration := range 2 {
		if iteration == 1 {
			// Cover adding another copy as well as moving an existing item.
			if err := opuser.UpsertUserShowcaseToy(ctx, testConfig.TestUserSuid, &ptransaction.UserShowcaseToy{ShowcaseToyId: toyID, Amount: 2}); err != nil {
				t.Fatal(err)
			}
			layout.ShowcaseToyAssetInfos[0].Count = 2
			layout.ShowcaseToyArrangementInfos = append(layout.ShowcaseToyArrangementInfos, &papi.ShowcaseToyArrangementInfo{
				UniqueNumber: 2, ToyAssetInfoUniqueNumber: 1, ShowcaseFaceNumber: face, PositionX: 0.2, PositionY: 0.1,
			})
		}
		overwrite := profileCall(t, showcases.Save, &papi.ShowcaseSaveRequest{Number: number, FrameId: frameID, LayoutInfo: layout})
		updated := overwrite.GetCommonResponse().GetUpdatedData()
		if len(updated.GetShowcases()) != 1 || !proto.Equal(updated.GetShowcases()[0], expectedShowcase) || updated.GetUser() != nil {
			t.Fatalf("layout-only overwrite changed thumbnail/details or consumed a reservation: %v", overwrite)
		}
		stored, err = opuser.FindUserShowcaseLayout(ctx, testConfig.TestUserSuid, number)
		if err != nil || !proto.Equal(stored, layout) {
			t.Fatalf("layout-only overwrite did not persist the new positions/goods: %v", err)
		}
		storedShowcase, err := opuser.FindUserShowcase(ctx, testConfig.TestUserSuid, number)
		if err != nil || !proto.Equal(storedShowcase, expectedShowcase) {
			t.Fatalf("layout-only overwrite failed to preserve the saved case metadata: %v", err)
		}
		currentUser, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
		if err != nil || !slices.Equal(currentUser.GetNextPhotoImageIds(), userAfterSecond.GetNextPhotoImageIds()) {
			t.Fatalf("layout-only overwrite changed photo reservations: %v", err)
		}
		metadata, err := opuser.FindUserPhotoImageMetadata(ctx, objectName)
		if err != nil || metadata.Generation != second.Generation || metadata.DownloadTokens != second.DownloadTokens {
			t.Fatalf("layout-only overwrite changed the published image: %v", err)
		}
	}
}

func TestShowcaseOriginalAndThumbnailReplacementScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	showcases := papi.NewShowcaseClient(testConfig.GrpcConn)
	photos := papi.NewPhotoClient(testConfig.GrpcConn)
	top := showcaseTop(t, showcases)
	var number int32
	var frameID string
	for _, info := range top.GetShowcaseInfos() {
		if info.GetIsUnlocked() {
			number = info.GetNumber()
			break
		}
	}
	for _, info := range top.GetFrameInfos() {
		if info.GetIsUnlocked() {
			frameID = info.GetId()
			break
		}
	}
	if number == 0 || frameID == "" {
		t.Fatal("no usable showcase/frame for paired upload")
	}
	user, err := opuser.FindUser(ctx, testConfig.TestUserSuid)
	if err != nil || len(user.GetNextPhotoImageIds()) == 0 {
		t.Fatalf("load reserved photo ID: %v", err)
	}
	assetID := user.GetNextPhotoImageIds()[0]
	for _, suffix := range []string{"", "_t"} {
		path := filepath.Join(config.GetResourceDir(), "Photo", testConfig.TestUserPublicId, assetID+suffix+".jpg")
		t.Cleanup(func() {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Errorf("clean up paired test image: %v", err)
			}
		})
	}
	auth, err := opuser.GetUserAuthByServerId(ctx, testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	authorization := "Firebase " + getPhotoFirebaseIdToken(t, auth)
	encode := func(width int, fill color.RGBA) []byte {
		t.Helper()
		bitmap := image.NewRGBA(image.Rect(0, 0, width, 2))
		for x := range width {
			for y := range 2 {
				bitmap.Set(x, y, fill)
			}
		}
		var data bytes.Buffer
		if err := jpeg.Encode(&data, bitmap, nil); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	oldOriginal := encode(4, color.RGBA{R: 255, A: 255})
	oldThumbnail := encode(2, color.RGBA{G: 255, A: 255})
	newOriginal := encode(4, color.RGBA{B: 255, A: 255})
	newThumbnail := encode(2, color.RGBA{R: 255, B: 255, A: 255})
	firstThumbnail := uploadShowcaseThumbnail(t, authorization, assetID+"_t", oldThumbnail)
	firstOriginal := uploadShowcaseThumbnail(t, authorization, assetID, oldOriginal)
	profileCall(t, showcases.Save, &papi.ShowcaseSaveRequest{Number: number, FrameId: frameID, AssetId: assetID, LayoutInfo: &papi.ShowcaseLayoutInfo{}})
	urlFor := func(kind penum.PhotoImageRequestType) *papi.PhotoImageUrl {
		t.Helper()
		response := profileCall(t, photos.GetImageURL, &papi.PhotoGetImageUrlRequest{Type: kind, AssetIds: []string{assetID}})
		if len(response.GetUrlList()) != 1 || response.GetUrlList()[0].GetAssetId() != assetID {
			t.Fatalf("unexpected image URL response: %v", response)
		}
		return response.GetUrlList()[0]
	}
	checkURL := func(raw string, metadata *umodel.UserPhotoImageMetadata) {
		t.Helper()
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host != "firebasestorage.googleapis.com" ||
			parsed.Path != "/v0/b/udc-idolypride/o/"+metadata.Name ||
			parsed.Query().Get("token") != metadata.DownloadTokens || parsed.Query().Get("v") != metadata.Generation {
			t.Fatalf("URL does not identify the selected image generation: %s", raw)
		}
	}
	getImage := func(raw string, expected []byte, generation string) {
		t.Helper()
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		response, err := testConfig.HttpClient.Get(testConfig.HttpUrl+parsed.RequestURI(), client.Config{DisablePathNormalizing: true})
		if err != nil || response.StatusCode() != 200 || !bytes.Equal(response.Body(), expected) || response.Header("x-goog-generation") != generation {
			t.Fatalf("photo URL returned stale or incorrect image bytes: %s err=%v", raw, err)
		}
	}
	firebaseURL := func(metadata *umodel.UserPhotoImageMetadata) string {
		// Firebase's metadata-based download URL uses name/bucket and the
		// first download token, not generation or our gRPC URL's v parameter.
		query := url.Values{"alt": {"media"}, "token": {strings.Split(metadata.DownloadTokens, ",")[0]}}
		return "https://firebasestorage.googleapis.com/v0/b/" + metadata.Bucket + "/o/" + url.PathEscape(metadata.Name) + "?" + query.Encode()
	}
	metadataFor := func(name string) *umodel.UserPhotoImageMetadata {
		t.Helper()
		response, err := testConfig.HttpClient.Get(testConfig.HttpUrl+"/v0/b/udc-idolypride/o/"+url.PathEscape(name), client.Config{
			Header: map[string]string{"Authorization": authorization}, DisablePathNormalizing: true,
		})
		if err != nil || response.StatusCode() != 200 {
			t.Fatalf("Firebase metadata lookup failed: %v", err)
		}
		return decodePhotoMetadata(t, response.Body())
	}
	// This is the route used by the actual Showcase client: GetMetadata,
	// build a token URL, then download. Photo/GetImageURL is not involved.
	showcaseTop(t, showcases)
	firstFirebaseURL := firebaseURL(metadataFor(firstOriginal.Name))
	getImage(firstFirebaseURL, oldOriginal, firstOriginal.Generation)
	firstURLs := urlFor(penum.PhotoImageRequestType_PhotoImageRequestType_Both)
	checkURL(firstURLs.GetOrigUrl(), firstOriginal)
	checkURL(firstURLs.GetTmbUrl(), firstThumbnail)
	if firstURLs.GetOrigUrl() == firstURLs.GetTmbUrl() {
		t.Fatal("separately uploaded thumbnail was aliased to the original")
	}
	// Warm both old URLs before atomically replacing either file.
	getImage(firstURLs.GetOrigUrl(), oldOriginal, firstOriginal.Generation)
	getImage(firstURLs.GetTmbUrl(), oldThumbnail, firstThumbnail.Generation)

	secondThumbnail := uploadShowcaseThumbnail(t, authorization, assetID+"_t", newThumbnail)
	intermediate := urlFor(penum.PhotoImageRequestType_PhotoImageRequestType_Both)
	if intermediate.GetOrigUrl() != firstURLs.GetOrigUrl() || intermediate.GetTmbUrl() == firstURLs.GetTmbUrl() {
		t.Fatal("thumbnail replacement did not independently update its cache key")
	}
	secondOriginal := uploadShowcaseThumbnail(t, authorization, assetID, newOriginal)
	for _, metadata := range []*umodel.UserPhotoImageMetadata{secondThumbnail, secondOriginal} {
		response, err := testConfig.HttpClient.Get(testConfig.HttpUrl+"/v0/b/udc-idolypride/o/"+url.PathEscape(metadata.Name), client.Config{
			Header: map[string]string{"Authorization": authorization}, DisablePathNormalizing: true,
		})
		if err != nil || response.StatusCode() != 200 || decodePhotoMetadata(t, response.Body()).Generation != metadata.Generation {
			t.Fatalf("paired upload metadata did not reflect latest generation: %v", err)
		}
	}
	// Real overwrite requests omit assetId; keep the same showcase image identity.
	saved := profileCall(t, showcases.Save, &papi.ShowcaseSaveRequest{Number: number, FrameId: frameID, LayoutInfo: &papi.ShowcaseLayoutInfo{}})
	if len(saved.GetCommonResponse().GetUpdatedData().GetShowcases()) != 1 || saved.GetCommonResponse().GetUpdatedData().GetShowcases()[0].GetThumbnailAssetId() != assetID {
		t.Fatal("save changed the existing showcase thumbnail identity")
	}
	showcaseTop(t, showcases)
	postSaveMetadata := metadataFor(secondOriginal.Name)
	secondFirebaseURL := firebaseURL(postSaveMetadata)
	if secondFirebaseURL == firstFirebaseURL {
		t.Fatal("Firebase metadata still produces the cached pre-overwrite cover URL")
	}
	if postSaveMetadata.DownloadTokens != secondOriginal.DownloadTokens ||
		metadataFor(secondOriginal.Name).DownloadTokens != postSaveMetadata.DownloadTokens {
		t.Fatal("metadata lookup or Showcase.Save/Top changed the token after publication")
	}
	getImage(secondFirebaseURL, newOriginal, secondOriginal.Generation)
	secondURLs := urlFor(penum.PhotoImageRequestType_PhotoImageRequestType_Both)
	checkURL(secondURLs.GetOrigUrl(), secondOriginal)
	checkURL(secondURLs.GetTmbUrl(), secondThumbnail)
	if firstURLs.GetOrigUrl() == secondURLs.GetOrigUrl() || firstURLs.GetTmbUrl() == secondURLs.GetTmbUrl() {
		t.Fatal("replacement reused an old URL cache key")
	}
	getImage(secondURLs.GetOrigUrl(), newOriginal, secondOriginal.Generation)
	getImage(secondURLs.GetTmbUrl(), newThumbnail, secondThumbnail.Generation)
	for _, oldURL := range []string{firstURLs.GetOrigUrl(), firstURLs.GetTmbUrl()} {
		parsed, err := url.Parse(oldURL)
		if err != nil {
			t.Fatal(err)
		}
		response, err := testConfig.HttpClient.Get(testConfig.HttpUrl+parsed.RequestURI(), client.Config{DisablePathNormalizing: true})
		if err != nil || response.StatusCode() != 403 {
			t.Fatalf("superseded image token is still accepted: %s err=%v", oldURL, err)
		}
	}
	if original := urlFor(penum.PhotoImageRequestType_PhotoImageRequestType_Original); original.GetOrigUrl() != secondURLs.GetOrigUrl() || original.GetTmbUrl() != "" {
		t.Fatal("original-only request did not select the original image")
	}
	if thumbnail := urlFor(penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail); thumbnail.GetTmbUrl() != secondURLs.GetTmbUrl() || thumbnail.GetOrigUrl() != "" {
		t.Fatal("thumbnail-only request did not select the _t image")
	}
}

func uploadShowcaseThumbnail(t *testing.T, authorization, assetID string, body []byte) *umodel.UserPhotoImageMetadata {
	t.Helper()
	objectName := "Photo/" + testConfig.TestUserPublicId + "/" + assetID
	start, err := testConfig.HttpClient.Post(testConfig.HttpUrl+"/v0/b/udc-idolypride/o", client.Config{
		Param: map[string]string{"name": objectName, "uploadType": "resumable"},
		Header: map[string]string{
			"Authorization": authorization, "X-Goog-Upload-Command": "start", "X-Goog-Upload-Protocol": "resumable", "X-Goog-Upload-Header-Content-Type": "image/jpeg",
		}, Body: map[string]string{"contentType": "image/jpeg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if start.StatusCode() != 200 {
		t.Fatalf("thumbnail upload start returned %d: %s", start.StatusCode(), start.Body())
	}
	uploadURL, err := url.Parse(start.Header("X-Goog-Upload-URL"))
	if err != nil || uploadURL.Host != "firebasestorage.googleapis.com" || uploadURL.Query().Get("upload_id") == "" {
		t.Fatalf("invalid thumbnail upload URL: %v", err)
	}
	final, err := testConfig.HttpClient.Post(fmt.Sprintf("%s%s?%s", testConfig.HttpUrl, uploadURL.Path, uploadURL.RawQuery), client.Config{
		Header: map[string]string{
			"Authorization": authorization, "Content-Type": "application/x-www-form-urlencoded", "X-Goog-Upload-Command": "upload, finalize", "X-Goog-Upload-Protocol": "resumable", "X-Goog-Upload-Offset": "0",
		}, Body: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.StatusCode() != 200 {
		t.Fatalf("thumbnail finalize returned %d: %s", final.StatusCode(), final.Body())
	}
	return decodePhotoMetadata(t, final.Body())
}
