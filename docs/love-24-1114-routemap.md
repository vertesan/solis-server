# Ending list

love-end-love-24-1114-003-bad: スズキスズキスズキEND
love-end-love-24-1114-008-happy-001: TRUEEND
love-end-love-24-1114-008-happy-002: 永遠に君の味方END
love-end-love-24-1114-008-happy-003: 解釈違いEND
love-end-love-24-1114-008-normal-001: 秘密の恋愛END
love-end-love-24-1114-008-normal-002: かけがえのない友達END
love-end-love-24-1114-008-normal-003: 生涯独身END
love-end-love-24-1114-008-bad-001: コワイコワイEND
love-end-love-24-1114-008-bad-002: 泡沫の夢END


# EPISODE 1 (adv_love_2411_01_01.txt)

choicegroup 1
0: 俺も嬉しい
  -> No effect
1: 特に何も言わない
  -> No effect

choicegroup 2
0: そういうつもりじゃない
  -> No effect
1: しょぼぼんって……
  -> No effect

[After all branches converge, to EPISODE 2]


# EPISODE 2 (adv_love_2411_02_01.txt)

choicegroup 1
0: 肯定する
  -> 優 point +1
1: 言い淀む
  -> No 優 point

choicegroup 2
0: 秘密にすると答える
  -> No 優 point
1: 交換条件を提示する
  -> 優 point +1

[After all branches converge, to EPISODE 3]


# EPISODE 3 (adv_love_2411_03_01.txt)

choicegroup 1
0: 言われたとおりにしてみる
  -> Happy end flag 1
1: やらない
  -> Happy end flag 1 not satisfied

choicegroup 2
0: 抱き寄せて優の顔を隠す
  -> Happy end flag 2
1: 呪文を唱える
  -> Happy end flag 2 not satisfied

[After all branches converge, to EPISODE 4]


# EPISODE 4 (adv_love_2411_04_01.txt)

choicegroup 1
0: 名前で呼んでみる
  -> Happy end flag 3
1: 名前で呼ばない
  -> Happy end flag 3 not satisfied

choicegroup 2
0: 告白を我慢する
  -> Happy end flag 4
  -> To EPISODE 5
1: 思い切って告白する
  -> Ending: スズキスズキスズキEND (love-end-love-24-1114-003-bad)


# EPISODE 5 (adv_love_2411_05_01.txt)

choicegroup 1
0: してみてもいいか尋ねる
  -> 優 point +3
1: しないと答える
  -> No 優 point

choicegroup 2
0: 試しに肯定してみる
  -> Happy end flag 5
1: 慌てて否定する
  -> Happy end flag 5 not satisfied

[After all branches converge, to EPISODE 6]


# EPISODE 6 (adv_love_2411_06_01.txt)

choicegroup 1
0: 待ってないと嘘をつく
  -> Happy end flag 6
1: 待ったと正直に言う
  -> Happy end flag 6 not satisfied

choicegroup 2
0: 優と呼ぶ
  -> Happy end flag 7
1: 優先輩と呼ぶ
  -> Happy end flag 7 not satisfied

[After all branches converge, to EPISODE 7]


# EPISODE 7 (adv_love_2411_07_01.txt)

choicegroup 1
0: 電話に出ない
  -> To EPISODE 8-bad
1: 電話に出る
  -> Happy end flag 8
  -> Continue

choicegroup 2
0: 地元のカフェに向かう
  -> No effect
1: 所属事務所に向かう
  -> No effect

choicegroup 3
0: これからだ、と答える
  -> Happy end flag 9
  -> No 優 point
1: めでたしめでたし……のつもりだった
  -> Happy end flag 9 not satisfied
  -> 優 point +2

[At the end of EPISODE 7]

If choicegroup 1 was "電話に出ない":
  -> To EPISODE 8-bad

Otherwise, calculate 優 points.

If 優 points are 4 or more:
  -> To EPISODE 8-bad

Otherwise, if all Happy end flags 1-9 are satisfied:
  -> To EPISODE 8-happy

Otherwise:
  -> To EPISODE 8-normal


# EPISODE 8-bad (adv_love_2411_08_03.txt)

choicegroup 1
0: 電話に出る
  -> Ending: コワイコワイEND (love-end-love-24-1114-008-bad-001)
1: 電話に出ない
  -> Ending: 泡沫の夢END (love-end-love-24-1114-008-bad-002)


# EPISODE 8-happy (adv_love_2411_08_01.txt)

choicegroup 1
0: 俺との結婚
  -> Ending: TRUEEND (love-end-love-24-1114-008-happy-001)
1: 役者として、海外進出！
  -> Ending: 永遠に君の味方END (love-end-love-24-1114-008-happy-002)
2: 劇場アニメのゲスト声優出演
  -> Ending: 解釈違いEND (love-end-love-24-1114-008-happy-003)


# EPISODE 8-normal (adv_love_2411_08_02.txt)

choicegroup 1
0: このまま隠し通す
  -> Ending: 秘密の恋愛END (love-end-love-24-1114-008-normal-001)
1: 今が別れ時なのかも
  -> Ending: かけがえのない友達END (love-end-love-24-1114-008-normal-002)
2: 優に委ねる
  -> Ending: 生涯独身END (love-end-love-24-1114-008-normal-003)
