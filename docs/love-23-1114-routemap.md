# Ending list

love-end-love-23-1114-002-bad: スズキスズキエンド
love-end-love-23-1114-008-happy-001: TRUE エンド
love-end-love-23-1114-008-happy-002: 充実した日々エンド
love-end-love-23-1114-008-happy-003: 小説の糧エンド
love-end-love-23-1114-008-normal-001: 現実は甘くないエンド
love-end-love-23-1114-008-normal-002: 互いの道エンド
love-end-love-23-1114-008-bad-001: 今更エンド
love-end-love-23-1114-008-bad-002: 見つめなおしエンド


# EPISODE 1 (adv_love_2311_01_01.txt)

choicegroup 1
0: 妹が出来た渚を祝福する
  -> No effect
1: 姉が出来たすみれを祝福する
  -> No effect

choicegroup 2
0: 可愛いと思う
  -> No Normal end flag
1: 分からないと言う
  -> Normal end flag

[After all branches converge, to EPISODE 2]


# EPISODE 2 (adv_love_2311_02_01.txt)

choicegroup 1
0: 逆に渚を気遣う
  -> No Normal end flag
1: 遠慮する
  -> Normal end flag

choicegroup 2
0: 協力する
  -> To EPISODE 3
1: 断る
  -> Ending: スズキスズキエンド (love-end-love-23-1114-002-bad)


# EPISODE 3 (adv_love_2311_03_01.txt)

choicegroup 1
0: ためらう
  -> Normal end flag
1: 誘いに乗る
  -> No Normal end flag

choicegroup 2
0: 恋愛系
  -> No Normal end flag
1: バトル系
  -> Normal end flag

[After all branches converge, to EPISODE 4]


# EPISODE 4 (adv_love_2311_04_01.txt)

choicegroup 1
0: 渚の手を握る
  -> No Normal end flag
1: 渚の手を握らない
  -> Normal end flag

choicegroup 2
0: 言い淀む
  -> Normal end flag
1: 素直に気持ちを伝える
  -> No Normal end flag

[After all branches converge, to EPISODE 5]


# EPISODE 5 (adv_love_2311_05_01.txt)

choicegroup 1
0: たしなめる
  -> Normal end flag
1: 渋々認める
  -> No Normal end flag

choicegroup 2
0: もう少し粘る
  -> No Normal end flag
1: 諦める
  -> Normal end flag

choicegroup 3
0: 続きが読みたいと伝える
  -> Normal end flag
1: 賞への応募を勧める
  -> No Normal end flag

[After all branches converge, to EPISODE 6]


# EPISODE 6 (adv_love_2311_06_01.txt)

choicegroup 1
0: 休憩を提案する
  -> No effect
1: 外出を提案する
  -> No effect

choicegroup 2
0: 応援する
  -> To EPISODE 7
1: 勉強を促す
  -> To EPISODE 7-bad


# EPISODE 7 (adv_love_2311_07_01.txt)

choicegroup 1
0: 将来への不安を訴える
  -> Normal end flag
1: 大学生活の魅力を語る
  -> No Normal end flag

choicegroup 2
0: 仮眠を提案する
  -> No Normal end flag
1: 帰宅を促す
  -> Normal end flag

[At the end of EPISODE 7]

If any Normal end flag has been set:
  -> To EPISODE 8-normal

Otherwise:
  -> To EPISODE 8-happy


# EPISODE 7-bad (adv_love_2311_08_03.txt)

choicegroup 1
0: やり直したいと伝える
  -> Ending: 今更エンド (love-end-love-23-1114-008-bad-001)
1: 何も言えない
  -> Ending: 見つめなおしエンド (love-end-love-23-1114-008-bad-002)


# EPISODE 8-happy (adv_love_2311_08_01.txt)

choicegroup 1
0: 挑戦するように勧める
  -> Ending: TRUE エンド (love-end-love-23-1114-008-happy-001)
1: やめるように助言する
  -> Ending: 充実した日々エンド (love-end-love-23-1114-008-happy-002)
2: 他社からのオファーを話す
  -> Ending: 小説の糧エンド (love-end-love-23-1114-008-happy-003)


# EPISODE 8-normal (adv_love_2311_08_02.txt)

choicegroup 1
0: 関係を続ける
  -> Ending: 現実は甘くないエンド (love-end-love-23-1114-008-normal-001)
1: 関係を終わらせる
  -> Ending: 互いの道エンド (love-end-love-23-1114-008-normal-002)
