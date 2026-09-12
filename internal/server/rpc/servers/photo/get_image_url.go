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
	lookupNames := make([]string, 0, 2*len(names))
	for index, assetID := range req.GetAssetIds() {
		if !isPhotoAssetID(assetID) {
			return nil, viber.InvalidArgumentf("invalid photo assetId %q", assetID)
		}
		names[index] = photoObjectPrefix + "/" + public.GetPublicUserId() + "/" + assetID
		lookupNames = append(lookupNames, names[index])
		if req.GetType() != penum.PhotoImageRequestType_PhotoImageRequestType_Original {
			lookupNames = append(lookupNames, names[index]+"_t")
		}
	}
	metadata, err := opuser.FindFinalizedUserPhotoImageMetadataByNames(ctx, suid, lookupNames)
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	metadataByName := make(map[string]*umodel.UserPhotoImageMetadata, len(metadata))
	for _, entry := range metadata {
		if entry.Generation != "" &&
			entry.PublicUserId == public.GetPublicUserId() {
			metadataByName[entry.Name] = entry
		}
	}

	urls := make([]*papi.PhotoImageUrl, 0, len(names))
	for index, name := range names {
		downloadURL := photoFirebaseDownloadURL(name, metadataByName[name])
		thumbnailURL := downloadURL
		if thumbnail := metadataByName[name+"_t"]; thumbnail != nil {
			thumbnailURL = photoFirebaseDownloadURL(thumbnail.Name, thumbnail)
		}
		// Older single-image photos have no separately uploaded thumbnail.
		// Retain their original-image fallback until a published _t exists.
		entry := &papi.PhotoImageUrl{AssetId: req.AssetIds[index]}
		switch req.GetType() {
		case penum.PhotoImageRequestType_PhotoImageRequestType_Original:
			entry.OrigUrl = downloadURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Thumbnail:
			entry.TmbUrl = thumbnailURL
		case penum.PhotoImageRequestType_PhotoImageRequestType_Both:
			entry.OrigUrl = downloadURL
			entry.TmbUrl = thumbnailURL
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
	if metadata != nil && metadata.Generation != "" {
		// Also make the published generation explicit for clients that obtain
		// image URLs through this RPC instead of Firebase metadata directly.
		query.Set("v", metadata.Generation)
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
