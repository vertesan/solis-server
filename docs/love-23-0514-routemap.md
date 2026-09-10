# Ending list

love-end-love-23-0514-001-bad: オトクオトクエンド
love-end-love-23-0514-008-happy-001: 大学生エンド
love-end-love-23-0514-008-happy-002: TRUE エンド
love-end-love-23-0514-008-happy-003: さよならエンド
love-end-love-23-0514-008-normal-001: 夢追いエンド
love-end-love-23-0514-008-normal-002: これからも一緒にエンド
love-end-love-23-0514-008-bad-001: 恋はタイミングエンド
love-end-love-23-0514-008-bad-002: 嫌われエンド
love-end-love-23-0514-008-bad-003: 勘違いエンド


# EPISODE 1 (adv_love_2305_01_01.txt)

choicegroup 1
0: 言い訳をする
  -> No effect
1: 謝る
  -> No effect

choicegroup 2
0: 一ノ瀬さんを追いかける
  -> To EPISODE 2
1: 一ノ瀬さんを追いかけない
  -> Ending: オトクオトクエンド (love-end-love-23-0514-001-bad)


# EPISODE 2 (adv_love_2305_02_01.txt)

choicegroup 1
0: 一ノ瀬さんをファミレスに誘う
  -> No effect
1: 一ノ瀬さんをファミレスに誘わない
  -> No effect

choicegroup 2
0: バイト探しを手伝う
  -> No effect
1: 怜の勉強を見てあげる
  -> No effect

[After all branches converge, to EPISODE 3]


# EPISODE 3 (adv_love_2305_03_01.txt)

choicegroup 1
0: 「先生になりたい」と答える
  -> Happy end flag 1
1: 「ない」と答える
  -> No effect

choicegroup 2
0: 息抜きに誘う
  -> No effect
1: 模試の復習をする
  -> No effect

[After all branches converge, to EPISODE 4]


# EPISODE 4 (adv_love_2305_04_01.txt)

choicegroup 1
0: 追いかける
  -> Happy end flag 2
1: そっと見送る
  -> No effect

choicegroup 2
0: 怜の実家に行くことを提案
  -> No effect
1: 気分転換に買い物に誘う
  -> Happy end flag 3

[After all branches converge, to EPISODE 5]


# EPISODE 5 (adv_love_2305_05_01.txt)

choicegroup 1
0: ダンスに集中するよう促す
  -> No effect
1: 勉強を頑張る理由を尋ねる
  -> No effect

choicegroup 2
0: 現実を伝える
  -> No effect
1: 嬉しいと伝える
  -> Happy end flag 4

[After all branches converge, to EPISODE 6]


# EPISODE 6 (adv_love_2305_06_01.txt)

choicegroup 1
0: 泣いている場合じゃないよ
  -> No effect
1: 泣いていいんだよ
  -> Happy end flag 5

choicegroup 2
0: テーマパークに怜を誘う
  -> No happy end flag
1: 一人でテーマパークに行く
  -> Happy end flag 6

choicegroup 3
0: 行く
  -> Happy end flag 7
  -> To EPISODE 7
1: 行かない
  -> To EPISODE 7-bad


# EPISODE 7 (adv_love_2305_07_01.txt)

choicegroup 1
0: 付き合ってません！
  -> Happy end flag 8 not satisfied
  -> To EPISODE 8-normal
1: まだ付き合っていません！
  -> Happy end flag 8
  -> To choicegroup 2

choicegroup 2
0: 「好きだ」と伝える
  -> Happy end flag 9
1: 「好きだ」と伝えない
  -> Happy end flag 9 not satisfied
  -> To EPISODE 8-normal

[At the end of EPISODE 7]

If all Happy end flags 1-9 are satisfied:
  -> To EPISODE 8-happy

Otherwise:
  -> To EPISODE 8-normal


# EPISODE 7-bad (adv_love_2305_08_03.txt)

choicegroup 1
0: 告白する
  -> Ending: 恋はタイミングエンド (love-end-love-23-0514-008-bad-001)
1: 見送る
  -> Ending: 嫌われエンド (love-end-love-23-0514-008-bad-002)
2: なんでこんなことに
  -> Ending: 勘違いエンド (love-end-love-23-0514-008-bad-003)


# EPISODE 8-happy (adv_love_2305_08_01.txt)

choicegroup 1
0: 大学に行こう
  -> Ending: 大学生エンド (love-end-love-23-0514-008-happy-001)
1: キャストになろう
  -> Ending: TRUE エンド (love-end-love-23-0514-008-happy-002)
2: 俺には分からない
  -> Ending: さよならエンド (love-end-love-23-0514-008-happy-003)


# EPISODE 8-normal (adv_love_2305_08_02.txt)

choicegroup 1
0: 告白する
  -> Ending: 夢追いエンド (love-end-love-23-0514-008-normal-001)
1: 告白しない
  -> Ending: これからも一緒にエンド (love-end-love-23-0514-008-normal-002)
