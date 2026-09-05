package showcase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ShowcaseServer) SaveDetail(ctx context.Context, req *papi.ShowcaseSaveDetailRequest) (*papi.ShowcaseSaveDetailResponse, error) {
	if req.GetNumber() <= 0 || strings.TrimSpace(req.GetName()) == "" {
		return nil, viber.InvalidArgumentf("number and name are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ShowcaseSaveDetailResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if err := requireUnlockedShowcaseNumber(ctx, suid, req.GetNumber()); err != nil {
			return nil, err
		}
		setting, err := opmaster.FindDefaultShowcaseSetting(ctx)
		if err != nil {
			return nil, err
		}
		if utf8.RuneCountInString(req.GetName()) > int(setting.GetShowcaseDetailNameLimit()) {
			return nil, viber.InvalidArgumentf("showcase name exceeds the master-data limit")
		}
		if utf8.RuneCountInString(req.GetComment()) > int(setting.GetShowcaseDetailDescriptionLimit()) {
			return nil, viber.InvalidArgumentf("showcase comment exceeds the master-data limit")
		}
		showcase, err := opuser.FindUserShowcase(ctx, suid, req.GetNumber())
		if err != nil {
			return nil, err
		}
		if req.GetMusicId() != "" {
			if _, err := opmaster.FindShowcaseMusic(ctx, req.GetMusicId()); err != nil {
				return nil, err
			}
			if _, err := opuser.FindUserShowcaseMusic(ctx, suid, req.GetMusicId()); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Error(codes.FailedPrecondition, "showcase BGM is not owned")
				}
				return nil, err
			}
		}
		showcase.Name = req.GetName()
		showcase.Comment = req.GetComment()
		showcase.MusicId = req.GetMusicId()
		showcase.IsOpen = req.GetIsOpen()
		if err := opuser.UpsertUserShowcase(ctx, suid, showcase); err != nil {
			return nil, err
		}
		response = &papi.ShowcaseSaveDetailResponse{CommonResponse: viber.NewCommonResponse(&papi.UpdatedData{
			Showcases: []*ptransaction.UserShowcase{showcase},
		})}
		return response, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}
