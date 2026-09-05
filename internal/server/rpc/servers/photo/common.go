package photo

import (
	"context"
	"slices"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"github.com/vertesan/solis-server/internal/server/rpc/servers/internal/viber"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// consumePhotoConsumptions aggregates identical resources before applying
// them, so UpdatedData contains one authoritative post-image per resource.
func consumePhotoConsumptions(
	ctx context.Context,
	suid string,
	consumptions []*pmaster.Consumption,
	updated *papi.UpdatedData,
) error {
	type key struct {
		resourceType penum.ResourceType
		resourceID   string
	}
	amounts := make(map[key]int64)
	order := make([]key, 0)
	for _, consumption := range consumptions {
		if consumption == nil {
			return status.Error(codes.FailedPrecondition, "photo consumption master data is missing")
		}
		for _, resource := range consumption.GetResources() {
			if resource.GetAmount() <= 0 {
				return status.Error(codes.FailedPrecondition, "photo consumption amount must be positive")
			}
			resourceID := ""
			if len(resource.GetResourceIds()) > 1 {
				return status.Error(codes.FailedPrecondition, "photo consumption has multiple alternative resource IDs")
			}
			if len(resource.GetResourceIds()) == 1 {
				resourceID = resource.GetResourceIds()[0]
			}
			entry := key{resourceType: resource.GetResourceType(), resourceID: resourceID}
			if _, exists := amounts[entry]; !exists {
				order = append(order, entry)
			}
			amounts[entry] += int64(resource.GetAmount())
		}
	}
	for _, entry := range order {
		if err := viber.ConsumeExchangeResource(ctx, suid, entry.resourceType, entry.resourceID, amounts[entry], updated); err != nil {
			return err
		}
	}
	return nil
}

func unlockedPhotoStage(stages []*papi.PhotoStage, stageID string) bool {
	for _, stage := range stages {
		if stage.GetStageId() == stageID {
			return stage.GetIsUnlocked()
		}
	}
	return false
}

func unlockedPhotoExpression(expressions []*papi.PhotoExpression, expressionID string) bool {
	for _, expression := range expressions {
		if expression.GetPhotoExpressionId() == expressionID {
			return expression.GetIsUnlocked()
		}
	}
	return false
}

func resolvePhotoStoryEpisode(ctx context.Context, partID string, chapter, route, episode int32) (string, error) {
	if partID == "" || chapter <= 0 || route <= 0 || episode <= 0 {
		return "", viber.InvalidArgumentf("storyPartId and positive chapter, route, and episode are required")
	}
	part, err := opmaster.FindStoryPart(ctx, partID)
	if err != nil {
		return "", viber.PhotoMasterLookupError("story part", partID, err)
	}
	for _, candidateChapter := range part.GetChapters() {
		if candidateChapter.GetChapter() != chapter || candidateChapter.GetRoute() != route {
			continue
		}
		for _, candidateEpisode := range candidateChapter.GetEpisodes() {
			if candidateEpisode.GetEpisode() == episode && candidateEpisode.GetStoryId() != "" {
				return candidateEpisode.GetStoryId(), nil
			}
		}
	}
	return "", viber.InvalidArgumentf("unknown story episode")
}

func uniqueRequiredPhotoIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, viber.InvalidArgumentf("at least one photoId is required")
	}
	return uniqueRequiredStrings(values, "photoId")
}

func uniqueRequiredStrings(values []string, name string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return nil, viber.InvalidArgumentf("%s is required", name)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func photoStoryCharacters(ctx context.Context, storyID string) ([]string, error) {
	story, err := opmaster.FindStory(ctx, storyID)
	if err != nil {
		return nil, err
	}
	result := slices.Clone(story.GetAdditionalDisplayCharacterIds())
	setting, err := viber.PhotoSetting(ctx)
	if err != nil {
		return nil, err
	}
	if setting.GetStoryPhotoShootingDefaultCharacterId() != "" {
		result = appendUniquePhotoString(result, setting.GetStoryPhotoShootingDefaultCharacterId())
	}
	return result, nil
}

func appendUniquePhotoString(values []string, value string) []string {
	if value != "" && !slices.Contains(values, value) {
		values = append(values, value)
	}
	return values
}
