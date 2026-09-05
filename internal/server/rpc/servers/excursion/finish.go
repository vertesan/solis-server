package excursion

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExcursionServer) Finish(ctx context.Context, req *papi.ExcursionFinishRequest) (*papi.ExcursionFinishResponse, error) {
	if req.GetCostumeId() == "" || req.GetHairId() == "" {
		return nil, status.Error(codes.InvalidArgument, "costumeId and hairId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.ExcursionFinishResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		state, err := opuser.FindUserExcursionState(ctx, suid)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, status.Error(codes.FailedPrecondition, "no excursion is active")
			}
			return nil, err
		}
		if state.ActiveCharacter == "" || state.ActivePlaceId == "" || state.StartedTime <= 0 {
			return nil, status.Error(codes.FailedPrecondition, "no excursion is active")
		}
		costume, err := validateExcursionWardrobe(
			ctx,
			suid,
			state.ActiveCharacter,
			state.ActivePlaceId,
			req.GetCostumeId(),
			req.GetHairId(),
		)
		if err != nil {
			return nil, err
		}

		updated := &papi.UpdatedData{}
		seenNumbers := make(map[int32]struct{}, len(req.GetGazeReactionNumbers()))
		for _, number := range req.GetGazeReactionNumbers() {
			if number <= 0 {
				return nil, status.Error(codes.InvalidArgument, "gaze reaction numbers must be positive")
			}
			if _, duplicate := seenNumbers[number]; duplicate {
				return nil, status.Error(codes.InvalidArgument, "gaze reaction numbers must be unique")
			}
			seenNumbers[number] = struct{}{}
			reaction, err := opmaster.FindExcursionGazeReaction(
				ctx,
				state.ActiveCharacter,
				state.ActivePlaceId,
				number,
			)
			if err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, status.Error(codes.InvalidArgument, "unknown gaze reaction")
				}
				return nil, err
			}
			if !excursionReactionAvailable(reaction, costume.GetId()) {
				return nil, status.Error(codes.PermissionDenied, "gaze reaction is unavailable for costume")
			}
			if _, err := opuser.FindUserExcursionGazeReaction(
				ctx,
				suid,
				state.ActiveCharacter,
				state.ActivePlaceId,
				int64(number),
			); err == nil {
				continue
			} else if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, err
			}
			userReaction := &ptransaction.UserExcursionGazeReaction{
				CharacterId:        state.ActiveCharacter,
				ExcursionPlaceId:   state.ActivePlaceId,
				GazeReactionNumber: int64(number),
			}
			if err := opuser.UpsertUserExcursionGazeReaction(ctx, suid, userReaction); err != nil {
				return nil, err
			}
			updated.ExcursionGazeReactions = append(updated.ExcursionGazeReactions, userReaction)
		}

		giftItem, err := opmaster.FindExcursionGiftItem(ctx, state.ActiveCharacter)
		if err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		gift := &ptransaction.UserGift{
			GiftId:       uuid.NewString(),
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   giftItem.GetId(),
			Amount:       1,
			Message:      "交流のおでかけ報酬",
			PostedTime:   now,
			LimitTime:    0,
		}
		if err := opuser.UpsertUserGift(ctx, suid, gift); err != nil {
			return nil, err
		}
		notifications, err := opuser.FindAllUserNotifications(ctx, suid)
		if err != nil {
			return nil, err
		}
		var giftNotification *ptransaction.UserNotification
		for _, notification := range notifications {
			if notification.GetNotificationType() == penum.NotificationType_NotificationType_Gift {
				giftNotification = notification
				break
			}
		}
		if giftNotification == nil || !giftNotification.GetValid() {
			if giftNotification == nil {
				giftNotification = &ptransaction.UserNotification{
					NotificationType: penum.NotificationType_NotificationType_Gift,
				}
			}
			giftNotification.Valid = true
			if err := opuser.UpsertUserNotification(ctx, suid, giftNotification); err != nil {
				return nil, err
			}
			updated.Notifications = append(updated.Notifications, giftNotification)
		}
		missions, err := viber.AdvanceUserMissions(
			ctx,
			suid,
			penum.MissionType_MissionType_ExcursionClearCount,
			1,
			state.ActiveCharacter,
		)
		if err != nil {
			return nil, err
		}
		updated.Missions = append(updated.Missions, missions...)

		state.ActiveCharacter = ""
		state.ActivePlaceId = ""
		state.StartedTime = 0
		if err := opuser.UpsertUserExcursionState(ctx, state); err != nil {
			return nil, err
		}
		response = &papi.ExcursionFinishResponse{
			RewardResults: []*ptransaction.RewardResult{{
				ResourceType: penum.ResourceType_ResourceType_Item,
				ResourceId:   giftItem.GetId(),
				AmountI64:    1,
				BeforeAmount: 0,
				AfterAmount:  0,
				IsGift:       true,
			}},
			CommonResponse: viber.NewCommonResponse(updated),
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func validateExcursionWardrobe(
	ctx context.Context,
	suid, characterID, placeID, costumeID, hairID string,
) (*pmaster.Costume, error) {
	if _, err := opuser.FindUserCostume(ctx, suid, costumeID); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.PermissionDenied, "costume is not owned")
		}
		return nil, err
	}
	if _, err := opuser.FindUserHair(ctx, suid, hairID); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, status.Error(codes.PermissionDenied, "hair is not owned")
		}
		return nil, err
	}
	costume, err := opmaster.FindCostume(ctx, costumeID)
	if err != nil {
		return nil, err
	}
	hair, err := opmaster.FindHair(ctx, hairID)
	if err != nil {
		return nil, err
	}
	if costume.GetCharacterId() != characterID || hair.GetCharacterId() != characterID {
		return nil, status.Error(codes.InvalidArgument, "costume or hair belongs to another character")
	}
	if len(hair.GetWearableCostumeIds()) > 0 && !slices.Contains(hair.GetWearableCostumeIds(), costumeID) {
		return nil, status.Error(codes.InvalidArgument, "hair cannot be worn with costume")
	}
	if slices.Contains(hair.GetNotWearableCostumeIds(), costumeID) {
		return nil, status.Error(codes.InvalidArgument, "hair cannot be worn with costume")
	}
	place, err := opmaster.FindExcursionPlace(ctx, placeID)
	if err != nil {
		return nil, err
	}
	if slices.Contains(place.GetNotWearableCostumeTypeIds(), costume.GetCostumeTypeId()) {
		return nil, status.Error(codes.InvalidArgument, "costume cannot be worn at excursion place")
	}
	return costume, nil
}

func excursionReactionAvailable(reaction *pmaster.ExcursionGazeReaction, costumeID string) bool {
	if len(reaction.GetUnlockCostumeIds()) > 0 && !slices.Contains(reaction.GetUnlockCostumeIds(), costumeID) {
		return false
	}
	return !slices.Contains(reaction.GetUnlockNotCostumeIds(), costumeID)
}
