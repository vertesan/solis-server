package test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/db/transaction"
	"github.com/vertesan/solis-server/internal/makino/dream"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/ptransaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func TestDreamGenerationScenario(t *testing.T) {
	setupTestUser(t)
	ctx := context.Background()
	suid := testConfig.TestUserSuid
	client := papi.NewDreamClient(testConfig.GrpcConn)
	base, material := dreamTestPhotos(t, ctx, suid)
	request := &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()}
	profileReject(t, codes.InvalidArgument, client.GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: base.GetPhotoId()})
	material.Locked = true
	if err := opuser.UpsertUserPhoto(ctx, suid, material); err != nil {
		t.Fatal(err)
	}
	preview := profileCall(t, client.ListMaterialPhoto, &papi.DreamListMaterialPhotoRequest{BasePhotoId: base.GetPhotoId()})
	if len(preview.GetUserMaterialPhotos()) != 0 {
		t.Fatal("locked material offered")
	}
	profileReject(t, codes.FailedPrecondition, client.GenerateArea, request)
	material.Locked = false
	material.PhotoRecipeId = "not-a-real-recipe"
	if err := opuser.UpsertUserPhoto(ctx, suid, material); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.GenerateArea, request)
	material.PhotoRecipeId = ""
	if err := opuser.UpsertUserPhoto(ctx, suid, material); err != nil {
		t.Fatal(err)
	}
	profile, err := opuser.FindUserProfile(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	profile.FavoritePhotoId = material.GetPhotoId()
	if err = opuser.UpsertUserProfile(ctx, suid, profile); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.GenerateArea, request)
	profile.FavoritePhotoId = ""
	if err = opuser.UpsertUserProfile(ctx, suid, profile); err != nil {
		t.Fatal(err)
	}
	preview = profileCall(t, client.ListMaterialPhoto, &papi.DreamListMaterialPhotoRequest{BasePhotoId: base.GetPhotoId()})
	if len(preview.GetUserMaterialPhotos()) != 1 {
		t.Fatalf("expected one material: %v", preview)
	}
	plan := preview.GetUserMaterialPhotos()[0]
	if plan.GetMaxQuestNumber() != 50 || plan.GetGenerateAreaItemConsumptionAmount() < 2 || plan.GetGenerateAreaItemConsumptionAmount() > 5 || plan.GetMaxGrowthPermil() <= 1000 {
		t.Fatalf("invalid generation plan %v", plan)
	}
	if err = opuser.UpsertUserItem(ctx, suid, &ptransaction.UserItem{ItemId: dream.ManualItemID, Amount: 0}); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.GenerateArea, request)
	if err = opuser.UpsertUserItem(ctx, suid, &ptransaction.UserItem{ItemId: dream.ManualItemID, Amount: 100}); err != nil {
		t.Fatal(err)
	}
	total, err := opuser.FindUserTotalCount(ctx, suid)
	if err != nil {
		t.Fatal(err)
	}
	savedCount := total.GetDreamAreaGenerateCount()
	total.DreamAreaGenerateCount = math.MaxInt64
	if err = opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
		t.Fatal(err)
	}
	profileReject(t, codes.FailedPrecondition, client.GenerateArea, request)
	if _, err = opuser.FindUserPhoto(ctx, suid, material.GetPhotoId()); err != nil {
		t.Fatal("failed generation consumed material", err)
	}
	if _, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatal("failed generation retained area", err)
	}
	total.DreamAreaGenerateCount = savedCount
	if err = opuser.UpsertUserTotalCount(ctx, suid, total); err != nil {
		t.Fatal(err)
	}
	generated := profileCall(t, client.GenerateArea, request)
	if generated.GetDreamUserPhoto().GetMaxQuestNumber() != plan.GetMaxQuestNumber() || generated.GetDreamUserPhoto().GetMaxGrowthPermil() != plan.GetMaxGrowthPermil() {
		t.Fatal("preview and generation disagree")
	}
	if len(generated.GetCommonResponse().GetDeletedData().GetPhotos()) != 1 {
		t.Fatal("consumed material absent from response")
	}
	if _, err = opuser.FindUserPhoto(ctx, suid, material.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatal("material remains owned", err)
	}
	items, err := opuser.FindUserItems(ctx, suid, dream.ManualItemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatal("unexpected manual rows")
	}
	item := items[0]
	if item.GetAmount() != 100-int64(plan.GetGenerateAreaItemConsumptionAmount()) {
		t.Fatal("manual consumption mismatch")
	}
	list := profileCall(t, client.ListQuest, &papi.DreamListQuestRequest{BasePhotoId: base.GetPhotoId()})
	if len(list.GetQuestInfos()) != 50 || !list.GetQuestInfos()[0].GetIsPlayable() {
		t.Fatal("invalid generated stages")
	}
	for i, q := range list.GetQuestInfos() {
		if q.GetIsSkipAvailable() != (i < 40) || q.GetIsPlayable() != (i == 0) || q.GetClearScore() <= 0 || q.GetMusicChartPatternId() == "" {
			t.Fatalf("invalid stage %v", q)
		}
	}
	area, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	owned, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return dream.ApplyProgress(ctx, suid, area, owned, 25, time.Now().UnixMilli())
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range owned.GetAbilities() {
		if a.GetEffectValue() <= base.GetAbilities()[i].GetEffectValue() {
			t.Fatal("progress did not improve photo")
		}
	}
	profileCall(t, client.ResetArea, &papi.DreamResetAreaRequest{BasePhotoId: base.GetPhotoId()})
	reset, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if reset.GetIsDreamBase() || reset.GetDreamMaterialPhotoLevel() != 0 {
		t.Fatal("reset flags retained")
	}
	for i, a := range reset.GetAbilities() {
		if !proto.Equal(a, base.GetAbilities()[i]) {
			t.Fatal("reset lost baseline options")
		}
	}
	if _, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatal("reset retained area")
	}
	itemsAfter, err := opuser.FindUserItems(ctx, suid, dream.ManualItemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsAfter) != 1 || itemsAfter[0].GetAmount() != item.GetAmount() {
		t.Fatal("reset refunded manuals")
	}
}

func TestDreamPhotoQualityAndResetScenario(t *testing.T) {
	setupTestUser(t)
	ctx, suid := context.Background(), testConfig.TestUserSuid
	base, material := dreamTestPhotos(t, ctx, suid)
	client, photoClient := papi.NewDreamClient(testConfig.GrpcConn), papi.NewPhotoClient(testConfig.GrpcConn)
	if err := opuser.UpsertUserItem(ctx, suid, &ptransaction.UserItem{ItemId: dream.ManualItemID, Amount: 100}); err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()})
	area, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	owned, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	err = transaction.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return dream.ApplyProgress(ctx, suid, area, owned, area.Data.GetMaxQuestNumber()/2, time.Now().UnixMilli())
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeArea, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	preview := profileCall(t, photoClient.GetDreamBasePhotoEnhanceLevelInfos, &papi.GetDreamBasePhotoEnhanceLevelInfosRequest{BasePhotoId: base.GetPhotoId()})
	if len(preview.GetLevelInfos()) != 2 || preview.GetLevelInfos()[1].GetLevel() != base.GetLevel()+5 {
		t.Fatalf("quality preview missing current/next levels: %v", preview)
	}
	for i, effect := range preview.GetLevelInfos()[0].GetEffectValueInfos() {
		if effect.GetEffectValue() != owned.GetAbilities()[i].GetEffectValue() {
			t.Fatal("current quality preview lost Dream growth")
		}
	}
	afterPreview, err := opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(beforeArea.Data, afterPreview.Data) || afterPreview.MaterialPhoto.GetLevel() != beforeArea.MaterialPhoto.GetLevel() {
		t.Fatal("read-only preview mutated stored Dream area")
	}
	nextLevel := base.GetLevel() + 5
	rarity, err := opmaster.FindPhotoRarityLevel(ctx, base.GetRarity(), nextLevel)
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(rarity.GetEnhanceRequiredItem()) * 2
	if cost <= 0 {
		t.Fatal("invalid fixture enhancement cost")
	}
	if err = opuser.UpsertUserItem(ctx, suid, &ptransaction.UserItem{ItemId: "item-photo_enhance", Amount: cost}); err != nil {
		t.Fatal(err)
	}
	profileCall(t, photoClient.Enhance, &papi.PhotoEnhanceRequest{PhotoId: base.GetPhotoId(), Level: nextLevel})
	enhanced, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if enhanced.GetLevel() != nextLevel || enhanced.GetDreamMaterialPhotoLevel() != nextLevel {
		t.Fatal("base/material quality did not advance together")
	}
	for i, effect := range preview.GetLevelInfos()[1].GetEffectValueInfos() {
		if effect.GetEffectValue() != enhanced.GetAbilities()[i].GetEffectValue() {
			t.Fatal("quality preview disagrees with enhancement")
		}
	}
	film, err := opuser.FindUserItems(ctx, suid, "item-photo_enhance")
	if err != nil {
		t.Fatal(err)
	}
	if len(film) != 1 || film[0].GetAmount() != 0 {
		t.Fatal("enhancement did not consume both base and material costs")
	}
	// A mission association unlocked after generation must not be rolled back
	// by resetting the area. Only the added Dream effect values are removed.
	enhanced.Abilities[0].MissionId = "dream-test-unlocked-option"
	enhanced.Abilities[0].IsAvailable = true
	if err = opuser.UpsertUserPhoto(ctx, suid, enhanced); err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.ResetArea, &papi.DreamResetAreaRequest{BasePhotoId: base.GetPhotoId()})
	reset, err := opuser.FindUserPhoto(ctx, suid, base.GetPhotoId())
	if err != nil {
		t.Fatal(err)
	}
	if reset.GetLevel() != nextLevel || reset.GetIsDreamBase() || reset.GetAbilities()[0].GetMissionId() != "dream-test-unlocked-option" || !reset.GetAbilities()[0].GetIsAvailable() {
		t.Fatal("reset discarded quality or mission unlock")
	}
	masters, err := dream.LoadAbilities(ctx, []*ptransaction.UserPhoto{reset})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range reset.GetAbilities() {
		expected, ok := testPhotoAbilityEffectValue(masters[a.GetPhotoAbilityId()], nextLevel, a.GetGrade())
		if !ok || a.GetEffectValue() != expected {
			t.Fatal("reset did not preserve ordinary enhanced value")
		}
	}
	// Regenerating and then selling the base also disposes of its area, without
	// returning either the material or the manuals consumed to create it.
	if err = opuser.UpsertUserPhoto(ctx, suid, material); err != nil {
		t.Fatal(err)
	}
	profileCall(t, client.GenerateArea, &papi.DreamGenerateAreaRequest{BasePhotoId: base.GetPhotoId(), MaterialPhotoId: material.GetPhotoId()})
	profileCall(t, photoClient.Delete, &papi.PhotoDeleteRequest{PhotoIds: []string{base.GetPhotoId()}})
	if _, err = opuser.FindUserDreamArea(ctx, suid, base.GetPhotoId()); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatal("selling base retained area", err)
	}
}
