package opuser

import (
	"context"
	"fmt"

	"github.com/vertesan/solis-server/internal/db/internal/depot"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func InsertUserAuth(ctx context.Context, userAuth *umodel.UserAuth) error {
	_, err := depot.User.Collection(uname.UserAuth).InsertOne(
		ctx,
		userAuth,
	)
	if err != nil {
		return fmt.Errorf("failed to insert userAuth: %w", err)
	}
	return nil
}

func DeleteUserAuth(ctx context.Context, suid string) error {
	if _, err := depot.User.Collection(uname.UserAuth).DeleteMany(
		ctx,
		bson.M{"serverUserId": suid},
	); err != nil {
		return err
	}
	return nil
}

// Changing a transfer password also invalidates any transfer token issued with
// the previous password. It does not revoke the user's current login sessions.
func UpdateUserAuthPassword(ctx context.Context, suid, password string) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(ctx,
		bson.M{"serverUserId": suid},
		bson.M{"$set": bson.M{"password": password}, "$unset": bson.M{"oneTimeToken": ""}},
	)
	if err != nil {
		return fmt.Errorf("failed to update user auth password: %w", err)
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func UpdateUserAuthOneTimeToken(ctx context.Context, suid string, oneTimeToken string) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(
		ctx,
		bson.M{"serverUserId": suid},
		bson.M{
			"$set": bson.M{"oneTimeToken": oneTimeToken},
		},
	)
	if err != nil {
		return fmt.Errorf("failed update one time token in db: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("failed update one time token in db: %w", mongo.ErrNoDocuments)
	}
	return nil
}

func UpdateUserAuthLastLoginTime(ctx context.Context, suid string, epoch int64) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(
		ctx,
		bson.M{"serverUserId": suid},
		bson.M{
			"$set": bson.M{"lastLoginAt": epoch},
		},
	)
	if err != nil {
		return fmt.Errorf("failed update last login time for userAuth: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("failed update last login time for userAuth: %w", mongo.ErrNoDocuments)
	}
	return nil
}

func InsertUserAuthRefreshToken(ctx context.Context, firebaseId string, refreshToken string) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(
		ctx,
		bson.M{"firebaseId": firebaseId},
		bson.D{
			{
				Key: "$addToSet",
				Value: bson.M{
					"refreshTokens": refreshToken,
				},
			},
			{
				Key: "$currentDate",
				Value: bson.M{
					"lastRefreshAt": true,
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed insert refresh token into db: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("no document found with firebaseId '%q'", firebaseId)
	}
	return nil
}

func GetUserAuthByRefreshToken(ctx context.Context, refreshToken string) (*umodel.UserAuth, error) {
	var user umodel.UserAuth
	err := depot.User.Collection(uname.UserAuth).FindOne(
		ctx,
		bson.M{"refreshTokens": refreshToken},
	).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving userAuth by refresh token from db: %w", err)
	}
	return &user, nil
}

func CountUserAuthByPublicOrServerUserId(ctx context.Context, publicId string, serverId string) (int64, error) {
	count, err := depot.User.Collection(uname.UserAuth).CountDocuments(
		ctx,
		bson.M{"$or": []bson.M{
			{"publicUserId": publicId},
			{"serverUserId": serverId},
		}},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to count userAuth: %w", err)
	}
	return count, nil
}

func GetUserAuthByPublicId(ctx context.Context, publicId string) (*umodel.UserAuth, error) {
	return getUserAuthByField(ctx, "publicUserId", publicId)
}

func GetUserAuthByServerId(ctx context.Context, serverId string) (*umodel.UserAuth, error) {
	return getUserAuthByField(ctx, "serverUserId", serverId)
}

func GetUserAuthByFirebaseId(ctx context.Context, firebaseId string) (*umodel.UserAuth, error) {
	return getUserAuthByField(ctx, "firebaseId", firebaseId)
}

func GetUserAuthByOneTimeToken(ctx context.Context, oneTimeToken string) (*umodel.UserAuth, error) {
	return getUserAuthByField(ctx, "oneTimeToken", oneTimeToken)
}

// ConsumeUserAuthOneTimeToken conditionally removes a matching nonempty token
// and returns its previous auth document, allowing only one caller to consume it.
func ConsumeUserAuthOneTimeToken(ctx context.Context, suid string, oneTimeToken string) (*umodel.UserAuth, error) {
	if oneTimeToken == "" {
		return nil, fmt.Errorf("one-time token must not be empty: %w", mongo.ErrNoDocuments)
	}
	var user umodel.UserAuth
	err := depot.User.Collection(uname.UserAuth).FindOneAndUpdate(
		ctx,
		bson.M{"serverUserId": suid, "oneTimeToken": oneTimeToken},
		bson.M{"$unset": bson.M{"oneTimeToken": ""}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before),
	).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to consume userAuth one-time token: %w", err)
	}
	return &user, nil
}

func getUserAuthByField(ctx context.Context, fieldName, value string) (*umodel.UserAuth, error) {
	var user umodel.UserAuth
	err := depot.User.Collection(uname.UserAuth).FindOne(
		ctx,
		bson.M{fieldName: value},
	).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve userAuth from db: %w", err)
	}
	return &user, nil
}
