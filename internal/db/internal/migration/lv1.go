package migration

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/solis/smodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

		// functionMaintenanceInfos

		// notifications
		if err := opsolis.UpsertNoticeCatalog(ctx, &smodel.NoticeCatalog{
			Notices: []*papi.NoticeInfo{
				{
					Id:                  "info-news-26-0812-01",
					Title:               "Welcome to IDOLY PRIDE private server",
					BannerAssetId:       "img_banner_l_campaign-26-0731-01",
					LinkType:            penum.LinkType_LinkType_WebView,
					LinkDetail:          "",
					DisplayNotification: true,
					StartTime:           1785466800000,
					ListTitle:           "Welcome to IDOLY PRIDE private server",
				},
			},
			MalfunctionNotices: []*papi.NoticeInfo{},
			PrNotices:          []*papi.NoticeInfo{},
		}); err != nil {
			return nil, err
		}

		// home events
		if err := opsolis.UpsertHomeEventInfo(ctx, &papi.HomeEventInfo{
			DreamInfo: &papi.HomeDreamInfo{
				Name:                    "イメージトレーニング",
				StartTime:               1784084400000,
				EndTime:                 1944275439000,
				AssetId:                 "event-dream-defalut",
				BannerAssetId:           "img_banner_l_event-dream-defalut",
				Order:                   60,
				UnlockConditionId:       "cd-dream-25-0320-play",
				IsUnlocked:              false,
				GenerateAreaItemId:      "item-dream-generate-dream-25-0320",
				IsReceivableLoginReward: false,
				EventMissionId:          "event-dream-26-07",
			},
		}); err != nil {
			return nil, err
		}

		// userAuth indexes
		if err := ensureUserAuthIndexes(ctx); err != nil {
			return nil, err
		}

		// update migration flag
		if err := opsolis.UpsertMigration(ctx, &smodel.Migration{Migration: 1}); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

// EnsureUserAuthIndexes creates the uniqueness constraints relied on by the
// authentication and account-management flows. Partial filters deliberately
// exclude the empty token values assigned to new accounts.
func ensureUserAuthIndexes(ctx context.Context) error {
	indexes := depot.User.Collection(uname.UserAuth).Indexes()
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "publicUserId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "serverUserId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "firebaseId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "refreshTokens", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("refreshTokens_nonempty_unique").
				SetPartialFilterExpression(bson.M{
					"refreshTokens.0": bson.M{"$exists": true},
				}),
		},
		{
			Keys: bson.D{{Key: "oneTimeToken", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.M{
					"oneTimeToken": bson.M{"$gt": ""},
				}),
		},
	}

	if _, err := indexes.CreateMany(ctx, models); err != nil {
		return fmt.Errorf("failed to ensure userAuth indexes: %w", err)
	}
	return nil
}
