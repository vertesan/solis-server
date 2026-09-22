package dream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const MonthlyManualAmount int64 = 5
const ManualItemID = "item-dream-generate-dream-25-0320"
const UnlockConditionID = "cd-dream-25-0320-play"

type MonthlyState struct {
	SeasonID       string
	EventMissionID string
	StartTime      int64
	EndTime        int64
	Missions       []*pmaster.Mission
	Group          *pmaster.EventMission
}

// MonthlyPeriod is half-open and anchored at 12:00 JST on the fifteenth.
func MonthlyPeriod(now int64) (start, end int64) {
	jst := time.FixedZone("JST", 9*60*60)
	t := time.UnixMilli(now).In(jst)
	boundary := time.Date(t.Year(), t.Month(), 15, 12, 0, 0, 0, jst)
	if t.Before(boundary) {
		boundary = boundary.AddDate(0, -1, 0)
	}
	return boundary.UnixMilli(), boundary.AddDate(0, 1, 0).UnixMilli()
}

// MonthlyCatalog reuses the newest available monthly template once master data
// runs out. Keeping known IDs allows the permanent event to remain client-safe.
func MonthlyCatalog(ctx context.Context, now int64) (*MonthlyState, error) {
	start, end := MonthlyPeriod(now)
	wanted := "event-dream-" + time.UnixMilli(start).In(time.FixedZone("JST", 9*60*60)).Format("06-01")
	groups, err := opmaster.FindAllEventMissions(ctx)
	if err != nil {
		return nil, err
	}
	var selected *pmaster.EventMission
	for _, group := range groups {
		id := group.GetId()
		if !strings.HasPrefix(id, "event-dream-") || len(id) != len("event-dream-26-09") || id > wanted {
			continue
		}
		if selected == nil || selected.GetId() < id {
			selected = group
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("no monthly Dream mission template is available")
	}
	missions, err := opmaster.FindMissionsByIds(ctx, selected.GetMissionIds())
	if err != nil {
		return nil, err
	}
	return &MonthlyState{SeasonID: strings.TrimPrefix(selected.GetId(), "event-"), EventMissionID: selected.GetId(),
		StartTime: start, EndTime: end, Missions: missions, Group: selected}, nil
}

// ReconcileMonthly must run inside the caller's transaction. The unique state
// key serializes concurrent visits and the gift and mission reset commit together.
// First visits receive the current month's grant, never historical backfill.
func ReconcileMonthly(ctx context.Context, suid string, now int64, updated *papi.UpdatedData) (*MonthlyState, error) {
	catalog, err := MonthlyCatalog(ctx, now)
	if err != nil {
		return nil, err
	}
	state, err := opuser.FindUserDreamState(ctx, suid)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if state != nil && state.PeriodStart >= catalog.StartTime {
		return catalog, nil
	}
	for _, master := range catalog.Missions {
		var threshold int64
		for _, tier := range master.GetProgresses() {
			if tier.GetThreshold() > 0 && (threshold == 0 || tier.GetThreshold() < threshold) {
				threshold = tier.GetThreshold()
			}
		}
		if threshold == 0 {
			continue
		}
		row := &ptransaction.UserMission{MissionId: master.GetId(), CurrentThreshold: threshold,
			StatusType: penum.MissionStatusType_MissionStatusType_Progress, LastUpdateTime: now}
		if err := opuser.UpsertUserMission(ctx, suid, row); err != nil {
			return nil, err
		}
		updated.Missions = append(updated.Missions, row)
	}
	manualID := ManualItemID
	if _, err := opmaster.FindItem(ctx, manualID); err != nil {
		return nil, err
	}
	if err := opuser.UpsertUserGift(ctx, suid, &ptransaction.UserGift{
		GiftId: fmt.Sprintf("dream-monthly-%d", catalog.StartTime), ResourceType: penum.ResourceType_ResourceType_Item,
		ResourceId: manualID, Amount: MonthlyManualAmount, Message: "イメージトレーニング 月間イメトレマニュアル", PostedTime: now,
	}); err != nil {
		return nil, err
	}
	// This package is also used by admin/User.Get, outside the RPC helper
	// package. Preserve other notification kinds and any existing Gift fields.
	var notification *ptransaction.UserNotification
	inUpdated := false
	for _, candidate := range updated.GetNotifications() {
		if candidate.GetNotificationType() == penum.NotificationType_NotificationType_Gift {
			notification, inUpdated = candidate, true
			break
		}
	}
	if notification == nil {
		notification, err = opuser.FindUserNotification(ctx, suid, penum.NotificationType_NotificationType_Gift)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}
	if notification == nil {
		notification = &ptransaction.UserNotification{NotificationType: penum.NotificationType_NotificationType_Gift}
	}
	if !notification.GetValid() {
		notification.Valid = true
		if err := opuser.UpsertUserNotification(ctx, suid, notification); err != nil {
			return nil, err
		}
		if !inUpdated {
			updated.Notifications = append(updated.Notifications, notification)
		}
	}
	if state == nil {
		state = &umodel.UserDreamState{SUID: suid}
	}
	state.PeriodStart, state.EventMissionID = catalog.StartTime, catalog.EventMissionID
	if err := opuser.UpsertUserDreamState(ctx, state); err != nil {
		return nil, err
	}
	return catalog, nil
}
