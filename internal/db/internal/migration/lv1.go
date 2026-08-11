package migration

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
)

// server first startup database initialization
func level1Migration() error {
	return transaction.WithTransaction(context.Background(), func(ctx context.Context) (any, error) {
		// serverProfile
		if err := opsolis.DeleteServerProfile(ctx); err != nil {
			return nil, err
		}
		if err := opsolis.InsertServerProfile(ctx, &smodel.ServerProfile{
			AppVersion:         "6.0.2",
			Platform:           "Android",
			MasterVersion:      "",
			FirebaseProductKey: "AIzaSyCabjC7_mOU21qkcNobnhN1khTi1-1F-kA",
			FirebaseProjectId:  "543065248020",
			PackageName:        "game.qualiarts.idolypride",
			OctoApiKey:         "xop8c3tqrl8dcgij6mc9y4yf5o7btknt",
			OctoManifestKey:    "aa8a30926db9d49410360d0a99aa735d035638dfc09ef99fb575d9c91a8f6cdc",
		}); err != nil {
			return nil, err
		}

		// division
		if err := opsolis.DeleteAllDivisions(ctx); err != nil {
			return nil, err
		}
		if err := opsolis.UpsertDivision(ctx, &papi.DivisionInfo{
			Id:       "hVs7e3Np-00001",
			LevelAvg: 1,
		}); err != nil {
			return nil, err
		}

		// campaignEffects (empty)

		// bannerInfo
		// menu banners
		if err := opsolis.UpsertMenuBanner(ctx, &papi.Banner{
			Id:              "banner-pr-raido-21-0826-02",
			AssetId:         "img_banner_s_pr-radio-21-0831",
			LinkType:        penum.LinkType_LinkType_ExternalWebPage,
			LinkDetail:      "https://youtube.com/playlist?list=PL0eMwNDVvw1Sa1Fp6eQj7yUw2BF5z86Ad",
			Order:           150,
			ViewConditionId: "cd-week-radio-tuesday",
		}); err != nil {
			return nil, err
		}

		// exchangeBoothIds

		// notiStoneShopItem

		// notiNormalShopItem

		// notiShowcaseShopItem

		// eventInfo

		// functionMaintenanceInfos

		// update migration flag
		if err := opsolis.UpsertMigration(ctx, &smodel.Migration{Migration: 1}); err != nil {
			return nil, err
		}
		return nil, nil
	})
}
