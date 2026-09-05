package message

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *MessageServer) Finish(ctx context.Context, req *papi.MessageFinishRequest) (*papi.MessageFinishResponse, error) {
	if req.GetMessageGroupId() == "" && req.GetTimelineId() == 0 {
		return &papi.MessageFinishResponse{}, nil
	}
	if req.GetMessageGroupId() == "" || req.GetTimelineId() <= 0 {
		return nil, viber.InvalidArgumentf("messageGroupId and a positive timelineId are required")
	}
	suid := viber.GetSuid(ctx).String()
	var response *papi.MessageFinishResponse
	err := transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		finishedTime := time.Now().UnixMilli()
		state, err := loadMessageTimelineState(ctx, suid)
		if err != nil {
			return nil, err
		}
		info, userMessage, master := findTimelineMessage(state, req.MessageGroupId, req.TimelineId)
		if userMessage == nil {
			return nil, status.Error(codes.NotFound, "message timeline was not found")
		}
		path, selections, err := resolveMessageDetails(master, req.SelectMessageDetailIds, "")
		if err != nil {
			return nil, err
		}
		terminal := path[len(path)-1]
		if userMessage.GetMessageStatusType() == penum.MessageStatusType_MessageStatusType_Finished {
			response = &papi.MessageFinishResponse{CommonResponse: viber.NewCommonResponse(nil)}
			return nil, nil
		}
		if err := validateMessageProgressExtension(master, info, userMessage, path); err != nil {
			return nil, err
		}
		updated := &papi.UpdatedData{}
		isReliabilityUp := false
		userMessage.MessageStatusType = penum.MessageStatusType_MessageStatusType_Finished
		userMessage.SelectMessageDetailIds = selections
		if err := opuser.UpsertUserMessage(ctx, suid, userMessage); err != nil {
			return nil, err
		}
		updated.Messages = append(updated.Messages, userMessage)

		reliability := terminal.GetReliability()
		characterId := terminal.GetCharacterId()
		if characterId == "" {
			characterId = master.GetCharacterId()
		}
		if reliability < 0 || reliability > 0 && characterId == "" {
			return nil, status.Error(codes.FailedPrecondition, "message terminal has invalid reliability data")
		}
		if reliability > 0 && characterId != "" {
			character, err := opuser.FindUserCharacter(ctx, suid, characterId)
			if err != nil {
				return nil, err
			}
			if reliability > math.MaxInt32-character.Reliability {
				return nil, status.Error(codes.FailedPrecondition, "character reliability overflows")
			}
			character.Reliability += reliability
			if err := opuser.UpsertUserCharacter(ctx, suid, character); err != nil {
				return nil, err
			}
			updated.Characters = append(updated.Characters, character)
			missions, err := viber.ReachUserMissions(
				ctx, suid, penum.MissionType_MissionType_ReliabilityAchievement, int64(character.GetReliability()), characterId,
			)
			if err != nil {
				return nil, err
			}
			updated.Missions = append(updated.Missions, missions...)
			isReliabilityUp = true
		}

		for _, detail := range path {
			telephoneID := detail.GetTelephoneId()
			if telephoneID == "" {
				continue
			}
			if _, err := opmaster.FindTelephone(ctx, telephoneID); err != nil {
				return nil, err
			}
			telephone, err := opuser.FindUserTelephone(ctx, suid, telephoneID)
			if errors.Is(err, mongo.ErrNoDocuments) {
				telephone = &ptransaction.UserTelephone{TelephoneId: telephoneID}
			} else if err != nil {
				return nil, err
			}
			if telephone.GetTelephoneStatusType() == penum.TelephoneStatusType_TelephoneStatusType_Finished {
				continue
			}
			if telephone.UnlockedTime == 0 {
				telephone.UnlockedTime = finishedTime
			}
			if telephone.ScheduledTime == 0 {
				telephone.ScheduledTime = telephone.UnlockedTime
			}
			telephone.TelephoneStatusType = penum.TelephoneStatusType_TelephoneStatusType_Finished
			if err := opuser.UpsertUserTelephone(ctx, suid, telephone); err != nil {
				return nil, err
			}
			updated.Telephones = append(updated.Telephones, telephone)
		}

		if err := opuser.UpsertUserMessageProgress(ctx, &umodel.UserMessageProgress{
			SUID: suid, MessageId: userMessage.MessageId,
			LastMessageDetailId: terminal.GetMessageDetailId(), LastMessageDetailTime: finishedTime,
		}); err != nil {
			return nil, err
		}

		common := viber.NewCommonResponse(updated)
		schedule, err := opuser.FindUserMessageSchedule(ctx, suid, userMessage.MessageId)
		if err == nil {
			if err := opuser.DeleteUserMessageSchedule(ctx, suid, userMessage.MessageId); err != nil {
				return nil, err
			}
			common.DeletedData.MessageSchedules = append(common.DeletedData.MessageSchedules, &ptransaction.UserMessageSchedule{
				MessageId: schedule.GetMessageId(),
			})
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		if err := reconcileNarrativeSchedulesAfterMessage(
			ctx, suid, finishedTime, userMessage.GetMessageId(), updated,
		); err != nil {
			return nil, err
		}
		response = &papi.MessageFinishResponse{IsReliabilityUp: isReliabilityUp, CommonResponse: common}
		return nil, nil
	})
	if err != nil {
		return nil, viber.GameplayError(err)
	}
	return response, nil
}

func reconcileNarrativeSchedulesAfterMessage(
	ctx context.Context,
	suid string,
	now int64,
	messageID string,
	updated *papi.UpdatedData,
) error {
	return viber.ReconcileNarrativeSchedules(ctx, suid, now, updated, viber.NarrativeUnlockFinishedMessage, messageID)
}
