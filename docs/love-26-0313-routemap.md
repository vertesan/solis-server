# Ending list

love-end-love-26-0313-001-bad: コドクボッチEND
love-end-love-26-0313-008-happy-001: TRUEEND
love-end-love-26-0313-008-happy-002: 君の専属マネージャーEND
love-end-love-26-0313-008-normal-001: 半額弁当END
love-end-love-26-0313-008-normal-002: もしも瑠依と恋をしたらEND
love-end-love-26-0313-008-normal-003: 時を刻む恋慕END
love-end-love-26-0313-008-bad-001: コネコネEND
love-end-love-26-0313-008-bad-002: 借金END
love-end-love-26-0313-008-bad-003: 瑠依のヒモEND


# EPISODE 1 (adv_love_2603_01_01.txt)

choicegroup 1
0: 真面目に自己紹介する
  -> No 莉央 negative point
1: ウケを狙う
  -> 莉央 negative point +2

choicegroup 2
0: 俺は仕事でミスをしたのか！？
  -> No effect
1: 配属初日から残業させる気か！！
  -> No effect

choicegroup 3
0: 飲みに行く
  -> To EPISODE 2
1: 誘いをきっぱり断る
  -> Ending: コドクボッチEND (love-end-love-26-0313-001-bad)


# EPISODE 2 (adv_love_2603_02_01.txt)

choicegroup 1
0: わざとミスなんてダメだ
  -> No 莉央 negative point
1: よし、わざとミスしよう
  -> 莉央 negative point +1

choicegroup 2
0: やります！
  -> No 莉央 negative point
1: 自信ないなぁ……
  -> 莉央 negative point +1

choicegroup 3
0: 嘘をついて定時退社する
  -> No effect
1: 真面目に残業する
  -> No effect

[After all branches converge, to EPISODE 3]


# EPISODE 3 (adv_love_2603_03_01.txt)

choicegroup 1
0: うっそぴょーん
  -> 莉央 negative point +2
1: 本気です。俺は莉央さんが好きです
  -> No 莉央 negative point

choicegroup 2
0: 瑠依に頼む
  -> 莉央 negative point +1
1: 瑠依に頼まない
  -> No 莉央 negative point

[After all branches converge, to EPISODE 4]


# EPISODE 4 (adv_love_2603_04_01.txt)

choicegroup 1
0: 仕事のことを考えていました
  -> 莉央 negative point +1
1: 莉央さんのことを考えていました
  -> No 莉央 negative point

choicegroup 2
0: 嫌じゃありません！
  -> No effect
1: 嬉しいけど恥ずかしいです……
  -> No effect

[After all branches converge, to EPISODE 5]


# EPISODE 5 (adv_love_2603_05_01.txt)

choicegroup 1
0: 今ならサボってもバレないぞ
  -> 莉央 negative point +3
1: 今なら莉央さんに悪戯してもバレないかな？
  -> No 莉央 negative point

choicegroup 2
0: 莉央に電話を掛ける
  -> No effect
1: 電話を我慢する
  -> No effect

choicegroup 3
0: 俺も徹夜で仕事を手伝います！
  -> No effect
1: 俺はベッドでふて寝します！
  -> No effect

[After all branches converge, to EPISODE 6]


# EPISODE 6 (adv_love_2603_06_01.txt)

choicegroup 1
0: ご飯
  -> No effect
1: ライス
  -> No effect
2: おこめ
  -> No effect

choicegroup 2
0: 仕事なら仕方ない
  -> No 莉央 negative point
1: 嫌だ、デートしたい
  -> 莉央 negative point +1

choicegroup 3
0: 瑠依に相談しない
  -> 莉央 negative point +1
1: 瑠依に相談する
  -> To choicegroup 4

choicegroup 4
0: 予約したレストランに瑠依と行く
  -> 莉央 negative point +1
1: やっぱり瑠依に相談するのはやめる
  -> No 莉央 negative point

[After all branches converge, to EPISODE 7]


# EPISODE 7 (adv_love_2603_07_01.txt)

choicegroup 1
0: 他に好きな人はいないと断言する
  -> No 莉央 negative point
1: なぜそう思うのか尋ねる
  -> 莉央 negative point +2

choicegroup 2
0: 分かりました。別れましょう
  -> No 莉央 negative point
1: 嫌だ。絶対に別れません
  -> 莉央 negative point +3

[At the end of EPISODE 7]

Calculate the total 莉央 negative points.

If 莉央 negative points are 0-1:
  -> To EPISODE 8-happy

If 莉央 negative points are 2-10:
  -> To EPISODE 8-normal

If 莉央 negative points are 11 or more:
  -> To EPISODE 8-bad


# EPISODE 8-happy (adv_love_2603_08_01.txt)

choicegroup 1
0: 莉央にプロポーズする
  -> To choicegroup 2
1: 仕事のパートナーとして莉央について行く
  -> Ending: 君の専属マネージャーEND (love-end-love-26-0313-008-happy-002)

choicegroup 2
0: ご飯
  -> Ending: TRUEEND (love-end-love-26-0313-008-happy-001)
1: お風呂
  -> Ending: TRUEEND (love-end-love-26-0313-008-happy-001)


# EPISODE 8-normal (adv_love_2603_08_02.txt)

choicegroup 1
0: 一人で帰る
  -> Ending: 半額弁当END (love-end-love-26-0313-008-normal-001)
1: 瑠依と帰る
  -> To choicegroup 2

choicegroup 2
0: 莉央に声を掛ける
  -> Ending: もしも瑠依と恋をしたらEND (love-end-love-26-0313-008-normal-002)
1: 莉央を無視して帰る
  -> Ending: 時を刻む恋慕END (love-end-love-26-0313-008-normal-003)


# EPISODE 8-bad (adv_love_2603_08_03.txt)

choicegroup 1
0: 莉央にコネ採用を頼む
  -> Ending: コネコネEND (love-end-love-26-0313-008-bad-001)
1: 莉央にコネ採用を頼まない
  -> To choicegroup 2

choicegroup 2
0: 金を貸してくれ！
  -> Ending: 借金END (love-end-love-26-0313-008-bad-002)
1: 俺と結婚してくれ！
  -> Ending: 瑠依のヒモEND (love-end-love-26-0313-008-bad-003)
