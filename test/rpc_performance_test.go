package test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/protobuf/proto"
)

// Opt-in timings use the real RPC transport and configured test user. They
// deliberately have no wall-clock pass/fail thresholds: host load varies.
func TestRPCPerformanceScenario(t *testing.T) {
	if os.Getenv("SOLIS_RUN_RPC_PERFORMANCE") != "1" {
		t.Skip("set SOLIS_RUN_RPC_PERFORMANCE=1 to measure Home.Enter and Mission.Receive")
	}
	for _, veteran := range []bool{false, true} {
		name := "newbie"
		if veteran {
			name = "god_veteran_like"
		}
		t.Run(name, func(t *testing.T) {
			setupTestUser(t)
			if veteran {
				promoteTestUser(t)
			}
			home := papi.NewHomeClient(testConfig.GrpcConn)
			mission := papi.NewMissionClient(testConfig.GrpcConn)
			enter := func() time.Duration {
				ctx, cancel, err := getFullGrpcCtx()
				if err != nil {
					t.Fatal(err)
				}
				defer cancel()
				start := time.Now()
				response, err := home.Enter(ctx, &papi.HomeEnterRequest{})
				elapsed := time.Since(start)
				if err != nil || response.GetEventInfo() == nil || response.GetCommonResponse() == nil {
					t.Fatalf("Home.Enter failed: %v", err)
				}
				return elapsed
			}
			t.Logf("Home.Enter first: %v", enter())
			times := make([]time.Duration, 5)
			for i := range times {
				times[i] = enter()
			}
			logRPCTimings(t, "Home.Enter repeated", times)
			listed := profileCall(t, mission.List, &papi.Empty{})
			master, initial := selectListedMission(t, listed.GetUserMissions(), penum.MissionType_MissionType_ProfileSetting, penum.MissionCategoryType_MissionCategoryType_Normal)
			for i := range times {
				// Restore only this test user's claimable task between samples;
				// fixture preparation is excluded from the RPC latency.
				row := proto.Clone(initial).(*ptransaction.UserMission)
				row.CurrentThreshold = master.GetProgresses()[0].GetThreshold()
				row.Progress, row.StatusType, row.LastUpdateTime = row.GetCurrentThreshold(), penum.MissionStatusType_MissionStatusType_Clear, time.Now().UnixMilli()
				if err := opuser.UpsertUserMission(context.Background(), testConfig.TestUserSuid, row); err != nil {
					t.Fatal(err)
				}
				ctx, cancel, err := getFullGrpcCtx()
				if err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				response, err := mission.Receive(ctx, &papi.MissionReceiveRequest{MissionIds: []string{master.GetId()}})
				times[i] = time.Since(start)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				assertMissionClaimResources(t, response, master.GetProgresses()[0].GetRewardSetId())
			}
			logRPCTimings(t, "Mission.Receive", times)
		})
	}
}

func logRPCTimings(t *testing.T, name string, times []time.Duration) {
	t.Helper()
	ordered := slices.Sorted(slices.Values(times))
	var total time.Duration
	for _, elapsed := range times {
		total += elapsed
	}
	t.Logf("%s: n=%d mean=%v median=%v min=%v max=%v", name, len(times), total/time.Duration(len(times)), ordered[len(ordered)/2], ordered[0], ordered[len(ordered)-1])
}
