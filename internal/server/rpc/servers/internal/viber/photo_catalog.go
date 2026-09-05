package viber

import (
	"context"
	"errors"

	"github.com/vertesan/solis-server/internal/db/store/opmaster"
	"github.com/vertesan/solis-server/internal/db/store/opuser"
	"github.com/vertesan/solis-server/internal/proto/papi"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"go.mongodb.org/mongo-driver/v2/mongo"
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

func ValidatePhotoStyleLists(characterIDs, costumeIDs, hairIDs []string) error {
	if len(characterIDs) == 0 {
		return InvalidArgumentf("at least one character is required")
	}
	return ValidateOptionalPhotoStyleLists(characterIDs, costumeIDs, hairIDs)
}

func ValidateOptionalPhotoStyleLists(characterIDs, costumeIDs, hairIDs []string) error {
	if len(characterIDs) != len(costumeIDs) || len(characterIDs) != len(hairIDs) {
		return InvalidArgumentf("characterIds, costumeIds, and hairIds must have equal lengths")
	}
	return nil
}

func ValidateOwnedPhotoStyles(ctx context.Context, suid string, characterIDs, costumeIDs, hairIDs []string) error {
	for index, characterID := range characterIDs {
		if characterID == "" || costumeIDs[index] == "" || hairIDs[index] == "" {
			return InvalidArgumentf("photo character, costume, and hair IDs cannot be empty")
		}
		if _, err := opuser.FindUserCharacter(ctx, suid, characterID); err != nil {
			return PhotoOwnershipError("character", characterID, err)
		}
		if _, err := opuser.FindUserCostume(ctx, suid, costumeIDs[index]); err != nil {
			return PhotoOwnershipError("costume", costumeIDs[index], err)
		}
		if _, err := opuser.FindUserHair(ctx, suid, hairIDs[index]); err != nil {
			return PhotoOwnershipError("hair", hairIDs[index], err)
		}
		costume, err := opmaster.FindCostume(ctx, costumeIDs[index])
		if err != nil {
			return PhotoMasterLookupError("costume", costumeIDs[index], err)
		}
		hair, err := opmaster.FindHair(ctx, hairIDs[index])
		if err != nil {
			return PhotoMasterLookupError("hair", hairIDs[index], err)
		}
		if costume.GetCharacterId() != characterID || hair.GetCharacterId() != characterID {
			return FailedPreconditionf("costume and hair must belong to character %q", characterID)
		}
	}
	return nil
}

func PhotoMasterLookupError(kind, id string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return InvalidArgumentf("unknown %s %q", kind, id)
	}
	return err
}

func PhotoOwnershipError(kind, id string, err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return FailedPreconditionf("%s %q is not owned or unlocked", kind, id)
	}
	return err
}
