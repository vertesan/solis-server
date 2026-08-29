package viber

import (
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
)

func FilterUserPhotoActivities(user []*papi.PhotoActivity, master []*pmaster.PhotoActivity) []*papi.PhotoActivity {
	valid := make(map[string]struct{}, len(master))
	for _, entry := range master {
		valid[entry.GetId()] = struct{}{}
	}
	return filterUserPhotoCatalog(user, valid, func(entry *papi.PhotoActivity) string {
		return entry.GetPhotoActivityId()
	})
}

func FilterUserPhotoMusics(user []*papi.PhotoMusic, master []*pmaster.PhotoQuestMusic) []*papi.PhotoMusic {
	valid := make(map[string]struct{}, len(master))
	for _, entry := range master {
		valid[entry.GetId()] = struct{}{}
	}
	return filterUserPhotoCatalog(user, valid, func(entry *papi.PhotoMusic) string {
		return entry.GetMusicId()
	})
}

func FilterUserPhotoStages(user []*papi.PhotoStage, master []*pmaster.PhotoQuestStage) []*papi.PhotoStage {
	valid := make(map[string]struct{}, len(master))
	for _, entry := range master {
		valid[entry.GetId()] = struct{}{}
	}
	return filterUserPhotoCatalog(user, valid, func(entry *papi.PhotoStage) string {
		return entry.GetStageId()
	})
}

func FilterUserPhotoExpressions(user []*papi.PhotoExpression, master []*pmaster.PhotoExpression) []*papi.PhotoExpression {
	valid := make(map[string]struct{}, len(master))
	for _, entry := range master {
		valid[entry.GetId()] = struct{}{}
	}
	return filterUserPhotoCatalog(user, valid, func(entry *papi.PhotoExpression) string {
		return entry.GetPhotoExpressionId()
	})
}

func filterUserPhotoCatalog[T any](user []T, valid map[string]struct{}, id func(T) string) []T {
	filtered := make([]T, 0, len(user))
	for _, entry := range user {
		if _, exists := valid[id(entry)]; exists {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
