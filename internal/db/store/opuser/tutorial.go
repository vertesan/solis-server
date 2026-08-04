package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/def/user/uname"
	"github.com/vertesan/solis-server/internal/db/def/user/ustruct"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindAllUserTutorials(ctx context.Context, suid string) ([]*ptransaction.UserTutorial, error) {
	return ucommon.FindAll[ptransaction.UserTutorial](
		ctx,
		uname.UserTutorial,
		bson.M{"SUID": suid},
	)
}

func UpsertUserTutorial(ctx context.Context, suid string, tutorial *ptransaction.UserTutorial) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserTutorial,
		bson.M{"SUID": suid, "tutorialType": tutorial.TutorialType},
		&ustruct.UserStruct[*ptransaction.UserTutorial]{
			SUID: suid,
			Data: tutorial,
		},
	)
}

func UpsertManyUserTutorials(ctx context.Context, suid string, tutorials []*ptransaction.UserTutorial) error {
	for _, tutorial := range tutorials {
		if err := UpsertUserTutorial(ctx, suid, tutorial); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllUserTutorials(ctx context.Context, suid string) error {
	return ucommon.Delete(
		ctx,
		uname.UserTutorial,
		bson.M{"SUID": suid},
	)
}
