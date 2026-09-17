package test

import (
	"context"
	"math"
	"testing"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestQuestContestRewardScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	q := questStartContestFixture(t)
	number := questStartDeck(t, 1000000)
	client := papi.NewQuestClient(testConfig.GrpcConn)
	passes, err := opmaster.FindQuestTicketItems(ctx, penum.ItemType_ItemType_QuestContestTicket)
	if err != nil || len(passes) != 1 {
		t.Fatalf("contest pass master: %v, %v", passes, err)
	}
	const plays int32 = 3
	passID := passes[0].GetId()
	passAmount := int64(q.GetTicketAmount()) * int64(plays)
	setCardTrainingItem(t, passID, passAmount)
	masterReward, err := opmaster.FindReward(ctx, q.GetClearRewardId())
	if err != nil {
		t.Fatal(err)
	}
	var mandatory, optional *pmaster.RewardResource
	for _, r := range masterReward.GetResources() {
		if r.GetResourceType() == penum.ResourceType_ResourceType_Item {
			if r.GetAmount() > 0 {
				mandatory = r
			} else if r.GetAmount() == 0 {
				optional = r
			}
		}
	}
	if mandatory == nil || optional == nil {
		t.Fatal("contest regression fixture needs fixed and zero-amount item rewards")
	}
	request := &papi.QuestStartRequest{QuestId: q.GetId(), DeckNumber: number, PlayCount: plays, IsSkip: true}
	// Failure after the pass debit and initial gold reward must roll back both.
	setCardTrainingItem(t, mandatory.GetResourceId(), math.MaxInt64)
	setCardTrainingItem(t, optional.GetResourceId(), 0)
	goldBefore, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.Start, request)
	goldAfter, err := opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(goldBefore, goldAfter) || questStartUsablePasses(t, passID) != passAmount ||
		questStartUsablePasses(t, mandatory.GetResourceId()) != math.MaxInt64 || questStartUsablePasses(t, optional.GetResourceId()) != 0 {
		t.Fatal("rejected contest settlement committed partial costs or rewards")
	}
	setCardTrainingItem(t, mandatory.GetResourceId(), 0)
	response := questStartMeasured(t, client, request)
	if !response.GetResult().GetCleared() || questStartUsablePasses(t, passID) != 0 {
		t.Fatal("successful contest did not clear and consume its count-scaled passes")
	}
	var fixedGranted, optionalGranted, goldGranted int64
	for _, r := range response.GetReward().GetClearRewards() {
		if r.GetResourceType() != penum.ResourceType_ResourceType_Item {
			continue
		}
		switch r.GetResourceId() {
		case mandatory.GetResourceId():
			fixedGranted += r.GetAmountI64()
		case optional.GetResourceId():
			optionalGranted += r.GetAmountI64()
		}
	}
	if fixedGranted != int64(mandatory.GetAmount())*int64(plays) || optionalGranted < 0 || optionalGranted > int64(plays) {
		t.Fatalf("invalid contest grants: fixed=%d optional=%d", fixedGranted, optionalGranted)
	}
	if questStartUsablePasses(t, mandatory.GetResourceId()) != fixedGranted || questStartUsablePasses(t, optional.GetResourceId()) != optionalGranted {
		t.Fatal("contest reward response does not match persisted items")
	}
	for _, rewards := range [][]*ptransaction.RewardResult{
		response.GetReward().GetClearRewards(), response.GetReward().GetScoreRankRewards(), response.GetReward().GetScoreRankAdditionalRewards(),
	} {
		for _, reward := range rewards {
			if reward.GetResourceType() == penum.ResourceType_ResourceType_Gold {
				goldGranted += reward.GetAmountI64()
			}
		}
	}
	goldAfter, err = opuser.FindUserPoint(ctx, testConfig.TestUserSuid, penum.PointType_PointType_Gold)
	if err != nil || goldAfter.GetAmount()-goldBefore.GetAmount() != goldGranted {
		t.Fatalf("contest gold was not credited as reported: %v", err)
	}
	stored, err := opuser.FindUserQuest(ctx, testConfig.TestUserSuid, q.GetId())
	if err != nil || stored.GetDailyClearCount() != plays || stored.GetHighestScore() <= 0 {
		t.Fatalf("contest clear was not persisted: %v", err)
	}
	assertQuestStartTrace(t, response)
}

func questStartContestFixture(t *testing.T) *pmaster.Quest {
	t.Helper()
	ctx := context.Background()
	q, err := opmaster.FindQuest(ctx, "qt-area-1-001")
	if err != nil {
		t.Fatal(err)
	}
	condition, err := opmaster.FindCondition(ctx, q.GetUnlockConditionId())
	if err != nil {
		t.Fatal(err)
	}
	areas, _ := questCatalogFixture(t)
	main := questAreasOfType(areas, penum.AreaType_AreaType_Main)
	var required int32
	for _, setting := range condition.GetSettings() {
		for _, area := range main {
			if area.GetId() == setting.GetClearArea().GetAreaId() {
				required = max(required, area.GetOrder())
			}
		}
	}
	for _, area := range main {
		if area.GetOrder() <= required {
			questSeedFullArea(t, area)
		}
	}
	maximum, err := opmaster.FindMaxManagerLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	questSetManagerLevel(t, maximum.GetLevel())
	return q
}
