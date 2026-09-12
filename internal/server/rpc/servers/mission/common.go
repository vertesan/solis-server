package mission

import (
	"context"
	"sort"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
)

func missionRows(ctx context.Context, suid string, catalog *viber.ActiveMissionCatalog, eventID string) ([]*ptransaction.UserMission, error) {
	all, err := opuser.FindAllUserMissions(ctx, suid)
	if err != nil {
		return nil, err
	}
	rows := []*ptransaction.UserMission{}
	for _, row := range all {
		if master := catalog.Missions[row.GetMissionId()]; master != nil && master.GetEventMissionId() == eventID {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		// Captured List responses group permanent achievements before daily
		// and weekly tasks; Event responses order their IDs directly.
		if eventID == "" {
			rank := func(id string) int {
				switch catalog.Missions[id].GetCategoryType() {
				case penum.MissionCategoryType_MissionCategoryType_Normal:
					return 0
				case penum.MissionCategoryType_MissionCategoryType_Daily:
					return 1
				case penum.MissionCategoryType_MissionCategoryType_Weekly:
					return 2
				default:
					return 3
				}
			}
			if a, b := rank(rows[i].GetMissionId()), rank(rows[j].GetMissionId()); a != b {
				return a < b
			}
		}
		return rows[i].GetMissionId() < rows[j].GetMissionId()
	})
	return rows, nil
}
