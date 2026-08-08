package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/master/mdoc"
	"github.com/vertesan/solis-server/internal/db/model/master/mname"
	"github.com/vertesan/solis-server/internal/db/model/solis/sdoc"
	"github.com/vertesan/solis-server/internal/db/model/solis/sname"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/transaction"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type indexSpec struct {
	database   string
	collection string
	keys       bson.D
	name       string
	partial    any
	unique     bool
}

type singletonSpec struct {
	database   *mongo.Database
	collection string
	id         string
}

// Ensure canonicalizes singleton documents and creates every application index
// required for logical uniqueness and indexed user-data access.
func Ensure(ctx context.Context) error {
	if err := Prepare(ctx); err != nil {
		return err
	}
	for _, spec := range indexSpecs() {
		database, err := databaseByName(spec.database)
		if err != nil {
			return err
		}
		indexOpts := options.Index().SetName(spec.name)
		if spec.unique {
			indexOpts.SetUnique(true)
		}
		if spec.partial != nil {
			indexOpts.SetPartialFilterExpression(spec.partial)
		}
		if _, err := database.Collection(spec.collection).Indexes().CreateOne(
			ctx,
			mongo.IndexModel{Keys: spec.keys, Options: indexOpts},
		); err != nil {
			return fmt.Errorf("failed to create index %q on [%s].[%s]: %w", spec.name, spec.database, spec.collection, err)
		}
	}
	return nil
}

// Prepare canonicalizes the singleton identifiers required by migrations and
// update checks. Indexes are installed only after those repairs finish so an
// authoritative replacement can clean legacy duplicates before unique index
// creation validates the resulting data.
func Prepare(ctx context.Context) error {
	return canonicalizeSingletons(ctx)
}

// canonicalizeSingletons transactionally moves legacy singleton documents to
// their fixed IDs and refuses ambiguous collections containing multiple rows.
func canonicalizeSingletons(ctx context.Context) error {
	specs := []singletonSpec{
		{database: depot.Solis, collection: sname.Migration, id: sdoc.MigrationDocumentID},
		{database: depot.Solis, collection: sname.ServerProfile, id: sdoc.ServerProfileDocumentID},
		{database: depot.Solis, collection: sname.OctoManifest, id: sdoc.OctoManifestDocumentID},
		{database: depot.Master, collection: mname.Version, id: mdoc.VersionDocumentID},
	}
	return transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		for _, spec := range specs {
			var documents []bson.M
			cursor, err := spec.database.Collection(spec.collection).Find(
				ctx,
				bson.M{},
				options.Find().SetLimit(2),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to inspect singleton [%s].[%s]: %w", spec.database.Name(), spec.collection, err)
			}
			if err := cursor.All(ctx, &documents); err != nil {
				return nil, fmt.Errorf("failed to decode singleton [%s].[%s]: %w", spec.database.Name(), spec.collection, err)
			}
			if len(documents) > 1 {
				return nil, fmt.Errorf("singleton [%s].[%s] contains more than one document", spec.database.Name(), spec.collection)
			}
			if len(documents) == 0 {
				continue
			}
			if id, ok := documents[0]["_id"].(string); ok && id == spec.id {
				continue
			}
			oldID := documents[0]["_id"]
			result, err := spec.database.Collection(spec.collection).DeleteOne(ctx, bson.M{"_id": oldID})
			if err != nil {
				return nil, fmt.Errorf("failed to remove legacy singleton [%s].[%s]: %w", spec.database.Name(), spec.collection, err)
			}
			if result.DeletedCount != 1 {
				return nil, fmt.Errorf("failed to remove legacy singleton [%s].[%s]: %w", spec.database.Name(), spec.collection, mongo.ErrNoDocuments)
			}
			documents[0]["_id"] = spec.id
			if _, err := spec.database.Collection(spec.collection).InsertOne(ctx, documents[0]); err != nil {
				return nil, fmt.Errorf("failed to canonicalize singleton [%s].[%s]: %w", spec.database.Name(), spec.collection, err)
			}
		}
		return nil, nil
	})
}

// databaseByName resolves the database identifier used by an indexSpec.
func databaseByName(name string) (*mongo.Database, error) {
	switch name {
	case "solis":
		return depot.Solis, nil
	case "user":
		return depot.User, nil
	default:
		return nil, fmt.Errorf("unknown database %q in index specification", name)
	}
}

// uniqueIndex builds a deterministic ascending unique-index specification for
// the supplied logical key fields.
func uniqueIndex(database string, collection string, fields ...string) indexSpec {
	keys := make(bson.D, 0, len(fields))
	for _, field := range fields {
		keys = append(keys, bson.E{Key: field, Value: 1})
	}
	return indexSpec{
		database:   database,
		collection: collection,
		keys:       keys,
		name:       "uq_" + strings.Join(fields, "_"),
		unique:     true,
	}
}

// queryIndex builds a non-unique lookup index from the same field description
// used by uniqueIndex.
func queryIndex(database string, collection string, fields ...string) indexSpec {
	spec := uniqueIndex(database, collection, fields...)
	spec.name = "ix_" + strings.Join(fields, "_")
	spec.unique = false
	return spec
}

// indexSpecs declares the complete set of logical-key and access-path indexes
// managed by database initialization.
func indexSpecs() []indexSpec {
	specs := []indexSpec{
		uniqueIndex("user", uname.User, "SUID"),
		uniqueIndex("user", uname.UserAccessory, "SUID", "accessoryId"),
		uniqueIndex("user", uname.UserBalance, "SUID"),
		uniqueIndex("user", uname.UserBuddy, "SUID"),
		uniqueIndex("user", uname.UserCard, "SUID", "cardId"),
		uniqueIndex("user", uname.UserCharacter, "SUID", "characterId"),
		uniqueIndex("user", uname.UserCompany, "SUID"),
		uniqueIndex("user", uname.UserCostume, "SUID", "costumeId"),
		uniqueIndex("user", uname.UserCurrentQuestArea, "SUID"),
		uniqueIndex("user", uname.UserDeck, "SUID", "number"),
		uniqueIndex("user", uname.UserDeckPosition, "SUID", "number", "position"),
		uniqueIndex("user", uname.UserEmblem, "SUID", "emblemId"),
		uniqueIndex("user", uname.UserEventInfo, "SUID"),
		uniqueIndex("user", uname.UserEventStoryInfo, "SUID", "id"),
		uniqueIndex("user", uname.UserExchangeBooth, "SUID"),
		uniqueIndex("user", uname.UserExtraStoryPartInfo, "SUID", "id"),
		uniqueIndex("user", uname.UserFanEventInfo, "SUID"),
		uniqueIndex("user", uname.UserGachaInfo, "SUID", "id"),
		uniqueIndex("user", uname.UserHair, "SUID", "hairId"),
		uniqueIndex("user", uname.UserHierarchy, "SUID"),
		uniqueIndex("user", uname.UserHierarchyNotiInfo, "SUID"),
		uniqueIndex("user", uname.UserHomePosition, "SUID", "homePositionType"),
		uniqueIndex("user", uname.UserItem, "SUID", "itemId", "expiredTime"),
		uniqueIndex("user", uname.UserLessonProgress, "SUID"),
		uniqueIndex("user", uname.UserMessage, "SUID", "messageId"),
		uniqueIndex("user", uname.UserNotiNormalShopItem, "SUID"),
		uniqueIndex("user", uname.UserNotiShowcaseShopItem, "SUID"),
		uniqueIndex("user", uname.UserNotiStoneShopItem, "SUID"),
		uniqueIndex("user", uname.UserNotification, "SUID", "notificationType"),
		uniqueIndex("user", uname.UserPhoto, "SUID", "photoId"),
		uniqueIndex("user", uname.UserPhotoActivity, "SUID", "photoActivityId"),
		uniqueIndex("user", uname.UserPhotoExpression, "SUID", "photoExpressionId"),
		uniqueIndex("user", uname.UserPhotoMusic, "SUID", "musicId"),
		uniqueIndex("user", uname.UserPhotoPose, "SUID", "photoPoseId"),
		uniqueIndex("user", uname.UserPhotoReport, "SUID"),
		uniqueIndex("user", uname.UserPhotoStage, "SUID", "stageId"),
		uniqueIndex("user", uname.UserPoint, "SUID", "pointType"),
		uniqueIndex("user", uname.UserProfile, "SUID"),
		uniqueIndex("user", uname.UserPromotionInfo, "SUID"),
		uniqueIndex("user", uname.UserPublic, "serverUserId"),
		uniqueIndex("user", uname.UserRefreshInfo, "SUID"),
		uniqueIndex("user", uname.UserShopId, "SUID"),
		uniqueIndex("user", uname.UserShowcaseLike, "SUID"),
		uniqueIndex("user", uname.UserShowcaseToyShopId, "SUID"),
		uniqueIndex("user", uname.UserStaff, "SUID", "parameterType"),
		uniqueIndex("user", uname.UserTotalCount, "SUID"),
		uniqueIndex("user", uname.UserTutorial, "SUID", "tutorialType"),
		uniqueIndex("user", uname.UserUnlockedShowcaseNumber, "SUID"),

		queryIndex("user", uname.UserStory, "SUID"),
		queryIndex("user", uname.UserTelephone, "SUID"),
		queryIndex("user", uname.UserMission, "SUID"),
		queryIndex("user", uname.UserCharacterMusic, "SUID"),
		queryIndex("user", uname.UserArea, "SUID"),
		queryIndex("user", uname.UserQuest, "SUID"),
		queryIndex("user", uname.UserHomeTalk, "SUID"),
		queryIndex("user", uname.UserCardSupport, "SUID"),
		queryIndex("user", uname.UserMessageSchedule, "SUID"),
		queryIndex("user", uname.UserMusic, "SUID"),
		queryIndex("user", uname.UserDecoration, "SUID"),
		queryIndex("user", uname.UserPhotoRecipe, "SUID"),
		queryIndex("user", uname.UserLove, "SUID"),
		queryIndex("user", uname.UserLeagueDeckPosition, "SUID"),
		queryIndex("user", uname.UserDuty, "SUID"),
		queryIndex("user", uname.UserDutyPoint, "SUID"),
		queryIndex("user", uname.UserShowcase, "SUID"),
		queryIndex("user", uname.UserShowcaseMusic, "SUID"),
		queryIndex("user", uname.UserShowcaseToy, "SUID"),
		queryIndex("user", uname.UserShowcaseMyset, "SUID"),
		queryIndex("user", uname.UserTourArea, "SUID"),
		uniqueIndex("user", uname.UserInvite, "SUID"),
		uniqueIndex("user", uname.UserActivityFanEventProgress, "SUID"),
		uniqueIndex("user", uname.UserStaffTrainingReset, "SUID"),

		uniqueIndex("solis", sname.Division, "id"),
		uniqueIndex("solis", sname.MasterTagPack, "type"),
		uniqueIndex("solis", sname.MenuBanner, "id"),
		uniqueIndex("solis", sname.Qseed, "device"),

		uniqueIndex("user", uname.UserAuth, "publicUserId"),
		uniqueIndex("user", uname.UserAuth, "serverUserId"),
		uniqueIndex("user", uname.UserAuth, "firebaseId"),
	}
	refreshTokens := uniqueIndex("user", uname.UserAuth, "refreshTokens")
	// Token-less legacy and test accounts may have an empty array. Restrict the
	// unique multikey index to documents that actually contain a string token
	// so any number of those accounts can coexist.
	refreshTokens.partial = bson.M{"refreshTokens": bson.M{"$type": "string"}}
	specs = append(specs, refreshTokens)
	oneTimeToken := uniqueIndex("user", uname.UserAuth, "oneTimeToken")
	oneTimeToken.partial = bson.M{"oneTimeToken": bson.M{"$gt": ""}}
	return append(specs, oneTimeToken)
}
