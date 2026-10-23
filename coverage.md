# gRPC API coverage

Last checked: 2026-10-23.

**240/425 APIs implemented; 185 unimplemented.** Of 67 services,
31 have all handlers, 13 have some, and 23 are unregistered.

Paths follow `/api.<Service>/<Method>`. Service links show the declared APIs;
counts link to implementations. In each row, all methods except those listed
as missing are implemented. **All** means the service is not registered.

This is handler coverage, not a guarantee of official behavior: social APIs
may return empty/self-only results, and some live skills and Exercise battle
lessons remain unsupported. `Mission/URLTransition` is an intentional no-op.

| Service | Implemented / total | Missing methods |
| --- | ---: | --- |
| [Accessory](internal/proto/papi/papi_grpc.pb.go#L77) | [3/3](internal/server/rpc/servers/accessory) | — |
| [Activity](internal/proto/papi/papi_grpc.pb.go#L435) | [18/18](internal/server/rpc/servers/activity) | — |
| [Advertisement](internal/proto/papi/papi_grpc.pb.go#L991) | 0/2 | **All** |
| [Anniversary](internal/proto/papi/papi_grpc.pb.go#L1119) | 0/1 | **All** |
| [Auth](internal/proto/papi/papi_grpc.pb.go#L1257) | [1/4](internal/server/rpc/servers/auth) | `Create`, `Delete`, `Recreate` |
| [Backside](internal/proto/papi/papi_grpc.pb.go#L1533) | 0/9 | **All** |
| [Birthday](internal/proto/papi/papi_grpc.pb.go#L1891) | [5/5](internal/server/rpc/servers/birthday) | — |
| [Card](internal/proto/papi/papi_grpc.pb.go#L2217) | [10/11](internal/server/rpc/servers/card) | `RankUp` |
| [Company](internal/proto/papi/papi_grpc.pb.go#L2699) | [11/11](internal/server/rpc/servers/company) | — |
| [Costume](internal/proto/papi/papi_grpc.pb.go#L3097) | [4/4](internal/server/rpc/servers/costume) | — |
| [Deck](internal/proto/papi/papi_grpc.pb.go#L3301) | [3/3](internal/server/rpc/servers/deck) | — |
| [Diary](internal/proto/papi/papi_grpc.pb.go#L3467) | [2/2](internal/server/rpc/servers/diary) | — |
| [Dice](internal/proto/papi/papi_grpc.pb.go#L3763) | 0/15 | **All** |
| [Division](internal/proto/papi/papi_grpc.pb.go#L4241) | [1/2](internal/server/rpc/servers/division) | `Move` |
| [Dokan](internal/proto/papi/papi_grpc.pb.go#L4393) | [3/3](internal/server/rpc/servers/dokan) | — |
| [Dream](internal/proto/papi/papi_grpc.pb.go#L4619) | [7/7](internal/server/rpc/servers/dream) | — |
| [Duty](internal/proto/papi/papi_grpc.pb.go#L4901) | 0/3 | **All** |
| [Exchange](internal/proto/papi/papi_grpc.pb.go#L5079) | [2/3](internal/server/rpc/servers/exchange) | `TryOn` |
| [Excursion](internal/proto/papi/papi_grpc.pb.go#L5269) | [3/4](internal/server/rpc/servers/excursion) | `SetWatchedGazeReaction` |
| [Exercise](internal/proto/papi/papi_grpc.pb.go#L5485) | [4/4](internal/server/rpc/servers/exercise) | — |
| [FishHeaven](internal/proto/papi/papi_grpc.pb.go#L5713) | 0/5 | **All** |
| [Forum](internal/proto/papi/papi_grpc.pb.go#L6015) | 0/9 | **All** |
| [Friend](internal/proto/papi/papi_grpc.pb.go#L6433) | [10/10](internal/server/rpc/servers/friend) | — |
| [Gacha](internal/proto/papi/papi_grpc.pb.go#L6841) | [6/7](internal/server/rpc/servers/gacha) | `Exchange` |
| [Gift](internal/proto/papi/papi_grpc.pb.go#L7123) | [3/3](internal/server/rpc/servers/gift) | — |
| [Gpp](internal/proto/papi/papi_grpc.pb.go#L7277) | 0/1 | **All** |
| [Guild](internal/proto/papi/papi_grpc.pb.go#L7403) | [3/3](internal/server/rpc/servers/guild) | — |
| [Gvg](internal/proto/papi/papi_grpc.pb.go#L7605) | 0/5 | **All** |
| [Hair](internal/proto/papi/papi_grpc.pb.go#L7811) | [1/1](internal/server/rpc/servers/hair) | — |
| [Health](internal/proto/papi/papi_grpc.pb.go#L7913) | [1/1](internal/server/rpc/servers/health) | — |
| [Hierarchy](internal/proto/papi/papi_grpc.pb.go#L8051) | [4/4](internal/server/rpc/servers/hierarchy) | — |
| [Home](internal/proto/papi/papi_grpc.pb.go#L8279) | [4/5](internal/server/rpc/servers/home) | `SetReview` |
| [Invite](internal/proto/papi/papi_grpc.pb.go#L8509) | 0/3 | **All** |
| [Item](internal/proto/papi/papi_grpc.pb.go#L8663) | [1/1](internal/server/rpc/servers/item) | — |
| [Ladder](internal/proto/papi/papi_grpc.pb.go#L8777) | 0/2 | **All** |
| [League](internal/proto/papi/papi_grpc.pb.go#L9001) | 0/9 | **All** |
| [LoginBonus](internal/proto/papi/papi_grpc.pb.go#L9323) | [2/2](internal/server/rpc/servers/login_bonus) | — |
| [Love](internal/proto/papi/papi_grpc.pb.go#L9643) | [17/17](internal/server/rpc/servers/love) | — |
| [Marathon](internal/proto/papi/papi_grpc.pb.go#L10401) | 0/21 | **All** |
| [Market](internal/proto/papi/papi_grpc.pb.go#L11047) | [3/3](internal/server/rpc/servers/market) | — |
| [Master](internal/proto/papi/papi_grpc.pb.go#L11249) | [1/5](internal/server/rpc/servers/master) | `Rule`, `GetInquiryTemplate`, `GetHelpCategory`, `GetVersion` |
| [Message](internal/proto/papi/papi_grpc.pb.go#L11515) | [6/6](internal/server/rpc/servers/message) | — |
| [Migration](internal/proto/papi/papi_grpc.pb.go#L11879) | [4/12](internal/server/rpc/servers/migration) | `LinkApple`, `MigrateApple`, `LinkGoogle`, `MigrateGoogle`, `RequestTwitterOAuthToken`, `LinkTwitter`, `MigrateTwitter`, `Unlink` |
| [Mission](internal/proto/papi/papi_grpc.pb.go#L12303) | [4/4](internal/server/rpc/servers/mission) | — |
| [Mosaic](internal/proto/papi/papi_grpc.pb.go#L12507) | 0/3 | **All** |
| [Notice](internal/proto/papi/papi_grpc.pb.go#L12685) | [3/3](internal/server/rpc/servers/notice) | — |
| [Photo](internal/proto/papi/papi_grpc.pb.go#L13469) | [21/26](internal/server/rpc/servers/photo) | `Report`, `ListSpecialShooting`, `CheckSpecialShooting`, `CreateSpecialShootings`, `ActivateStoryEpisodeShooting` |
| [PhotoContest](internal/proto/papi/papi_grpc.pb.go#L12911) | 0/7 | **All** |
| [PhotoPanorama](internal/proto/papi/papi_grpc.pb.go#L14245) | [3/3](internal/server/rpc/servers/photo_panorama) | — |
| [Profile](internal/proto/papi/papi_grpc.pb.go#L14531) | [12/12](internal/server/rpc/servers/profile) | — |
| [Pvp](internal/proto/papi/papi_grpc.pb.go#L14967) | 0/5 | **All** |
| [Quest](internal/proto/papi/papi_grpc.pb.go#L15341) | [15/15](internal/server/rpc/servers/quest) | — |
| [Race](internal/proto/papi/papi_grpc.pb.go#L15939) | 0/12 | **All** |
| [Roulette](internal/proto/papi/papi_grpc.pb.go#L16351) | 0/3 | **All** |
| [Salary](internal/proto/papi/papi_grpc.pb.go#L16517) | [2/2](internal/server/rpc/servers/salary) | — |
| [Satellite](internal/proto/papi/papi_grpc.pb.go#L16645) | 0/1 | **All** |
| [Shelf](internal/proto/papi/papi_grpc.pb.go#L16855) | 0/10 | **All** |
| [Shop](internal/proto/papi/papi_grpc.pb.go#L17335) | [4/13](internal/server/rpc/servers/shop) | `GetConditionRewardPackageItem`, `RegisterPurchaseTransaction`, `CancelPurchaseTransaction`, `PurchaseStone`, `RecoverPurchaseStone`, `CheckPurchaseStone`, `RegisterBirthday`, `ReceiveConditionReward`, `TryOn` |
| [Showcase](internal/proto/papi/papi_grpc.pb.go#L17929) | [16/16](internal/server/rpc/servers/showcase) | — |
| [Staff](internal/proto/papi/papi_grpc.pb.go#L18445) | [3/3](internal/server/rpc/servers/staff) | — |
| [Story](internal/proto/papi/papi_grpc.pb.go#L18683) | [7/8](internal/server/rpc/servers/story) | `ReadAnniversaryStory` |
| [System](internal/proto/papi/papi_grpc.pb.go#L18991) | [3/3](internal/server/rpc/servers/system) | — |
| [Telephone](internal/proto/papi/papi_grpc.pb.go#L19169) | [3/3](internal/server/rpc/servers/telephone) | — |
| [Tour](internal/proto/papi/papi_grpc.pb.go#L19467) | 0/13 | **All** |
| [Tutorial](internal/proto/papi/papi_grpc.pb.go#L19965) | 0/8 | **All** |
| [User](internal/proto/papi/papi_grpc.pb.go#L20261) | [1/2](internal/server/rpc/servers/user) | `ProvideExternalReward` |
| [Warning](internal/proto/papi/papi_grpc.pb.go#L20389) | 0/1 | **All** |


Audited against [service registrations](internal/server/rpc/rpc.go), generated
RPC declarations, and concrete server methods. Inherited `Unimplemented*Server`
stubs are not counted. Update this file when adding or registering APIs.
