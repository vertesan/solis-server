package profile

import (
	"context"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func (s *ProfileServer) FindUser(ctx context.Context, req *papi.ProfileFindUserRequest) (*papi.ProfileFindUserResponse, error) {
	if req.GetUserId() == "" {
		return nil, viber.InvalidArgumentf("userId is required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ProfileFindUserResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		public, err := opuser.FindUserPublic(ctx, suid)
		if err != nil {
			return nil, err
		}
		// The private server never looks up or exposes another account.
		if req.GetUserId() != public.GetPublicUserId() && req.GetUserId() != suid {
			// Official missing-user searches succeed with a default profile and
			// a present commonResponse. A gRPC NotFound error has no message body
			// and does not take the client's normal "no matching user" path.
			response = &papi.ProfileFindUserResponse{CommonResponse: viber.NewCommonResponse(nil)}
			return nil, nil
		}
		self, err := viber.LoadSelfProfileInfo(ctx, suid)
		if err != nil {
			return nil, err
		}
		user, err := opuser.FindUser(ctx, suid)
		if err != nil {
			return nil, err
		}
		profile, err := opuser.FindUserProfile(ctx, suid)
		if err != nil {
			return nil, err
		}
		gradeID, err := profileHierarchyGrade(ctx, suid)
		if err != nil {
			return nil, err
		}
		response = &papi.ProfileFindUserResponse{
			UserId:                        suid,
			Name:                          user.GetName(),
			ManagerExp:                    user.GetManagerExp(),
			ManagerLevel:                  user.GetManagerLevel(),
			DivisionId:                    user.GetDivisionId(),
			GuildId:                       user.GetGuildId(),
			EmblemId:                      user.GetEmblemId(),
			FavoriteCard:                  self.GetFavoriteCard(),
			FavoritePhoto:                 self.GetFavoritePhoto(),
			FavoriteCharacterIds:          profile.GetFavoriteCharacterIds(),
			Message:                       profile.GetMessage(),
			BirthMonth:                    int32(profile.GetBirthMonth()),
			BirthDay:                      int32(profile.GetBirthDay()),
			BackgroundType:                profile.GetBackgroundType(),
			LayoutType:                    profile.GetLayoutType(),
			HierarchyCurrentDetailGradeId: gradeID,
			HierarchyCurrentRank:          viber.HierarchyPrivateServerRank,
			DecorationId:                  profile.GetDecorationId(),
			InformationType:               profile.GetInformationType(),
			ColorType:                     profile.GetColorType(),
			CommonResponse:                viber.NewCommonResponse(nil),
		}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
