package photo

import (
	"context"
	"net/url"
	"strings"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *PhotoServer) GetImageURL(
	ctx context.Context,
	req *papi.PhotoGetImageUrlRequest,
) (*papi.PhotoGetImageUrlResponse, error) {
	if req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Original &&
		req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail &&
		req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Both {
		return nil, viber.InvalidArgumentf("unsupported photo image request type")
	}

	suid := viber.GetSuid(ctx).String()
	public, err := opuser.FindUserPublic(ctx, suid)
	if err != nil {
		return nil, viber.GameplayError(err)
	}

	names := make([]string, len(req.GetAssetIds()))
	for index, assetID := range req.GetAssetIds() {
		if !isPhotoAssetID(assetID) {
			return nil, viber.InvalidArgumentf("invalid photo assetId %q", assetID)
		}
		names[index] = photoObjectPrefix + "/" + public.GetPublicUserId() + "/" + assetID
	}
	metadata, err := opuser.FindFinalizedUserPhotoImageMetadataByNames(ctx, suid, names)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	metadataByName := make(map[string]*umodel.UserPhotoImageMetadata, len(metadata))
	for _, entry := range metadata {
		if entry.UploadState == umodel.UserPhotoImageUploadFinalized &&
			entry.PublicUserId == public.GetPublicUserId() {
			metadataByName[entry.Name] = entry
		}
	}

	urls := make([]*papi.PhotoImageUrl, 0, len(names))
	for index, name := range names {
		downloadURL := photoFirebaseDownloadURL(name, metadataByName[name])
		entry := &papi.PhotoImageUrl{AssetId: req.AssetIds[index]}
		switch req.GetType() {
		case penum.PhotoImageRequestType_PhotoImageRequestType_Original:
			entry.OrigUrl = downloadURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail:
			entry.TmbUrl = downloadURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Both:
			entry.OrigUrl = downloadURL
			entry.TmbUrl = downloadURL
		}
		urls = append(urls, entry)
	}
	return &papi.PhotoGetImageUrlResponse{UrlList: urls}, nil
}

const (
	photoFirebaseBucket = "udc-idolypride"
	photoObjectPrefix   = "Photo"
)

func photoFirebaseDownloadURL(name string, metadata *umodel.UserPhotoImageMetadata) string {
	query := url.Values{"alt": []string{"media"}}
	if metadata != nil && metadata.DownloadTokens != "" {
		query.Set("token", metadata.DownloadTokens)
	}
	return "https://firebasestorage.googleapis.com/v0/b/" + photoFirebaseBucket + "/o/" + url.PathEscape(name) + "?" + query.Encode()
}

func isPhotoAssetID(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 251 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			strings.ContainsRune("-_.~", rune(character)) {
			continue
		}
		return false
	}
	return true
}
