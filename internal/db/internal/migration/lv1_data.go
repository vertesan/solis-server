package migration

import (
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/proto/papi"
)

type level1Data struct {
	serverProfile *smodel.ServerProfile
	division      *papi.DivisionInfo
	menuBanner    *papi.Banner
	noticeCatalog *smodel.NoticeCatalog
	homeEventInfo *papi.HomeEventInfo
}

type serverProfileData struct {
	AppVersion         string `yaml:"appVersion"`
	Platform           string `yaml:"platform"`
	MasterVersion      string `yaml:"masterVersion"`
	FirebaseProductKey string `yaml:"firebaseProductKey"`
	FirebaseProjectId  string `yaml:"firebaseProjectId"`
	PackageName        string `yaml:"packageName"`
	OctoApiKey         string `yaml:"octoApiKey"`
	OctoManifestKey    string `yaml:"octoManifestKey"`
}

func loadLevel1Data() (*level1Data, error) {
	profileData := new(serverProfileData)
	if err := unmarshalMigrationYAML("initdata/server_profile.yaml", profileData); err != nil {
		return nil, err
	}

	division := new(papi.DivisionInfo)
	if err := unmarshalMigrationProtoYAML("initdata/division.yaml", division); err != nil {
		return nil, err
	}

	menuBanner := new(papi.Banner)
	if err := unmarshalMigrationProtoYAML("initdata/menu_banner.yaml", menuBanner); err != nil {
		return nil, err
	}

	noticeResponse := new(papi.NoticeListResponse)
	if err := unmarshalMigrationProtoYAML("initdata/notice_catalog.yaml", noticeResponse); err != nil {
		return nil, err
	}

	homeEventInfo := new(papi.HomeEventInfo)
	if err := unmarshalMigrationProtoYAML("initdata/home_event_info.yaml", homeEventInfo); err != nil {
		return nil, err
	}

	return &level1Data{
		serverProfile: &smodel.ServerProfile{
			AppVersion:         profileData.AppVersion,
			Platform:           profileData.Platform,
			MasterVersion:      profileData.MasterVersion,
			FirebaseProductKey: profileData.FirebaseProductKey,
			FirebaseProjectId:  profileData.FirebaseProjectId,
			PackageName:        profileData.PackageName,
			OctoApiKey:         profileData.OctoApiKey,
			OctoManifestKey:    profileData.OctoManifestKey,
		},
		division:   division,
		menuBanner: menuBanner,
		noticeCatalog: &smodel.NoticeCatalog{
			Notices:            noticeResponse.Notices,
			MalfunctionNotices: noticeResponse.MalfunctionNotices,
			PrNotices:          noticeResponse.PrNotices,
		},
		homeEventInfo: homeEventInfo,
	}, nil
}
