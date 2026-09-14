# Unimplemented gRPC APIs

Last audited: 2026-09-14

This inventory uses the generated API declarations in
`internal/proto/papi/papi_grpc.pb.go` as the source of truth. It compares them
with registrations in `internal/server/rpc/rpc.go` and concrete methods on the
server types in `internal/server/rpc/servers/<server>/*.go`, searched
recursively. Each server has its own package, with one file per RPC. Helpers
shared by that server's RPCs live in its `common.go`; helpers shared across
servers live in `internal/server/rpc/servers/internal/viber`.

An RPC is classified as unimplemented when either:

- its entire generated service has no registered server, or
- its service is registered, but the concrete server does not override the
  generated `Unimplemented<Service>Server` method.

This is an implementation-presence audit, not a review of game-logic
completeness. A concrete handler is counted as implemented even if particular
inputs are unsupported or its behavior is intentionally minimal.

## Summary

| Category | Services | RPCs |
| --- | ---: | ---: |
| Generated API surface | 67 | 425 |
| Registered services | 42 | 266 |
| Concrete handlers | 42 | 226 |
| Entirely unregistered services | 25 | 159 |
| Missing methods on registered services | 15 | 40 |
| Total unimplemented RPCs | 40 | 199 |

The “Total unimplemented RPCs” service count is the 25 entirely unregistered
services plus 15 registered services that are only partially implemented.

## Entirely unregistered services

These services have generated definitions but no registration or concrete
server under `internal/server/rpc/servers`. Requests are handled by the gRPC
unknown-service handler.

- `Advertisement` (2):
  - `/api.Advertisement/Start`
  - `/api.Advertisement/End`
- `Anniversary` (1):
  - `/api.Anniversary/OutputTransitionLog`
- `Backside` (9):
  - `/api.Backside/Top`
  - `/api.Backside/Ranking`
  - `/api.Backside/StageStart`
  - `/api.Backside/StageAction`
  - `/api.Backside/StageLiveBonus`
  - `/api.Backside/StagePracticeUpdate`
  - `/api.Backside/StageReset`
  - `/api.Backside/StageSurpriseFinish`
  - `/api.Backside/StageDeckPositionChange`
- `Dice` (15):
  - `/api.Dice/Top`
  - `/api.Dice/MovePanel`
  - `/api.Dice/UseItem`
  - `/api.Dice/DiscardItem`
  - `/api.Dice/PurchaseItem`
  - `/api.Dice/SellItem`
  - `/api.Dice/FinishShopPanel`
  - `/api.Dice/FinishStoryPanel`
  - `/api.Dice/UseStaminaRecoveryItem`
  - `/api.Dice/ListDeck`
  - `/api.Dice/StartQuest`
  - `/api.Dice/StartGoalQuest`
  - `/api.Dice/ChooseLiveBonus`
  - `/api.Dice/ListRanking`
  - `/api.Dice/SaveDeck`
- `Dream` (7):
  - `/api.Dream/Top`
  - `/api.Dream/ListMaterialPhoto`
  - `/api.Dream/ListQuest`
  - `/api.Dream/GenerateArea`
  - `/api.Dream/StartQuest`
  - `/api.Dream/SkipQuest`
  - `/api.Dream/ResetArea`
- `Duty` (3):
  - `/api.Duty/Top`
  - `/api.Duty/ExchangeReward`
  - `/api.Duty/ReceiveTotalObtainReward`
- `Exercise` (4):
  - `/api.Exercise/Top`
  - `/api.Exercise/SaveDeck`
  - `/api.Exercise/Start`
  - `/api.Exercise/ListAsset`
- `FishHeaven` (5):
  - `/api.FishHeaven/Top`
  - `/api.FishHeaven/ClearMiniGameTutorial`
  - `/api.FishHeaven/StartMiniGame`
  - `/api.FishHeaven/FinishMiniGame`
  - `/api.FishHeaven/ListRanking`
- `Forum` (9):
  - `/api.Forum/ListThread`
  - `/api.Forum/ListThreadReply`
  - `/api.Forum/CreateThread`
  - `/api.Forum/SendThreadReply`
  - `/api.Forum/LikeReply`
  - `/api.Forum/DislikeReply`
  - `/api.Forum/BookmarkThread`
  - `/api.Forum/UnbookmarkThread`
  - `/api.Forum/BlockUser`
- `Gpp` (1):
  - `/api.Gpp/ReceiveReward`
- `Gvg` (5):
  - `/api.Gvg/Top`
  - `/api.Gvg/ListOpponent`
  - `/api.Gvg/ListRanking`
  - `/api.Gvg/Start`
  - `/api.Gvg/ListAsset`
- `Invite` (3):
  - `/api.Invite/Top`
  - `/api.Invite/ReceiveHostRewards`
  - `/api.Invite/EnterCode`
- `Ladder` (2):
  - `/api.Ladder/Top`
  - `/api.Ladder/ReachPanel`
- `League` (9):
  - `/api.League/Top`
  - `/api.League/GetLiveResult`
  - `/api.League/SaveDeck`
  - `/api.League/GetLastSeasonResult`
  - `/api.League/ListPointRanking`
  - `/api.League/ListCardHighestScoreRanking`
  - `/api.League/ListCardSkillRateRanking`
  - `/api.League/ListCardMvpRanking`
  - `/api.League/GetDailyDetail`
- `Marathon` (21):
  - `/api.Marathon/Top`
  - `/api.Marathon/UnlockQuest`
  - `/api.Marathon/StartQuest`
  - `/api.Marathon/ListQuestAsset`
  - `/api.Marathon/UseQuestStaminaRecoveryItem`
  - `/api.Marathon/ListDeck`
  - `/api.Marathon/SaveDeck`
  - `/api.Marathon/BuddyDeck`
  - `/api.Marathon/DrawBoxGacha`
  - `/api.Marathon/ResetBoxGacha`
  - `/api.Marathon/ListRaid`
  - `/api.Marathon/CheckRaidProgress`
  - `/api.Marathon/ListRaidHistory`
  - `/api.Marathon/GetRaidHistoryDetail`
  - `/api.Marathon/ListRaidRanking`
  - `/api.Marathon/StartRaid`
  - `/api.Marathon/JoinRaid`
  - `/api.Marathon/FinishRaid`
  - `/api.Marathon/UseRaidStaminaRecoveryItem`
  - `/api.Marathon/ListRaidDeck`
  - `/api.Marathon/SaveRaidDeck`
- `Mosaic` (3):
  - `/api.Mosaic/Top`
  - `/api.Mosaic/Exchange`
  - `/api.Mosaic/ReceiveTotalConsumptionReward`
- `PhotoContest` (7):
  - `/api.PhotoContest/Top`
  - `/api.PhotoContest/ListPhoto`
  - `/api.PhotoContest/Ranking`
  - `/api.PhotoContest/ReceiveSectionRewards`
  - `/api.PhotoContest/CheckShooting`
  - `/api.PhotoContest/SubmitShooting`
  - `/api.PhotoContest/CreateShooting`
- `Pvp` (5):
  - `/api.Pvp/Top`
  - `/api.Pvp/ListOpponent`
  - `/api.Pvp/ListRanking`
  - `/api.Pvp/Start`
  - `/api.Pvp/ListAsset`
- `Race` (12):
  - `/api.Race/Top`
  - `/api.Race/MovePanel`
  - `/api.Race/ReceiveActivityLesson`
  - `/api.Race/EnhanceLiveBonus`
  - `/api.Race/ResetLiveBonus`
  - `/api.Race/StartLivePanelQuest`
  - `/api.Race/StartLiveAreaQuest`
  - `/api.Race/ListQuestAsset`
  - `/api.Race/ListDeck`
  - `/api.Race/SaveDeck`
  - `/api.Race/ResetCardCoolTime`
  - `/api.Race/Ranking`
- `Roulette` (3):
  - `/api.Roulette/Top`
  - `/api.Roulette/Check`
  - `/api.Roulette/Draw`
- `Satellite` (1):
  - `/api.Satellite/InitCode`
- `Shelf` (10):
  - `/api.Shelf/Top`
  - `/api.Shelf/AlbumTop`
  - `/api.Shelf/CheckShooting`
  - `/api.Shelf/SavePhoto`
  - `/api.Shelf/SetPhoto`
  - `/api.Shelf/CreatePhoto`
  - `/api.Shelf/CreateAlbumPhoto`
  - `/api.Shelf/DeletePhoto`
  - `/api.Shelf/SwitchTemplate`
  - `/api.Shelf/UnsetPhoto`
- `Tour` (13):
  - `/api.Tour/Top`
  - `/api.Tour/Ranking`
  - `/api.Tour/AreaTop`
  - `/api.Tour/AreaActivity`
  - `/api.Tour/AreaRefresh`
  - `/api.Tour/AreaScout`
  - `/api.Tour/AreaLive`
  - `/api.Tour/AreaLiveBonus`
  - `/api.Tour/AreaPvp`
  - `/api.Tour/AreaClear`
  - `/api.Tour/AreaNext`
  - `/api.Tour/AreaRetire`
  - `/api.Tour/DeckSave`
- `Tutorial` (8):
  - `/api.Tutorial/Init`
  - `/api.Tutorial/Progress`
  - `/api.Tutorial/StartQuest`
  - `/api.Tutorial/StartFanEvent`
  - `/api.Tutorial/ReceivePromotionReward`
  - `/api.Tutorial/ReceiveLessonReward`
  - `/api.Tutorial/CreateShooting`
  - `/api.Tutorial/Skip`
- `Warning` (1):
  - `/api.Warning/SetDisplayed`

## Unimplemented methods on registered services

These services are registered, but the listed RPCs still dispatch to inherited
generated stubs. The unary interceptor converts their `codes.Unimplemented`
result into the client’s “return to title” response.

- `Auth` (3 of 4):
  - `/api.Auth/Create`
  - `/api.Auth/Delete`
  - `/api.Auth/Recreate`
- `Card` (1 of 11):
  - `/api.Card/RankUp`
- `Division` (1 of 2):
  - `/api.Division/Move`
- `Exchange` (1 of 3):
  - `/api.Exchange/TryOn`
- `Excursion` (1 of 4):
  - `/api.Excursion/SetWatchedGazeReaction`
- `Gacha` (1 of 7):
  - `/api.Gacha/Exchange`
- `Home` (1 of 5):
  - `/api.Home/SetReview`
- `Master` (4 of 5):
  - `/api.Master/Rule`
  - `/api.Master/GetInquiryTemplate`
  - `/api.Master/GetHelpCategory`
  - `/api.Master/GetVersion`
- `Migration` (8 of 12):
  - `/api.Migration/LinkApple`
  - `/api.Migration/MigrateApple`
  - `/api.Migration/LinkGoogle`
  - `/api.Migration/MigrateGoogle`
  - `/api.Migration/RequestTwitterOAuthToken`
  - `/api.Migration/LinkTwitter`
  - `/api.Migration/MigrateTwitter`
  - `/api.Migration/Unlink`
- `Notice` (1 of 3):
  - `/api.Notice/Get`
- `Photo` (5 of 26):
  - `/api.Photo/Report`
  - `/api.Photo/ListSpecialShooting`
  - `/api.Photo/CheckSpecialShooting`
  - `/api.Photo/CreateSpecialShootings`
  - `/api.Photo/ActivateStoryEpisodeShooting`
- `Quest` (2 of 15):
  - `/api.Quest/Start`
  - `/api.Quest/StartBulkDaily`
- `Shop` (9 of 13):
  - `/api.Shop/GetConditionRewardPackageItem`
  - `/api.Shop/RegisterPurchaseTransaction`
  - `/api.Shop/CancelPurchaseTransaction`
  - `/api.Shop/PurchaseStone`
  - `/api.Shop/RecoverPurchaseStone`
  - `/api.Shop/CheckPurchaseStone`
  - `/api.Shop/RegisterBirthday`
  - `/api.Shop/ReceiveConditionReward`
  - `/api.Shop/TryOn`
- `Story` (1 of 8):
  - `/api.Story/ReadAnniversaryStory`
- `User` (1 of 2):
  - `/api.User/ProvideExternalReward`

## Fully implemented registered service surfaces

For comparison, every generated RPC currently has a concrete handler for these
registered services: `Accessory`, `Activity`, `Birthday`, `Company`, `Costume`, `Deck`, `Diary`,
`Dokan`, `Friend`, `Gift`, `Guild`, `Hair`, `Health`, `Hierarchy`, `Item`, `LoginBonus`,
`Love`, `Market`, `Message`, `Mission`, `PhotoPanorama`, `Profile`, `Salary`, `Showcase`,
`Staff`, `System`, and `Telephone`.
