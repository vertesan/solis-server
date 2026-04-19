package opuser

import (
	"context"
	"errors"
	"fmt"
	"vertesan/solis-server/db/def/user/uname"
	"vertesan/solis-server/db/def/user/ustruct"
	"vertesan/solis-server/db/internal/depot"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func InsertUserAuth(ctx context.Context, userAuth *ustruct.UserAuth) error {
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

func UpdateOneTimeToken(ctx context.Context, suid string, oneTimeToken string) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(
		ctx,
		bson.M{"serverUserId": suid},
		bson.M{
			"$set": bson.M{"oneTimeToken": oneTimeToken},
		},
	)
	if err != nil || result.ModifiedCount == 0 {
		if err == nil && result.ModifiedCount == 0 {
			err = errors.New("modified count = 0")
		}
		return fmt.Errorf("failed update one time token in db: %w", err)
	}
	return nil
}

func InsertRefreshToken(ctx context.Context, firebaseId string, refreshToken string) error {
	result, err := depot.User.Collection(uname.UserAuth).UpdateOne(
		ctx,
		bson.M{"firebaseId": firebaseId},
		bson.D{
			{
				Key: "$push",
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
	if result.ModifiedCount == 0 {
		return fmt.Errorf("no document found with firebaseId '%q'", firebaseId)
	}
	return nil
}

func GetUserAuthByRefreshToken(ctx context.Context, refreshToken string) (*ustruct.UserAuth, error) {
	var user ustruct.UserAuth
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

func GetUserAuthByPublicId(ctx context.Context, publicId string) (*ustruct.UserAuth, error) {
	return getUserAuthByField(ctx, "publicUserId", publicId)
}

func GetUserAuthByServerId(ctx context.Context, serverId string) (*ustruct.UserAuth, error) {
	return getUserAuthByField(ctx, "serverUserId", serverId)
}

func GetUserAuthByFirebaseId(ctx context.Context, firebaseId string) (*ustruct.UserAuth, error) {
	return getUserAuthByField(ctx, "firebaseId", firebaseId)
}

func GetUserAuthByOneTimeToken(ctx context.Context, oneTimeToken string) (*ustruct.UserAuth, error) {
	return getUserAuthByField(ctx, "oneTimeToken", oneTimeToken)
}

func getUserAuthByField(ctx context.Context, fieldName, value string) (*ustruct.UserAuth, error) {
	var user ustruct.UserAuth
	err := depot.User.Collection(uname.UserAuth).FindOne(
		ctx,
		bson.M{fieldName: value},
	).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve userAuth from db: %w", err)
	}
	return &user, nil
}
