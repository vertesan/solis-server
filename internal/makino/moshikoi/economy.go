package moshikoi

import (
	"math"

	"github.com/vertesan/solis-server/internal/db/model/user/umodel"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Catalog) LessonProgress(state *umodel.UserLoveState) *papi.LoveLessonProgress {
	anchor := state.LastLessonRewardTime
	if anchor == 0 {
		anchor = c.Config.StartTime
	}
	return &papi.LoveLessonProgress{LastReceiveTime: anchor, MaxRewardTime: anchor + c.Config.LessonMaxHours*60*60*1000}
}

// LessonReward grants each claim at the level before claiming, matching the
// captured official response when a claim crosses a level boundary. A drink
// grants only its two hours and does not consume the ordinary elapsed rewards.
func (c *Catalog) LessonReward(state *umodel.UserLoveState, now int64, promote bool) (exp, points int64, err error) {
	step := c.Config.LessonStepSeconds * 1000
	anchor := c.LessonProgress(state).GetLastReceiveTime()
	if step <= 0 {
		return 0, 0, status.Error(codes.FailedPrecondition, "invalid love lesson interval")
	}
	steps := max(int64(0), min(now-anchor, c.Config.LessonMaxHours*60*60*1000)) / step
	if promote {
		steps = c.Config.PromoteMinutes * 60 * 1000 / step
	}
	level := c.LevelData(state.Exp)
	if steps > 0 && (level.GetLessonStepRewardExp() > math.MaxInt64/steps || level.GetLessonStepRewardStoryReadItemAmount() > math.MaxInt64/steps) {
		return 0, 0, status.Error(codes.OutOfRange, "love lesson reward overflow")
	}
	exp = steps * level.GetLessonStepRewardExp()
	points = steps * level.GetLessonStepRewardStoryReadItemAmount()
	if state.Exp > math.MaxInt64-exp {
		return 0, 0, status.Error(codes.OutOfRange, "love experience overflow")
	}
	state.Exp += exp
	if !promote && now >= anchor {
		state.LastLessonRewardTime = now - (now-anchor)%step
	}
	return exp, points, nil
}
