// Package reward contains reward formats shared by gameplay and administration.
package reward

import (
	"strconv"
	"strings"

	"github.com/vertesan/solis-server/internal/proto/penum"
	"github.com/vertesan/solis-server/internal/proto/pmaster"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ParseAuto decodes an inline reward. A nil result with no error means the ID
// is not an inline reward and must instead be looked up in the master catalog.
func ParseAuto(rewardID string) ([]*pmaster.RewardResource, error) {
	if !strings.HasPrefix(rewardID, "rd_auto;") {
		return nil, nil
	}
	parts := strings.Split(rewardID, ";")
	if len(parts) != 4 {
		return nil, status.Error(codes.FailedPrecondition, "malformed automatic reward")
	}
	resourceType, ok := autoResourceTypes[parts[1]]
	if !ok {
		return nil, status.Errorf(codes.Unimplemented, "automatic reward type %q is unsupported", parts[1])
	}
	amount, err := strconv.ParseInt(parts[3], 10, 32)
	if err != nil || amount <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "malformed automatic reward amount")
	}
	return []*pmaster.RewardResource{{ResourceType: resourceType, ResourceId: parts[2], Amount: int32(amount)}}, nil
}

var autoResourceTypes = map[string]penum.ResourceType{
	"item":                                 penum.ResourceType_ResourceType_Item,
	"accessory":                            penum.ResourceType_ResourceType_Accessory,
	"costume":                              penum.ResourceType_ResourceType_Costume,
	"emblem":                               penum.ResourceType_ResourceType_Emblem,
	"gold":                                 penum.ResourceType_ResourceType_Gold,
	"stone":                                penum.ResourceType_ResourceType_Stone,
	"paid_stone":                           penum.ResourceType_ResourceType_PaidStone,
	"activate_photo_shooting_story_ticket": penum.ResourceType_ResourceType_ActivatePhotoShootingStoryTicket,
	"card_rarity_exp":                      penum.ResourceType_ResourceType_CardRarityExp,
	"photo_recipe":                         penum.ResourceType_ResourceType_PhotoRecipe,
	"showcase_toy":                         penum.ResourceType_ResourceType_ShowcaseToy,
	"showcase_music":                       penum.ResourceType_ResourceType_ShowcaseMusic,
	"photo_pose":                           penum.ResourceType_ResourceType_PhotoPose,
	"photo_all_in_one":                     penum.ResourceType_ResourceType_PhotoAllInOne,
	"company_point":                        penum.ResourceType_ResourceType_CompanyPoint,
	"guild_medal":                          penum.ResourceType_ResourceType_GuildMedal,
}
