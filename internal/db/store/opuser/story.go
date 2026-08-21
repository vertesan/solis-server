package opuser

import (
	"context"
	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/model/user/uname"
	"github.com/vertesan/solis-server/internal/db/store/opuser/internal/ucommon"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func FindUserStory(ctx context.Context, suid string, storyId string) (*ptransaction.UserStory, error) {
	return ucommon.FindOne[ptransaction.UserStory](
		ctx,
		uname.UserStory,
		bson.M{"SUID": suid, "storyId": storyId},
	)
}

func FindAllUserStories(ctx context.Context, suid string) ([]*ptransaction.UserStory, error) {
	return ucommon.FindAll[ptransaction.UserStory](
		ctx,
		uname.UserStory,
		bson.M{"SUID": suid},
	)
}

func UpsertUserStory(ctx context.Context, suid string, story *ptransaction.UserStory) error {
	return ucommon.UpsertOne(
		ctx,
		uname.UserStory,
		bson.M{"SUID": suid, "storyId": story.StoryId},
		&umodel.UserStruct[*ptransaction.UserStory]{
			SUID: suid,
			Data: story,
		},
	)
}

func UpsertManyUserStories(ctx context.Context, suid string, stories []*ptransaction.UserStory) error {
	return ucommon.ForEachAtomically(ctx, stories, func(ctx context.Context, story *ptransaction.UserStory) error {
		return UpsertUserStory(ctx, suid, story)
	})
}

func DeleteUserStory(ctx context.Context, suid string, storyId string) error {
	return ucommon.Delete(
		ctx,
		uname.UserStory,
		bson.M{"SUID": suid, "storyId": storyId},
	)
}
