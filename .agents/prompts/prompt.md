Your task is to implement a series of gRPC services related to user login.

The normal login flow is as follows based on the intercepted communication traffic between client and the official server.

```
/api.System/Check
/api.Auth/Login
/api.Master/Get
/api.User/Get
/api.Home/Login
/api.Dokan/List
/api.Home/Enter
/api.Notice/List
```

From this point on, no communication occurs if users don't take actions.

The intercepted request and response body is placed in the `traffic/` directory, which is originally in protobuf format but converted to JSON format for readability. Please use these traffic files as a reference to implement APIs that simulate the official server.

There are 3 directories under `traffic/`, `newbie_first_login` `newbie` and `veteran`, all of them contain the prementioned API traffics but refer to 3 different kinds of user traffic.

`newbie_first_login`: Traffic from a fresh new account's very first login. Note `UserGetResponse` in this folder is for reference only, don't take it too seriously, because we are not going to implement the first login (register) features. In our private server, all users are supposed to be created using `/admin/user/create`, so we are not going to return a response like `UserGetResponse` in this folder even if the user is logging in for the first time.
You may see some traffic that I didn't mentioned before, here is a complete scenario flow for newbies' very first login:

```
/api.System/Check
/api.Auth/Login
/api.Master/Get
/api.User/Get
(api.Tutorial related traffic here, but we are not going to implement tutorial features in our private server, so just ignored this)
/api.Home/Login
/api.Dokan/List (Dokan refers to Japanese "ドカン", meaning onetime popup promotion overlays after user login)
/api.Dokan/SetWatched
/api.Story/ListEvent
/api.Dokan/OutputWatchedLog
/api.System/CheckApp
(/api.Dokan/List, /api.Dokan/SetWatched, /api.Dokan/OutputWatchedLog repeat several rounds)
/api.Home/Enter
/api.Notice/List
```

`newbie`: Traffic from a fresh new account, after the first login.

`veteran`: Traffic from a veteran account. Because many fields in the response are empty for newbie accounts, which may not reflect the real world situation very good, I prepared some traffic from a veteran account. You can find more realistic data in this directory, but some responses tend to be very large, inspect them with cautions.

You can use `jq` or any other tools to inspect these traffic if you think may come in handy.

I also prepared a JSON version of the entire master database under `resources/masterdata/` in case of you need to inspect the master data, but most of these files are very large, inspect with cautions.

Aside from these gRPC services, you also need to implement `/admin/user/create` and `/admin/user/delete` admin APIs, for user creation and deletion. These 2 HTTP APIs have already been implemented, but you are free to review and change them if you find issues.


Some important precautions:

- Do not edit `/api.System/Check` `/api.Auth/Login` `/api.Master/Get`, they have already been completed
- Other APIs are for you to implement. Some of them have already been implemented but you are free to review and change them if you find issues
- Use existing APIs as a reference, try to follow the patterns, but don't force yourself to do so if you feel it's inappropriate
- Although we are writing a private server, don't hardcode the response fields unless you have good reasons to do so, most of the data is supposed to be synced with the database. But fields like `tourRewardResultInfos` and `photoContestRewardResultInfos` in `HomeEnterResponse` mean very little to us, we are not aiming to implement all the functions of the official server, such unimportant fields are fine to be discarded, the boundary here is if things work. If you are not sure whether a field should be hardcoded or not, ask me
- Pay attention to the distinction between server wide data and user data. Data like user division and ongoing event info are apparently server wide, instead of writing those data into user-specific database, you should use `solis` database. If you think there is server wide data needs to be initialized, write that in `internal/db/internal/migration/lv1.go`. If you are not sure whether a field is server wide or user specific, ask me to decide
- No need to 100% reproduce the official server responses, the boundary here is to make things work. Things like notices and time limited events are rotated frequently, a few samples should be enough for us to test.
- Most of the responses from gRPC services have `commonResponse` field, which contains `UpdatedData` and `DeletedData`, informing the client to synchronize local user data cache with the server. Because it appears frequently and to make sure client data is synced whenever user data is get changed on the server side, perhaps you need to find a smart way to handle it
- Remember to write test. You may need to create mock data (create user) in the test function, try logging in, and do cleanups. Some of it has already been implemented in `test/user_test.go`, but it's your call to change it

---

I've reviewed and modified some of your code. Based on the result of the review, here are some additional rules:

- Don't write tests outside `test/` directory. There are some pre-works must be done before every test, such as starting the main server and configure the http/grpc clients, if you write code elsewhere, those pre-works are not get executed
- Don't create your own new users in test even if you do cleanups afterwards, always use test user defined in testConfig

Now, this time, your task is to implement some more APIs, which includes:

```
/api.Accessory/Enhance
/api.Accessory/LimitBreak
/api.Card/Enhance
/api.Card/SkillEnhance
/api.Card/LimitBreak
/api.Card/LiveAbilityEnhance
/api.Card/SetSupport
/api.Division/List
/api.Exchange/List
/api.Exchange/Execute
/api.Shop/List
/api.Shop/ListItem
/api.Shop/GetLoginBonusPackageItem
```

Same as before, the intercepted request and response body is placed in the `traffic/` directory, but only from veteran account this time, because most of these features are only available after the user reached a specific level.

My instruction from the last conversation that "just make it work is enough" does not apply this time. When implementing these APIs, remember to update user data in the database when you should, instead of just simply returning a response to make the client work. For example, if a user is trying to enhance a card, you should check if the user has enough resources to do so, then consume the resources, update the user card, return the response. Everything is just like a real server with righteous logic. If you find any missing user data collections, add them into the functions for creating and deleting users.

After that, implement an admin API `/admin/user/god`, which accept a `PublicUserId` or `ServerUserId` in request, and unlock all features for the specified user, which include but NOT LIMITED TO user level (manager level), cards, costumes, stories. You may compare the UserGetResponse and HomeEnterResponse from veteran user and newbie user to guess what user collections you should update, and include a list of collections you updated for me to review in your reply.
I need you to update user data dynamically. By "dynamically", it means you need to check the master database, find the data you need, and update the user data based on it.
For example, to update user level you need to check `ManagerLevel` collection in the master database, find the max level, then use the data you found to update user EXP. To update user cards, you need to iterate all cards in the master database, and insert those cards to `userCard` collection.


---


I've reviewed and modified some of your code.
Please fix your code under the following instructions:

- Do not add files that are not gRPC servers in the `internal/server/rpc/servers` package. If you need common functions that can be used across servers, write them in `internal/server/rpc/servers/internal/viber` for better code management.
- Same as above, do not add files that are not HTTP handlers in `internal/server/http/game` or `internal/server/http/admin`. Instead, write common functions in `internal/server/http/internal/diver`.
- Use yaml instead of JSON to store static `catalogdata`.

---

I've reviewed and modified some of your code. Note your previous task overwrote some of my modified code, I have already done the restoration but please be careful this time.

Please fix your code under the following instructions for migration related packages.

- Remove redundant divisions. We are not going to implement multiple divisions in our private server, only keeps `hVs7e3Np-00001`.
- Move hardcoded init data to yaml files like catalog data.
- Don't use global variables to store raw yaml data since the global variables live the entire program lifecycle, those one-time data is supposed to be reallocated after use.

---

I've reviewed and modified some of your code.
Please refactor `testFreshCommerceCatalogIdentity`, `testUserGod`, `testCommerceAPIs` and `runAccessoryCardScenario`, move those tests out of `user_test.go`.
We are not going to test everything in `user_test.go`, one integration test function is supposed to test only one complete scenario flow. If you need to do some pre-works or after-works such as creating and deleting the test user, please do that in every test function, or maybe create a helper function so you can reuse it. 

---

Some values in database, such as overall average user level in a division and shop items, are subject to change from time to time, we are not able to predict the outcomes. Please fix this kind of assertions in the test cases.

---

I've reviewed and modified some of your code.
Make `godMinimumResourceAmount`, `godMinimumBalanceAmount` and `godMinimumPointAmount` in `makino/user/god.go` public, add user points/resource amount assertions to god test.

---

I've reviewed and modified some files since our last conversation.

This time, your task is to implement more APIs, which includes:

```
/api.Migration/List
/api.Market/ListItem
/api.Market/Purchase
/api.Market/Reset
/api.Gacha/List
/api.Gacha/Draw
/api.Gacha/Draw
/api.Gacha/Probability
/api.Gacha/History
/api.Gacha/SelectCard
/api.Gacha/ReceiveFreePremiumGachaTicket
/api.Costume/SetLiveCostume
/api.Costume/SetCostume
/api.Costume/CheckBulk
/api.Story/Read
/api.Story/ReadEvent
/api.Story/ReadExtra
/api.Story/ListEvent
/api.Story/ReadCharacterCompanyEnjoyStory
/api.Diary/GetInfos
/api.Diary/Read
/api.Message/ListGroup
/api.Message/Timeline
/api.Message/Finish
/api.Message/SaveHistory
/api.Message/Save
/api.Telephone/CallHistory
/api.Gift/List
/api.Gift/Receive
/api.Gift/ListHistory
/api.LoginBonus/List
/api.LoginBonus/Receive
/api.Excursion/List
/api.Excursion/Start
/api.Excursion/Finish
```

The intercepted request and response body is placed in the `traffic/mixed/round1` directory, most originates from veteran account, with a small number from newbie account.
Like before, when implementing these APIs, remember to update user data in the database when you should, instead of just simply returning a response to make the client work, everything should work like a real server with righteous logic. If you find any missing user data collections, add them into the functions for creating and deleting users. Also don't forget to update `/admin/user/god` if you find anything missing for promoting a god user.

Some hints:
- You may see a lot of traffic targeting a same API, such as `/api.Gacha/Draw`, there are 17 pairs of requests and responses for this single API. That's because there are many different kinds of gacha in the game, you can gain a better understanding of how the official API is implemented by investigating the differences between these traffic patterns.
- Things like login bonus, is supposed to be refreshed on a daily bias. A free market reset chance is supposed to be available twice a day at 0:00 and 12:00. All prementioned times are based on UTC+9.
- You may need to modify the previous implemented APIs based on the traffic I gave you this time.
- Use MongoDB MCP server instead of mongosh to interact with database when possible.

---

Don't modify any files, just answer my questions this time.

I tested the server with the official client, it generally works well, but still have many flaws:

- Looks like `/api.Shop/ListItem` always return an empty list. Is shop items server-wide data or user-specific?
- About `/api.Market/ListItem`, what's the current implementation for selecting market items? Random or fixed? If random, what's the rules? If fixed, where can I find the fixed item list?

---

I have put intercepted request and response body of `/api.Shop/ListItem` into the `traffic/mixed/round2` directory, there are 2 types of the traffic: from our private server and the official version, you may investigate the difference to solve the issue I mentioned before.
I also put 2 pairs of `/api.Shop/Purchase` traffic into the directory, please implement the API.
Use MongoDB MCP server instead of mongosh to interact with database when possible.

---

Please refine your implementation under the following instructions:

- After testing with the official client, I found there are too many gacha entries in the game gacha interface making me feel overwhelmed, most of them are time limited free ticket gacha. Most likely caused by there are too many types of gacha tickets in the user's possession. Please exclude such type of tickets from godfying a user.
- Remove time limited shops (including 衣装ショップ). We don't want to manage such a time sensitive shop list in our private server. If we need to adjust shop items, we can simply add or remove items to normal item shop. The remaining shops should be normal shop, dia shop, 2 login pack shops, and a premium gacha ticket shop.
- A bug: empty shop item list received after trying to entering premium gacha ticket shop. Treat it as a critical bug, because the client will go unresponsive if it received an empty shop item list when trying to entering a shop according to the current official client's implementation. Make sure the returned item list is not empty when `ListItem` is invoked, if a shop is indeed empty from a user's perspective, don't show that shop to the user in the first place.
- Make sure no real-money shop items in our private server. If some items originally require real-money purchase, make it free. Also, remove purchase limit for all items.
- Godfying a user should unlock all main story episodes and mark all main story episodes read. Otherwise users will have to read main story from the beginning to unlock episodes one by one. Other stories such as character stories don't need to change.

---

There are too many migration layers in our project. Our project is still under development, not publicly released. Since level 2~4 are only for updating shop catalog and it is already migrated in our local database, please remove level 2~4 mirations and intergrate shop catalog migration into level 1, and directly update migration flag in our database to 1.
I forgot to start the mongodb before our last conversation, now it is stated, you can use mongo db MCP server to interact with it.

---

You don't need to modify files this time, just answer my questions.
I see shop items productId are still non-empty in dia_items.yaml, but I think you have implemented no real-money purchase features. How did you implemented it?

---

Looks like some problems occured when executing `TestCommerceAPIs`, please fix it:
```
--- FAIL: TestCommerceAPIs (2.18s)
    commerce_test.go:134: Shop.List response is inconsistent with current catalog: shop=id:"shop_login_l......
```

---

This time, your task is to implement `/api.Home/SetCharacterPosition`, the intercepted request and response body is placed in the `traffic/mixed/round3` directory.

---

The user home position data is also supposed to be retrieved by the User/Get API we implemented recently, did you check that API if it works with expectations?

---

I agree. Please add `User/Get` invocation after SetCharacterPosition and assert those values for the test.

---

This time, your task is to implement `Activity` related APIs. There are 4 kinds of idol activities in the game: lesson, fan event, promotion, refreshment. All 4 are somewhat like "放置ゲーム" in Japanese term, refers to games where users will receive rewards by simply waiting for time to pass. These APIs are:

```
/api.Activity/ReceiveLessonReward
/api.Activity/PromoteLesson
/api.Activity/StartFanEvent
/api.Activity/LoadFanEvent
/api.Activity/FetchFanEventSteps
/api.Activity/SaveFanEvent
/api.Activity/FinishFanEvent
/api.Activity/GetFanEventRanking
/api.Activity/StartPromotion
/api.Activity/LoadPromotion
/api.Activity/UsePromotionItem
/api.Activity/ReceivePromotionReward
/api.Activity/FetchPromotionSteps
/api.Activity/FinishPromotion
/api.Activity/StartRefresh
/api.Activity/LoadRefresh
/api.Activity/UseRefreshItem
/api.Activity/FinishRefresh
```

The intercepted request and response body is placed in the `traffic/mixed/round4` directory.
Like before, when implementing these APIs, remember to update user data in the database when you should, instead of just simply returning a response to make the client work, everything should work like a real server with the righteous logic.
After implementation, check if there are related previous implemented APIs need to be refined based on the APIs we implemented this time, especially `Home/Enter` API, 4 kinds of activities are supposed to wired with its response.
Note `Activity/GetFanEventRanking` API originally returns server-wide ranked users, but in our private server we only need to return the user himself, even if there are multiple users in our server.

---

Based on real-device testing, I found many activity related issues (not technical, but rather game logic ones). Let me give you a rough overview about idol activities using the official help documentation (with my own notes in parentheses).


## お仕事 (activity)
お仕事では、「ファン」の獲得・「コアファン率」の上昇・「元気」の回復を行うことができます。

■お仕事の詳細
お仕事には、ファンイベント・プロモーション・リフレッシュの3種類があります。
お仕事は、ゲームをプレイしていない間も時間経過で進行します。
また、お仕事の種類に応じて、様々な報酬を獲得することができます。

■お仕事時の衣装とヘアスタイル
お仕事を行うアイドルの選択時に、衣装やヘアスタイルを変更することができます。
お仕事の内容によって選択できる衣装は異なり、一部お仕事では衣装やヘアスタイルを選択できない場合があります。

■お仕事Lv
各アイドルにはお仕事Lvがあり、お仕事をすることでLvが上昇します。
お仕事Lvが上昇すると元気の最大値が上昇します。
また、お仕事Lvの上限はメインライブの進捗で解放していきます。

■元気
「ファンイベント」と「プロモーション」は元気がゼロになるまで進行します。
時間経過か「リフレッシュ」で「元気」を回復することが出来ます。
時間経過による回復は30秒に1回復します。
お仕事の進行中や、お仕事の完了をしていない場合は「元気」が回復しません。

■ファン数とコアファン率
ライブの来場ファン数に関わります。
詳細は「ヘルプ > ライブ > ■来場ファン数」 をご覧ください。


## ファンイベント
ファンイベント終了時の「ファン満足度 (`activityPoint`?)」に応じたコアファン率の上昇と、報酬の獲得を行うことができます。

■ファンイベントの詳細
アイドルを3人選択してファンイベントを開始します。
種類とLvに応じて、ファン満足度の獲得効率が変わります。
ファンイベントは途中で終了することができません。
ファンイベントを行っているアイドル全員の元気がゼロになった時点で終了となります。(Idols don't share a same stamina pool. For example, if idol A's stamina drops to 0 but the other 2 idols still have plenty of stamina, A's activity terminates at that point but the other 2 idols continue.)

■ファンレターと応援
ファンレターを持ったファンが登場することがあります (random)。
ファンレターをもらうと、「応援ゲージ (`currentCheerPoint`)」が上昇します。
応援ゲージが溜まる (`currentCheerPoint` reaches 100) と、他のアイドルを応援に呼ぶ (`execCheer`, then `currentCheerPoint` -100) ことができます。
他のアイドルが応援に来ている間は、ファン満足度の獲得効率が上昇します。

**My additional comments**
Let's call "ファンレターを持ったファン" as "special fans".
My own understanding of fields in `FanEventProgress`:
- `specialFanCheerPoint`: How much cheer points can be obtained from each special fan.
- `newSpecialFansCount`: How many unrealized special fans appeared since the last load. Once realized, this field should be cleared.

■フィーバードリンク
一定ステップの間、FEVER状態になり、応援ゲージ+1回
※応援ゲージ上昇効果は適用されません

■FEVER状態
「フィーバードリンク」を消費して (`usedSuperModeItem`)、一定の間FEVER状態になります。
FEVER状態になると、直ちに応援ゲージ+1回 (`currentCheerPoint` +100)。
FEVER状態の間は、ファン満足度の獲得効率が上昇します。
FEVER状態の間は、タイミングよくTAPすることでファン満足度の獲得効率が上昇します。

**My additional comments**
Looks like how much `activityPoint` can be obtained during the fever mode is calculated on client side depending on user actions, since the server cannot predict the user's behavior, we should not validate `activityPoint` in `Activity/SaveFanEvent`.

■ファンイベントのLv
ファンイベントを行うことで、ファンイベントのLvが上昇します。
ファンイベントのLvが上昇すると、そのファンイベントの効率が上昇します。
メインライブを進行させることで各ファンイベントのLv上限が上昇します。
ファンイベントのLvごとにコアファン率に上限があり、アイドルのコアファン率が上限に達しているとコアファン率は増加しません。
コアファン率の上限はアイドルのファン数によっても変動します。

■ランキング
ランキングから、グループ内のハイスコアランキングを確認することができます。

■コアファン率
ライブの来場ファン数に関わります。
詳細は「ヘルプ > ライブ > ■来場ファン数」 をご覧ください。

■獲得アイテム
ファンイベントを行うことで報酬を獲得できます。
下記のファンイベントでは「フォトレシピ」を入手することがあります。
・握手会：Lv6以上
・撮影会：Lv4以上
・サイン会：Lv2以上
また、「スイーツギフト」を消費すると、アイテムの獲得効率が2倍になります。


## プロモーション
時間経過に応じたファンの獲得と、報酬の獲得を行うことができます。

■プロモーションの詳細
プロモーションを行うには、アイドルを3人選択する必要があります。
プロモーションの種類とLvに応じて、効率が変わります。
プロモーションは途中で終了させることができます。
途中で終了した場合、その時点までのファンの増加と報酬の獲得を行うことができます。

■プロモーションのLv
プロモーションを行うことで、プロモーションのLvが上昇します。
プロモーションのLvが上昇すると、そのプロモーションの効率が上昇します。
メインライブを進行させることで各プロモーションのLv上限が上昇します。
プロモーションのLvごとに獲得可能なファン数に上限があり、アイドルのファン数が上限に達しているとファンは増加しません。

■プロモーションの時間短縮
「スキップウォッチ」や「SPスキップウォッチ」を消費することで、プロモーションの時間短縮をすることができます。

■ファン数
ライブの来場ファン数に関わります。
詳細は「ヘルプ > ライブ > ■来場ファン数」 をご覧ください。

■獲得アイテム
プロモーションを行うことで報酬を獲得できます。
下記のプロモーションでは「レタッチインク」を入手することがあります。
・宣伝：Lv12以上
・撮影：Lv6以上
・出演：Lv6以上
下記のプロモーションでは「スピリットライザー」を入手することがあります。
・宣伝：Lv45以上
・撮影：Lv40以上
・出演：Lv40以上


## リフレッシュ
時間経過に応じて、アイドルの元気を回復させることができます。

■リフレッシュの詳細
リフレッシュは、コインを消費して行います。
必要コイン・回復量・所要時間はリフレッシュ内容によって異なります。
リフレッシュは途中終了することができません。

■リフレッシュの時間短縮
「アワーグラス」を消費することで、リフレッシュの時間短縮をすることができます。


I also prepared some more fan event related traffics for you to examine, the intercepted request and response body is placed in the `traffic/mixed/round5` directory. All round5 traffics are origin from a single fan event activity execution, ordered by time sequence.

Some special points:
- `ActivitySaveFanEventRequest7`: sent immediately after executing cheer
- `ActivitySaveFanEventRequest11`: sent immediately after using fever drink
- `ActivitySaveFanEventRequest12`: sent immediately after using cheer (cheer points obtained from using fever drink)
- traffics between `ActivitySaveFanEventRequest11` and `ActivitySaveFanEventResponse15` are all sent during fever mode's duration

With the aforementioned information in mind, please fix the activity related game logic.

---

Based on the real-device test, the following error occurred at the first `Activity/SaveFanEvent` invocation after using fever drink (the `Activity/SaveFanEvent` triggered when using a fever drink itself looks working well).

```
2026-08-22T17:04:11.094+0900    INFO    GRPC    interceptor/interceptor.go:51   [gRPC] /api.Activity/FetchFanEventSteps {"layer": "grpc", "api": "/api.Activity/FetchFanEventSteps"}
2026-08-22T17:04:25.762+0900    INFO    GRPC    interceptor/interceptor.go:51   [gRPC] /api.Activity/SaveFanEvent       {"layer": "grpc", "api": "/api.Activity/SaveFanEvent"}
2026-08-22T17:04:25.766+0900    ERROR   GRPC    interceptor/interceptor.go:98   RPC failed with error{error 26 0  transaction failed: rpc error: code = InvalidArgument desc = fan event steps must be sequential and elapsed}        {"layer": "grpc", "api": "/api.Activity/SaveFanEvent", "request_id": "639230150682005230", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
```

The intercepted request and response body on our own private server is placed in the `traffic/mixed/fix-activity` directory, the aforementioned error occurred when `ActivitySaveFanEventRequest4 (error)` was sent.

Maybe we should not verify step integrity on server side? Please investigate the error and fix it, then tell me the potential cause of this error.

---

This time your task is to resolve the following issues.

- There is a chance that `Home/SetCharacterPosition` API returns empty `characterId`, which causes no character appears on the specified position.
- In `Activity/PromoteLesson`, promotions using dia has daily limit, but using item is supposed to have no limit.

---

This time please fix the following shop related issues.

- normal shop (`shop_top_shop`): Purchase request ends normally without error. After purchase, items can be aquired normally, but dia cannot. (e.g. If both items and dia are included in a bundle, items are added to user's possessions but dia does not change after purchase)
- dia shop (`shop_top_dia`): Bundles cannot be purchased, no communication happens (the client does not send `Shop/Purchase` request at all). I guess some fields in shopItems may affect client's purchass behavior? Since we cannot debug the client, we have no concrete information about what exactly the error is, just try to guess the possible cause and implement the potential fix. Then tell me your thoughts on this issue.

---

This time, your task is to implement the following 2 APIs:

```
/api.Story/ListExtra`
/api.Costume/CheckCostume`
```

The intercepted request and response body is placed in the `traffic/mixed/round6` directory.

---

I've updated `AGENTS.md`, added the rules about how to use `resources/masterdata/` directory in case you need.

This time your task is to fix or implement the `Message` and `Telephone` related services, which include:

```
/api.Message/ListGroup
/api.Message/Timeline (return all history messages of a single message group, messages that haven't taken place yet should not be included)
/api.Message/Receive
/api.Message/Save
/api.Message/Finish
/api.Message/SaveHistory
/api.Telephone/Receive
/api.Telephone/Finishes
/api.Telephone/CallHistory
```

Before we start, let me give you an overview about these features using the official help documentation.


## メッセージと電話
アイドルとメッセージのやりとりができます。また、電話がかかってくることもあります。

■メッセージの詳細
アイドルとメッセージでやりとりをすることができます。
メッセージの内容は、アイドルの「信頼度」に応じて変化します。
メッセージのやりとりをすると、アイドルの信頼度が上昇することがあります。

■電話の詳細
アイドルと電話をすることができます。
電話の内容は、アイドルの「信頼度」に応じて変化します。

■プッシュ通知
ゲームをプレイしていない間もアイドルからのメッセージは届きます。
プッシュ通知をオンにすることで、メッセージが届いた際、通知を受け取ることができます。


When messaging with characters, players can select a reply from a list of available options. A single conversation may involve multiple rounds of such interaction.
Let's use a `TimelineInfo` from a `Message/Timeline` response as an example.

```json
...
{
  "messageGroupId": "message_group_ngs",
  "timelineId": "1737867240000000000",
  "messageId": "message-instant-ngs-00003",
  "arrivedTime": "1737867240000",
  "selectMessageDetailIds": [
    "3-1",
    "6-1",
    "10-2"
  ],
  "lastMessageDetailId": "12",
  "lastMessageDetailTime": "1738169869413",
  "finished": true
},
...
```

- `messageId` is an identifier for a document in `Message` master data collection.
- `selectMessageDetailIds` recorded which replies did the user choose. Unless there is not options for the message or the conversation has not finished, it must not be empty (otherwise the client will crash, tested on real-device).
- `lastMessageDetailId` recorded the current progress of this message, usually the last one of the `messageDetailId` in a `Message` master data document for finished conversations, but may vary if the user exit the message UI in the middle of a conversation.
- For newly received unread messages, leave `lastMessageDetailId` empty and set `lastMessageDetailTime` to zero.
- There can be only 1 or 0 unfinished conversation at a time.
- The order of `TimelineInfo`s is ordered by `timelineId` (or `arrivedTime`?) desc.
Note the analysis above is my own assessment, feel free to correct me if you think I'm wrong.

I prepared a bunch of traffics from the official server for you, the intercepted request and response body is placed in the `traffic/mixed/messages` directory.

Some additional analysis of my own (feel free to correct them).

For `Message`:

- Most messages have unlock conditions, most of these conditions are `cd-ms-read-message-*` (unlock after read specific message) and `cd-st-read-st-*` (unlock after read specific story), so we should add logic for checking if new messages can be unlocked (scheduled) to `Story/Read` and `Message/Finish`. Besides that, according to the evidence from the official server response, `Home/Enter` also can have `commonResponse.updatedData.messageSchedules`, may be we also need to add such logic to `Home/Enter`.
- The rules of schedule time of new unlocked messages are vary from message type. For story messages, it's a random delay time up to 1 hour. For else, it's a random delay time up to 12 hours.
- After finished a message, envovled character's reliability should be raised by 10.
- Only 1 message for a character can be scheduled at a time even if there are more than 1 unlocked but unread/unscheduled messages exist. Also, if the user has unfinished message, don't schedule new message until that message has been finished. Schedule each one by one using the order in which they appear in the master database.

For `Telephone`:

- Most unlock conditions are `cd-ms-read-message-*`. Like `Message`, we should add logic to check new unlocked telephones to `Message/Finish` and `Home/Enter`.
- Like `Message`, the schedule time for telephone should be a random delay up to 18 hours.


After implementing `Message` and `Telephone` APIs, change the behavior for `/admin/user/god` API.

Add a `feature` field to request body, for example
```go
type UserGodRequest struct {
	PublicUserId string `json:"publicUserId,omitempty"`
	ServerUserId string `json:"serverUserId,omitempty"`
	Feature      GodFeature `json:"feature,omitempty"`
}

type GodFeature struct {
	Message string `json:"message,omitempty"`
}
```
This field allows users to select the features they wish to unlock, for now it's message (telephone included) only but we may add more fields for users to select in the future.
The value of `message` can be one of `none`, `all`, `enjoy`, or not exists.
`none` means don't unlock messages and telephones.
`all` means unlock all messages and telephones and mark then all read (simply use the first option in the select list).
`enjoy` means only unlock unconditional messages and telephones, don't mark then read (let them be scheduled one by one normally).
Note you may need to set corresponding stories read/unread to ensure data relevance. 
If `message` is absent, defaults to `all`.

---

You don't need to edit files this time, just answer my questions.

How did you implement schedule related features?
Did you add `messageSchedules` to `commonResponse.deletedData` in `Message/Finish` and `Telehpne/Finishes`?

---

This time your task is to add some features to gRPC interceptor.

**Users must re-login after the date changes**

Check request time in interceptor, if it's the first time for a user in a day (based on UTC+9), add the following HTTP headers to gRPC response and return empty body:

```
x-error-action: title (force client returning to title)
x-error-message-bin: 5pel5LuY44GM5aSJ44KP44KK44G+44GX44Gf (base64 encoded "日付が変わりました")
x-error-code: 1003
```


**Add idempotency check for requests which have `x-app-request-id` in their header**

Client will include a `x-app-request-id` header in the request for actions require ensuring idempotency.
The values look like uint64 values. such as 639230407163253360 and 639230409085488700.
Check the header and return an error if the idempotency is violated.

---

Previously we changed the type of shop `shop_top_dia`, the new shop works perfectly, but there is still an old version of dia shop's entrance displayed in the game's shop page. I think perhaps the game client hardcoded the dia shop entrance since it has no expire date and should always be there.
So, let's restore the dia shop's type, let the purchase to fail (since the client does not crash and no request will be sent). As an alternative, let's dulplicate the same items (paid dia) into normal shop so the purchase can be processed normally by our server. Please place these dia items at the beginning.

---

This time your task is to implement `/api.Hair/CheckHair` API.
We don't have a real server response for now, but perhaps you can refer to `/api.Costume/CheckCostume`, they share many similar respects.

---

This time your task is to implement `Company` service.
Let me give you an overview about the `Company` using the official help documentation (with my own notes in parentheses).


## 事務所 (Company)

「事務所ポイント」のランクと順位を確認することができます。

■事務所ランク (CompanyGrade) の詳細
事務所ランクは、「事務所ポイント」が指定の累計ポイントを超えると上昇します。
「事務所ポイント」は「交流Pt」「実績Pt」「勤怠Pt」の合計になります。

■交流 (CompanyEnjoy) の詳細
プレゼントやミッションのクリアに応じてアイドルごとに「交流Pt」を獲得することができます。
交流Ptや交流Lvはアイドルごとに異なり、指定の累計ポイントを超えると対象のアイドルの交流Lvが上昇します。

■実績の詳細
実績のミッションのクリアに応じて「実績Pt」を獲得することができます。
指定の累計累計ポイントを超えると実績Lvが上昇します。

■勤怠の詳細 (No need to implement this feature since we don't have enough information for now)
各月の最終日12:00に、1ヶ月間の活動内容に応じたダイヤと、給料に応じた勤怠Ptを獲得することができます。
計測は各月の最終日前日23:59までで、最終日は計測対象外です。


## 交流 (CompanyEnjoy)

アイドルとの交流や交流Lvを確認することができます。
■交流 (CompanyEnjoy) の詳細
プレゼントやミッションのクリアに応じてアイドルごとに「交流Pt」を獲得することができます。
交流Ptや交流Lvはアイドルごとに異なり、指定の累計ポイントを超えると対象のアイドルの交流Lvが上昇します。

■総合交流Lv
メモリーズやコラボを除く、加入している全てのアイドルの交流Ptの累計ポイントによって、総合交流Lvが決まります。
総合交流Lvは、指定の累計ポイントを超えると上昇します。

■メモリーズ・コラボ
メモリーズやコラボアイドルは時間や世界の異なる特別なアイドルです。
メモリーズやコラボアイドルとは交流することができません。
また、メモリーズやコラボアイドルは、加入アイドルの交流Ptの平均値によって交流Lvが変化します。

■思い出
「交流ストーリー」「アイドル編ストーリー」「会話」を確認することができます。

■交流ストーリー
アイドルたちの日常を描いたストーリーです。
交流ストーリーは、交流ストーリーの対象のアイドルの交流Lvを、一定のLvまで上げることで解放されます。
交流ストーリーはストーリー撮影機能で撮影することができません。

■プレゼント
アイドルの交流Ptを上げることができます。
一部のプレゼントは、アイドルに応じて上昇する交流Ptが変化します。

■ミッション
クリアすると対象のアイドルの交流Ptを獲得することができます。

■交流ボーナス
交流Lvに応じて、対象のアイドルのエールを獲得することができ、ライブやお仕事で適用されます。
対象のアイドルが指定の交流Lvに到達すると、エールが強化されます。

■報酬
交流Lvが上昇すると報酬を獲得することができます。

■ランキング
ランキングから、グループ内の対象アイドルの交流Lvによる順位を確認することができます。

■ポーズ撮影 (ExpressionShooting)
指定したメンバー・お仕事・衣装・ヘアスタイルでのお仕事シーン(3D)を撮影し、フォトを獲得することができます。
撮影時には、ポーズ・表情・地点・視点を指定して撮影を行うことができます。

■おでかけ (Excursion)
アイドルとおでかけして、アイドルの反応を楽しんだり、撮影を行うことができます。


The intercepted request and response body is placed in the `traffic/mixed/company` directory.
The followings are some things you need to pay attention to during implementation.

- Make good use of the master database. A lot of information such as `CompanyGrade` can be retrieved from there.
- You only need to implement methods under `Company` this time. Features such as 撮影 and おでかけ are mentioned in the official help document but we are going to implement them in the future, not this time.
- The captured traffic is not a complete set of methods under `Company` service. If you are not sure about how to implement an API without the official traffic as a factual evidence, do not implement it and tell me what you need in the reply. I will see if I can grab some samples to you.
- `Company/ListRanking` and `Company/ListEnjoyCharacterRanking` APIs originally return server-wide user ranking, but in our private server we only need to return the user himself, even if there are multiple users in our server.
- Don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time.

---

I conducted a new round of traffic capture, but only able to trigger `Company/ReceiveGradeRewards`, the other 2 got no luck. Please implement `Company/ReceiveGradeRewards` based on the official traffic, and try to implement the other 2 refer to other `Company` related APIs. Since we lack factual evidence, please tell me which points you might be unsure about after implementation.
The captured request and response body is placed in the `traffic/mixed/company_round2` directory. Despite the traffic for new API, I also added some more traffic for already-implemented company related APIs, perhaps you can utilize these traffic to refine our implementation if you find mistakes we made before.

---

Let's implement "撮影" related features. Before implementing gRPC APIs for game logic, we need to first implement user photo image upload/download HTTP APIs.
The official team uses firebase storage to store user photos, so we must simulate the same behavior in our private server.
The captured HTTP traffic is recorded in `traffic/http/photo.txt`, please use it as a factual evidence to implement these APIs.
The followings are things you need to pay attention to during implementation.

- The name of the photo has the structure of "Photo/${USER_ID}/${FILE_NAME}". For example, in "Photo/AL7XXMDK/5ba5bdae-8fc8-470e-9c97-292520a10b80", "Photo" is a const, "AL7XXMDK" is a user public ID (always 8-character capitalized alphanumeric string), the last part of the path is a random string, usually UUID but not always.
- Use database to store photo image metadata .
- Use file system to store photo images. For example, if the name is "Photo/AL7XXMDK/5ba5bdae-8fc8-470e-9c97-292520a10b80", you should save the file to "${SOLIS_RESOURCE_DIR}/Photo/AL7XXMDK/5ba5bdae-8fc8-470e-9c97-292520a10b80.jpg".
- Before creating directories and saving files, remember to check the legality of the incoming inputs.
- I have already written a little part, please complete it based on what I've done.

---

You don't need to edit any files this time, just answer my questions.

I see you load the entire image bytes into memory and decode it to test if it's a valid JPEG file.
Is it possible to direct the request body stream to file without loading the contents into memory? I know this will introduce a little security flaw, but in this case I want to prioritize resource usage.

---

Since Fiber already pre-buffered 4 MiB body and the image file uploaded from the client is usually less than 1MiB, I think we don't have to force ourselves to stream the body directly into file.
But decoding the entire image file is a bit of overkill, please remove the relevant logic.

---

We have completed the photo image upload/download feature, now let's implement the rest of "撮影" related features. This time they are gRPC APIs.
Let me first use the official help document to explain what is "撮影" in the game.


## 撮影

「撮影フィルム」を消費してアイドルを撮影し、フォトを獲得することができます。

### 撮影の詳細

撮影には、ライブ撮影とお仕事撮影、ポーズ撮影、ストーリー撮影の4種類があります。
フォトの品質・レアリティ・オプションはランダムで決まります。
撮影フィルムのLvが高いほど、高い品質のフォトを獲得できる可能性が上がります。
フォトを「現像」することで、被写体にしたアイドルの信頼度が上昇します。
稀に、アイドルの専用フォトを獲得することができます。(dedicated photos are always ★5)

### 撮影の注意点

撮影する楽曲・ステージ・お仕事・ストーリー内容は、フォトの品質・レアリティ・オプションには影響がありません。
撮影したフォトの出来栄えは、フォトの品質・レアリティ・オプションには影響がありません。

### 獲得フォトのレアリティ

レアリティは「★1 ~ ★5」の中から抽選されます。
撮影する楽曲・ステージ・お仕事・ストーリー内容は、レアリティには影響がありません。
撮影したフォトの出来栄えは、レアリティには影響がありません。

### 獲得フォトの品質

フォトの品質は使用する「撮影フィルム」によって変動します。
Lvの高いフィルムほど、高品質なフォトの獲得を期待できます。
撮影する楽曲・ステージ・お仕事・ストーリー内容は、フォトの品質には影響がありません。
撮影したフォトの出来栄えは、フォトの品質には影響がありません。

### 信頼度

現像時、フォトに最も大きく写したアイドルの信頼度が上昇します。
(Let's don't use image recognition for this since that would introduce resource-intensive computations, instead use known information from request to determine which idol got photographed. If we do not have sufficient information to determine, randomly choose an idol.)
信頼度が高くなると、届くメッセージの内容やホーム画面での会話内容が変化します。


## ライブ撮影

指定したメンバー・楽曲・衣装・ヘアスタイル・会場でのライブシーンを撮影し、フォトを獲得することができます。


## お仕事撮影

指定したメンバー・お仕事・衣装・ヘアスタイルでのお仕事シーン(3D)を撮影し、フォトを獲得することができます。
撮影時、ジャイロをONにすることで、ジャイロ操作で撮影を行うことができます。
一部アイドルでは撮影できないお仕事があります。(Please disregard this restriction, make all idols available for all situations)


## ポーズ撮影

指定したメンバー・お仕事・衣装・ヘアスタイルでのお仕事シーン(3D)を撮影し、フォトを獲得することができます。
撮影時には、ポーズ・表情・地点・視点を指定して撮影を行うことができます。
一部アイドルでは撮影できないお仕事があります。
また、一部衣装やヘアスタイル、お仕事では使用できないポーズがあります。(Please disregard this restriction, make all idols & outfits & hairstyles & poses available for all situations)


## ストーリー撮影

本編、番外編、アイドル編のストーリーを撮影し、フォトを獲得することができます。
ストーリーで撮影したフォトを現像する場合、対象アイドルがフォトの内容と一致しない場合がございます。
また、アイドル専用フォトを獲得できる「専用フォト確定フィルム」など、一部フィルムはストーリー撮影で使用することができません。予めご了承ください。


## グッズ撮影

自分でカスタマイズしたグッズケースを撮影し、フォトを獲得することができます。
対象アイドルがフォトの内容と一致しない場合がございます。
また、アイドル専用フォトを獲得できる「専用フォト確定フィルム」など、一部フィルムはグッズ撮影で使用することができません。予めご了承ください。


## 特別撮影

特別な撮影をすることができます。
特別撮影では、通常の撮影とは異なるフィルムを消費して撮影を行います。
撮影に必要なフィルムについては、開催時のお知らせをご確認ください。


The APIs you need to implement this time are:

```
/api.Photo/SwitchLock
/api.Photo/Delete
/api.Photo/Enhance
/api.Photo/Reroll
/api.Photo/Retouch
/api.Photo/GetImageURL
/api.Photo/CheckShooting
/api.Photo/CreateShootings
/api.Photo/ListShooting
/api.Photo/ExtendLimit
/api.Photo/SaleRecipe
/api.Photo/GetDreamBasePhotoEnhanceLevelInfos
/api.Photo/CheckStoryEpisodeShooting
/api.Photo/CheckCardStoryShooting
/api.Photo/CreateStoryEpisodeShootings
/api.Photo/CreateCardStoryShootings
/api.Photo/CheckExpressionShooting
/api.Photo/CreateExpressionShootings
/api.Photo/CheckExcursionShooting
/api.Photo/CreateExcursionShootings
```

A complete traffic flow (in this case, it's "ライブ撮影") for taking a photo including game logic APIs and firebase uploading APIs are as follows, ordered by time sequence.

```
/api.Photo/CheckShooting
thumbnail firebase resumable upload start 
full image firebase resumable upload start
thumbnail firebase resumable upload finalize 
full image firebase resumable upload finalize
thumbnail get metadata
full image get metadata
/api.Photo/CreateShootings
thumbnail get image
full image get image
```

The captured request and response body this time is placed in the `traffic/mixed/photo` directory.
The followings are things you need to pay attention to during implementation.

- In `/api.Photo/GetImageURL` response, you will see many photo images use CDN url. Since we don't use CDN for our project, instead of returning CDN url, let's just return the same url for getting images we previously implemented (`https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/*`).
- Don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time.
- Include things you are not sure about in your reply after complete implementation, if any.

---

Let me answer your uncertainties, please use these information to refine related features.

- Probability data is turely a mystery, but we don't have to force ourselves to copy the official figures regarding this matter. Please tell me how and where did you implement the distributions in the reply, I'd like to make a review and tell you how to refine after that.
- `manualCount` means whether the photo was taken manually by the user. The game features a auto-photo function for convenience, this count value is more like used for statistics.
- I have included a series of new traffic in `traffic/mixed/photo_round2` directory, featuring character dedicated photo creation, capacity overflow gift delivery, gift receiving. You can use these traffic as a factual evidence.

---

Please move the hardcoded photo rarity table and dedicated photo chance variable to a more easy-to-find place (maybe a global variable at the beginning of the file could be a good choice), makes it easier for me to quickly locate these settings later if I want to change them.

---

I have modified some files on my own. Specifically, it's about how to insert and update user recipes when godfy a user, please take care and do not overwrite my changes.
This time, please check and refine photo related features using the description from the official help document below.


## フォト

所持しているフォトの確認・強化・売却を行うことができます。

### フォトの詳細
ユニット編成で、アイドルのサポートに編成することで効果を発揮します。
フォトの名前は自分で変更することができます。
所持数の上限は未拡張時で600枚、最大拡張時には1600枚で、超過分はギフトに付与されます。
編成可能アイドルが設定されているフォトもあります。

### フォトの品質
フォトの品質に応じて、フォトの「オプション」の効果値が異なります。
アドバンスフィルムを消費して、フォトの品質を上昇させることができます。

### フォトの★(レアリティ)
★(レアリティ)は１~5まであります。
★(レアリティ)が高い程、効果値が高いオプションが付く可能性があります。

### フォトの売却
「売却」にチェックを入れることで売却するフォトを選択できます。
「一括条件指定」で条件を指定することで、フォトを一括選択できます。
フォトを売却することで、リサイクルフィルムを獲得することができます。
獲得できるリサイクルフィルムは、売却するフォトの品質とレアリティが高い程多くなります。
保護しているフォト、名刺に設定しているフォトは売却できません。


## 振り直し

リサイクルフィルムを消費して、フォトの「オプション」を付け替えることができます。

### 振り直しの詳細
全てのオプションが、ランダムで付け替わります。
編成可能キャラも振り直しの変更効果対象となります。
フォトの★(レアリティ)と品質に応じて、リサイクルフィルムの消費量が異なります。

### 振り直しの注意点
振り直し実行後、元のオプションに戻すことはできません。
フォトの★(レアリティ)と品質は変化しません。
オプションが固定のフォトは振り直しを行うことができません。
また、イメージトレーニングのステージを生成済みのフォトは振り直しを行うことができません。


## 品質アップ

アドバンスフィルムを消費して、フォトの品質を上昇させることができます。

### 品質アップの詳細
既に付与されているオプションの効果値が上昇します。
オプションの種類が増減したり変化したりすることはありません。
アップできる品質はマネージャーレベルに応じて変動します。
レタッチフォトの品質アップには、追加でレタッチインクが必要となります。
また、イメージトレーニングのステージを生成済みのフォトの品質アップには、追加のアドバンスフィルムが必要です。
必要量は素材フォトの品質によって変動します。
品質アップを行うことで、イメージトレーニングのステージ数や上昇値が上昇することがあります。
ただし、各種イベント内の効果による品質上昇では、イメージトレーニングのステージ数や上昇値は変動しません。


## フォトレタッチ (Retouch)

フォトレシピとレタッチインクを消費して、フォトの★(レアリティ)、品質、オプションをフォトレシピの内容に変更します。

### フォトレタッチの詳細
フォトレシピに記載されているいずれかの品質となります。
レタッチ後に適用されるオプションは、レタッチ画面で表示されている内容になります。
効果値は通常のフォトと同様に付与されたオプションごとに異なります。

### フォトレタッチの注意点
フォトレタッチ後、元の状態に戻すことはできません。
オプションが固定されているフォトもレタッチが可能です。
フォトレシピ所持数の上限に達した際は、超過分はギフトに付与されます。

### レタッチフォトの注意点
フォトレタッチを行ったフォトは「レタッチフォト」になり、レアリティが緑色の★で表記されます。
レタッチフォトはユニット編成でアイドル1人につき、1枚のみ装備することができます。
レタッチフォトを品質アップする際には「アドバンスフィルム」に加えて、「レタッチインク」が必要になります。
レタッチフォトはオプションが固定となり、振り直しを行うことができません。
また、イメージトレーニングのステージを生成済みのフォトはレタッチすることができません。


**My comments about `Photo/Retouch`**

For `Photo/Retouch`, original photo rarity & status & abilities does not affect the photo after retouching, the produced photo's rarity & status & abilities are only affected by selected photo recipe.
For example, if we use a ★1 Lv30 photo as a source material, attach it with a ★5 Lv170 recipe, the produced photo will be a ★5 Lv170 photo with abilities defined by the recipe.

---

Based on the real-device test, I found some issues for our previous works.

- SD only (no 3D model) costumes (`costumeTypeId` starts with `sd-*`, for now they are "親善大使" and "一日署長" costumes) should not be included in `UserGetResponse.costumes`. Because they have no 3D models, characters will disappear after choosing wearing those costumes.
- Remove `notiStoneShopItem` and `notiNormalShopItem` from `HomeEnterResponse`. These fields are used to inform users these are new or unchecked merchandise in the shop, a small red dot will appear in the upper right corner of the unchecked merchandise icon. Sometimes this kind of notification can be annoying, let's don't need to apply this feature in our private server.

Please fix the above issues.

---

This time your task is to fix/implement the following features.

- I tried to implement the feature to check whether the called gRPC service has been installed in interceptor (see `interceptor.go` line 153~162), and send the user to title page if the called service has not yet been implemented, but after real-device testing, I found that it did not work. Please fix my implementation and make it work.
- Add `Fan` and `Bond` to `GodFeature`. Like `Message`, these fields let admin choose how deep to godfy a user.
`none` takes no effect.
`enjoy` is in line with `none` at this point, reserved for potential changes.
`all` raises character light/middle/heavy fan amount to 500000/300000/200000, reliability to 1000.
If absent, defaults to `all`.
Perhaps you also need to update related company and mission progress after that. (not sure, just guess)

---

Please remove my previously added does-not-work code in interceptor if it's useless since the unimplemented requests are directed to `UnknownServiceHandler` now.

---

This time your task is to implement `PhotoPanorama` server, which is called "360°LIVE" in game.
From the server side's perspective, it is almost no difference to normal photo creation work flow, so perhaps you can use your previous work as a reference. The captured traffic is placed in the `traffic/mixed/PhotoPanorama` directory.

---

Don't edit code files this time.
Please find currently unimplemented gRPC servers and services, write the results in `docs/unimplemented.md`.

---

Please list each API on a separate line.

---

From now on, each time we implemented new APIs, please remember to update `docs/unimplemented.md` to reflect the latest situation.

This time your task is to implement the `Showcase` server.
Let me first use the official help document to explain this feature.


## グッズケース

獲得したゲーム内グッズを自由に飾る事ができます。

### カスタマイズ

獲得したゲーム内グッズはグッズケースに自由に飾る事ができます。

#### ゲーム内グッズ
ゲーム内グッズは、各種グッズショップやガチャ、アイドル獲得時、各種イベント報酬などで獲得することができます。
ゲーム内グッズは重複獲得可能で、所持している分だけグッズケースに配置が可能です。
ただし、フィギュアやケース、BGMは1種類につき1つしか所持できません。

#### 配置
ゲーム内グッズは各種グッズに記載された「床」「壁」に配置可能です。
配置できるグッズ数はケースの種類によって異なる場合があります。

#### 台座
複数サイズの台座が存在します。
台座を組み合わせることで様々な配置ができます。
台座を並べた場合、その境界線にグッズや台座は配置できません。
また、一部組み合わせられないグッズや台座の組み合わせがございます。予めご了承ください。

#### ケース
グッズを配置するケースはグッズケースショップにて購入することで使用可能になります。

#### BGM
グッズケースの撮影時やギャラリーにて再生されます。
ケースに対して1曲のみ設定が可能です。


### グッズショップ

グッズメダルやダイヤなどを消費して、好きなグッズを獲得することができます

#### 各種ショップ
グッズケース機能で使用できる商品が購入可能なショップが表示されます。

#### カタログ
グッズメダルやダイヤなどを消費して各種ゲーム内グッズを購入できます。
また、ショップなどで手に入れた「缶バッジパック」は、【グッズショップ > 開封】から開封することができます。

#### 開封
各種「缶バッジパック」を開封することができます。
缶バッジパックでは対象の缶バッジをランダムで獲得できます。
獲得できる缶バッジについては、缶バッジパック開封画面の「詳細」をご確認ください。


### ギャラリー

他マネージャーが公開しているグッズケースを見たり、いいねすることができます。
他マネージャーのグッズケースを撮影することはできません。

#### 「おすすめ」「フレンド」のグッズケース表示
他マネージャーが公開しているグッズケースやフレンドが公開しているグッズケースを確認することができます。

表示されるグッズケースは
・ケース所有ユーザーのログイン状況
・編集状況
・いいね状況
・これまでの表示状況
などの情報に基づいて抽選されます。
抽選結果はマネージャーによって異なり、1日に数回更新されます。

#### 各種検索表示
・フレンド検索
・ID検索
・ハッシュタグ検索
によって、グッズケースを自由に検索することができます。

#### 他マネージャーへのいいね
他マネージャーのケースにはいいねをすることができます。
「ハートギフト」がある状態で他マネージャーのグッズケースにいいねすると、フレンドメダルを獲得することができます。
ハートギフトの所持上限は30で、毎日3ずつ回復します。
ハートギフトを所持していない状態でいいねしてもフレンドメダルは獲得できません。

なお、一度実行したいいねは取り消すことはできません。
一度実行したいいねは毎月1日にリセットされ、再度同じケースにいいねができるようになります。

#### 他マネージャーからいいね
他マネージャーからいいねされると、翌日以降の初ログイン時にフレンドメダルを獲得することができます。
その時に獲得できるフレンドメダルの最大数は10個です。

#### 他マネージャーへの表示
各ケースの詳細から公開設定を変更することができます。
非公開に設定したケースは他マネージャーに表示されません。


The captured traffic is placed in `traffic/mixed/showcase`.
Please pay attention to the following things during implementation:

- Don't create a yaml to store goods shop items like normal shop, instead, read the shop items from master data `ShowcaseToy`. If some fileds are lack of, such as price, just give them a fair value, we don't have to copy the official data faithfully. But if you find some fields that really need to be recorded, skip implementing related APIs and tell me your concerns in reply.
- The Gallery is a multiplayer online feature in the official server, but in our private server we don't need to implement multiplayer features. For list APIs, just returning an empty user list; for interaction APIs, perhaps simply returning a dummy response could do the thing since they are never supposed to be invoked if there are no other users.
- Don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time.

---

This time your task is to implement `Hierarchy` and `Guild` server.
The `Hierarchy` is a server wide user ranking system, users who collects more Venus Points ranks higher. In our private server we don't need to implement multi-user features, returning the user themselves in the list ranking response would be enough.
The `Guild` is a faction system. There are 8 guilds in total, users can join any guilds at any time (except during the GvG event, but I think it's fair enough to ignore this since we don't need to implement multi-user features).
The captured traffic is placed in `traffic/mixed/hierarchy_guild`.

---

My fault, we missed a scenario.
For the first time of the day entering the guild top page, users will obtain a certain number of union medals:

1日1回、チェックインを行うことで「ユニオンメダル」を獲得することができます。
チェックインはVENUSユニオンの画面を表示するだけで達成されます。
チェックインは毎日00:00に更新されます。

The actual number varies in the enhance times of `Staff` (something we will implement later), but there is no such data in the master database, so just using a constant number 100 would be fine.
The captured traffic for this kind of scenario is `traffic/mixed/hierarchy_guild/GuildTopResponse26.json`.

---

This time your task is to implement `Staff` server.
The following is the official help document explanation for `Staff` feature.


# スタッフ育成

ユニオンメダルを消費して、各能力値の強化を行うことができます。

## スタッフ育成
強化した能力値は、加入している全アイドルに適用されます。
強化対象の能力値はボーカル・ダンス・ビジュアル・メンタル・スタミナ・クリティカルです。

### 強化回数
各能力値の強化回数に応じて、それぞれユニオンメダルの消費量が異なります。
通算強化回数に応じて、チェックイン時のユニオンメダルの獲得量が上昇します。(If there is no such data in the master database, use a constant 100)

### Lvリセット
スタッフのレベルの振り分けをリセットすることができます。
リセットした場合は、育成に使用した素材は返却されます。
リセットは毎月2回まで可能で、リセット可能回数は毎月1日の0:00に更新されます。
毎月1回目は無料で、2回目はレッドダイヤ及びブルーダイヤ50個でリセット可能です。
(In our private server, let's make reset free and unlimited)
※通算強化回数はリセット後から現在までの回数をカウントしており、前回リセットするまでの強化回数は含まれません。
※スタッフ育成回数に関するミッションについて、リセット後の値がリセット前の値を超えるまでは進捗しません。


## スタッフ育成潜在強化

スタッフライザーによって、スタッフ育成の潜在ランクを上昇させることができます。
スタッフ育成のLv上限はスタッフ育成の潜在ランクに応じて増加します。

### スタッフ育成潜在強化の詳細
任意のスタッフ育成の潜在強化をした場合、該当のスタッフ育成は潜在ランクが1段階上昇し、Lv上限が上昇します。 

### 潜在強化する方法
潜在強化は以下の方法で行うことができます。 
スタッフライザーを使用：
スタッフライザーを消費して任意のスタッフ育成を潜在強化することができます。
必要なスタッフライザーの数は潜在ランクによって異なります。

### スタッフライザーの獲得場所
スタッフライザーは、主に「事務所ランク」などで獲得することができます。


The captured traffic is placed in the `traffic/mixed/staff` directory.
Note you may need to update corresponding parameter for user cards after each enhance operation.

---

This time your task is to implement the following services across different servers:

```
/api.Item/Sell
/api.Accessory/Sell
/api.Migration/SetPassword
/api.Home/ReadTalk
/api.Card/ActivityAbilityEnhance
/api.Card/Reset
/api.Card/RemoveSupport
/api.Card/UpdateDisplayType
```

The captured traffic is placed in the `traffic/mixed/misc1` directory.
Like before, don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time.

---

We've got too many files directly under `internal/server/rpc/servers` which can reduce readability.
Please refactor gRPC server source files, create a separate folder for each server and a separate file for each service.
For example, for `/api.Card` server, create a `card` directory under `servers`, then create `enhance.go` in it for `/api.Card/Enhance` service, `reset.go` for `/api.Card/Reset`, etc.
If there are private functions be used across services, create a `common.go` under the server package and place those functions in. For functions only be used by a single service, keep it in the same service source file.
If there are functions be used across servers, just like before, place those functions into `internal/server/rpc/servers/internal/viber`.
Also please check if there are functions in `viber` that are not actually used across servers. If there are, place them back into the corresponding server package. The `viber` package is supposed to store common functions used across servers.

---

I don't see `viber` exists anymore. Did you deleted it because there is no cross-server functions?

---

Sorry for the misunderstanding, I've confirmed existence of the package.

This time your task is to implement `Profile` and `Friend` servers, which are both related to social aspects of the game.
The official help document has the following descriptions for the `Profile` feature.

## MYプロフィールの確認、登録

プロフィールの設定を行うことができます。

## マネージャーID

プレイヤー名（最大10文字）
所属ユニオン
所属グループ
注力アイドル
誕生日

## 称号

所持している称号から、設定することができます。

## コメント

最大60文字で、コメントを設定することができます。

## 背景変更

所持アイドルもしくはフォトの中から名刺の背景を設定することができます。

## 名刺交換

他のマネージャーと名刺を交換することができます。
交換したマネージャーとはフレンドになります。
フレンドになったマネージャーのアイドルは一部ライブで助っ人として編成可能となります。
名刺コードとマネージャーIDのどちらかで名刺交換の申請を行うことができます。

## フレンド名刺

交換した他のマネージャーの名刺を確認することができます。

## 名刺デコレーション

名刺をデコレーションすることができます。

## 助っ人設定
名刺画面より「助っ人」の設定が可能です。
設定した助っ人はフレンドとなったマネージャーがライブ時に編成可能となります。
自身の助っ人を他のマネージャーがライブに参加させた場合、協力報酬として「フレンドメダル」を獲得することができます。
協力報酬はログインしなかった日を除き、1日5人分まで獲得できます。
フレンドメダルは交換所で様々な報酬と交換可能です。

## マネージャー検索

Lvの近いマネージャーを優先的に検索して一覧に表示します。


The `Friend` feature is nothing special, just like any other games, users can list their friends, search (randomly) new users, send friend requests, approve or reject other's requests. However in our private server, we don't want to implement multi-user features, we need to make the user can only perceive themselves, but these requests are still possible be triggered by the client, so we still have to implement the multi-user related RPCs, but we can make them return an empty list or mock responses instead of real data.

The captured traffic is placed in the `traffic/mixed/social` directory.
The followings are things you need to pay attention to during implementation.

- There is no captured `UpdateBirthday` traffic because it's not possible for my account to trigger it. For each account, birthday can only be set once. After that it becomes immutable (this is controlled on the client side). So you will have to speculate the implementation out of thin air, but I guess it won't be very difficult for such a simple feature.
- Include things you are not sure about in your reply after complete implementation if any.

---

Looks like your previous task is interrupted due to usage limit, now it is reset, please continue your task.

---

This time your task is to implement the `Love` server, which is called "もし恋" in the game.
The official help document gives "もし恋" the following descriptions, contents in parentheses are my own comments.


# もし恋

## もし恋とは

「もし恋 ~もしも君と恋をしたら~」は星見プロのアイドルを起用した新作恋愛ゲームです。
星見プロのマネージャーとして「アイドル達の新しい一面を見せることが出来る」ゲームになっているかテストプレイを行います。

ストーリーでは選択肢を選択することができ、その選択によって物語の結末が変化します。
様々なエンドのクリアを目指してストーリーを進めましょう。


## 遊び方

1. 「もし恋Exp」「もし恋ストーリーPt」を獲得しましょう。
「もし恋Exp」「もし恋ストーリーPt」はイベント開催日から時間経過で蓄積します。
イベントのホームから受け取ることができ、最大12時間、蓄積可能です。

2. 「もし恋Lv」を上げてストーリーを解放しましょう。
時間経過によって獲得できる「もし恋Exp」によって、「もし恋Lv」が上がります。
もし恋Lvが上がるとストーリーが解放されます。

3. ストーリーを進めましょう。
時間経過によって獲得できる「もし恋ストーリーPt」を消費して、ストーリーを読むことができます。
ストーリー内では選択肢を選択することができ、その選択によって物語の結末が変化します。
様々なエンドのクリアを目指してストーリーを進めましょう。

4. 課題をクリアして、報酬を獲得しましょう。
課題をクリアすることで、もし恋を進めやすくなるアイテムなどが獲得できます。

5. ヒロインとの距離を縮めましょう。
イベントのホームにはヒロインが登場します。
ゲームの進捗や「プレゼント」を渡したりすることで、ヒロインとの距離が徐々に縮まっていきます。


## もし恋Lv

もし恋Lvを上げることでストーリーが解放されます。
もし恋Lvを上げるためには、「もし恋Exp」が必要です。
もし恋Expは時間経過で蓄積し、イベントのホームから受け取ることができます。
最大12時間の報酬を蓄積可能です。


## ストーリー

ストーリーを読むためには「もし恋ストーリーPt」を消費します。
もし恋ストーリーPtは、もし恋Expと同様にイベントのホームから受け取ることができます。
(After unlocking all endings, no Moshikoi Story Pt will be consumed anymore)


## 課題

課題をクリアすることで、2時間分の報酬を獲得することができる「もし恋スキップドリンク」や、ヒロインとの距離を縮める「プレゼント」など、ゲーム内の各種アイテムを獲得することができます。
課題とその受け取りはイベント開催期間中のみ有効です。

### 限定課題
イベントの初回開催のみ有効な課題です。

### 課題
初回開催、復刻開催時どちらも有効な課題です。


## お仕事

イベント開催期間中、「IDOLY PRIDE」のファンイベントやプロモーションを行うことで、「もし恋スキップドリンク」を獲得することができます。(Let's ignore this in our private server, no drinks can be obtained from the Working)
課題などとあわせて、もし恋の攻略に向けてプレイしましょう。


## ヒロイン

イベントのホーム画面にはヒロインが登場します。
ゲームの進捗や「プレゼント」を渡したりすることで、ヒロインとの距離が徐々に縮まっていきます。


## アイテム

### もし恋Exp
もし恋Lvを上げるために必要です。
イベントのホームから受け取ることができます。

### もし恋スキップドリンク
2時間分の「もし恋Exp」「もし恋ストーリーPt」を受け取ることができます。

### もし恋ストーリーPt
ストーリーを読むために必要です。
イベントのホームから受け取ることができます。

### プレゼント
ホームのヒロインとの距離を縮めるために必要です。
課題をクリアすることで獲得できます。


## ヒント機能

一定のもし恋Lvに到達するとヒント機能が解放されます。
初期状態でヒント機能はOFFですが、ヒント機能を使用した場合、ストーリーの選択肢表示時に以下のヒントを表示します。
- ヒントLv1：「大事な場面」が分かります
- ヒントLv2：「大事な選択肢」が分かります

ヒント機能は攻略が進めやすくなるサポート機能であり、OFFがデフォルト設定となります。
実際に使用するかどうかは選択可能で、イベントホームの設定やストーリー再生画面から変更することができます。
ヒント機能を使用した場合でも、報酬や課題クリア状況には影響ありませんのでご安心ください。


## セーブ・ロード機能

いずれかのエンドを一つクリアすることで解放されます。
セーブ・ロード機能では、それまでのプレイ状況を残しておくことができ、ロードすることでセーブ当時の状態から復帰することができます。
ただし、セーブ・ロード機能で保存される内容は1話からその時点までのストーリーの選択状況のみが対象です。
ヒロインとの距離や課題内容はセーブの対象外で、ロードを行ってもその時の状態には巻き戻りません。


There are multiple series of Moshikoi event, the complete list of currently implemented official stories is as follows, potential future additions are almost for sure.

- love-23-0514: 一ノ瀬怜 編
- love-23-1114: 伊吹渚 編
- love-24-0517: 早坂芽衣 編
- love-24-1114: 鈴村優 編
- love-25-0426: サキュマホ (kinda different, we will talk about this later)
- love-26-0313: 神崎莉央 編

This time, as a start point, we only need to implement `love-23-0514`. But be advised that we will implement other series later, so keep an eye on scalability and flexibility for potential subsequent additions when you implement the `Love` server.

There is no useful data in the database regarding how in-game choices affect the final ending, so I placed a hand-worte route map at `docs/love-23-0514-routemap.md` for `love-23-0514`.
The entire story scripts extracted from the game assets are placed in `resources/moshikoi` in case of you need.

Despite it has some special aspects, Moshikoi itself is a type of event in game. Pay attention to check if `Loves` field in `UserGetResponse` is properly implemented and `EventInfo` field in `HomeEnterResponse` contains corresponding event.

The captured traffic is placed in the `traffic/mixed/love` directory. Traffic under `veteran` directory reflects an already-all-cleared account's status, while `newbie` reflects how does a fresh new user gradually clear all endings from the very beginning.

The followings are some additional things you need to pay attention to during implementation.

- Make the Moshikoi event has no end time in our private server. We'd like users can play this event anytime.
- If you need to write a data structure to store route map data, write it in `internal/makino/moshikoi`. Note we will add the rest of Moshikoi series later, so be wise on handling scalability.
- Remember to add Moshikoi related contents for `god` API.
- Don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time.
- Include things you are not sure about in your reply, if any.

---

I accidentally closed the IDE window before confirming your work. Have you done your last work? If not, please continue.

---

Based on the real device test, I found these issues you need to deal with:

- Current implementation for `Profile/FindUser` returns an empty response body, which will cause an unpredictable behavior on the client side. The official response for searching an unexist user is placed in `traffic/mixed/fix-0911`, please use it as a factual evidence to fix our implementation.
- The client goes freezed when trying to enter a goods shop in the Showcase top page. This happens before invoking `Shop/ListItem`, so I think it may be caused by some improperly populated fields in `Showcase/Top`'s response. I placed an official response of `Showcase/Top` in the `traffic/mixed/fix-0911` directory, perhaps you can use it to find some hints.
- Please review `Card/SetSupport` and related features based on the following official help documentation.
```
# レベルアシスト
加入しているアイドルのうち上位5人のLvに合わせて、他のアイドルのLvを引き上げることができます。

## レベルアシストの詳細
加入しているアイドルのうち、Lv上位5人が自動的に「トレーナー」になります。
トレーナー5人の中での最低Lvで「アシストLv」は更新されます。
レベルアシスト対象に設定したアイドルは★毎の最大Lvに関わらずアシストLvと同じLvになります。
レベルアシスト対象から外すと、アイドルのLvはもとに戻ります。

## アシスト枠　
レベルアシスト対象に設定することができる枠を指します。
アシスト枠は、「レベルアシストPt」やダイヤで解放することができます。
レベルアシスト対象に設定したアイドルは、対象から外すまでレベルアップができなくなります。
レベルアシストに設定後、12時間はアシスト枠から外すことは出来ません。

## アシストLv
アシストLvは一度更新すれば、下がることはありません。
トレーナーを「契約更新」しても、アシストLvは更新されません。
またアシストLvには制限が設けられており、アシストLvは上限値以上にはなりません。
契約更新の詳細は「ヘルプ > 育成 > 契約更新」をご覧ください。
```

- A photo image related issue. The issue can be reproduced by taking the following steps:
  1. Create a new showcase and save (`Showcase/Save`)
  2. Add new goods or change the coordinate of the existing goods
  3. Save the showcase again. Before `Showcase/Save` is invoked, `https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2Fd223e8b1-b639-45da-b4e5-d67a731fbfe0&uploadType=resumable` returns HTTP 409 "photo already exists or is being finalized".
  My speculation is the photo's name should be able to be reused, the newer one overwrites the older one.

---

Oh, sorry I forgot to place those traffic. Please stop for now and I will let you know when we are ready.

---

OK, now we are ready, please continue your work.

---

Good news and bad news.
`Profile/FindUser` and photo name reusing issues are gone, but clicking goods shop still freezes the client, and another new issue arised.

**Entering the goods shop**
The official traffic flow for entering a showcase shop is:
1. The user enters the Showcase top page (`Showcase/Top`)
2. The user clicks goods shop list on the UI (`Showcase/Top` again, I don't know if it's identical with the one entering the Showcase top page)
3. The user clicks one of the goods shop on the goods shop list page (`Shop/ListItem`)
For our current private server implementation, the client freezes at step.2 after `Showcase/Top` is invoked.
The captured traffic of our private server and the official server is placed in `traffic/mixed/fix-0912/ShowcaseTop`, you can use it to help you investigate the root cause.

**`Showcase/Save` failed with an error**
The fresh new save can complete without problems, but a save overwrite existing one will fail with the error "RPC failed with error{error 26 0  transaction failed: rpc error: code = InvalidArgument desc = number, frameId, assetId, and layoutInfo are required}".
The captured traffic of our private server and the official server is placed in `traffic/mixed/fix-0912/ShowcaseSave`.

Fix the above issues and include the causes in your reply.

---

We can now entering the goods shop and saving the showcase, but after overwriting the showcase's thumbnail is not changed, still displays the old showcase's thunmbnail.
The scenario flow for this is:
1. The user edit an existing showcase and click save on the client
2. The client automatically shoots a photo for the latest showcase, and upload them (an original photo and a thumbnail) through firebase storage API
3. The client invokes `Showcase/Save`

I guess the issue's cause is we didn't properly save the new photo (overwrite the old photo with the new one)? Please investigate the issue and fix it.
The invoked APIs at the step.2 on our private server are as follows:

```
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a_t&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a_t&uploadType=resumable&upload_id=AJjja9SyH8bct968G4EN2FP-BVDG9SG1We-SsY962fRScJ_sCdd7S9dw_krdmMNNY1OcOUFYdcsUN6bM-DkMphBmcyJSFobDLYeRll&upload_protocol=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a&uploadType=resumable&upload_id=AJjja9QdzeQUv8RUyey0MEif594YeB16YhFrIMrsrlO2RdnAGRR32mY0tv6f0T35glyLRjioxEykOtntRAQup5XqxujSEkzq1UdPBR&upload_protocol=resumable
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a_t
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2Ff5283737-a553-497d-8625-53def601ab3a
```

---

The showcase's cover still won't get changed based on real device test.
I compared the traffic flow of the official server and our private server and found something, see this first.

The official traffic flow:
```
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef_t&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef&uploadType=resumable&upload_id=AJjja9b4UUb5nSazixM6dTxxDb7UYgwV237txtMZbeHkLuDuI1OplkNQdqYhmSq5mzNx5E6YlhAt1nFgP5OwCtvMXQthPuU28BIPAPpwjgJIywHc&upload_protocol=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef_t&uploadType=resumable&upload_id=AJjja9amZMT4VlGLYfPLTkHbNSehjlIzRGNpXQac9jPmoDGn855h9TQKxPAXp6D0s1RlXlfDE-ubVZO9grqF_9-z3xl-ThLjn0GIfYsPDyeD2ytt&upload_protocol=resumable
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef_t
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef
POST https://api.game-idolypride.jp/api.Showcase/Save
POST https://api.game-idolypride.jp/api.Showcase/Top
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FAL7XXMDK%2F5f266c3a-f9b2-4286-b455-69ce6dc1e9ef?alt=media&token=ff80e73a-4cde-4fcc-9d05-24916ec12385
```

Our private server traffic flow:
```
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc_t&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc&uploadType=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc_t&uploadType=resumable&upload_id=AJjja94MNO8EXoWaFnsnMGtEJN69kk6qWwh8MV1btQdBGBXIMhPwDNTDyj38mWbxqqgZocZkrnaew8vXIihtXP8aBQ4nWsw7w2MAIB&upload_protocol=resumable
POST https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o?name=Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc&uploadType=resumable&upload_id=AJjja9z8MUc_g7B_-1KWvgw7MLB4rxt9YNYSYZk2WMXnctOOlXTjzNbeDYqFGokZiJFYLhhnakqex4JQwAQa8g2jErrWIZqY_bmJyJ&upload_protocol=resumable
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc_t
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc
POST https://api.game-idolypride.jp/api.Showcase/Save
POST https://api.game-idolypride.jp/api.Showcase/Top
GET https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc
```

You can see, not like the official version, the client didn't send the last request to retrieve the latest image on our private server.
I guess the cause is mostly lying within the response of `https://firebasestorage.googleapis.com/v0/b/udc-idolypride/o/Photo%2FABCDEFGH%2F3d1152f8-2ec5-4fdd-84f5-6b55d4971ccc`, which makes the client think it does not need to send another request.
Please investigate again and fix the issue.

---

Great, I have confirmed the overwriting issue of photo has gone.

This time your task is to implement these APIs:

- `/api.Card/ReleaseSupport`: Unlock a new support slot. Required amount of assist point or diamond is vary from already unlocked slots, you can get this data from https://docs.google.com/spreadsheets/d/1R-7bH4gMyOvNU3JuXRRT34ysNGOsatIDlTLdl5p9ArQ/
- `/api.Story/ReleaseEpisode`: Unlock a new story episode. You can get the required amount of items from the master database.
- The `Health` service. A simple ping service, nothing fancy.
- The `Salary` service. See the official help document descriptions below.

## 勤怠
毎月獲得することができる給料や勤怠Ptと、その内訳を確認することができます。

## 給料
各月の最終日12:00に、1ヶ月間の活動内容に応じたダイヤを獲得することができます。
計測は各月の最終日前日23:59までで、最終日は計測対象外です。

- The `Mission` service. The only rpc `URLTransition` I didn't have luck to capture, don't know what's it for, let's return an empty `CommonResponse` for now.
- The `Birthday` service. After the user register their birthday at `Profile/UpdateBirthday`, they will receive a special dedicated birthday event starts from 0:00 of their birthday and last for 7 days (until 6 days later 23:59:59):
  - 3 favorite characters (registered from `Profile/UpdateFavoriteCharacters`) will come one by one (from the third one to the first one) to give blessings. Random if not set.
  - To capture birthday related traffic from the official server I used 2 new accounts: one set the birthday to the current date, one set to tomorrow. The former one instantly received the birthday event after back to the home page, the later one received a reminder after back to the home page which reminds the user their birthday is approaching don't forget to set the favorite characters.
  - Determining whether the current time period is within the user's birthday period is most like done during `Home/Login` or `Home/Enter`, please pay attention to the traffic after the user invoked `Profile/UpdateBirthday`.


The captured traffic is placed in `traffic/mixed/feature-0912`. Note the `others` directory contains many files from multiple accounts, they are ordered chronologically, but may not be consecutively.
Don't forget to check if there are previous implemented APIs need to be refined based on the features we implemented this time, and include things you are not sure about in your reply if any.

---

Several fixes need to be dealt with for your previous implementation.

Let me correct my previous instruction first:
3 favorite characters will come one by one from the first one to the third one.

You got 2 false assumptions there:
- Salary work duration counts all idols (e.g. Idol A 100h, idol B 150h, work duration = 250h)
- Birthday reminders appear 7 days beforehand

The birthday captions for the rest characters are now placed in `traffic/mixed/feature-0912/birthdaytop`.
2 remaining character birthday captions not captured:
- kana: 一緒に\n写真撮ろ～！
- miho: 私だってお祝い\nくらいしますよ

---

Now everything looks functioning good, but we got performance issues here.
I need you to improve performance for 2 APIs:

- `Home/Enter`: The most important one. Home entering API is the most frequently invoked RPC among all others. Based on local real device test, our server will take up to 2000ms to send back the response. By comparison, the official server only need about 700ms to do this, not even saying we have a higher network latency connecting to the remote server. Even worse, the 700ms is a veteran account's average lantency entering the home page, a newbie account only need about 300ms to finish this on the official server.
- `Mission/Receive`: This API is often invoked consecutively, users will experience noticeable lag if it takes too much time. Our server costs about 500ms to return the response, by comparison the official server only need about 120ms.

Make sure not breaking the existing business logic when you do this. Include what you have done to improve the performance in your reply.

---

This time your task is to implement the `Deck` service, which is called "ユニット編成" in the game.
The official help document for "ユニット編成" has the following descriptions.

## ユニット編成
ライブをするユニットは5人1組で編成します。
編成したユニットには自由に名前をつけることができます。
同名アイドルを編成することはできますが、ライブ中の映像では他のアイドルが代わりに登場します。
ユニットパワーは、編成したアイドル・フォト・アクセサリに「スタッフ育成」の数値も加えて算出されます。
スタッフ育成の詳細は「ヘルプ > VENUSユニオン > スタッフ育成」をご覧ください。
編成中のアイドルを入れ替えた場合、入れ替え前のアイドルに編成していたフォトとアクセサリは、そのまま入れ替え後のアイドルに適用されます。

Most of the mentioned features are implemented by the client.
The captured traffic is placed in the directory `traffic/mixed/deck`.
Note the `Deck/BuddyList` API lists all buddy cards (set by `Profile/UpdateBuddyCard`) of the user's friends. We don't want to implement multi-user features so return an empty card list would be enough.

---

Now let's implement the `Quest` service, which is called "ライブ" in the game.
See below for descriptions from the official help document regarding to Live types.


# ライブ
ユニットを編成してライブに挑戦することができます。


## メインライブ
クリアする事で報酬を獲得することができます。また、メインライブの進捗に合わせて様々な機能が解放されます。

### メインライブの詳細
エリア内のライブは、自由な順番で挑戦できます。
アイテム消費は無く、何度でも挑戦することができます。
1度クリアしたライブは再挑戦できません。
エリア内の全てのライブをクリアすると、次のエリアが解放されます。
一部のエリアは、マネージャーLvが指定Lvに到達していないと挑戦できません。


## デイリーライブ
各種パスを消費してライブを行い、指定の報酬を獲得できます。

### デイリーライブの詳細
デイリーライブには、コイン・レッスンピース・撮影フィルムを獲得することができる3種類のライブがあります。
それぞれ、コインパス・レッスンパス・撮影パスを消費して挑戦できます。
各ライブは難易度に応じて報酬が異なります。
また、スコアランクに応じて追加報酬を獲得できます。
クリアできなかった場合、パスは減少しません。

### FREEパス
毎日00:00に、各ライブのFREEパスを5枚ずつ獲得することができます。
0:00の時点で使用されていない前日のFREEパスは消失します。

### FREEパス分の一括プレイ (`Quest/StartBulkDaily`)
一括プレイでは、クリア済みのデイリーライブの最高難易度が、最終プレイ時の編成で実行されます。

### スペシャルデイリーライブ
期間限定でスペシャルデイリーライブが開催されることがあります。
毎日00:00に獲得できるFREEパスや、スペシャルパスを消費することで挑戦できます。
イベント期間終了後も、所持しているスペシャルパスは消失しません。


## ハイスコアライブ (`Contest`)
ハイスコアパスを消費して挑戦することができ、スコアに応じて報酬を獲得できます。

### ハイスコアライブの詳細
スコアランクに応じた報酬と、各楽曲の合計スコアに応じたミッション報酬を獲得できます。
また、各楽曲のランキングで一定の順位に到達すると報酬を獲得できます。

### ランキング
ランキングは各楽曲の最大獲得スコアの順位で競います。
ランキングは「グループ」ごとに分けられています。
グループの詳細は、「ヘルプ > アカウント > グループ」をご覧ください。


## VENUSタワー (`Tower`)
クリアする事で報酬を獲得することができます。

### VENUSタワーの詳細
初回クリア時に報酬を獲得することができます。
アイテム消費は無く、何度でも挑戦することができます。
クリアすると次のステージに挑戦できるようになります。
1度クリアしたステージは挑戦できなくなります。

### ランキング
グループ内の到達STAGEによる順位を確認することができます。


## VENUSバトル (`PvP`)
挑戦回数やバトルパスを消費して挑戦することができ、VENUSレートの順位に応じた報酬を獲得できます。

### VENUSバトルの詳細
グループ内の他マネージャーのユニットと課題楽曲によるライブバトルを行い、獲得スコアで勝敗を決めます。
VENUSバトルでは「シーズン」ごとに一定の期間で開催され、期間終了時のVENUSレートによって順位が決定します。
獲得できるVENUSレートの量は、対戦相手のVENUSレートと勝敗に応じて異なります。

### 挑戦報酬
挑戦時に対戦相手のレートと勝敗に応じてVENUSレートを獲得する事ができ、自分の順位に応じて報酬を獲得できます。

### 順位報酬
シーズン終了時には、自身の順位に応じた期間報酬が獲得できる他、VENUSポイントの増減も行われます。

### 挑戦回数
挑戦回数は1日3回で、00:00に更新されます。
挑戦回数がなくなった場合、1日3回までバトルパスを消費して挑戦することができます。


The captured traffic is placed in the directory `traffic/mixed/quest`, which came from 3 accounts: a veteran account, a fresh newbie, and a neither of the two. Note not all of these traffic should you inspect this time, see below instructions for details.

- Do not implement `Quest/Start` and `Quest/StartBulkDaily` this time, especially `Quest/Start`. It is (maybe) the most complicated API among all others, I will assign you a focused task to implement it later.
- Unless you really need, do not inspect response traffic of `Quest/Start` this time, each of those is ought to be very large, do not waste your context window for it.
- Some `Quest/Top` responses contain "SP" daily live. As mentioned in the official document, they are time limited event. We don't need this kind of event in our private server, leave it.
- Do not add multi-user features. Ensure no ongoing PvP (Venus Battle) event, keep latest clear list empty, and return the user themselves in the ranking responses.
- I've got no luck to trigger some RPCs, you'll have to try to guess and implement RPCs which's traffic is absent.
- For god users, make all main, tower, daily quests unlocked and cleared.
- Include things you are not sure about in your reply or just ask on the fly.

---

Now let's start work on the `Quest/Start` RPC, this is a big one, maybe the most complicated one among all others.
First of all, for this conversation, I don't need you to implement the API; instead, I want you to first understand the overall how does "Live" work and make a plan. I will do my best to send you the necessary information and help you to understand; you need to write down the information you need (perhaps in the `docs` directory?) to implement the API based on the captured traffic, the information I provide, the official documentation, and your own deductions. If you think you still lack of information, do not start implementation, tell me what you need in your reply and I'll see what I can do. We can repeat this for several rounds, until you believe you have obtained all the information needed to implement this API.

The official help documentation for "Live" is as below.

# ライブ
ユニットを編成してライブに挑戦することができます。

## ライブの概要
楽曲に設定されたビートやスキルチャンスでスコアを獲得し、目標スコアに到達することでライブクリアになります。
一部ライブにはスコアランクがあり、Cランク以上のスコアを獲得することでライブクリアになります。
ライブクリアをすると報酬を獲得できます。

### 会場座席数
来場ファン数の最大値を表します。

### 来場ファン数
来場ファン数と、その内訳を確認することができます。
来場ファン数は、ファン数とコアファン率に応じて決まります。
ファン数：各アイドルのファンの人数を表します。
コアファン率：ファンの中で来場してくれる人の割合を表します。
来場ファン数に応じて、スコア・SPスキル成功率・Aスキル成功率が上昇します。

### 目標スコア
到達すると、ライブクリアになります。

### 楽曲Lv
ライブの難易度を表します。

### ライブ詳細
クリア報酬：ライブクリア時に獲得できる報酬です。
ライブ特徴：各ライブに設定されている特徴を確認することができます。
発動ライブエール：ライブ中に発動するライブエールの内訳を確認することができます。
エール発動アイドル：エールが発動しているアイドルを確認することができます。
直近クリア：直近でライブクリアをしたマネージャーのライブを観戦したり分析することができます。

### ライブ中に確認できること
ビートとスキルチャンスに合わせて獲得するスコアを確認することができます。
スキルは、流れてくるスキルチャンスに合わせて発動します。
スキルの成功率は、アイドルのメンタルとライブの要求メンタルに応じて異なります。
クリティカルが発生すると、通常よりも多くのスコアを獲得することができます。

### ライブ後に確認できること
獲得スコアとその内訳を確認することができます。
「レーン情報」では、スキルの発動有無を確認することができます。
「リプレイ」では、ライブ映像を振り返ることができます。
「分析」では、より詳細にライブを分析することができます。

### ライブの注意点
必要なメンタルとクリティカルはライブに応じて異なります。


## ライブスキルの状態と効果
ライブスキルには、様々な状態と効果を発生させるものがあります。

### 能力値上昇と低下
ボーカル・ダンス・ビジュアルに作用し、それぞれの値が変化します。
最大で20段階まで変化します。

### ブースト効果について
対象の能力値（ボーカル・ダンス・ビジュアル）に作用し、それぞれの値が上昇します。
段階数に応じて、スキル発動時のスタミナ消費量が増加します。
最大で20段階まで変化します。
ブースト効果は能力値上昇の効果と重複して効果を発揮することができます。
ブースト効果のスタミナ消費増加については、低下効果回復等の効果によって除去されません。

### ビートブースト効果について
ビートのスコアの値が上昇します。
段階数に応じて、毎ビートごとにスタミナを消費します。
最大で20段階まで変化します。
ブースト効果はビートスコア上昇の効果と重複して効果を発揮することができます。
ブースト効果のスタミナ消費については、低下効果回復等の効果によって除去されません。

### クリティカルに関する状態と効果　
クリティカル率上昇：クリティカル率が上昇します。
クリティカル係数上昇：クリティカル発生時のスコアボーナスが上昇します。
最大で20段階まで変化します。

### スタミナに関する状態と効果　
消費スタミナ低下：スキル発動時のスタミナ消費量が減少します。
スタミナ消費増加：スキル発動時のスタミナ消費量が増加します。
最大で20段階まで変化します。

### スコアに関する状態と効果
スコア上昇：獲得するスコアが上昇します。
ビートスコア上昇：ビートのスコアが上昇します。
Pスキルスコア上昇：Pスキル及びフォトの特殊スキルにスコア獲得効果がある場合、獲得スコアが上昇します。
Aスキルスコア上昇：Aスキルのスコアが上昇します。
SPスキルスコア上昇：SPスキルのスコアが上昇します。
コンボスコア上昇：コンボによるスコアボーナスが上昇します。コンボ数が多いほど高い効果を発揮します。
上記は最大で20段階まで変化します。
Pスキルスコア追加：Pスキルに対して、スコア獲得効果が追加で付与されます。
上記は最大で30段階まで変化します。
Aスキルスコア追加：Aスキルに対して、スコア獲得効果が追加で付与されます。
上記は最大で30段階まで変化します。
SPスキルスコア追加：SPスキルに対して、スコア獲得効果が追加で付与されます。
上記は最大で30段階まで変化します。

### 上限解放効果について
上限解放効果を受けているアイドルは、指定されたスキルの最大段階数を超えて変化させることができます。
効果時間が設定されており、終了したタイミングで上限を超えた段階数は元の最大段階数が上限となります。

### 超化効果について
超化効果を受けているアイドルは、指定されたスキルと超化効果が同時に発動している場合、その効果値が通常の段階数より高くなります。

### 一部スコア獲得効果についての注意事項
強化効果が多い程効果上昇：効果の上昇に関して最大9種類までの制限があります。
ただし、【スコア獲得-強化効果が多い程】の強化効果数上限解放効果を受けたアイドルは上記の最大種類を超えてスコアを獲得することができます。
効果時間が設定されており、終了したタイミングで上限を超えた種類数は無効となり、元の最大9種類が上限となります。
スキル成功率が高い程効果上昇：効果の上昇に関して最大成功率200%までの制限があります。

### 集目とステルスに関する状態と効果　　
集目状態：来場している他のアイドルのファンが自身に注目し、獲得スコアが上昇します。
ステルス状態：
来場している自身のファンが他のアイドルに注目し、他のアイドルの獲得スコアが上昇します。
最大で10段階まで変化します。

### 低下効果反転について
発動時の低下効果が除去され、効果別に対応する効果へ変化します。
ボーカル・ダンス・ビジュアル低下→同一段階のボーカル・ダンス・ビジュアル上昇に変化します。
スタミナ消費増加→同一段階の消費スタミナ低下に変化します。
不調→残り効果時間と同一時間分CTが減少します。

### その他の状態と効果
コンボ継続：コンボが継続不可の状況でも継続します。
不調：SPスキル及びAスキルが発動できなくなります。
低下効果防止：一定ビート数効果がある低下効果を受けなくなります。ただし、強化効果消去やスタミナ消費等の即時性の効果は防止対象になりません。
低下効果回復：発動時の低下効果が除去されます。
低下効果反射：発動中に受けた低下効果を発動者に対して跳ね返します。
テンションUP：最大で10段階まで変化します。Aスキル、SPスキル、Pスキルのスコアが上昇し、Pスキル以外のスキル成功率がわずかに低下します。
また、各ライブに対してメンタルを余剰に上げる事でテンションUP中でもスキル成功率を100%にする事は可能です。
スキル成功率上昇：最大で10段階まで変化します。AスキルとSPスキルの成功率が上昇します。また、ライブバトルで相手からAスキル、SPスキルの発動権利を奪いやすくなります。
スキルチャンス譲渡：スキルチャンス譲渡状態でスキルチャンスを獲得した場合、発動可能なスキルを持つアイドルがスキルを発動します。
状態変化：対象の状態が別の状態に変化します。なお、状態変化の対象アイドルが低下効果防止状態であっても、状態変化の効果は発動します。
CT増加：対象のCT中のスキルのCTを増加します。
カバー：自身以外の味方を対象とした低下効果(強化効果消去やスタミナ消費等の即時性の効果を含む)を代わりに自身が受けるようになります。
封印：Pスキルが発動できなくなります。
強化段階数を制限：強化効果の最大段階数に対して指定割合の段階数までしか強化効果が発揮されないようになります。
強化効果低下：付与されている全ての強化効果の段階数を低下します。この効果によって段階数が0になった強化効果は失われます。

### 同一ビートで発動したPスキルとAおよびSPスキルのPスキル効果範囲について
- ライブバトルにおけるAおよびSPスキルの発動権利の計算は、Pスキルの効果は考慮されません。
- 獲得スコアに対する計算は、同一ビートにて発動したPスキルの効果が考慮されます。
※新たな発動条件によっては例外となるスキルが登場する可能性があります。

### 1人のアイドルが複数のPスキルを同一ビートで発動可能状態になった場合の挙動について
- 同じアイドルのPスキルは同時に発動できず、1ビート毎に1つとなります。
- 発動順はそれぞれのスキル自身の発動条件等によって決定されます。


## ライブ分析 (this is a client side feature, but maybe still has reference value)
ライブ分析機能では、普段のライブよりも詳細にライブを分析することができます。

### ライブ分析とは
スコアライブの詳細やライブバトルでの相手のスコアやパラメーターを確認することができる機能です。
ライブリザルトやランキングなどの「分析」からそのライブを分析することができます。

### 表示情報
①スキル情報
スワイプによって次のスキルに移動することができます。
この時、画面上部に表示されている情報は、選択されているスキルの発動アイドルの情報です。

②アイドルの個別情報
画面下部のアイドルサムネイルをタップすると、選択されたレーンの情報を表示します。
このとき、スクロール操作によってビート単位で移動することが可能です。
画面上部に表示されている情報は、そのレーンのアイドルの情報です。
再度アイドルサムネイルをタップすると、レーン選択を解除できます。

### 画面詳細

#### 「アイドル情報」
画面上部に表示される情報です。
その時のアイドルの状態を確認できます。
「現在の効果」をそれぞれタップすることで、その要因となったアイドルなどの情報が確認できます。

#### 「レーン情報」
各レーンのスキル情報などを表示します。

#### 「Pスキル」ボタン
「Pスキル」ボタンをチェックすることで、A・SPスキル以外の情報も表示されます。

#### 「切り替えボタン」(ライブバトルのみ)
ライブバトルの場合、画面下部のボタンから「自分」と「相手」の情報を切り替える事ができます。

#### その他
各種画面上の表示情報は、表示されているスキルやビート発動後の情報です。
機能追加前に実行されたライブについては、ライブ分析機能を使用することができません。


The most sophisticated part is the skills. There are many kinds of skills defined in the game (see `SkillEfficacyType` in `penum` package), you must be wise on handling scalability, perhaps making an interface for skills or something.
There are 2 excel files regarding game skill rules in the `docs` directory compiled by players based on statistical data. `docs/IDOLY PRIDE ライブの詳細.xlsx` explains the underlying sequence within a single beat, `docs/IDOLY PRIDE Score.xlsx` explains the skill effects. Make good use of them.

The traffics is placed in the `traffic/mixed/quest` directory.
I suggest you to create a dedicated package `internal/makino/live` to hanlde live related features instead of writing in `internal/server/rpc/servers/quest`, because obviously it's gonna be a lot of new source files you need to add, and some other RPCs such as `Dream/StartQuest` has a similar function like `Quest/Start` that need to invoke live start.

---

Let me try to answer your questions.

1. Start from validated subset, then expand the border to unvalidated ones on your speculating if possible, such as guess the effect based on the skill name, but do let me know that you are speculating rather than basing this on solid evidence if you do so. If you really don't know how to achieve it let me know, I will try if I can grab a capture or two (but eventually some skills will remain unvalidated because I don't own a complete idol card set within my account).

2. 
  **stat-composition**:
  Let `X` be the `UserCard.Vocal` (`Dance` and `Visual` follow the same rules), then deck attribute `Y` is:
  `Y = X * (1 + bonusPermil / 1000) + bonusValue`
  in which, `bonusPermil` is the sum of all ratio value bonus from cards' `liveAbility` (such as `lba-vocal_multiply`), equipped accessories, equipped photos and character company level bonus. `bonusValue` is the sum of all real value bonus including staff training bonus, equipped accessories and equipped photos.
  `Y` is the initial attribute stat regarding the live calculation, all buffs such as "ボーカルアップ" is applied based on this value.
  `mental` and `technique` are similar as well, let `Y` be the deck attribute:
  `Y = 100 + bonusValue`
  in which, 100 is a constant, `bonusValue` is the sum of `liveAbility`, staff training bonus, equipped accessories, equipped photos and character company level bonus.

  **mental/success-rate**:
  Let `X` be the skill success rate,
  `X = min(1, 1000 * mental * R / mentalThreshold / P)`
  in which, `R` is the audience occupy rate of the venue, `mentalThreshold` is from the master data `Quest.mentalThreshold`, `P` is from the master data `QuestPressure.weightPermil`.

  **critical-rate**:
  Let `X` be the critical rate,
  `X = min(1, technique / 1000 / L)`
  in which, `L` is from the master data `Quest.difficultyLevel`.

  **audience-allocation**: Based on the character's fan amount.
  Let `X_n` be the amount of a character's audience without considering the venue's maximum capacity:
  `X_n = lightFan * 0.05 + middleFan * 0.2 + heavyFan * 0.5`
  Then the capacity enriched audience amount `Y_n` is:
  `Y_n = capacity * X_n / sum(X_n, n=1~5)` (for normal live)
  `Y_n = capacity * X_n / sum(X_n, n=1~10)` (for live battle)
  The final audience amount `Z_n` is;
  `Z_n = min(X_n, Y_n)`

3. Most of the skill parameters are listed in `docs/IDOLY PRIDE Score.xlsx`, if you don't understand some of them, or need more data, please let me know what you need.
NPC configurations can be retrieved from veteran account's `QuestTopResponse`. Field `cardInfos` is NPC's deck.
I've updated the master database to the latest version wiring with the official server, now it should have the data you need.

4. Newly captured traffic is placed in the directory `traffic/mixed/quest/round2`, it's consecutive and ordered chronologically.

---

**In the success-rate formula, how exactly is R calculated: total venue attendance ÷ capacity, the individual idol’s audience ÷ capacity, or the idol’s audience relative to a one-fifth share of capacity? The new full-venue captures don’t distinguish these.**

total venue attendance / capacity. No difference among 5 deck idols.

**Is there a minimum critical chance or another base modifier? Your formula predicts about 1.2% for round2’s unbuffed lanes 2–5, while their recorded ordinary beats contain 34 criticals in 1,040 opportunities. I won’t add an inferred floor without clarification.**

Actually, the critical chance formula has not been fully proven; it is merely a conjecture by players. So you can use this formula directly, or infer it yourself based on the samples we have captured.

---

- How the minimum audience of 10 applies when calculated attendance is zero, and how seat allocation rounds.
  I think there is no difference when characters have initial 10 audience, why do you ask?
  Rounding method for seat allocation is not confirmed yet, you can create your own rules, or infer from the existing data.

- How NPC stat bonuses are derived beyond cardInfos: captured mental/technique values include 130/130 and 7,796/8,754.
  Consider `cardInfos` is already the final NPC card status, no bonuses applied.

- A successful daily multi-play capture to establish reward and resource scaling—the new three-play example failed.
  Newly captured traffic is placed in the directory `traffic/mixed/quest/round3`. There are also some `Pvp/Start` traffic you can make use of to understand how does the live battle work.

---

Let me correct my previous answer.
`Top.cardInfos` has no numerical attributes but it has some kind of bonus applied, although the rules is not confirmed.
I've placed captured tower 975's `Quest/GetLatestClearLiveResult` traffic in the directory `traffic/mixed/quest/round4`. While the amount of sample is limited, perhaps you can try to fathom NPC's attribute from it.

---

If there's no data to match the official, we will have to make our own. Make a rule to apply additive adjustments for NPC while ensuring game balance.

---

If you got no questions, please start implementing the RPC.

---

Looks like your work is interrupted due to usage limit, now it's recovered, please continue your work.

---

By the way, please take performance efficiency into account when you implementing the RPC, we don't want it cost too much memory or consume too much time.

---

Please describe the remaining uncertainties and unimplemented mechanisms in your reply regarding the Live feature, including skill and buff types.

---

I have answered the questions you raised in `docs/instruction-260916.md`, the lines start with ">" are my comments.
In addition to that, please make modifications based on the following instructions.

- Use protobuf generated const names instead of literal `int` number for skill types (and other similar types such as target types) in our code. These integers are subject to change (although not likely) by the official teams, using const names is a more robust way to resist this kind of changes.
- In addition to `Quest/Start` traffic we've seen before, newly added traffic (`traffic/mixed/quest/round5`) contains `Gvg/Start` traffic, which is one kind of multi-user live battle we don't need to implement but the underlying mechanism remains the same with other kind of lives, be sure you don't miss it. Traffic in the directory `traffic/mixed/quest/round5` is ordered chronologically and consecutively.

---

I've added my own comments to `live-coverage.md`, please check lines start with ">".
In addition to that, some more answers to your questions:

- **Chance reassignment versus target selection**
  First, I think maybe there is a misunderstanding regarding to the lane number.
  There are 5 lanes for each Live, the official data (like responses and database) treat the lane number as 4-2-1-3-5 from the left to right, but the players usually treat the lane number as 1-2-3-4-5 from the left to right. So, for official data, the center is lane 1, but for player data the center becomes lane 3. We should follow the official styled lane number in our implementation, if you previously treat the lane number as 1-2-3-4-5, please check all lane related stuffs if they are correctly implemented and then fix.
  Now back to your question.
  There is a definition for A-skill chance reassignment in "IDOLY PRIDE ライブの詳細.xlsx" sheet "日本語版", find the "【SPスキル、Aスキルの時】スキルを発動するアイドルを決める" part.
  When an effect requests two scorers but more qualify, they are not choosed randomly, the lower lane number ones take precedence.
  New captures in `traffic/mixed/quest/round6` have these situations covered, see it for yourself to confirm the answers.
- **BeatBoost**
  Unfortunately I don't have these 2 cards at this point, please treat each stack a 5% boost for beat score as a temporary solution, I will try to gather real server data in the future.

A remind: authority should be calculated after A/SP migration, which means migrated effects are already applied at the calculation time.

Additionally, if the assumptions I provide contain discrepancies with the actual situation you observe from the official traffic, be sure let me know, I will double check and decide whether to keep the assumption or change.

Please continue to refine the Live feature based on these information, then include remaining uncertainties in your reply.

---

**Quest character advantage**: permil applies to the final score, not attributes
**Songs requiring six or more performers**: `music-clb-011` was a time limited collaboration stage currently not playable in the official game, leave it for now
**Exact critical rate formulas**: the only things we know is it's bond to card's technique and quest's `difficultyLevel`
**Exact scoring formulas**:
for A, SP and Passive skill score `S`:
`S = V * magnification * weight * P_m * P_aud * P_combo * P_crit * P_chara * alpha + S_fin`
in which,
`V` is the card's attribute stat corresponding lane attribute at the skill activation point;
`magnification` is the skill's power plus buffs (e.g. the SP skill itself has "1050%のスコア獲得", the card has buff `SkillEfficacyType_SpecialScoreMultiplierAdd` for 6 stacks at the skill activation time, then `magnification = 10.5 + 0.4 * 6`);
`weight` is the weight from master data `Quest[*].activeSkillWeightPermil` or `Quest[*].specialSkillWeightPermil`, passive skill always use 100%;
`P_m` is the sum of bonuses from photos, card live abilities, character company score bonuses, buffs (e.g. equipped photos have A skill bonus +45%, all card live abilities have A skill bonus +20%, card character's company bonuses for A skill have 10%, the card has buffs `SkillEfficacyType_ScoreUp` for 6 stacks and `SkillEfficacyType_ActiveSkillScoreUp` for 8 stacks at the skill activation time, then `P_m = 1 + 0.45 + 0.2 + 0.1 + 0.025 * 6 + 0.05 * 8`);
`P_aud` is the audience bonus;
`P_combo` is the current combo bonus with buffs applied (e.g. current combo is 83 , the card has buffs `SkillEfficacyType_ComboScoreUp` for 4 stacks at this point, then `P_combo = 1 + 0.3 * (1 + 0.1 * 4)`);
`P_crit` is basic critical bonus plus all other critical bonuses(e.g. equipped photos have 75% critical bonus in total, card abilities have 10% critical bonus in total, character company critical bonus have 10% in total, the card has buff `SkillEfficacyType_CriticalBonusPermilUp` for 12 stacks, then `P_crit = 1 + 0.5 + 0.75 + 0.1 + 0.1 + 0.05 * 12`);
`P_chara` is the quest character advantage;
`alpha` is a score jitter, ranged from 0.95 to 1.05;
`S_fin` is the fixed score bonus from photos and live abilities.

for each normal beat score `S`:
`S = 8 * V / N * P_m * P_aud * P_combo * P_crit * P_chara * alpha + S_fin`
in which, 
`V` is the sum of card's 3 attributes stat corresponding quest's beat attribute weight at the skill activation point (e.g. the card has vo/da/vi for 100000/140000/120000, quest attribute weight permil for vo/da/vi is 600/200/200, then `V = (100000 * 600 + 140000 * 200 + 120000 * 200) / 1000`);
`N` is the total beat amount of the quest for a single lane;
other parameters have meanings similar to those described above.

**Migration combined with reassigned opportunities**: consider reassigned skill anchors are as if they had been at that lane all along, the source lane never had those skill anchors
**Recursive reassignment**: iirc for now there is only 1 card has reassignment skill thus it's not producible, but for implementation let's make reassigned skill chance be not reassignable again
**BeatBoost’s existing stamina drain**: make it has no stamina drain for now

---

Looks like your task is interrupted due to usage limit, now it's reset, please continue your work.

---

Based on the real device test, we found some issues.

- An error raised on successfully finishing a contest live (failure cases won't trigger the error, successful daily live won't trigger either):
> RPC failed with error{error 26 0  transaction failed: rpc error: code = FailedPrecondition desc = reward "rd-qt-area-1-001" contains an unsupported random resource}  {"layer": "grpc", "api": "/api.Quest/Start", "request_id": "639252041451966940", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
- Score ranking rank up pop up appears after finishing a live every time. Score ranking rank up pop up tells users their new score has hit a new rank among all users in contest live (e.g. from rank 4891 to rank 13). Once that rank has been hit, there will be no pop ups for that rank anymore in the future (e.g. a user hit rank 1 once, after a while dropped to rank 6, then the user hit rank 1 again, no pop ups for the second time). Besides, the score rank up pop up is supposed to be only available in contest live, no other types of live (daily, tower, etc.). One sample contest live response from our private server is placed at `traffic/mixed/fix-quest/ours - QuestStartResponse.json`, there must be some fields affect the appearance of the pop up.

Please try to fix these issues.

---

Now we are complete implementing the Live features. Check Live related implementations again, find unimplemented features, uncertainties and assumptions, write them down for the future improvements.
Please converge your conclusions into a single new file, not existing files.

---

Please fix multi-recipient implementation gap, and implement the remaining `Quest/StartBulkDaily` RPC, the sample captures are already in place:

```
./traffic/mixed/quest/newbie_2/002-QuestStartBulkDailyResponse.json
./traffic/mixed/quest/round3/017-QuestStartBulkDailyResponse.json
./traffic/mixed/quest/veteran/006-QuestStartBulkDailyResponse.json
./traffic/mixed/quest/newbie_1/069-QuestStartBulkDailyResponse.json
./traffic/mixed/quest/newbie_1/136-QuestStartBulkDailyResponse.json
```

`StartBulkDaily` use the latest cleared deck to bulk complete the quest. If the deck has been changed since the last clear, the changes will reflect to the bulk dailies afterwards.
The `StartBulkDaily` only consumes free passes. e.g. a player has 5 free coin passes, 3 piece passes, 0 film passes, the API consumes all of the remaining free passes. Once free passes are exhausted, 「一括プレイ」button will disappear from the daily quest UI.

**The newbie captures show two quests both cleared with deck 1, then bulk uses each quest's distinct earlier formation. Should we follow your stated rule—reload the current saved deck—or preserve a per-quest formation snapshot like those captures?**

> Preserve each quest's last-clear formation

---

Based on real device test we found these bugs you need to fix:

**score rank up popup was fixed in lives other than contest**
Contest live still has rank up popup every time even if high score is not updated

**lives other than contest don't show score and grade progress for each beat**
Contest live did this well, but other lives did not

**Moshikoi `Mission/Receive`**
Clicking receive button for a specific mission 「TRUEエンドをクリア」 on Moshikoi's daily mission page will trigger the error:
```
RPC failed with error{error 26 0  transaction failed: rpc error: code = Unimplemented desc = automatic reward type "emblem" is unsupported}      {"layer": "grpc", "api": "/api.Mission/Receive", "request_id": "639252874932382550", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
```
Perhaps this is because that quest should not belong to daily missions?

**about blue diamonds**
Error raised when drawing normal gacha if the user doesn't have enough red dia 
```
RPC failed with error{error 26 0  transaction failed: rpc error: code = ResourceExhausted desc = insufficient gacha resource}    {"layer": "grpc", "api": "/api.Gacha/Draw", "request_id": "639252879025532860", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
```
Blue diamond is paid currency, users can use blue diamonds instead of free red diamonds for purchases and drawings, but the reverse is not possible.
Examples:
case 1: a 5000 blue dia bundle/drawing
Only blue dia is acceptable
case 2: a 2700 red dia bundle/drawing
Consumes red dia first, then blue dia (e.g. a user has 1000 red dia and 2000 blue dia, this will consume 1000 red dia and 1700 blue dia)

Captured `Quest/Start` traffic from our private server is placed in `traffic/mixed/fix-0918`, this may help you investigating the issues.

---

Don't create new documents in `docs` unless I told you to do so.

This time your task is to implement the `Dream` service, which is called "イメージトレーニング" in game.
See the official help documentation for this feature first.


# イメージトレーニング

イメージトレーニングは常時開催されるイベント形式のコンテンツです。
ステージのクリアによってフォトの各オプションの効果値を強化することができます。
生成済みのステージはいつでもプレイ可能です。

イメージトレーニングは【ホーム > イベント】からプレイ可能です。
毎月15日の12:00にミッションがリセットされ、「イメトレマニュアル」がギフトに送られます。(5 per month)

## ステージの生成

所持しているフォトの中から
・「ベースフォト」：強化するフォト（★4以上）
・「素材フォト」：強化に必要なフォト（★4以上）
を選択し、「ベースフォト」のステージを生成することができます。

ステージの生成には「イメトレマニュアル」を消費します。
消費量はベースフォトと素材フォトの内容に応じて変動します。

ベースフォトと素材フォトは「通常フォト同士」「レタッチフォト同士」どちらかである必要があります。
また、ステージが生成されたフォトはレアリティ★がピンク色の枠で表記されます。

## ステージの挑戦

ベースフォトの各オプションの効果値は、生成したステージの進捗に応じて上昇します。
ベースフォトと素材フォトのレアリティや品質、オプションの類似度に応じて、難易度を示すGRADE、ステージ数、上昇値、成長率が変動します。
また、メインライブの進捗に応じた一部ステージのSKIPや助っ人の編成が可能です。

## ステージのリセット

【イメージトレーニング > ステージ一覧 > フォト選択 > フォト詳細】から、ベースフォトのリセットを行うことができます。
リセットを行うとそれまでのステージ進捗や、各効果値の上昇がリセットされます。
リセットを行っても生成時に使用したイメトレマニュアルや素材フォトは返却されません。

## 注意点

ステージ生成に使用した素材フォトは消失します。
ステージ生成済みのフォトの品質アップには、追加のアドバンスフィルムが必要です。

下記のフォトは素材フォトとして使用できません。
・ロック状態のフォト
・名刺に使用されているフォト
・ステージ生成済みのフォト

また、一部フォトはイメージトレーニングのベースフォトとして使用できません。
・けいおん！コラボ「コラボミッションフォト」


The exact area generation rules are unclear, but from my observation, it has something to do with:
- the highest attribute of the crafted photo
- the photo's type (normal, retouched, character dedicated, server predefined)
- the photo's rarity, level
Like wise, the rules regarding how much will the stat values ​​increase after enhancement is also unclear.
Try to figure out the approximate rules from the captured traffic and applies to our private server, the captured traffic has plenty of samples should be enough to support your investigation, no need to make the rules be exact in line with the official server.

The official rules of how many stages can be skipped is also unclear, but to make it simple, let's make it a 4/5 of the total stage amount. (e.g. generated area has 50 stages in total, can skip the first 40 stages)

---

Looks like your previous task is interrupted due to usage limit, now it is reset, please continue your task.

---

This time your task is to add another new Moshikoi series `love-23-1114`.
The route map is placed at `docs/love-23-1114-routemap.md`, the scenario scripts are placed in `resources/moshikoi`.
Remember to add permanent event entrance to `Home/Enter` response and don't forget to write tests to ensure all endings are reachable.

---

Based on the real device test, sometimes RPC `Quest/ListAsset` will be invoked before `Dream/StartQuest`, and will 100% produce the following error:
```
RPC failed with error{error 26 0  rpc error: code = InvalidArgument desc = questId and a saved deck number are required}   {"layer": "grpc", "api": "/api.Quest/ListAsset", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
```
Perhaps this is because `Dream` has its own unique deck for each generated area (not sure, need investigation).
Please try to fix the issue.
Don't forget to stop the server after you have completed your work.

---

This time your task is to implement the `Exercise` service, which is also called "研修" in the game.
Just like the name suggests, it plays a role helping users improve their understanding about how the Live works. Users will try to use a specific set of cards, photos and accessories to clear carefully controlled Lives to exercise themselves. The rules for exercise Lives are roughly the same with normal Lives, and follow these rules:

- lives are carefully controlled, they can only be cleared if users adjust their deck correctly
- the warehouse (cards, photos, accessories, etc.) in Exercise is not related to the current status of user themselves, see master data collections start with "Exercise" in their names
- no jitters or criticals for score calculation (assumption)
- results are idempotent (identical decks produce identical results)

The captured chronological traffic is placed in `traffic/mixed/exercise`.
(If there is really no jitters and criticals, perhaps you can try to figure out the beat calculation formula from the captured snapshots?)

---

Looks like your previous task is interrupted due to usage limit, now it is reset, please continue your task.

---

You don't need to edit files this time, just answer my questions. 
Looks like you've inferred the beat score calculation formula, do you think this formula is applicable for normal songs? If so, can you try to infer how did the official team determine the magnitude of `beatScoreCoefficientPermil`?

---

Since we have no means to retrieve the accurate formula from the official server, an approximate solution is acceptable, and your assumption is better than the older `8x` one. Please apply this formula to normal Lives.

---

This time your task is to add new Moshikoi series `love-24-0517` and `love-24-1114`.
The route map is placed at `docs/love-24-0517-routemap.md` and `docs/love-24-1114-routemap.md`, the scenario scripts are placed in `resources/moshikoi`.
Remember to add permanent event entrance to `Home/Enter` response and don't forget to write tests to ensure all endings are reachable.

---

Now another new Moshikoi series `love-26-0313`.
The route map is placed at `docs/love-26-0313-routemap.md`, the scenario scripts are placed in `resources/moshikoi`.
Remember to add permanent event entrance to `Home/Enter` response and don't forget to write tests to ensure all endings are reachable.

---

Now it's time to implement the last Moshikoi series (for now), this time it's a little different from the previous ones, which has the ID of `love-extra-25-0426` and is being called "サキュ×まほ" in the game.
The `love-extra` is almost identical in function to the previous implementation, with a different point that:

- `love-extra` has multiple heroines, user's choices will affect which specific heroine they are going to reach the ending with, or if not specific, a harem ending.

The captured traffic is placed in `traffic/mixed/love-extra` ordered chronologically and consecutively, but be sure to note that this time, these traffic files are captured from an old version of game, more specifically, from version "4.10.3", which is far behind the current version "6.0.2". So if there are fields missing or cannot be exactly mapped to the current protobuf fields, the current version shall prevail.

The route map is placed at `docs/love-extra-25-0426-routemap.md`, the scenario scripts are placed in `resources/moshikoi` with the prefix of `adv_love_2505`.
Remember to add permanent event entrance to `Home/Enter` response and don't forget to write tests to ensure all endings are reachable.

---

I've got a question. In "サキュ×まほ", aside from Rui, do we have other routes terminate without an ending? If so, change the routemap and send such routes to corresponding bad endings.

---

Fix moshikoi related implementations according to the following instructions:

- Remove `love-extra-*` from `LoveInfos` list in the response of `/api.Love/Title`. Extra love events are only mean to be aquired from `/api.Love/TitleExtra`.
- Remove multiple love event entrances from `eventInfo.loveEventInfos` in the response of `/api.Home/Enter` and leave only one (except `love-extra-*`).
  Based on the current implementation of our private server, each Love event will have its own entrance banner on the home page because all of them are included in the `eventInfo.loveEventInfos` list, resulting in a lot of banners being displayed on the homepage. Besides, unlike `/api.Love/TitleExtra`, the request of `/api.Love/Title` API is empty, making clicking on different banners has no difference at all, users can choose which Love story they want to play on the Love title page and then enter Love event main page (by invoking `/api.Love/Top`). So I think it's fair enough to leave only one love banner and keep all other love-extra banners.
  But this will result in a question: which `loveEventInfo` should we remain? I think we can randomly pick one from all available Love events: each time `/api.Home/Enter` is invoked we just randomly choose one. I'm not really sure at this point if this will make any issues on the client since this is not an official behavior but let's give it a try.

---

Check places where `papi.NoticeInfo` should be returned (such as in the `papi.LoveTopResponse`).
If we leave these fileds `nil`, the client will froze when clicking notice button on the corresponding pages. So instead of leaving them `nil`, let's return a dummy instance:

```
NoticeInfo: &papi.NoticeInfo{
  Id: "",
  Title: "",
  BannerAssetId: "",
  LinkType: penum.LinkType_LinkType_WebView,
  LinkDetail: "",
  DisplayNotification: false,
}
```

---

Fix the following issues:

- I have not checked all, but when invoking `/api.Gacha/Probability` from several gacha pages the following error will rise:
```
RPC failed with error{error 26 0  rpc error: code = FailedPrecondition desc = gacha reward probability is invalid}   {"layer": "grpc", "api": "/api.Gacha/Probability", "suid": "dcf66c07-2aab-4274-892a-f51407629a19"}
```
- When creating a new user, fix `inviteHostUserId` to `00000000`. If we leave it nil or empty string the client will show notification to inform users to enter their inviter ID, which causes notification fatigue.
- Fix the value of `showcaseLike.rewardLikeCount` field in `UserGetResponse` to 0 and don't apply daily recovery. When this value reaches 30 the client will display a notification button to inform users to consume the points, which causes notification fatigue.
- For now in our private server, we have a lot of different events, gachas, login bonuses. Many of those are we don't need. Remove or keep contents according to the following:
  - Events: Only keep もし恋 (Love), サキュ×まほ (Love Extra), イメージトレーニング (Dream). Remove other events such as beginner mission event.
  - Login bonuses: Remove start dash login bonus.
  - Gachas (only applies to card gacha): Don't add beginner gachas, time-limited gachas, fixed rate gachas and birthday gachas when creating a new user, too many gachas will make users feel overwhelmed.

---

You don't need to edit files this time, just answer my questions.
Instead of removing unnecessary entries from where they are added, I see you added filter functions such as `IsGachaEnabled` and `loginBonusEnabled` to filter out those entries.
Why do you implement it like this? Is it possible to remove those entries from where they are added? If so, how?

---

A gacha yielding issue needs to be fixed.
In our current implementation, gachas only yield `pickupCardIds` defined in the master database.
But that is not the true purpose of this list. `pickupCardIds` only tells the client which cards should be displayed in the gacha page. For most of gachas such as limited gachas and birthday gachas, these cards are usually equal to the yielding cards, but for normal gachas that's a different story. Normal gacha should always yield all cards with equal probability, `pickupCardIds` is just for display purposes only. In another word, the yielding cards and probabilities are controlled by the server, not the master database. (In fact, limited cards are excluded from the yielding list of normal gacha in the official server, but let's ignore this in our server)

---

Oh, by "equal probability" I mean equal probability in its own rarity tier.
For example rarity 5 cards have an overall yielding probability of 3.5%, so each rarity 5 card should have an equal probability in this 3.5% only.

---

Good work, that's almost enough APIs for our private server to make the game work properly, now let's focus on customizing our private server to make some interesting things that the official server will never do.
First, create a new command line binary tool at `cmd/makino`.
Consider this binary a server administrator management tool, providing a convenient way to tweak server configurations and database, which includes:

- Create a new user
- Godify a user
- Delete a user
- Change server notice 
- Change shop, exchange items
- Change current ongoing gacha

More features may be added in the future.
For creating, godifying, deleting users, the tool can directly invoke admin APIs since these features already have been implemented on the server side. For changing configurable server data, the tool maybe need to directly interact with the database.
The CLI example usage can be something like this:

```
# create a user
makino user create --suid "xxxxx" --id "xxxxx" --password "xxxxx"
# godify a user
makino user god --id "xxxxx" --message "enjoy"
# delete a user
makino user delete --id "xxxxx"
# overwrite server notice using all yaml files under a directory
makino server notice --overwrite --dir "./server-notice"
# add server notice
makino server notice --add --file "./server-notice/notice-2006-10-13.yaml"
# clear server notice
makino server notice --clear
```

Remember to recover the database's state at the end of test if you modified data during the test.

By the way, currently, it seems like aforementioned configurable server data files are scattered across many different places in the project directory, and some of them are embedded into the binary. Since we are going to make it configurable, please make the following refactoring:

- To make it easier to locate these files, please consolidate them into a single, clearly identifiable location.
- Make the server read these data from the database on the fly instead of reading from binary or files on the file system, so our tool can just interact with the database to make changes instead of modifing files. After refactoring these yaml files only serve as initialization data that will be read only once during the server's first startup (from this perspective, perhaps lv1 migration could be a good place to do this).

---

I see you use `config/catalogs` as the replacement of `internal/db/internal/migration/catalogdata`.
Since those files only serve as initialization seeds that are read only once during the database initialization, I think keep them in the old place `internal/db/internal/migration/catalogdata` could be a better choice. Instead, `config/catalogs` can be used as the default directory for `makino` CLI tool.

---

Please remove the `.gitkeep` files in the catalog directories, instead create example yaml files (more precisely, the initial state of the server) and place them into the directory, so the users have samples to refer to and don't have to create files from scratch.

---

Sorry now I think my previous instruction was not good because it would introduce an extra set of source-of-truth. Every time we change the initial state of the server catalogs we will have to do twice if we keep a copy.
So please undo the previous changes, instead change the layout of directory `internal/db/internal/migration/catalogdata`, create exact same name folders as folders under `config/catalogs`, and put the corresponding yamls into that folder, so users know which file they should copy from. For yaml files not served as configurable catalog just leave them where they are.
After that, show me a simple example for how do I add a item into an existing shop (say normal shop).

---

This time I need you to make a plan, do not edit source code.
In order to further customize the game, we need to modify the master database.
Current used encrypted master database raw files are placed in `resources/db`, these are also files which `/:db/:filename<regex([a-z0-9]{64})>` API returns to the client.
These files can be decrypted using `DecryptMasterDbFile` in `internal/utils/crypto/master.go`.
To make costomization possible, we need to create a reverse version of this function and add some new features to `makino` CLI.

The complete workflow of the customization shows as below:

1. Server admin uses `makino` to export the current master database (specific or all) to yaml file(s). Note all `data` fields also need to be converted to plain text to make it easier for users to modify.
2. Admin edits one or some yaml files in plain text and save.
3. Admin run `makino`, which reads the modified database (specific or all) from yaml file(s), converts `data` to valid protobuf binary, uses the binary to update collections in `master` database, encrypts the database back to SQLite raw file(s) with sqlcipher and place them into `resources/db`, deletes the stale database raw file(s), then updates `solis.masterTagPack` manifest accordingly, so the client can download the modified version of the master database file and successfully decrypt the file on the device.

Note the following things:

- There is delete operation, do it carefully. And make sure these files are restored after testing.
- Add features for `internal/db/internal/update/update.go`, whenever an update is detected, despite upserting to collection `masterTagPack`, also upserting to the collection `masterTagPackOrig` to keep a clean version of the original master database manifests. Don't write extra code just for triggering the first backup, instead you can modify `solis.serverProfile.masterVersion` manually to trigger it.
- 

---

Some questions for your plan:

1. Why do you add the `/admin/master/import` API to the server instead of just implementing the publication feature to `makino` CLI?
2. Is it possible to create a SQLCipher entrypted SQLite database from scratch and fill it with our customized data instead of relying on copying an existing one then modifying?

---

Yes, implement this plan

---

Make master data importing also be available when the server is shutted down.

---

Looks like your previous work is interrupted due to usage limit, now it's recovered, please continue your work.
By the way, is it possible to not add the `--offline` flag, just detect the server's status and branch the way for import?
For example, when run `makino master import --file config/master/Card.yaml`, if the server is online, use coordinated publication; if not, run offline import.

---

Add a feature that when no `--file` or `--dir` is supplied for `makino master export`, output the yaml contents to stdout.

---

Hey, your previous test caused the OS crash, because `--all` output too many contents in one shot, let's change our strategy.
If `--all` is supplied, `--dir` must be supplied. Even if `--all` is not supplied, if you want to test the outcomes, redirect the output to a temp file first then check the file.

---

Add a feature to reset the master database to the original state: `makino master reset`. Like import and export, it should support both single and all database.
But since we delete the original database file everytime after a new customized one get imported, the original database may be already deleted hence there is no way to reset it to the original state. So change the deletion logic: if the stale database file is original one, keep it; otherwise remove it.

---

Now, add another sub-command `makino custom`, it provides a group of convenient templates to quickly customize (or recover) the master database.

- `makino custom home-action`: maps all entries in `LoveHomeAction` master database to `HomeAction` with empty `conditionId` and 100 `weight`. For example:
  an `LoveHomeAction` entry 
  ```
  data:
    "id": "love-home-action-26-0313-05-present-02"
    "loveId": "love-26-0313"
    "characterId": "char-rio"
    "motionAssetId": "mot_adv_chr_cmn_glad-012_in"
    "voiceAssetId": "sud_vo_home_rio_love-01-04-present-02"
    "text": "ありがとう。いつも私がもらって\nばかりだわ。貴方も何か欲しいもの\nないの？プレゼントするわよ。"
    "hiddenFavorabilityGte": "7226"
    "unlockLoveStoryEndIds":
      - "love-end-love-26-0313-008-happy-001"
    "weight": 1000
    "actionType": "LoveHomeActionType_Present"
    "costumeId": "cos-rio-casl-02"
  ```
  maps to `HomeAction` as
  ```
  data:
    "homeActionId": "love-home-action-26-0313-05-present-02"
    "characterId": "char-rio"
    "motionAssetId": "mot_adv_chr_cmn_glad-012_in"
    "voiceAssetId": "sud_vo_home_rio_love-01-04-present-02"
    "text": "ありがとう。いつも私がもらって\nばかりだわ。貴方も何か欲しいもの\nないの？プレゼントするわよ。"
    "conditionId": ""
    "weight": 100
  ```
- `makino custom photo`: unlocks all maps, characters, costumes, poses for all `PhotoActivity`, `PhotoExpression`, `PhotoQuestMusic` and `PhotoQuestStage` master database.
  To do this, remove fields `viewConditionId`, `impossibleShootingCharacterIds`, `impossiblePhotoPoseIds` for all `PhotoActivity` and `PhotoExpression` entries, then collect all existing `costumeTypeId` from `Costume` master database and overwrite field `costumeTypeIds` for all `PhotoActivity` and `PhotoExpression` entries. Also, remove fields `viewConditionId` and `unlockConditionId` for all `PhotoQuestMusic` and `PhotoQuestStage` entries.

All of these commands need to have an option `--reset`, which reset the affected master databases to their original state.

---

We've got a bug.
In our `UnaryServerInterceptor` (`internal/server/rpc/internal/interceptor/interceptor.go`), if we need to force the user to go to the title or home, we add related action headers and return a nil response, that's OK at this point.
However, if we go through the internal gRPC message process flow, the msgHeader function (`rpc_util.go`) ill always attach 5 byte header to the message, which is invoked by sendResponse (attached `server.go`), causing the response HTTP body always contains 5 byte header (in a nil response it is exactly 0x00 0x00 0x00 0x00 0x00, checked by traffic capture). But if we see the captured official response there isn't a single byte in the response body for title or home action response.
You may think this is not a big problem because no matter it's 0 byte or 5 byte response the client's gRPC package will handle this correctly, but the fact is that the client refuse to go to the title or home page when receiving our server's response, and the only difference between ours and the official's is the 5 byte body.
Please try to find a clean way to wipe out the automatically attached 5 byte header when nil response is returned, best if gRPC package provides us an option or something to tweak this.

---

Test confirmed, client now successfully accepts redirection responses.
Now let's start refining server configuration related components.
Remove these ENVs:
- SOLIS_APP_VERSION
- SOLIS_DEV_NO_CODEC
- SOLIS_DEV_NO_CERT
Make the server always need to configure a codec and certification.

Rename these ENVs:
- SOLIS_RSA_PRIV_KEY -> SOLIS_JWT_RSA_PRIV_KEY_PATH
- SOLIS_RSA_PUB_KEY -> SOLIS_JWT_RSA_PUB_KEY_PATH
- SOLIS_HMAC_KEY -> SOLIS_JWT_HMAC_KEY
- SOLIS_DEV_GRPC_CERT_KEY_PATH -> SOLIS_GRPC_CERT_PRIV_KEY_PATH
- SOLIS_DEV_GRPC_CERT_PATH -> SOLIS_GRPC_CERT_PUB_KEY_PATH

---

Now we are going to prepare for the public release.
Build a docker image for our server and makino CLI. You need to create 2 files in the project's root directory (ignore `docker` directory, it's a temporary directory that will be removed later):
- `Dockerfile`: docker build file, need to build 2 images: the server and the makino CLI. Keep the final images small, place both binaries under `/app/` directory in the container.
- `compose.yaml`: a sample docker compose file. Contains our image and mongodb image: `mongodb/mongodb-community-server:latest`, and everything else need to run the server properly. For mongodb container, you can refer to `docker/compose.yaml`. For our server, make sure that:
  - secrets are mounted properly
  - resources directory, config directory, `.env` file, and other resources (if any) are mountable
  - server can be configured by both environment variables defined in `compose.yaml` and `.env` file, but the latter one takes precedence
  - the container is ready to use out of the box. If everything is configured properly, one command `docker compose up` is enough to boot the server

Then create a `README.md` in the project's root directory, explain what this project is, the essential technique details, how to build, how to deploy, how to use (server and makino CLI), and claim we are not affiliated with the official team, everything we created is not stealed from the official code repository, we write the entire project on our own. Also warn users this project is not suitable for public serve, because we didn't reinforce the AuthN and AuthZ strategies, no countermeasures against various types of attacks.

---

Please make changes according to the following instructions.

- Upgrade all dependencies to the latest version and solve compatibility issues if any.
- Remove `SOLIS_DB_URI`, `MONGODB_INITDB_ROOT_USERNAME`, `MONGODB_INITDB_ROOT_PASSWORD` from ENV file. Instead use `SOLIS_DB_USERNAME` and `SOLIS_DB_PASSWORD`. Make the program assemble the connection string at runtime. These ENVs are also used for initialize mongodb.
- Don't build an extra `makino` image, put all 2 binaries into one single container. So users can run `makino` just inside the running server container.
- Rethink if we really have to mount the `.env` file into the container. If `docker compose up` automatically read `.env` file and populate those variables, perhaps we don't need to mount the file into container?
- Add `SOLIS_DB_DATA_DIR` environment variable (defaults to `./data/`) to configure data directory for mongodb and mount it to mongodb's container, so users can backup data directory on their host machine. What is `mongo-config:/data/configdb` used for? Do we have to add that volume?
- Put secret files into `secrets` section of the compose file instead of mounting them as normal files. These files don't have to be placed in `/app/secrets`.
- The content of the README you wrote is too long-winded, please keep it concise, you don't need to explain every detail. The readers' interests are usually only "what can this program do?" and "how do I use this program?". Also add a "TL;DR" chapter at the beginning to explain how to use the repository. You can use github Alerts syntax (such as `> [!IMPORTANT]` and `> [!WARNING]`) to highlight contents, but do not overuse it.

---

OK, now I have a backup of your previous version.
Below is an outline of the contents of README.md, please use it to write a concise and friendly explanation about this project.

```
# Overview
Overview of the project

> [!IMPORTANT]
> claim that we are not affiliated with the official team


## TL;DR
How to quickly build the program and run

## Resources
The structure of a valid `resources` directory. Ask users to obtain resources by themselves, we do not provide them

## Building

### Container image

### Native binaries 

## Configurations
Tell users to rename `.env.example` to `.env` them edit it, and a concise explanation about how does the program use the environment variables.

### Secrets
How to create secret files

## Usage

> [!WARNING]
> tell users do not serve the program on public internet

### Solis server

### Makino CLI

```

---

Our gRPC interceptor can abstract error messages received from services before returning the error to the client, but HTTP middleware cannot, so many handlers only return abstract messages like `return fiber.NewError(500, "internal error")`, without logging detailed errors on the server, makes it hard for server admins to understand what have gone wrong when error emerges.
Please refer to gRPC implementation, make these handlers return detailed messages, log the messages in middleware, and abstract messages according to error code (e.g. 5xx for "internal error", 4xx for their own general meanings), finally return the messages to the client.

---

Create a coverage.md, describe which gRPC APIs are implemented, which are not, link the file into README.md.

---



---

- rankup card not rankuped
- home page showcase gallary max notification, invite code entering notification, event notification, 

