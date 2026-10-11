package test

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opsolis"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	usercard "github.com/vertesan/solis-server/internal/makino/user/card"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGachaScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)
	client := papi.NewGachaClient(testConfig.GrpcConn)
	userClient := papi.NewUserClient(testConfig.GrpcConn)
	homeClient := papi.NewHomeClient(testConfig.GrpcConn)

	grpcCtx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	userBeforeList, err := userClient.Get(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("User.Get before Gacha.List failed: %v", err)
	}
	if len(userBeforeList.GetGachaButtons()) == 0 {
		t.Fatal("User.Get before Gacha.List returned no current gacha buttons")
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	homeBeforeList, err := homeClient.Enter(grpcCtx, &papi.HomeEnterRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Home.Enter before Gacha.List failed: %v", err)
	}
	if len(homeBeforeList.GetGachaInfos()) == 0 {
		t.Fatal("Home.Enter before Gacha.List returned no current gacha catalog")
	}
	if !sameStrings(testUserGachaButtonIDs(userBeforeList.GetGachaButtons()), testGachaInfoButtonIDs(homeBeforeList.GetGachaInfos())) {
		t.Fatalf("User.Get and Home.Enter current gacha buttons differ: user=%v home=%v", testUserGachaButtonIDs(userBeforeList.GetGachaButtons()), testGachaInfoButtonIDs(homeBeforeList.GetGachaInfos()))
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.List(grpcCtx, &papi.Empty{})
	cancel()
	if err != nil {
		t.Fatalf("Gacha.List failed: %v", err)
	}
	if list.GetCommonResponse() == nil || len(list.GetGachas()) == 0 {
		t.Fatalf("Gacha.List returned no current catalog: %v", list)
	}
	notices := testGachaNoticeCatalog(t)
	homeNotices := make(map[string]*papi.NoticeInfo, len(homeBeforeList.GetGachaInfos()))
	for _, info := range homeBeforeList.GetGachaInfos() {
		homeNotices[info.GetId()] = info.GetNoticeInfo()
	}
	if !sameStrings(testGachaInfoIDs(homeBeforeList.GetGachaInfos()), testGachaInfoIDs(list.GetGachas())) {
		t.Fatalf("Home.Enter and Gacha.List current roster differ: home=%v list=%v", testGachaInfoIDs(homeBeforeList.GetGachaInfos()), testGachaInfoIDs(list.GetGachas()))
	}
	for _, info := range list.GetGachas() {
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil {
			t.Fatalf("Gacha.List returned unknown gacha %q: %v", info.GetId(), err)
		}
		if !sameStrings(gachaButtonIDs(info.GetButtons()), masterGacha.GetButtonIds()) {
			t.Fatalf("Gacha.List buttons are not master-derived for %q: got=%v want=%v", info.GetId(), gachaButtonIDs(info.GetButtons()), masterGacha.GetButtonIds())
		}
		stored, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil || !proto.Equal(stored, info) {
			t.Fatalf("Gacha.List did not persist rendered state for %q: stored=%v response=%v err=%v", info.GetId(), stored, info, err)
		}
		expectedNoticeID := testGachaNoticeID(info.GetId())
		if expectedNoticeID == "" {
			exactID := "info-" + info.GetId()
			if _, exists := notices[exactID]; exists {
				expectedNoticeID = exactID
			}
		}
		if expected := notices[expectedNoticeID]; expected != nil {
			if !proto.Equal(info.GetNoticeInfo(), expected) {
				t.Fatalf("Gacha.List changed catalog notice %q for %q: %v", expectedNoticeID, info.GetId(), info.GetNoticeInfo())
			}
		} else {
			assertDummyNoticeInfo(t, "Gacha.List "+info.GetId(), info.GetNoticeInfo())
		}
		if !proto.Equal(homeNotices[info.GetId()], info.GetNoticeInfo()) {
			t.Fatalf("Home.Enter and Gacha.List notice differ for %q", info.GetId())
		}
	}
	assertGuaranteedRarityGachaCatalogs(t, client, list.GetGachas())
	assertPremiumGachaProbability(t, client, list.GetGachas())
	assertGachaEvolutionMasterRules(t)

	t.Run("GuaranteedRarityDraw", func(t *testing.T) {
		info, masterGacha, button := chooseMasterCardRewardGacha(t, list.GetGachas())
		if info == nil {
			t.Skip("current visible gacha catalog has no executable master-card-reward gacha")
		}
		cards, err := opuser.FindAllUserCards(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		cardBefore := make(map[string]*ptransaction.UserCard, len(cards))
		for _, card := range cards {
			cardBefore[card.GetCardId()] = card
		}
		missionBefore := testGachaMissionSnapshot(t)
		giftBefore := testPendingGachaGiftAmount(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
		notifiedBefore := testGachaGiftNotified(t)
		buttonBefore := findTestGachaButton(info, button.GetId())
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: 1})
		cancel()
		if err != nil {
			t.Fatalf("guaranteed-rarity Gacha.Draw failed: %v", err)
		}
		poolRule := testGuaranteedRarityPool(info.GetId())
		for _, reward := range response.GetRewards() {
			card, err := opmaster.FindCard(context.Background(), reward.GetResourceId())
			if err != nil {
				t.Fatal(err)
			}
			if reward.GetResourceType() != penum.ResourceType_ResourceType_Card || card.GetInitialRarity() < poolRule.MinimumRarity {
				t.Fatalf("guaranteed-rarity draw returned ineligible reward: %v card=%v", reward, card)
			}
		}
		if len(response.GetCardRewards()) == 0 {
			t.Fatalf("master-card-reward gacha %q returned no CardRewards", info.GetId())
		}
		for _, reward := range response.GetCardRewards() {
			if reward.GetResourceType() != penum.ResourceType_ResourceType_CardRarityExpLimitChange || reward.GetAmount() <= 0 {
				t.Fatalf("master-card-reward gacha returned unexpected CardReward metadata: %v", reward)
			}
		}
		assertGachaDuplicateCardProgress(t, cardBefore, missionBefore, giftBefore, notifiedBefore, response)
		assertGachaDrawRewardClassification(t, masterGacha, button, buttonBefore.GetTotalCount(), 1, response)
		if response.GetGachaMovie() != nil {
			t.Fatalf("guaranteed-rarity draw returned unobserved gachaMovie data: %v", response.GetGachaMovie())
		}
	})

	drawInfo, drawMaster, drawButton, drawConsumption := chooseGachaDrawTarget(t, list.GetGachas())
	t.Logf("draw target: gacha=%s button=%s type=%s", drawInfo.GetId(), drawButton.GetId(), drawMaster.GetGachaType())
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	probability, err := client.Probability(grpcCtx, &papi.GachaProbabilityRequest{GachaId: drawInfo.GetId()})
	cancel()
	if err != nil {
		t.Fatalf("Gacha.Probability failed: %v", err)
	}
	if probability.GetCommonResponse() == nil || len(probability.GetProbabilityInfos()) == 0 {
		t.Fatalf("Gacha.Probability returned no master-derived pool: %v", probability)
	}
	var probabilityTotal int64
	for _, entry := range probability.GetProbabilityInfos() {
		probabilityTotal += int64(entry.GetProbability())
	}
	if probabilityTotal != 100000 {
		t.Fatalf("Gacha.Probability total = %d, want 100000", probabilityTotal)
	}

	buttonBefore := findTestGachaButton(drawInfo, drawButton.GetId())
	if buttonBefore == nil {
		t.Fatalf("missing selected button state %q", drawButton.GetId())
	}
	costType, costID, costAmount := testGachaConsumption(drawConsumption)
	resourceBefore := testGachaResourceAmount(t, costType, costID)
	exchangeBefore := int64(0)
	if drawMaster.GetExchangeResourceType() == penum.ResourceType_ResourceType_Item && drawMaster.GetExchangeResourceId() != "" {
		exchangeBefore = testGachaResourceAmount(t, drawMaster.GetExchangeResourceType(), drawMaster.GetExchangeResourceId())
	}
	premiumRemainingBefore := int32(0)
	if drawMaster.GetGachaType() == penum.GachaType_GachaType_Premium {
		premiumRemainingBefore = drawInfo.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount()
	}
	cardsBeforeDraw, err := opuser.FindAllUserCards(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	cardBeforeDraw := make(map[string]*ptransaction.UserCard, len(cardsBeforeDraw))
	for _, card := range cardsBeforeDraw {
		cardBeforeDraw[card.GetCardId()] = card
	}
	missionsBeforeDraw := testGachaMissionSnapshot(t)
	limitBreakGiftBefore := testPendingGachaGiftAmount(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
	notifiedBeforeDraw := testGachaGiftNotified(t)
	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	draw, err := client.Draw(grpcCtx, &papi.GachaDrawRequest{GachaButtonId: drawButton.GetId(), ExecCount: 1})
	cancel()
	if err != nil {
		t.Fatalf("Gacha.Draw failed: %v", err)
	}
	if draw.GetCommonResponse() == nil || len(draw.GetRewards()) != int(drawButton.GetDrawCount()) || len(draw.GetRewardResults()) == 0 {
		t.Fatalf("Gacha.Draw returned incomplete results: %v", draw)
	}
	assertGachaRewardResultsCanonical(t, draw.GetRewardResults())
	assertGachaDuplicateCardProgress(t, cardBeforeDraw, missionsBeforeDraw, limitBreakGiftBefore, notifiedBeforeDraw, draw)
	assertGachaDrawRewardClassification(t, drawMaster, drawButton, buttonBefore.GetTotalCount(), 1, draw)
	if draw.GetGachaMovie() != nil {
		t.Fatalf("Gacha.Draw returned unobserved gachaMovie data: %v", draw.GetGachaMovie())
	}
	if draw.GetButton().GetTodayCount() != buttonBefore.GetTodayCount()+1 || draw.GetButton().GetTotalCount() != buttonBefore.GetTotalCount()+1 {
		t.Fatalf("Gacha.Draw counter mismatch: before=%v after=%v", buttonBefore, draw.GetButton())
	}
	if costAmount > 0 {
		resourceAfter := testGachaResourceAmount(t, costType, costID)
		if resourceAfter != resourceBefore-costAmount {
			t.Fatalf("Gacha.Draw did not persist consumption: type=%v id=%q before=%d cost=%d after=%d", costType, costID, resourceBefore, costAmount, resourceAfter)
		}
	}
	if drawMaster.GetExchangeResourceType() == penum.ResourceType_ResourceType_Item && drawMaster.GetExchangeResourceId() != "" {
		exchangeAfter := testGachaResourceAmount(t, drawMaster.GetExchangeResourceType(), drawMaster.GetExchangeResourceId())
		if exchangeAfter-exchangeBefore != int64(drawButton.GetDrawCount()) {
			t.Fatalf("Gacha.Draw did not grant master exchange currency: before=%d after=%d draws=%d", exchangeBefore, exchangeAfter, drawButton.GetDrawCount())
		}
	}
	storedAfterDraw, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, drawInfo.GetId())
	if err != nil {
		t.Fatal(err)
	}
	storedButton := findTestGachaButton(storedAfterDraw, drawButton.GetId())
	if storedButton == nil || !proto.Equal(storedButton, draw.GetButton()) {
		t.Fatalf("Gacha.Draw button state was not persisted: stored=%v response=%v", storedButton, draw.GetButton())
	}
	if drawMaster.GetGachaType() == penum.GachaType_GachaType_Premium {
		expected := premiumRemainingBefore
		premiumRewardID := drawInfo.GetPremiumInfo().GetFeaturedRewards()[0].GetResourceId()
		for _, reward := range draw.GetRewards() {
			expected--
			if reward.GetResourceId() == premiumRewardID {
				expected = 200
			}
		}
		if storedAfterDraw.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount() != expected {
			t.Fatalf("premium guarantee countdown mismatch: before=%d expected=%d after=%d", premiumRemainingBefore, expected, storedAfterDraw.GetPremiumInfo().GetFixedPremiumRewardRequiredDrawCount())
		}
	}

	grpcCtx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	history, err := client.History(grpcCtx, &papi.GachaHistoryRequest{GachaType: drawMaster.GetGachaType()})
	cancel()
	if err != nil {
		t.Fatalf("Gacha.History failed: %v", err)
	}
	if history.GetCommonResponse() == nil || len(history.GetHistories()) < len(draw.GetRewards()) {
		t.Fatalf("Gacha.History omitted the persisted draw: %v", history)
	}
	for i := 0; i < len(draw.GetRewards()); i++ {
		want := draw.GetRewards()[len(draw.GetRewards())-1-i]
		if history.GetHistories()[i].GetGachaId() != drawInfo.GetId() ||
			history.GetHistories()[i].GetResourceType() != want.GetResourceType() ||
			history.GetHistories()[i].GetResourceId() != want.GetResourceId() {
			t.Fatalf("Gacha.History contains unexpected newest draw: %v", history.GetHistories()[i])
		}
	}

	t.Run("DuplicateCardOverflow", func(t *testing.T) {
		maxRarity, err := opmaster.FindMaxCardRarity(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if maxRarity.GetRarity() <= 1 || maxRarity.GetRequiredExp() <= 1 {
			t.Fatalf("invalid max card rarity master data: %v", maxRarity)
		}
		cards, err := opuser.FindAllUserCards(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		before := make(map[string]*ptransaction.UserCard, len(cards))
		for _, card := range cards {
			card.Rarity = maxRarity.GetRarity() - 1
			card.RarityTotalExp = maxRarity.GetRequiredExp() - 1
			if err := opuser.UpsertUserCard(context.Background(), testConfig.TestUserSuid, card); err != nil {
				t.Fatal(err)
			}
			before[card.GetCardId()] = proto.Clone(card).(*ptransaction.UserCard)
		}
		missionsBefore := testGachaMissionSnapshot(t)
		giftBefore, giftCountBefore := testPendingGachaGifts(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
		notifiedBefore := testGachaGiftNotified(t)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		overflowDraw, err := client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: drawButton.GetId(), ExecCount: 1})
		cancel()
		if err != nil {
			t.Fatalf("overflow Gacha.Draw failed: %v", err)
		}
		assertGachaRewardResultsCanonical(t, overflowDraw.GetRewardResults())
		assertGachaDuplicateCardProgress(t, before, missionsBefore, giftBefore, notifiedBefore, overflowDraw)
		var overflowAmount int64
		for _, result := range overflowDraw.GetRewardResults() {
			if result.GetResourceType() != penum.ResourceType_ResourceType_Card {
				continue
			}
			for _, gift := range result.GetAdditionalGifts() {
				if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetResourceId() == "item-card_limit_break" {
					overflowAmount += gift.GetAmount()
				}
			}
		}
		if overflowAmount <= 0 {
			t.Fatal("max-rarity duplicate draw returned no item-card_limit_break overflow gifts")
		}
		_, giftCountAfter := testPendingGachaGifts(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
		distinctOverflowCards := 0
		for _, result := range overflowDraw.GetRewardResults() {
			if result.GetResourceType() != penum.ResourceType_ResourceType_Card {
				continue
			}
			for _, gift := range result.GetAdditionalGifts() {
				if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetResourceId() == "item-card_limit_break" && gift.GetAmount() > 0 {
					distinctOverflowCards++
					break
				}
			}
		}
		if giftCountAfter-giftCountBefore != distinctOverflowCards {
			t.Fatalf("overflow mailbox rows increased by %d, want one per distinct card (%d)", giftCountAfter-giftCountBefore, distinctOverflowCards)
		}
	})

	t.Run("SelectCard", func(t *testing.T) {
		info, master := chooseSelectableGacha(t, list.GetGachas())
		if info == nil {
			t.Skip("current visible gacha catalog has no card-selection gacha")
		}
		selected := append([]string(nil), master.GetSelectCardIds()[:master.GetSelectPickupAmount()]...)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.SelectCard(ctx, &papi.GachaSelectCardRequest{GachaId: info.GetId(), SelectedCardIds: selected})
		cancel()
		if err != nil {
			t.Fatalf("Gacha.SelectCard failed: %v", err)
		}
		stored, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil || !sameStrings(stored.GetSelectedCardIds(), selected) {
			t.Fatalf("Gacha.SelectCard was not persisted: selected=%v stored=%v err=%v", selected, stored, err)
		}
	})

	t.Run("Continuous", func(t *testing.T) {
		info, button := chooseContinuousGacha(t, list.GetGachas())
		if info == nil {
			t.Skip("current visible gacha catalog has no continuous gacha")
		}
		if info.GetContinuousInfo() == nil {
			t.Fatalf("continuous gacha %q has no persistent state", info.GetId())
		}
		total := info.GetContinuousInfo().GetTotalDrawCount()
		buttonTotal := findTestGachaButton(info, button.GetId()).GetTotalCount()
		for _, execCount := range []int32{1, 10} {
			ctx, cancel, err := getFullGrpcCtx()
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: execCount})
			cancel()
			if err != nil {
				t.Fatalf("continuous Gacha.Draw(execCount=%d) failed: %v", execCount, err)
			}
			if len(response.GetRewards()) != int(button.GetDrawCount()) {
				t.Fatalf("continuous draw returned %d rewards, want button drawCount %d", len(response.GetRewards()), button.GetDrawCount())
			}
			assertGachaCardBatchGuarantee(t, response.GetRewards(), button.GetDrawCount())
			total += button.GetDrawCount()
			buttonTotal++
			if response.GetContinuousResult().GetTotalDrawCount() != total {
				t.Fatalf("continuous cumulative total = %d, want %d", response.GetContinuousResult().GetTotalDrawCount(), total)
			}
			if response.GetButton().GetTotalCount() != buttonTotal {
				t.Fatalf("continuous button total = %d, want %d", response.GetButton().GetTotalCount(), buttonTotal)
			}
			historyCtx, historyCancel, err := getFullGrpcCtx()
			if err != nil {
				t.Fatal(err)
			}
			historyResponse, err := client.History(historyCtx, &papi.GachaHistoryRequest{GachaType: penum.GachaType_GachaType_Continuous})
			historyCancel()
			if err != nil {
				t.Fatalf("continuous Gacha.History failed: %v", err)
			}
			if len(historyResponse.GetHistories()) < len(response.GetRewards()) {
				t.Fatalf("continuous history contains %d rows, want at least %d", len(historyResponse.GetHistories()), len(response.GetRewards()))
			}
			for i := range response.GetRewards() {
				want := response.GetRewards()[len(response.GetRewards())-1-i]
				got := historyResponse.GetHistories()[i]
				if got.GetResourceType() != want.GetResourceType() || got.GetResourceId() != want.GetResourceId() {
					t.Fatalf("continuous history[%d] = %v, want reverse draw reward %v", i, got, want)
				}
			}
		}
		stored, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil || stored.GetContinuousInfo().GetTotalDrawCount() != total {
			t.Fatalf("continuous state was not persisted: stored=%v err=%v", stored.GetContinuousInfo(), err)
		}
	})

	t.Run("ExpiringItemConsumptionStacks", func(t *testing.T) {
		info, button, consumption, execCount := chooseItemConsumptionGacha(t, list.GetGachas())
		if info == nil {
			t.Skip("current visible gacha catalog has no multi-execution item-consumption button")
		}
		resource := consumption.GetResources()[0]
		itemID := resource.GetResourceIds()[0]
		totalCost := int64(resource.GetAmount()) * int64(execCount)
		now := time.Now().UnixMilli()
		for i := int64(0); i < min(totalCost, 3); i++ {
			item := &ptransaction.UserItem{ItemId: itemID, Amount: 1, ExpiredTime: now + 60_000 + i}
			if err := opuser.UpsertUserItem(context.Background(), testConfig.TestUserSuid, item); err != nil {
				t.Fatal(err)
			}
		}
		stacks, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, itemID)
		if err != nil {
			t.Fatal(err)
		}
		expected := expectedGachaItemStackConsumption(stacks, totalCost, now)
		if len(expected) < 2 {
			t.Skip("could not prepare multiple usable expiring item stacks")
		}
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: execCount})
		cancel()
		if err != nil {
			t.Fatalf("item-stack Gacha.Draw failed for %q: %v", info.GetId(), err)
		}
		if len(response.GetConsumptionResults()) != len(expected) {
			t.Fatalf("item-stack Gacha.Draw returned %d consumption results, want %d: %v", len(response.GetConsumptionResults()), len(expected), response.GetConsumptionResults())
		}
		for i, want := range expected {
			if !proto.Equal(response.GetConsumptionResults()[i], want.result) {
				t.Fatalf("item-stack consumption result[%d] = %v, want %v", i, response.GetConsumptionResults()[i], want.result)
			}
			updatedItem := findTestUpdatedGachaItem(response.GetCommonResponse().GetUpdatedData(), itemID, want.expiredTime)
			if updatedItem == nil || updatedItem.GetAmount() != want.result.GetAfterAmount() {
				t.Fatalf("item-stack post-image missing for expiry %d: updated=%v result=%v", want.expiredTime, updatedItem, want.result)
			}
		}
	})

	t.Run("ReceiveFreePremiumTicket", func(t *testing.T) {
		premium := choosePremiumGacha(list.GetGachas())
		if premium == nil {
			t.Skip("current visible gacha catalog has no receivable premium ticket")
		}
		itemID := premium.GetPremiumInfo().GetFreeGachaTicketItemId()
		before := testGachaResourceAmount(t, penum.ResourceType_ResourceType_Item, itemID)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.ReceiveFreePremiumGachaTicket(ctx, &papi.GachaReceiveFreePremiumGachaTicketRequest{GachaId: premium.GetId()})
		cancel()
		if err != nil {
			t.Fatalf("Gacha.ReceiveFreePremiumGachaTicket failed: %v", err)
		}
		if response.GetCommonResponse() == nil || len(response.GetRewardResults()) != 1 {
			t.Fatalf("Gacha.ReceiveFreePremiumGachaTicket returned incomplete response: %v", response)
		}
		after := testGachaResourceAmount(t, penum.ResourceType_ResourceType_Item, itemID)
		if after-before != response.GetRewardResults()[0].GetAmountI64() {
			t.Fatalf("premium ticket grant was not persisted: before=%d after=%d result=%v", before, after, response.GetRewardResults()[0])
		}
		ctx, cancel, err = getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.ReceiveFreePremiumGachaTicket(ctx, &papi.GachaReceiveFreePremiumGachaTicketRequest{GachaId: premium.GetId()})
		cancel()
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("second premium ticket claim returned %v, want FailedPrecondition", err)
		}
		if got := testGachaResourceAmount(t, penum.ResourceType_ResourceType_Item, itemID); got != after {
			t.Fatalf("rejected premium ticket claim changed item amount: before=%d after=%d", after, got)
		}
	})

	t.Run("DiscountBoundary", func(t *testing.T) {
		info, button, consumption := chooseDiscountBoundary(t, list.GetGachas())
		if button == nil {
			t.Skip("current visible gacha catalog has no unused discount boundary")
		}
		state := findTestGachaButton(info, button.GetId())
		var current int32
		if button.GetDiscountLimitType() == penum.GachaLimitType_GachaLimitType_Daily {
			current = state.GetTodayCount()
		} else {
			current = state.GetTotalCount()
		}
		execCount := button.GetDiscountCount() - current + 1
		costType, costID, _ := testGachaConsumption(consumption)
		beforeResource := testGachaResourceAmount(t, costType, costID)
		beforeState := proto.Clone(state).(*papi.GachaButtonInfo)
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Draw(ctx, &papi.GachaDrawRequest{GachaButtonId: button.GetId(), ExecCount: execCount})
		cancel()
		if status.Code(err) != codes.InvalidArgument && status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("discount-boundary draw returned %v, want InvalidArgument or stricter button limit", err)
		}
		afterInfo, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil {
			t.Fatal(err)
		}
		if afterState := findTestGachaButton(afterInfo, button.GetId()); !proto.Equal(afterState, beforeState) {
			t.Fatalf("rejected discount-boundary draw changed button state: before=%v after=%v", beforeState, afterState)
		}
		if got := testGachaResourceAmount(t, costType, costID); got != beforeResource {
			t.Fatalf("rejected discount-boundary draw consumed resources: before=%d after=%d", beforeResource, got)
		}
	})
}

func assertPremiumGachaProbability(t *testing.T, client papi.GachaClient, infos []*papi.GachaInfo) {
	t.Helper()
	for _, info := range infos {
		if info.GetId() != "gacha-premium-26-0724" || !info.GetUnlocked() {
			continue
		}
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Probability(ctx, &papi.GachaProbabilityRequest{GachaId: info.GetId()})
		cancel()
		if err != nil {
			t.Fatalf("premium Gacha.Probability failed: %v", err)
		}
		var total int64
		lastProbability := int32(0)
		for _, probability := range response.GetProbabilityInfos() {
			total += int64(probability.GetProbability())
			if probability.GetResourceId() == "item-enjoy-present-general-02-01" {
				lastProbability = probability.GetProbability()
			}
		}
		if total != 99994 || lastProbability != 10767 {
			t.Fatalf("premium probability rounding mismatch: total=%d final=%d", total, lastProbability)
		}
		return
	}
}

func TestGachaDuplicateEvolutionScenario(t *testing.T) {
	setupTestUser(t)
	promoteTestUser(t)

	masterCard, ownedCard, levels := chooseTestGachaEvolutionCard(t)
	initialLevel := levels[0]
	card := proto.Clone(ownedCard).(*ptransaction.UserCard)
	card.EvolutionLevel = initialLevel.GetLevel()
	card.EvolutionLevelTotalExp = int64(initialLevel.GetExp())
	card.Supported = false
	card.Level = card.GetBaseLevel()

	maxRarity, err := opmaster.FindMaxCardRarity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	card.Rarity = maxRarity.GetRarity()
	card.RarityTotalExp = maxRarity.GetRequiredExp()
	attributes, err := usercard.CalcUserCardBaseAttr(context.Background(), masterCard, card.GetBaseLevel(), card.GetRarity())
	if err != nil {
		t.Fatal(err)
	}
	card.BaseVocal, card.BaseDance = attributes.Vocal, attributes.Dance
	card.BaseVisual, card.BaseStamina = attributes.Visual, attributes.Stamina
	card.Vocal, card.Dance = attributes.Vocal, attributes.Dance
	card.Visual, card.Stamina = attributes.Visual, attributes.Stamina
	photo, skills, abilities, err := usercard.CalcUnlocksWithEvolution(
		context.Background(), card.GetBaseLevel(), card.GetEvolutionLevel(), masterCard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if skills[3] != "" {
		t.Fatalf("initial evolution level unexpectedly unlocked skill4 for %q: %q", masterCard.GetId(), skills[3])
	}
	card.BasePhotoEquipableCount, card.PhotoEquipableCount = photo, photo
	card.BaseSkillId1, card.BaseSkillId2 = skills[0], skills[1]
	card.BaseSkillId3, card.BaseSkillId4 = skills[2], skills[3]
	card.SkillId1, card.SkillId2 = skills[0], skills[1]
	card.SkillId3, card.SkillId4 = skills[2], skills[3]
	card.BaseLiveAbilityId, card.BaseActivityAbilityId = abilities[0], abilities[1]
	card.LiveAbilityId, card.ActivityAbilityId = abilities[0], abilities[1]
	card.DisplayType = penum.CardDisplayType_CardDisplayType_Unknown
	if err := opuser.UpsertUserCard(context.Background(), testConfig.TestUserSuid, card); err != nil {
		t.Fatal(err)
	}

	expected := proto.Clone(card).(*ptransaction.UserCard)
	increasedAfterLevel, err := usercard.ApplyDuplicateEvolution(context.Background(), masterCard, expected)
	if err != nil {
		t.Fatal(err)
	}
	if increasedAfterLevel <= initialLevel.GetLevel() {
		t.Fatalf("one master duplicate did not cross evolution level for %q: levels=%v expected=%v", masterCard.GetId(), levels, expected)
	}
	evolutions, err := opmaster.FindCardEvolutions(
		context.Background(), masterCard.GetId(), initialLevel.GetLevel(), expected.GetEvolutionLevel(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(evolutions) == 0 {
		t.Fatalf("evolution card %q has no crossed CardEvolution reward", masterCard.GetId())
	}
	expectedCostumes := make(map[string]struct{})
	expectedHairs := make(map[string]struct{})
	for _, evolution := range evolutions {
		if costumeID := evolution.GetRewardCostumeId(); costumeID != "" {
			expectedCostumes[costumeID] = struct{}{}
			if err := opuser.DeleteUserCostume(context.Background(), testConfig.TestUserSuid, costumeID); err != nil {
				t.Fatal(err)
			}
			costume, err := opmaster.FindCostume(context.Background(), costumeID)
			if err != nil {
				t.Fatal(err)
			}
			if costume.GetDefaultHairId() != "" {
				expectedHairs[costume.GetDefaultHairId()] = struct{}{}
			}
		}
		if hairID := evolution.GetRewardHairId(); hairID != "" {
			expectedHairs[hairID] = struct{}{}
		}
	}
	if len(expectedCostumes) == 0 {
		t.Fatalf("evolution card %q has no costume reward in the crossed master rows", masterCard.GetId())
	}
	if len(expectedHairs) == 0 {
		t.Fatalf("evolution card %q has no direct or costume-default hair reward in the crossed master rows", masterCard.GetId())
	}
	costumeIDs := make([]string, 0, len(expectedCostumes))
	for costumeID := range expectedCostumes {
		costumeIDs = append(costumeIDs, costumeID)
	}
	hairIDs := make([]string, 0, len(expectedHairs))
	for hairID := range expectedHairs {
		hairIDs = append(hairIDs, hairID)
	}
	sort.Strings(costumeIDs)
	sort.Strings(hairIDs)
	t.Logf("evolution fixture card=%s costumes=%v hairs=%v", masterCard.GetId(), costumeIDs, hairIDs)
	for hairID := range expectedHairs {
		if err := opuser.DeleteUserHair(context.Background(), testConfig.TestUserSuid, hairID); err != nil {
			t.Fatal(err)
		}
	}

	giftID := uuid.NewString()
	if err := opuser.UpsertUserGift(context.Background(), testConfig.TestUserSuid, &ptransaction.UserGift{
		GiftId:       giftID,
		ResourceType: penum.ResourceType_ResourceType_Card,
		ResourceId:   masterCard.GetId(),
		Amount:       1,
		Message:      "gacha duplicate evolution integration scenario",
		PostedTime:   time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel, err := getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	response, err := papi.NewGiftClient(testConfig.GrpcConn).Receive(ctx, &papi.GiftReceiveRequest{GiftIds: []string{giftID}})
	cancel()
	if err != nil {
		t.Fatalf("Gift.Receive evolution duplicate failed: %v", err)
	}
	if !sameStrings(response.GetReceivedGiftIds(), []string{giftID}) {
		t.Fatalf("Gift.Receive did not consume evolution duplicate fixture: %v", response)
	}
	cardResult := findTestGachaRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Card, masterCard.GetId())
	if cardResult == nil || cardResult.GetIncreasedAfterEvolutionLevel() != increasedAfterLevel {
		t.Fatalf("evolution duplicate result = %v, want increasedAfterEvolutionLevel %d", cardResult, increasedAfterLevel)
	}
	stored, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, masterCard.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetEvolutionLevel() != expected.GetEvolutionLevel() ||
		stored.GetEvolutionLevelTotalExp() != expected.GetEvolutionLevelTotalExp() {
		t.Fatalf("evolution duplicate was not persisted: stored=%v expected=%v", stored, expected)
	}
	if stored.GetDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
		t.Fatalf("evolution-enabled card display type changed to %v, want preserved Unknown", stored.GetDisplayType())
	}
	assertGachaEvolutionUnlocks(t, masterCard, stored)
	if stored.GetSkillId4() != masterCard.GetSkillId4() || stored.GetBaseSkillId4() != masterCard.GetSkillId4() {
		t.Fatalf("evolution duplicate did not unlock skill4 for %q: stored=%v masterSkill4=%q", masterCard.GetId(), stored, masterCard.GetSkillId4())
	}
	_, ordinarySkills, _, err := usercard.CalcUnlocks(context.Background(), stored.GetBaseLevel(), masterCard)
	if err != nil {
		t.Fatal(err)
	}
	for slot := 0; slot < 3; slot++ {
		baseIDs := []string{stored.GetBaseSkillId1(), stored.GetBaseSkillId2(), stored.GetBaseSkillId3()}
		effectiveIDs := []string{stored.GetSkillId1(), stored.GetSkillId2(), stored.GetSkillId3()}
		if baseIDs[slot] != ordinarySkills[slot] || effectiveIDs[slot] != ordinarySkills[slot] {
			t.Fatalf(
				"evolution duplicate backfilled ordinary skill slot %d for %q: base=%q effective=%q ordinary=%q",
				slot+1, masterCard.GetId(), baseIDs[slot], effectiveIDs[slot], ordinarySkills[slot],
			)
		}
	}
	for costumeID := range expectedCostumes {
		if _, err := opuser.FindUserCostume(context.Background(), testConfig.TestUserSuid, costumeID); err != nil {
			t.Fatalf("evolution reward costume %q was not persisted: %v", costumeID, err)
		}
		if findTestGachaRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Costume, costumeID) == nil {
			t.Fatalf("evolution reward costume %q was omitted from results: %v", costumeID, response.GetResults())
		}
		if findTestUpdatedGachaCostume(response.GetCommonResponse().GetUpdatedData(), costumeID) == nil {
			t.Fatalf("evolution reward costume %q was omitted from updated data: %v", costumeID, response.GetCommonResponse().GetUpdatedData())
		}
	}
	for hairID := range expectedHairs {
		if _, err := opuser.FindUserHair(context.Background(), testConfig.TestUserSuid, hairID); err != nil {
			t.Fatalf("evolution reward hair %q was not persisted: %v", hairID, err)
		}
		if findTestGachaRewardResult(response.GetResults(), penum.ResourceType_ResourceType_Hair, hairID) == nil {
			t.Fatalf("evolution reward hair %q was omitted from results: %v", hairID, response.GetResults())
		}
		if findTestUpdatedGachaHair(response.GetCommonResponse().GetUpdatedData(), hairID) == nil {
			t.Fatalf("evolution reward hair %q was omitted from updated data: %v", hairID, response.GetCommonResponse().GetUpdatedData())
		}
	}

	maxGiftID := uuid.NewString()
	if err := opuser.UpsertUserGift(context.Background(), testConfig.TestUserSuid, &ptransaction.UserGift{
		GiftId:       maxGiftID,
		ResourceType: penum.ResourceType_ResourceType_Card,
		ResourceId:   masterCard.GetId(),
		Amount:       1,
		Message:      "gacha max evolution integration scenario",
		PostedTime:   time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel, err = getFullGrpcCtx()
	if err != nil {
		t.Fatal(err)
	}
	maxResponse, err := papi.NewGiftClient(testConfig.GrpcConn).Receive(ctx, &papi.GiftReceiveRequest{GiftIds: []string{maxGiftID}})
	cancel()
	if err != nil {
		t.Fatalf("Gift.Receive max-evolution duplicate failed: %v", err)
	}
	maxCardResult := findTestGachaRewardResult(maxResponse.GetResults(), penum.ResourceType_ResourceType_Card, masterCard.GetId())
	if maxCardResult == nil || maxCardResult.GetIncreasedAfterEvolutionLevel() != 0 {
		t.Fatalf("max-evolution duplicate result = %v, want no further level increase", maxCardResult)
	}
	maxStored, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, masterCard.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if maxStored.GetEvolutionLevel() != stored.GetEvolutionLevel() ||
		maxStored.GetEvolutionLevelTotalExp() != stored.GetEvolutionLevelTotalExp() {
		t.Fatalf("max-evolution duplicate exceeded its cap: before=%v after=%v", stored, maxStored)
	}
	for costumeID := range expectedCostumes {
		if findTestGachaRewardResult(maxResponse.GetResults(), penum.ResourceType_ResourceType_Costume, costumeID) != nil {
			t.Fatalf("max-evolution duplicate re-emitted costume %q: %v", costumeID, maxResponse.GetResults())
		}
	}
	for hairID := range expectedHairs {
		if findTestGachaRewardResult(maxResponse.GetResults(), penum.ResourceType_ResourceType_Hair, hairID) != nil {
			t.Fatalf("max-evolution duplicate re-emitted hair %q: %v", hairID, maxResponse.GetResults())
		}
	}
}

func chooseGachaDrawTarget(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.Gacha, *pmaster.GachaButton, *pmaster.Consumption) {
	t.Helper()
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil {
			continue
		}
		if masterGacha.GetGachaType() != penum.GachaType_GachaType_Normal && masterGacha.GetGachaType() != penum.GachaType_GachaType_Stamp {
			continue
		}
		for _, buttonID := range masterGacha.GetButtonIds() {
			button, err := opmaster.FindGachaButton(context.Background(), buttonID)
			if err != nil || button.GetDrawCount() <= 0 || button.GetLimitCount() > 0 && button.GetLimitCount() < 2 {
				continue
			}
			consumptionID := button.GetConsumptionId()
			if button.GetDiscountConsumptionId() != "" && button.GetDiscountCount() > 0 {
				consumptionID = button.GetDiscountConsumptionId()
			}
			if consumptionID == "" {
				continue
			}
			consumption, err := opmaster.FindConsumption(context.Background(), consumptionID)
			if err != nil || len(consumption.GetResources()) != 1 {
				continue
			}
			type_ := consumption.GetResources()[0].GetResourceType()
			if type_ == penum.ResourceType_ResourceType_Stone || type_ == penum.ResourceType_ResourceType_PaidStone {
				return info, masterGacha, button, consumption
			}
		}
	}
	t.Fatal("current visible gacha catalog has no currency-backed draw button")
	return nil, nil, nil, nil
}

func chooseTestGachaEvolutionCard(
	t *testing.T,
) (*pmaster.Card, *ptransaction.UserCard, []*pmaster.CardEvolutionLevel) {
	t.Helper()
	masterCards, err := opmaster.FindAllCards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, masterCard := range masterCards {
		if masterCard.GetCardEvolutionLevelGroupId() == "" || masterCard.GetSkillId4() == "" {
			continue
		}
		owned, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, masterCard.GetId())
		if err != nil {
			continue
		}
		levels, err := opmaster.FindCardEvolutionLevels(context.Background(), masterCard.GetCardEvolutionLevelGroupId())
		if err != nil {
			t.Fatal(err)
		}
		if len(levels) < 2 || levels[0].GetDuplicateObtainExp() <= 0 ||
			int64(levels[0].GetExp())+int64(levels[0].GetDuplicateObtainExp()) < int64(levels[1].GetExp()) {
			continue
		}
		return masterCard, owned, levels
	}
	t.Fatal("god test user owns no one-duplicate evolution card")
	return nil, nil, nil
}

func chooseMasterCardRewardGacha(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.Gacha, *pmaster.GachaButton) {
	t.Helper()
	for _, listed := range infos {
		poolRule := testGuaranteedRarityPool(listed.GetId())
		if !listed.GetUnlocked() || poolRule == nil || !poolRule.UseMasterCardRewards {
			continue
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), listed.GetId())
		if err != nil || len(masterGacha.GetCardRewards()) == 0 {
			continue
		}
		info, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, listed.GetId())
		if err != nil {
			continue
		}
		for _, state := range info.GetButtons() {
			button, err := opmaster.FindGachaButton(context.Background(), state.GetId())
			if err != nil || button.GetDrawCount() <= 0 {
				continue
			}
			var limitCurrent int32
			switch button.GetLimitType() {
			case penum.GachaLimitType_GachaLimitType_Daily:
				limitCurrent = state.GetTodayCount()
			case penum.GachaLimitType_GachaLimitType_Total:
				limitCurrent = state.GetTotalCount()
			default:
				if button.GetLimitCount() > 0 {
					continue
				}
			}
			if button.GetLimitCount() > 0 && limitCurrent >= button.GetLimitCount() {
				continue
			}
			consumptionID := button.GetConsumptionId()
			if button.GetDiscountConsumptionId() != "" && button.GetDiscountCount() > 0 {
				var discountCurrent int32
				switch button.GetDiscountLimitType() {
				case penum.GachaLimitType_GachaLimitType_Daily:
					discountCurrent = state.GetTodayCount()
				case penum.GachaLimitType_GachaLimitType_Total:
					discountCurrent = state.GetTotalCount()
				default:
					discountCurrent = button.GetDiscountCount()
				}
				if discountCurrent < button.GetDiscountCount() {
					consumptionID = button.GetDiscountConsumptionId()
				}
			}
			if consumptionID != "" {
				consumption, err := opmaster.FindConsumption(context.Background(), consumptionID)
				if err != nil {
					continue
				}
				supported := true
				for _, resource := range consumption.GetResources() {
					switch resource.GetResourceType() {
					case penum.ResourceType_ResourceType_Item, penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
					default:
						supported = false
					}
				}
				if !supported {
					continue
				}
			}
			return info, masterGacha, button
		}
	}
	return nil, nil, nil
}

func chooseSelectableGacha(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.Gacha) {
	t.Helper()
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err == nil && masterGacha.GetSelectPickupAmount() > 0 && int(masterGacha.GetSelectPickupAmount()) <= len(masterGacha.GetSelectCardIds()) {
			return info, masterGacha
		}
	}
	return nil, nil
}

func choosePremiumGacha(infos []*papi.GachaInfo) *papi.GachaInfo {
	for _, info := range infos {
		if info.GetUnlocked() && info.GetPremiumInfo().GetIsFreeGachaTicketReceivable() && info.GetPremiumInfo().GetFreeGachaTicketItemId() != "" {
			return info
		}
	}
	return nil
}

func chooseContinuousGacha(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.GachaButton) {
	t.Helper()
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil || masterGacha.GetGachaType() != penum.GachaType_GachaType_Continuous {
			continue
		}
		for _, buttonID := range masterGacha.GetButtonIds() {
			button, err := opmaster.FindGachaButton(context.Background(), buttonID)
			if err == nil && button.GetDrawCount() > 0 {
				return info, button
			}
		}
	}
	return nil, nil
}

func chooseDiscountBoundary(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.GachaButton, *pmaster.Consumption) {
	t.Helper()
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		currentInfo, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil {
			continue
		}
		for _, state := range currentInfo.GetButtons() {
			button, err := opmaster.FindGachaButton(context.Background(), state.GetId())
			if err != nil || button.GetDiscountConsumptionId() == "" || button.GetDiscountCount() <= 0 {
				continue
			}
			var current int32
			switch button.GetDiscountLimitType() {
			case penum.GachaLimitType_GachaLimitType_Daily:
				current = state.GetTodayCount()
			case penum.GachaLimitType_GachaLimitType_Total:
				current = state.GetTotalCount()
			default:
				continue
			}
			if current >= button.GetDiscountCount() {
				continue
			}
			consumption, err := opmaster.FindConsumption(context.Background(), button.GetDiscountConsumptionId())
			if err == nil && len(consumption.GetResources()) == 1 {
				return currentInfo, button, consumption
			}
		}
	}
	return nil, nil, nil
}

func chooseItemConsumptionGacha(t *testing.T, infos []*papi.GachaInfo) (*papi.GachaInfo, *pmaster.GachaButton, *pmaster.Consumption, int32) {
	t.Helper()
	const execCount int32 = 3
	for _, info := range infos {
		if !info.GetUnlocked() {
			continue
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil || masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous {
			continue
		}
		currentInfo, err := opuser.FindUserGachaInfo(context.Background(), testConfig.TestUserSuid, info.GetId())
		if err != nil {
			continue
		}
		for _, state := range currentInfo.GetButtons() {
			button, err := opmaster.FindGachaButton(context.Background(), state.GetId())
			if err != nil || button.GetDrawCount() <= 0 {
				continue
			}
			var limitCurrent int32
			switch button.GetLimitType() {
			case penum.GachaLimitType_GachaLimitType_Daily:
				limitCurrent = state.GetTodayCount()
			case penum.GachaLimitType_GachaLimitType_Total:
				limitCurrent = state.GetTotalCount()
			default:
				if button.GetLimitCount() > 0 {
					continue
				}
			}
			if button.GetLimitCount() > 0 && limitCurrent+execCount > button.GetLimitCount() {
				continue
			}
			consumptionID := button.GetConsumptionId()
			if button.GetDiscountConsumptionId() != "" && button.GetDiscountCount() > 0 {
				var discountCurrent int32
				switch button.GetDiscountLimitType() {
				case penum.GachaLimitType_GachaLimitType_Daily:
					discountCurrent = state.GetTodayCount()
				case penum.GachaLimitType_GachaLimitType_Total:
					discountCurrent = state.GetTotalCount()
				default:
					discountCurrent = button.GetDiscountCount()
				}
				if discountCurrent < button.GetDiscountCount() {
					if discountCurrent+execCount > button.GetDiscountCount() {
						continue
					}
					consumptionID = button.GetDiscountConsumptionId()
				}
			}
			if consumptionID == "" {
				continue
			}
			consumption, err := opmaster.FindConsumption(context.Background(), consumptionID)
			if err != nil || len(consumption.GetResources()) != 1 {
				continue
			}
			resource := consumption.GetResources()[0]
			if resource.GetResourceType() != penum.ResourceType_ResourceType_Item || resource.GetAmount() <= 0 || len(resource.GetResourceIds()) == 0 {
				continue
			}
			return currentInfo, button, consumption, execCount
		}
	}
	return nil, nil, nil, 0
}

func testGachaConsumption(consumption *pmaster.Consumption) (penum.ResourceType, string, int64) {
	if consumption == nil || len(consumption.GetResources()) == 0 {
		return penum.ResourceType_ResourceType_Unknown, "", 0
	}
	resource := consumption.GetResources()[0]
	resourceID := ""
	if len(resource.GetResourceIds()) > 0 {
		resourceID = resource.GetResourceIds()[0]
	}
	return resource.GetResourceType(), resourceID, int64(resource.GetAmount())
}

func testGachaResourceAmount(t *testing.T, resourceType penum.ResourceType, resourceID string) int64 {
	t.Helper()
	switch resourceType {
	case penum.ResourceType_ResourceType_Item:
		items, err := opuser.FindUserItems(context.Background(), testConfig.TestUserSuid, resourceID)
		if err != nil {
			t.Fatal(err)
		}
		var amount int64
		now := time.Now().UnixMilli()
		for _, item := range items {
			if item.GetExpiredTime() == 0 || item.GetExpiredTime() > now {
				amount += item.GetAmount()
			}
		}
		return amount
	case penum.ResourceType_ResourceType_Stone, penum.ResourceType_ResourceType_PaidStone:
		balance, err := opuser.FindUserBalance(context.Background(), testConfig.TestUserSuid)
		if err != nil {
			t.Fatal(err)
		}
		if resourceType == penum.ResourceType_ResourceType_PaidStone {
			return int64(balance.GetPaidBalance())
		}
		return int64(balance.GetFreeBalance())
	default:
		t.Fatalf("unsupported test gacha resource type: %v", resourceType)
		return 0
	}
}

func findTestGachaButton(info *papi.GachaInfo, id string) *papi.GachaButtonInfo {
	for _, button := range info.GetButtons() {
		if button.GetId() == id {
			return button
		}
	}
	return nil
}

func gachaButtonIDs(buttons []*papi.GachaButtonInfo) []string {
	ids := make([]string, 0, len(buttons))
	for _, button := range buttons {
		ids = append(ids, button.GetId())
	}
	return ids
}

func testGachaInfoIDs(infos []*papi.GachaInfo) []string {
	ids := make([]string, 0, len(infos))
	for _, info := range infos {
		ids = append(ids, info.GetId())
	}
	return ids
}

func testGachaInfoButtonIDs(infos []*papi.GachaInfo) []string {
	ids := make([]string, 0)
	for _, info := range infos {
		ids = append(ids, gachaButtonIDs(info.GetButtons())...)
	}
	return ids
}

func testUserGachaButtonIDs(buttons []*ptransaction.UserGachaButton) []string {
	ids := make([]string, 0, len(buttons))
	for _, button := range buttons {
		ids = append(ids, button.GetGachaButtonId())
	}
	return ids
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}

type testGachaRewardIdentity struct {
	resourceType penum.ResourceType
	resourceID   string
}

func assertGachaRewardResultsCanonical(t *testing.T, results []*ptransaction.RewardResult) {
	t.Helper()
	seen := make(map[testGachaRewardIdentity]struct{}, len(results))
	for _, result := range results {
		if result == nil {
			t.Fatal("Gacha.Draw returned a nil reward result")
		}
		identity := testGachaRewardIdentity{resourceType: result.GetResourceType(), resourceID: result.GetResourceId()}
		if _, exists := seen[identity]; exists {
			t.Fatalf("Gacha.Draw returned duplicate reward result identity: type=%v id=%q", identity.resourceType, identity.resourceID)
		}
		seen[identity] = struct{}{}
		assertGachaRewardResultsCanonical(t, result.GetDuplicateRewardResults())

		giftSeen := make(map[testGachaRewardIdentity]struct{}, len(result.GetAdditionalGifts()))
		for _, gift := range result.GetAdditionalGifts() {
			if gift == nil {
				t.Fatalf("Gacha.Draw returned a nil additional gift for type=%v id=%q", identity.resourceType, identity.resourceID)
			}
			giftIdentity := testGachaRewardIdentity{resourceType: gift.GetResourceType(), resourceID: gift.GetResourceId()}
			if _, exists := giftSeen[giftIdentity]; exists {
				t.Fatalf("Gacha.Draw returned duplicate additional gift identity: type=%v id=%q", giftIdentity.resourceType, giftIdentity.resourceID)
			}
			giftSeen[giftIdentity] = struct{}{}
		}
	}
}

func assertGachaDuplicateCardProgress(
	t *testing.T,
	cardBefore map[string]*ptransaction.UserCard,
	missionBefore map[string]*ptransaction.UserMission,
	limitBreakGiftBefore int64,
	notifiedBefore bool,
	draw *papi.GachaDrawResponse,
) {
	t.Helper()
	drawCounts := make(map[string]int64)
	for _, reward := range draw.GetRewards() {
		if reward.GetResourceType() == penum.ResourceType_ResourceType_Card {
			drawCounts[reward.GetResourceId()] += reward.GetAmount()
		}
	}
	if len(drawCounts) == 0 {
		return
	}
	rarities, err := opmaster.FindAllCardRarities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rarities) == 0 {
		t.Fatal("card rarity master data is empty")
	}
	maxRarity := rarities[len(rarities)-1]
	maxExp := int64(maxRarity.GetRequiredExp())
	var expectedGiftIncrease int64
	rarityGains := make([]testGachaRarityGain, 0, len(drawCounts))
	for cardID, count := range drawCounts {
		before := cardBefore[cardID]
		if before == nil {
			after, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, cardID)
			if err != nil {
				t.Fatal(err)
			}
			if after.GetDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
				t.Fatalf("new gacha card %q display type = %v, want default Unknown", cardID, after.GetDisplayType())
			}
			continue
		}
		masterCard, err := opmaster.FindCard(context.Background(), cardID)
		if err != nil {
			t.Fatal(err)
		}
		duplicateExp, err := opmaster.FindCardDuplicateExp(context.Background(), masterCard.GetInitialRarity())
		if err != nil {
			t.Fatal(err)
		}
		expectedDuplicateRewards := expectedGachaDuplicateRewards(
			t, masterCard, before, count, duplicateExp.GetExp(), rarities,
		)
		expectedEvolution := proto.Clone(before).(*ptransaction.UserCard)
		expectedIncreasedEvolutionLevel := int32(0)
		for range count {
			increased, err := usercard.ApplyDuplicateEvolution(context.Background(), masterCard, expectedEvolution)
			if err != nil {
				t.Fatal(err)
			}
			expectedIncreasedEvolutionLevel = max(expectedIncreasedEvolutionLevel, increased)
		}
		totalGrant := int64(duplicateExp.GetExp()) * count
		expectedExp := min(int64(before.GetRarityTotalExp())+totalGrant, maxExp)
		expectedOverflow := max(int64(before.GetRarityTotalExp())+totalGrant-maxExp, 0)
		expectedRarity := before.GetRarity()
		for _, rarity := range rarities {
			if int64(rarity.GetRequiredExp()) <= expectedExp && rarity.GetRarity() > expectedRarity {
				expectedRarity = rarity.GetRarity()
			}
		}
		expectedGiftIncrease += expectedOverflow

		after, err := opuser.FindUserCard(context.Background(), testConfig.TestUserSuid, cardID)
		if err != nil {
			t.Fatal(err)
		}
		if int64(after.GetRarityTotalExp()) != expectedExp {
			t.Fatalf("duplicate card %q rarity EXP = %d, want %d (before=%d grant=%d)", cardID, after.GetRarityTotalExp(), expectedExp, before.GetRarityTotalExp(), totalGrant)
		}
		if after.GetRarity() != expectedRarity {
			t.Fatalf("duplicate card %q rarity = %d, want %d at total EXP %d", cardID, after.GetRarity(), expectedRarity, expectedExp)
		}
		if after.GetEvolutionLevel() != expectedEvolution.GetEvolutionLevel() ||
			after.GetEvolutionLevelTotalExp() != expectedEvolution.GetEvolutionLevelTotalExp() {
			t.Fatalf(
				"duplicate card %q evolution = level%d/exp%d, want level%d/exp%d",
				cardID, after.GetEvolutionLevel(), after.GetEvolutionLevelTotalExp(),
				expectedEvolution.GetEvolutionLevel(), expectedEvolution.GetEvolutionLevelTotalExp(),
			)
		}
		if expectedRarity > before.GetRarity() || expectedEvolution.GetEvolutionLevel() > before.GetEvolutionLevel() {
			assertGachaCardRarityAttributes(t, masterCard, before, after)
			assertGachaEvolutionUnlocks(t, masterCard, after)
		}
		if expectedRarity > before.GetRarity() {
			rarityGains = append(rarityGains, testGachaRarityGain{
				cardID: cardID, characterID: masterCard.GetCharacterId(), amount: int64(expectedRarity - before.GetRarity()),
			})
		}
		updatedCard := findTestUpdatedGachaCard(draw.GetCommonResponse().GetUpdatedData(), cardID)
		if updatedCard == nil || updatedCard.GetRarityTotalExp() != after.GetRarityTotalExp() {
			t.Fatalf("duplicate card %q was not returned in updatedData with persisted rarity EXP: updated=%v stored=%v", cardID, updatedCard, after)
		}
		result := findTestGachaRewardResult(draw.GetRewardResults(), penum.ResourceType_ResourceType_Card, cardID)
		if result == nil || result.GetAmountI64() != count {
			t.Fatalf("duplicate card %q result amount = %v, want %d", cardID, result, count)
		}
		if result.GetIncreasedAfterEvolutionLevel() != expectedIncreasedEvolutionLevel {
			t.Fatalf(
				"duplicate card %q increasedAfterEvolutionLevel = %d, want %d",
				cardID, result.GetIncreasedAfterEvolutionLevel(), expectedIncreasedEvolutionLevel,
			)
		}
		assertGachaDuplicateRewardAmounts(t, cardID, result.GetDuplicateRewardResults(), expectedDuplicateRewards)
		var overflowResult int64
		for _, gift := range result.GetAdditionalGifts() {
			if gift.GetResourceType() == penum.ResourceType_ResourceType_Item && gift.GetResourceId() == "item-card_limit_break" {
				overflowResult += gift.GetAmount()
			}
		}
		if overflowResult != expectedOverflow {
			t.Fatalf("duplicate card %q overflow gift = %d, want %d", cardID, overflowResult, expectedOverflow)
		}
	}
	limitBreakGiftAfter := testPendingGachaGiftAmount(t, penum.ResourceType_ResourceType_Item, "item-card_limit_break")
	if limitBreakGiftAfter-limitBreakGiftBefore != expectedGiftIncrease {
		t.Fatalf("duplicate-card overflow gifts were not persisted: before=%d after=%d want increase=%d", limitBreakGiftBefore, limitBreakGiftAfter, expectedGiftIncrease)
	}
	if expectedGiftIncrease > 0 {
		foundNotification := false
		for _, notification := range draw.GetCommonResponse().GetUpdatedData().GetNotifications() {
			if notification.GetNotificationType() == penum.NotificationType_NotificationType_Gift && notification.GetValid() {
				foundNotification = true
				break
			}
		}
		// A monthly Dream gift may already have lit the mailbox. The shared
		// notification helper emits only changes, not redundant true->true rows.
		if !foundNotification && !notifiedBefore {
			t.Fatal("duplicate-card overflow did not return an active gift notification")
		}
		if !testGachaGiftNotified(t) {
			t.Fatal("duplicate-card overflow left mailbox notification inactive")
		}
	}
	assertGachaRarityMissionProgress(t, missionBefore, rarityGains, draw.GetCommonResponse().GetUpdatedData())
}

func testGachaGiftNotified(t *testing.T) bool {
	t.Helper()
	row, err := opuser.FindUserNotification(context.Background(), testConfig.TestUserSuid, penum.NotificationType_NotificationType_Gift)
	if err != nil {
		t.Fatal(err)
	}
	return row.GetValid()
}

func expectedGachaDuplicateRewards(
	t *testing.T,
	masterCard *pmaster.Card,
	before *ptransaction.UserCard,
	count int64,
	duplicateExp int32,
	rarities []*pmaster.CardRarity,
) map[testGachaRewardIdentity]int64 {
	t.Helper()
	result := make(map[testGachaRewardIdentity]int64)
	currentRarity := before.GetRarity()
	currentExp := int64(before.GetRarityTotalExp())
	maxExp := int64(rarities[len(rarities)-1].GetRequiredExp())
	for range count {
		duplicate, err := opmaster.FindCardDuplicateReward(
			context.Background(), masterCard.GetInitialRarity(), currentRarity,
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range testResolveGachaReward(t, duplicate.GetRewardId()) {
			identity := testGachaRewardIdentity{
				resourceType: resource.GetResourceType(),
				resourceID:   resource.GetResourceId(),
			}
			result[identity] += int64(resource.GetAmount())
		}

		currentExp = min(currentExp+int64(duplicateExp), maxExp)
		for _, rarity := range rarities {
			if int64(rarity.GetRequiredExp()) <= currentExp && rarity.GetRarity() > currentRarity {
				currentRarity = rarity.GetRarity()
			}
		}
	}
	return result
}

func assertGachaDuplicateRewardAmounts(
	t *testing.T,
	cardID string,
	actual []*ptransaction.RewardResult,
	expected map[testGachaRewardIdentity]int64,
) {
	t.Helper()
	assertGachaRewardResultsCanonical(t, actual)
	if len(actual) != len(expected) {
		t.Fatalf("duplicate card %q returned %d compensation results, want %d: %v", cardID, len(actual), len(expected), actual)
	}
	for identity, amount := range expected {
		got := findTestGachaRewardResult(actual, identity.resourceType, identity.resourceID)
		if got == nil || got.GetAmountI64() != amount {
			t.Fatalf("duplicate card %q compensation type=%v id=%q = %v, want amount %d", cardID, identity.resourceType, identity.resourceID, got, amount)
		}
		if identity.resourceType != penum.ResourceType_ResourceType_Card && !got.GetIsTruncate() &&
			got.GetBeforeAmount() >= 0 && amount >= 0 && got.GetAfterAmount() != got.GetBeforeAmount()+amount {
			t.Fatalf(
				"duplicate card %q grouped compensation type=%v id=%q after=%d, want identity-local %d+%d=%d",
				cardID, identity.resourceType, identity.resourceID, got.GetAfterAmount(),
				got.GetBeforeAmount(), amount, got.GetBeforeAmount()+amount,
			)
		}
	}
}

func testPendingGachaGiftAmount(t *testing.T, resourceType penum.ResourceType, resourceID string) int64 {
	amount, _ := testPendingGachaGifts(t, resourceType, resourceID)
	return amount
}

func testPendingGachaGifts(t *testing.T, resourceType penum.ResourceType, resourceID string) (int64, int) {
	t.Helper()
	gifts, err := opuser.FindAllUserGifts(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	var amount int64
	var count int
	for _, gift := range gifts {
		if gift.GetResourceType() == resourceType && gift.GetResourceId() == resourceID {
			amount += gift.GetAmount()
			count++
		}
	}
	return amount, count
}

func findTestUpdatedGachaCard(updated *papi.UpdatedData, cardID string) *ptransaction.UserCard {
	for _, card := range updated.GetCards() {
		if card.GetCardId() == cardID {
			return card
		}
	}
	return nil
}

func findTestUpdatedGachaCostume(updated *papi.UpdatedData, costumeID string) *ptransaction.UserCostume {
	for _, costume := range updated.GetCostumes() {
		if costume.GetCostumeId() == costumeID {
			return costume
		}
	}
	return nil
}

func findTestUpdatedGachaHair(updated *papi.UpdatedData, hairID string) *ptransaction.UserHair {
	for _, hair := range updated.GetHairs() {
		if hair.GetHairId() == hairID {
			return hair
		}
	}
	return nil
}

func findTestGachaRewardResult(results []*ptransaction.RewardResult, resourceType penum.ResourceType, resourceID string) *ptransaction.RewardResult {
	for _, result := range results {
		if result.GetResourceType() == resourceType && result.GetResourceId() == resourceID {
			return result
		}
	}
	return nil
}

type testGachaRarityGain struct {
	cardID      string
	characterID string
	amount      int64
}

func assertGachaCardRarityAttributes(
	t *testing.T,
	masterCard *pmaster.Card,
	before *ptransaction.UserCard,
	card *ptransaction.UserCard,
) {
	t.Helper()
	base, err := usercard.CalcUserCardBaseAttr(context.Background(), masterCard, card.GetBaseLevel(), card.GetRarity())
	if err != nil {
		t.Fatal(err)
	}
	if card.GetBaseVocal() != base.Vocal || card.GetBaseDance() != base.Dance || card.GetBaseVisual() != base.Visual || card.GetBaseStamina() != base.Stamina {
		t.Fatalf("card %q base attributes were not recalculated for rarity %d: card=%v expected=%v", card.GetCardId(), card.GetRarity(), card, base)
	}
	effective := base
	if card.GetSupported() {
		effective, err = usercard.CalcUserCardBaseAttr(context.Background(), masterCard, card.GetLevel(), card.GetRarity())
		if err != nil {
			t.Fatal(err)
		}
	}
	if card.GetVocal() != effective.Vocal || card.GetDance() != effective.Dance || card.GetVisual() != effective.Visual || card.GetStamina() != effective.Stamina {
		t.Fatalf("card %q effective attributes were not recalculated for rarity %d: card=%v expected=%v", card.GetCardId(), card.GetRarity(), card, effective)
	}
	if card.GetDisplayType() != before.GetDisplayType() {
		t.Fatalf("duplicate card %q changed user-selected display type: before=%v after=%v", card.GetCardId(), before.GetDisplayType(), card.GetDisplayType())
	}
}

func testGachaMissionSnapshot(t *testing.T) map[string]*ptransaction.UserMission {
	t.Helper()
	missions, err := opuser.FindAllUserMissions(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]*ptransaction.UserMission, len(missions))
	for _, mission := range missions {
		result[mission.GetMissionId()] = proto.Clone(mission).(*ptransaction.UserMission)
	}
	return result
}

func assertGachaRarityMissionProgress(
	t *testing.T,
	before map[string]*ptransaction.UserMission,
	gains []testGachaRarityGain,
	updated *papi.UpdatedData,
) {
	t.Helper()
	if len(before) == 0 || len(gains) == 0 {
		return
	}
	ids := make([]string, 0, len(before))
	for id := range before {
		ids = append(ids, id)
	}
	masters, err := opmaster.FindMissionsByIds(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	afterMissions := testGachaMissionSnapshot(t)
	for _, master := range masters {
		userBefore := before[master.GetId()]
		if userBefore == nil || userBefore.GetStatusType() != penum.MissionStatusType_MissionStatusType_Progress && userBefore.GetStatusType() != penum.MissionStatusType_MissionStatusType_Clear {
			continue
		}
		switch master.GetMissionType() {
		case penum.MissionType_MissionType_CardLimitBreakCount,
			penum.MissionType_MissionType_CharacterLimitBreakCount,
			penum.MissionType_MissionType_CharacterTotalCardRarityAchievement:
		default:
			continue
		}
		var increment int64
		for _, gain := range gains {
			if testGachaMissionTargetsMatch(master.GetTargetIds(), gain.cardID, gain.characterID) {
				increment += gain.amount
			}
		}
		if increment == 0 {
			continue
		}
		var maxThreshold int64
		for _, progress := range master.GetProgresses() {
			maxThreshold = max(maxThreshold, progress.GetThreshold())
		}
		expected := min(userBefore.GetProgress()+increment, maxThreshold)
		after := afterMissions[master.GetId()]
		if after == nil {
			t.Fatalf("gacha rarity mission %q disappeared", master.GetId())
		}
		if after.GetProgress() != expected {
			t.Fatalf("gacha rarity mission %q progress = %d, want %d", master.GetId(), after.GetProgress(), expected)
		}
		found := false
		for _, mission := range updated.GetMissions() {
			if mission.GetMissionId() == master.GetId() && mission.GetProgress() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("gacha rarity mission %q was persisted but omitted from updatedData", master.GetId())
		}
	}
}

func testGachaMissionTargetsMatch(targets []string, cardID, characterID string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, target := range targets {
		if target == cardID || target == characterID {
			return true
		}
	}
	return false
}

func testGachaNoticeCatalog(t *testing.T) map[string]*papi.NoticeInfo {
	t.Helper()
	result := make(map[string]*papi.NoticeInfo)
	catalog, err := opsolis.FindNoticeCatalog(context.Background())
	if err != nil {
		return result
	}
	for _, group := range [][]*papi.NoticeInfo{catalog.Notices, catalog.MalfunctionNotices, catalog.PrNotices} {
		for _, notice := range group {
			result[notice.GetId()] = notice
		}
	}
	return result
}

func testGachaNoticeID(gachaID string) string {
	aliases := map[string]string{
		"gacha-all-star-01-26-0817":              "info-gacha-all-star-01-26-0817",
		"gacha-beginner-21-1126":                 "info-gacha-beginner-21-1126",
		"gacha-beginner-dream-23-0329":           "info-gacha-beginner-dream-23-0329",
		"gacha-beginner-fixed-selection-24-1228": "info-gacha-beginner-fixed-selection-24-1228",
		"gacha-continuous-26-0718":               "info-gacha-continuous-26-0718",
		"gacha-excursion-01-26-0731":             "info-gacha-excursion-01-26-0731",
		"gacha-excursion-02-26-0731":             "info-gacha-excursion-02-26-0731",
		"gacha-excursion-01-26-0817-free":        "info-gacha-excursion-01-26-0817",
		"gacha-fs-set-bloom-25-1224-02":          "info-gacha-fs-set-bloom-25-1224-02",
		"gacha-normal-26-0718":                   "info-gacha-normal-21-0811-01",
		"gacha-paid-21-0928-r5-beginner":         "info-gacha-paid-21-0928-r5-beginner",
		"gacha-premium-26-0724":                  "info-gacha-premium-26-0724",
		"gacha-rev-select-01-26-0819":            "info-gacha-rev-select-01-26-0819",
		"gacha-rev-step-rockon-26-0812-1st":      "info-gacha-rev-step-rockon-26-0812-1st",
		"gacha-rev-step-rockon-26-0819-1st":      "info-gacha-rev-step-rockon-26-0819-1st",
		"gacha-ticket-21-0816-01-r5":             "info-gacha-ticket-21-0731-01-r5",
	}
	return aliases[gachaID]
}

func assertGachaEvolutionMasterRules(t *testing.T) {
	t.Helper()
	masterCards, err := opmaster.FindAllCards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	userCards, err := opuser.FindAllUserCards(context.Background(), testConfig.TestUserSuid)
	if err != nil {
		t.Fatal(err)
	}
	userByID := make(map[string]*ptransaction.UserCard, len(userCards))
	for _, card := range userCards {
		userByID[card.GetCardId()] = card
	}
	for _, masterCard := range masterCards {
		if masterCard.GetCardEvolutionLevelGroupId() == "" || userByID[masterCard.GetId()] == nil {
			continue
		}
		levels, err := opmaster.FindCardEvolutionLevels(context.Background(), masterCard.GetCardEvolutionLevelGroupId())
		if err != nil {
			t.Fatal(err)
		}
		if len(levels) < 2 {
			t.Fatalf("evolution card %q has fewer than two progression levels: %v", masterCard.GetId(), levels)
		}

		spawned, err := usercard.SpawnUserCard(context.Background(), masterCard)
		if err != nil {
			t.Fatal(err)
		}
		_, initialSkills, _, err := usercard.CalcUnlocksWithEvolution(context.Background(), 1, levels[0].GetLevel(), masterCard)
		if err != nil {
			t.Fatal(err)
		}
		if spawned.GetEvolutionLevel() != levels[0].GetLevel() || spawned.GetEvolutionLevelTotalExp() != int64(levels[0].GetExp()) ||
			spawned.GetSkillId4() != initialSkills[3] || spawned.GetBaseSkillId4() != initialSkills[3] {
			t.Fatalf("spawned evolution card %q does not reflect its initial master releases: card=%v skills=%v", masterCard.GetId(), spawned, initialSkills)
		}
		if spawned.GetDisplayType() != penum.CardDisplayType_CardDisplayType_Unknown {
			t.Fatalf("spawned evolution card %q display type = %v, want Unknown", masterCard.GetId(), spawned.GetDisplayType())
		}

		candidate := proto.Clone(userByID[masterCard.GetId()]).(*ptransaction.UserCard)
		candidate.EvolutionLevel = levels[0].GetLevel()
		candidate.EvolutionLevelTotalExp = int64(levels[0].GetExp())
		increased, err := usercard.ApplyDuplicateEvolution(context.Background(), masterCard, candidate)
		if err != nil {
			t.Fatal(err)
		}
		expectedTotal := min(
			int64(levels[0].GetExp())+int64(levels[0].GetDuplicateObtainExp()),
			int64(levels[len(levels)-1].GetExp()),
		)
		expectedLevel := levels[0].GetLevel()
		for _, level := range levels {
			if int64(level.GetExp()) <= expectedTotal {
				expectedLevel = level.GetLevel()
			}
		}
		if candidate.GetEvolutionLevelTotalExp() != expectedTotal || candidate.GetEvolutionLevel() != expectedLevel {
			t.Fatalf(
				"master duplicate evolution for %q = level%d/exp%d, want level%d/exp%d",
				masterCard.GetId(), candidate.GetEvolutionLevel(), candidate.GetEvolutionLevelTotalExp(), expectedLevel, expectedTotal,
			)
		}
		if expectedLevel > levels[0].GetLevel() && increased != expectedLevel {
			t.Fatalf("master duplicate evolution for %q reported increased level %d, want %d", masterCard.GetId(), increased, expectedLevel)
		}
		_, evolvedSkills, _, err := usercard.CalcUnlocksWithEvolution(
			context.Background(), candidate.GetBaseLevel(), candidate.GetEvolutionLevel(), masterCard,
		)
		if err != nil {
			t.Fatal(err)
		}
		if expectedLevel > levels[0].GetLevel() && masterCard.GetSkillId4() != "" && evolvedSkills[3] != masterCard.GetSkillId4() {
			t.Fatalf("evolution level %d for %q did not unlock master skill4: got=%q want=%q", expectedLevel, masterCard.GetId(), evolvedSkills[3], masterCard.GetSkillId4())
		}
		for slot := 0; slot < 3; slot++ {
			if evolvedSkills[slot] != initialSkills[slot] {
				t.Fatalf(
					"evolution level %d for %q backfilled ordinary skill slot %d: before=%q after=%q",
					expectedLevel, masterCard.GetId(), slot+1, initialSkills[slot], evolvedSkills[slot],
				)
			}
		}
		if expectedLevel > levels[0].GetLevel() {
			evolutions, err := opmaster.FindCardEvolutions(context.Background(), masterCard.GetId(), levels[0].GetLevel(), expectedLevel)
			if err != nil {
				t.Fatal(err)
			}
			if len(evolutions) == 0 {
				t.Fatalf("evolution card %q crosses to level %d without a master CardEvolution row", masterCard.GetId(), expectedLevel)
			}
		}
		return
	}
	t.Fatal("god test user owns no master evolution-enabled card")
}

func assertGachaEvolutionUnlocks(t *testing.T, masterCard *pmaster.Card, card *ptransaction.UserCard) {
	t.Helper()
	basePhoto, baseSkills, baseAbilities, err := usercard.CalcUnlocksWithEvolution(
		context.Background(), card.GetBaseLevel(), card.GetEvolutionLevel(), masterCard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if card.GetBasePhotoEquipableCount() != basePhoto ||
		card.GetBaseSkillId1() != baseSkills[0] || card.GetBaseSkillId2() != baseSkills[1] ||
		card.GetBaseSkillId3() != baseSkills[2] || card.GetBaseSkillId4() != baseSkills[3] ||
		card.GetBaseLiveAbilityId() != baseAbilities[0] || card.GetBaseActivityAbilityId() != baseAbilities[1] {
		t.Fatalf("card %q base evolution unlock projection is stale: card=%v skills=%v abilities=%v photo=%d", card.GetCardId(), card, baseSkills, baseAbilities, basePhoto)
	}
	photo, skills, abilities := basePhoto, baseSkills, baseAbilities
	if card.GetSupported() {
		photo, skills, abilities, err = usercard.CalcUnlocksWithEvolution(
			context.Background(), card.GetLevel(), card.GetEvolutionLevel(), masterCard,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if card.GetPhotoEquipableCount() != photo ||
		card.GetSkillId1() != skills[0] || card.GetSkillId2() != skills[1] ||
		card.GetSkillId3() != skills[2] || card.GetSkillId4() != skills[3] ||
		card.GetLiveAbilityId() != abilities[0] || card.GetActivityAbilityId() != abilities[1] {
		t.Fatalf("card %q effective evolution unlock projection is stale: card=%v skills=%v abilities=%v photo=%d", card.GetCardId(), card, skills, abilities, photo)
	}
}

type testGachaRarityPool struct {
	MinimumRarity        int32
	UseMasterCardRewards bool
}

func testGuaranteedRarityPool(gachaID string) *testGachaRarityPool {
	switch gachaID {
	case "gacha-all-star-01-26-0817":
		return &testGachaRarityPool{MinimumRarity: 5, UseMasterCardRewards: true}
	case "gacha-ticket-21-0816-01-r5", "gacha-paid-21-0928-r5-beginner":
		return &testGachaRarityPool{MinimumRarity: 5}
	default:
		return nil
	}
}

func testGachaTermEnd(t *testing.T, conditionID string) int64 {
	t.Helper()
	if conditionID == "" {
		return 0
	}
	condition, err := opmaster.FindCondition(context.Background(), conditionID)
	if err != nil {
		t.Fatal(err)
	}
	var end int64
	for _, setting := range condition.GetSettings() {
		if term := setting.GetTerm(); term != nil && term.GetNowBefore() > end {
			end = term.GetNowBefore()
		}
	}
	return end
}

func assertGuaranteedRarityGachaCatalogs(
	t *testing.T,
	client papi.GachaClient,
	infos []*papi.GachaInfo,
) {
	t.Helper()
	for _, info := range infos {
		poolRule := testGuaranteedRarityPool(info.GetId())
		if poolRule == nil || !info.GetUnlocked() {
			continue
		}
		ctx, cancel, err := getFullGrpcCtx()
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Probability(ctx, &papi.GachaProbabilityRequest{GachaId: info.GetId()})
		cancel()
		if err != nil {
			t.Fatalf("guaranteed-rarity Gacha.Probability(%q) failed: %v", info.GetId(), err)
		}
		if len(response.GetProbabilityInfos()) == 0 {
			t.Fatalf("guaranteed-rarity gacha %q returned an empty probability pool", info.GetId())
		}
		masterGacha, err := opmaster.FindGacha(context.Background(), info.GetId())
		if err != nil {
			t.Fatal(err)
		}
		masterCardRewards := make(map[string]struct{}, len(masterGacha.GetCardRewards()))
		for _, reward := range masterGacha.GetCardRewards() {
			masterCardRewards[reward.GetCardId()] = struct{}{}
		}
		for _, probability := range response.GetProbabilityInfos() {
			if probability.GetResourceType() != penum.ResourceType_ResourceType_Card {
				t.Fatalf("guaranteed-rarity gacha %q contains non-card probability: %v", info.GetId(), probability)
			}
			card, err := opmaster.FindCard(context.Background(), probability.GetResourceId())
			if err != nil {
				t.Fatal(err)
			}
			if card.GetInitialRarity() < poolRule.MinimumRarity {
				t.Fatalf("guaranteed-rarity gacha %q contains rarity-%d card %q below minimum %d", info.GetId(), card.GetInitialRarity(), card.GetId(), poolRule.MinimumRarity)
			}
			if poolRule.UseMasterCardRewards {
				if _, exists := masterCardRewards[card.GetId()]; !exists {
					t.Fatalf("master-derived gacha %q probability contains card %q absent from CardRewards", info.GetId(), card.GetId())
				}
			}
		}
	}
}

func assertGachaCardBatchGuarantee(t *testing.T, rewards []*ptransaction.Reward, buttonDrawCount int32) {
	t.Helper()
	const batchSize = 10
	const minimumRarity = 4
	if buttonDrawCount != batchSize || len(rewards) == 0 {
		return
	}
	for offset := 0; offset < len(rewards); offset += batchSize {
		end := min(offset+batchSize, len(rewards))
		if end-offset != batchSize {
			continue
		}
		found := false
		for _, reward := range rewards[offset:end] {
			if reward.GetResourceType() != penum.ResourceType_ResourceType_Card {
				return
			}
			card, err := opmaster.FindCard(context.Background(), reward.GetResourceId())
			if err != nil {
				t.Fatal(err)
			}
			if card.GetInitialRarity() >= minimumRarity {
				found = true
			}
		}
		if !found {
			t.Fatalf("card gacha batch %d..%d contains no rarity-%d-or-higher card", offset, end, minimumRarity)
		}
	}
}

func assertGachaDrawRewardClassification(
	t *testing.T,
	masterGacha *pmaster.Gacha,
	button *pmaster.GachaButton,
	previousTotal int32,
	execCount int32,
	draw *papi.GachaDrawResponse,
) {
	t.Helper()
	if masterGacha.GetGachaType() == penum.GachaType_GachaType_Continuous {
		execCount = 1
	}
	expected := make(map[testGachaRewardIdentity]int64)
	if masterGacha.GetExchangeResourceType() != penum.ResourceType_ResourceType_Unknown || masterGacha.GetExchangeResourceId() != "" {
		identity := testGachaRewardIdentity{resourceType: masterGacha.GetExchangeResourceType(), resourceID: masterGacha.GetExchangeResourceId()}
		expected[identity] += int64(button.GetDrawCount()) * int64(execCount)
	}
	bonusOccurrences := int32(0)
	if button.GetBonusRewardId() != "" && button.GetBonusCount() > 0 {
		bonusOccurrences = (previousTotal+execCount)/button.GetBonusCount() - previousTotal/button.GetBonusCount()
	}
	bonusResources := testResolveGachaReward(t, button.GetBonusRewardId())
	for _, resource := range bonusResources {
		if bonusOccurrences <= 0 {
			break
		}
		identity := testGachaRewardIdentity{resourceType: resource.GetResourceType(), resourceID: resource.GetResourceId()}
		expected[identity] += int64(resource.GetAmount()) * int64(bonusOccurrences)
	}
	assertGachaRewardResultsCanonical(t, draw.GetDrawRewardResults())
	if len(draw.GetDrawRewardResults()) != len(expected) {
		t.Fatalf("Gacha.Draw drawRewardResults count = %d, want %d from master exchange/bonus: %v", len(draw.GetDrawRewardResults()), len(expected), draw.GetDrawRewardResults())
	}
	for identity, amount := range expected {
		result := findTestGachaRewardResult(draw.GetDrawRewardResults(), identity.resourceType, identity.resourceID)
		if result == nil || result.GetAmountI64() != amount {
			t.Fatalf("Gacha.Draw draw reward type=%v id=%q = %v, want amount %d", identity.resourceType, identity.resourceID, result, amount)
		}
	}
	if len(draw.GetBonusRewards()) != 0 {
		t.Fatalf("Gacha.Draw misclassified draw rewards as bonusRewards: %v", draw.GetBonusRewards())
	}
	if bonusOccurrences > 0 {
		expiredTime := testGachaTermEnd(t, masterGacha.GetViewConditionId())
		if expiredTime > 0 {
			expiredTime += 72 * time.Hour.Milliseconds()
		}
		for _, resource := range bonusResources {
			if resource.GetResourceType() != penum.ResourceType_ResourceType_Item {
				continue
			}
			item := findTestUpdatedGachaItem(draw.GetCommonResponse().GetUpdatedData(), resource.GetResourceId(), expiredTime)
			if item == nil {
				t.Fatalf("gacha button item bonus %q omitted expiry %d post-image", resource.GetResourceId(), expiredTime)
			}
		}
	}
}

func testResolveGachaReward(t *testing.T, rewardID string) []*pmaster.RewardResource {
	t.Helper()
	if rewardID == "" {
		return nil
	}
	if strings.HasPrefix(rewardID, "rd_auto;") {
		parts := strings.Split(rewardID, ";")
		if len(parts) != 4 || parts[1] != "item" {
			t.Fatalf("unsupported automatic gacha bonus reward %q", rewardID)
		}
		amount, err := strconv.ParseInt(parts[3], 10, 32)
		if err != nil || amount <= 0 {
			t.Fatalf("invalid automatic gacha bonus reward %q", rewardID)
		}
		return []*pmaster.RewardResource{{
			ResourceType: penum.ResourceType_ResourceType_Item,
			ResourceId:   parts[2],
			Amount:       int32(amount),
		}}
	}
	reward, err := opmaster.FindReward(context.Background(), rewardID)
	if err != nil {
		t.Fatal(err)
	}
	return reward.GetResources()
}

type testGachaItemConsumption struct {
	expiredTime int64
	result      *ptransaction.ConsumptionResult
}

func expectedGachaItemStackConsumption(items []*ptransaction.UserItem, amount, now int64) []testGachaItemConsumption {
	usable := make([]*ptransaction.UserItem, 0, len(items))
	for _, item := range items {
		if item.GetAmount() > 0 && (item.GetExpiredTime() == 0 || item.GetExpiredTime() > now) {
			usable = append(usable, proto.Clone(item).(*ptransaction.UserItem))
		}
	}
	sort.SliceStable(usable, func(i, j int) bool {
		left, right := usable[i].GetExpiredTime(), usable[j].GetExpiredTime()
		if left == 0 {
			return false
		}
		if right == 0 {
			return true
		}
		return left < right
	})
	remaining := amount
	result := make([]testGachaItemConsumption, 0, len(usable))
	for _, item := range usable {
		if remaining == 0 {
			break
		}
		consumed := min(item.GetAmount(), remaining)
		result = append(result, testGachaItemConsumption{
			expiredTime: item.GetExpiredTime(),
			result: &ptransaction.ConsumptionResult{
				ResourceType: penum.ResourceType_ResourceType_Item,
				ResourceId:   item.GetItemId(),
				Amount:       consumed,
				BeforeAmount: item.GetAmount(),
				AfterAmount:  item.GetAmount() - consumed,
			},
		})
		remaining -= consumed
	}
	return result
}

func findTestUpdatedGachaItem(updated *papi.UpdatedData, itemID string, expiredTime int64) *ptransaction.UserItem {
	for _, item := range updated.GetItems() {
		if item.GetItemId() == itemID && item.GetExpiredTime() == expiredTime {
			return item
		}
	}
	return nil
}
