# Ending list

love-end-love-24-0517-003-bad: ブワッブワッエンド
love-end-love-24-0517-008-happy-001: 部活よりも恋愛エンド
love-end-love-24-0517-008-happy-002: 駅伝優勝エンド
love-end-love-24-0517-008-happy-003: TRUE エンド
love-end-love-24-0517-008-normal-001: 結婚約束エンド
love-end-love-24-0517-008-normal-002: お友達からよろしくエンド
love-end-love-24-0517-008-bad-001: 謝罪エンド
love-end-love-24-0517-008-bad-002: 未読無視エンド


# EPISODE 1 (adv_love_2405_01_01.txt)

choicegroup 1
0: 遊びに来たと言う
  -> No Normal end flag
1: 誤魔化す
  -> Normal end flag

choicegroup 2
0: 一緒に遊ぶ
  -> No Normal end flag
1: 断る
  -> Normal end flag

[After all branches converge, to EPISODE 2]


# EPISODE 2 (adv_love_2405_02_01.txt)

choicegroup 1
0: 楽しくなかったと答える
  -> Normal end flag
1: 楽しかったと答える
  -> No Normal end flag

choicegroup 2
0: 手を取る
  -> No Normal end flag
1: 断る
  -> Normal end flag

[After all branches converge, to EPISODE 3]


# EPISODE 3 (adv_love_2405_03_01.txt)

choicegroup 1
0: 楽しみにしていたと言う
  -> No Normal end flag
1: そんなことないと言う
  -> Normal end flag

choicegroup 2
0: 告白しない
  -> Normal end flag
1: 告白する
  -> No Normal end flag

choicegroup 3
0: 恋を教える
  -> To EPISODE 4
1: 恋を教えない
  -> Ending: ブワッブワッエンド (love-end-love-24-0517-003-bad)


# EPISODE 4 (adv_love_2405_04_01.txt)

choicegroup 1
0: 歩いて登る
  -> Normal end flag
1: 芽衣をおんぶして登る
  -> No Normal end flag

[After all branches converge, to EPISODE 5]


# EPISODE 5 (adv_love_2405_05_01.txt)

choicegroup 1
0: 芽衣を迎えに行く
  -> No Normal end flag
1: 芽衣を迎えに行かない
  -> Normal end flag

choicegroup 2
0: 奢る
  -> No Normal end flag
1: 奢らない
  -> Normal end flag

choicegroup 3
0: はずれを引いた
  -> Normal end flag
1: 当たりを引いた
  -> No Normal end flag

[After all branches converge, to EPISODE 6]


# EPISODE 6 (adv_love_2405_06_01.txt)

choicegroup 1
0: 芽衣を家まで送っていく
  -> No Normal end flag
1: 芽衣を見送る
  -> Normal end flag

choicegroup 2
0: 告白しない
  -> To EPISODE 7-bad
1: 告白する
  -> To EPISODE 7


# EPISODE 7 (adv_love_2405_07_01.txt)

choicegroup 1
0: 勉強に専念すると伝える
  -> Normal end flag
1: 復帰を考えていることを伝える
  -> No Normal end flag

choicegroup 2
0: うまくいくと伝える
  -> No Normal end flag
1: 分からないと伝える
  -> Normal end flag

[At the end of EPISODE 7]

If any Normal end flag has been set:
  -> To EPISODE 8-normal

Otherwise:
  -> To EPISODE 8-happy


# EPISODE 7-bad (adv_love_2405_08_03.txt)

choicegroup 1
0: 謝罪する
  -> Ending: 謝罪エンド (love-end-love-24-0517-008-bad-001)
1: 告白する
  -> Ending: 未読無視エンド (love-end-love-24-0517-008-bad-002)


# EPISODE 8-happy (adv_love_2405_08_01.txt)

choicegroup 1
0: 陸上は続けない
  -> Ending: 部活よりも恋愛エンド (love-end-love-24-0517-008-happy-001)
1: 芽衣に判断を委ねる
  -> Ending: 駅伝優勝エンド (love-end-love-24-0517-008-happy-002)
2: 陸上を再開する
  -> Ending: TRUE エンド (love-end-love-24-0517-008-happy-003)


# EPISODE 8-normal (adv_love_2405_08_02.txt)

choicegroup 1
0: 必ずまた会いに行くと約束する
  -> Ending: 結婚約束エンド (love-end-love-24-0517-008-normal-001)
1: 別れることを伝える
  -> Ending: お友達からよろしくエンド (love-end-love-24-0517-008-normal-002)
