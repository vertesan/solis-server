# Ending list

love-end-love-25-0426-005-succubus-rui-happy: ラブラブ大魔王END  
love-end-love-25-0426-005-succubus-rui-bad: 悪魔召喚END  
love-end-love-25-0426-005-succubus-rui-harem-happy: ドキドキ取り合いEND  
love-end-love-25-0426-005-succubus-rui-harem-bad-01: 一人ぼっちの日常END  
love-end-love-25-0426-005-succubus-rui-harem-bad-02: ボンバーEND  
love-end-love-25-0426-005-succubus-rui-harem-bad-03: ビンタ×４END  
love-end-love-25-0426-005-succubus-chs-happy: 人間さんと仲良しEND  
love-end-love-25-0426-005-succubus-chs-bad: 忘却END  
love-end-love-25-0426-005-magical-smr-happy: 解けない魔法END  
love-end-love-25-0426-005-magical-smr-bad: 新しい関係END  
love-end-love-25-0426-005-magical-ngs-happy: 魔法少女にゃぎさEND  
love-end-love-25-0426-005-magical-ngs-bad: 鈴の音END

# EPISODE 1 (adv_love_2505_01_01.txt)

choicegroup 1  
0: ありがとう  
  -> No effect  
1: おしるこはないの？  
  -> No effect

choicegroup 2  
0: います  
  -> No effect  
1: いません  
  -> No effect

choicegroup 3  
0: 違います  
  -> No effect  
1: なんのことですか？  
  -> No effect

choicegroup 4  
0: ルイ  
  -> Rui Pattern A condition 1  
1: チサ  
  -> Chisa good end prerequisite  
2: どちらも選べない  
  -> Rui Pattern B condition 1

choicegroup 5  
0: すみれだよね？  
  -> No effect  
1: ヴィオレットって？  
  -> No effect

[After all branches converge, to EPISODE 2]

# EPISODE 2 (adv_love_2505_02_01.txt)

choicegroup 1  
0: すみれ達のことだよね？  
  -> No effect  
1: ヴィオレットのことだよね？  
  -> No effect

choicegroup 2  
0: 分かった  
  -> No effect  
1: そんなに悪い子だった？  
  -> No effect

choicegroup 3  
0: すみれは凄いんだね  
  -> No effect  
1: やられた子はどうなるの？  
  -> No effect

choicegroup 4  
0: 怖い  
  -> Rui Pattern A condition 2  
1: すみれが心配  
  -> Rui Pattern B condition 2  
2: 別に怖くない  
  -> Neither Rui pattern condition satisfied

choicegroup 5  
0: すみれ達と帰る  
  -> To EPISODE 3-magical  
1: ルイ達のことが知りたい  
  -> To EPISODE 3-succubus

# EPISODE 3-succubus (adv_love_2505_03_01.txt)

choicegroup 1  
0: ルイを起こす  
  -> Rui Pattern B condition 3  
1: 様子を見る  
  -> Rui Pattern A condition 3

choicegroup 2  
0: おかげさまで  
  -> Rui Pattern B condition 4  
1: ルイが隣で寝てた  
  -> Rui Pattern A condition 4

choicegroup 3  
0: ルイが魅力的だから  
  -> Rui Pattern A condition 5  
1: チサが悪い子に見えなくて  
  -> Neither Rui pattern condition satisfied  
2: 二人のことが知りたいから  
  -> Rui Pattern B condition 5

choicegroup 4  
0: 分かった  
  -> No effect  
1: それは出来ない  
  -> No effect

choicegroup 5  
0: 綺麗だと思って  
  -> Rui Pattern A condition 6  
1: なんでもない  
  -> Neither Rui pattern condition satisfied  
2: すみれのことが気になって  
  -> Rui Pattern B condition 6

choicegroup 6  
0: ルイ  
  -> To EPISODE 4-succubus-rui  
1: チサ  
  -> To EPISODE 4-succubus-chs

# EPISODE 3-magical (adv_love_2505_03_02.txt)

choicegroup 1  
0: 監禁される  
  -> No effect  
1: 遊びに行く  
  -> No effect

choicegroup 2  
0: 買ってあげる  
  -> No effect  
1: 買わない  
  -> No effect

choicegroup 3  
0: 買ってあげる  
  -> No effect  
1: 買わない  
  -> No effect

choicegroup 4  
0: 渚のお兄ちゃんにもなる  
  -> Sumire good end prerequisite not satisfied  
1: すみれだけのお兄ちゃんでいる  
  -> Sumire good end prerequisite

choicegroup 5  
0: 渚  
  -> To EPISODE 4-magical-ngs  
1: すみれ  
  -> To EPISODE 4-magical-smr

choicegroup 6 (Sumire branch)  
0: 手を握る  
  -> No effect  
1: 頭を撫でる  
  -> No effect

# EPISODE 4-succubus-rui (adv_love_2505_04_01.txt)

choicegroup 1  
0: 興味がある  
  -> No effect  
1: 興味ない  
  -> No effect

choicegroup 2  
0: 手を貸すよ  
  -> No effect  
1: 頑張って  
  -> No effect

choicegroup 3  
0: 分かった  
  -> Rui Pattern A condition 7  
1: それは出来ない  
  -> Neither Rui pattern condition satisfied  
2: 考えさせて  
  -> Rui Pattern B condition 7

choicegroup 4  
0: ルイが好きだから  
  -> Rui Pattern A condition 8  
1: 争いを止めたいから  
  -> Rui Pattern B condition 8

choicegroup 5  
0: ずっとルイのそばにいる  
  -> If all Rui Pattern A conditions satisfied: To EPISODE 5-succubus-rui-good  
  -> If all Rui Pattern B conditions satisfied: To EPISODE 5-succubus-rui-bad

1: お互いの話を聞く  
  -> If all Rui Pattern A conditions satisfied: To EPISODE 5-harem-bad  
  -> If all Rui Pattern B conditions satisfied: To EPISODE 5-harem-happy

[At the end of EPISODE 4-succubus-rui]

Rui Pattern A requires all eight Pattern A choices from EPISODES 1, 2, 3-succubus, and 4-succubus-rui.

Rui Pattern B requires all eight Pattern B choices from the same episodes.

Mixed patterns or unsatisfied conditions:
  -> To EPISODE 5-succubus-rui-bad, then 悪魔召喚END (love-end-love-25-0426-005-succubus-rui-bad).

# EPISODE 4-succubus-chs (adv_love_2505_04_02.txt)

choicegroup 1  
0: 似てた  
  -> No effect  
1: 似てない  
  -> No effect

choicegroup 2  
0: してみたい  
  -> No effect  
1: 落ちたりしない？  
  -> No effect

choicegroup 3  
0: チサが好きだから  
  -> No effect  
1: 放っておけないから  
  -> No effect

choicegroup 4  
0: 立派なサキュバスだ  
  -> No effect  
1: 人間だと思う  
  -> No effect  
2: チサはチサだ  
  -> No effect

choicegroup 5  
0: 戦おう  
  -> To EPISODE 5-succubus-chs-bad

1: 自分の心に従おう  
  -> If EPISODE 1 choicegroup 4 was "チサ": To EPISODE 5-succubus-chs-good  
  -> Otherwise: To EPISODE 5-succubus-chs-bad

# EPISODE 4-magical-smr (adv_love_2505_04_03.txt)

choicegroup 1  
0: 修行について行く  
  -> No effect  
1: 家で待っている  
  -> No effect

choicegroup 2  
0: すみれ  
  -> No effect  
1: すみれお姉ちゃん  
  -> No effect

choicegroup 3  
0: ご飯粒を取ってあげる  
  -> No effect  
1: ご飯粒のことを教えてあげる  
  -> No effect

choicegroup 4  
0: 人見知りしてたよね  
  -> No effect  
1: あまり会話がなかったよね  
  -> No effect

choicegroup 5  
0: 魔法少女になったのはいつ？  
  -> No effect  
1: どうやって魔法少女になったの？  
  -> No effect

choicegroup 6  
0: 家にいた方がいい  
  -> No effect  
1: 迎えに行った方がいい  
  -> No effect

choicegroup 7  
0: 絶対にダメだ  
  -> No effect  
1: それを使えば倒せるの？  
  -> No effect

choicegroup 8  
0: 行く  
  -> No effect  
1: 行かない  
  -> No effect

choicegroup 9  
0: すみれに究極魔法を教えない  
  -> If EPISODE 3-magical choicegroup 4 was "すみれだけのお兄ちゃんでいる": To EPISODE 5-magical-smr-good  
  -> Otherwise: To EPISODE 5-magical-smr-bad

1: すみれに究極魔法を教える  
  -> To EPISODE 5-magical-smr-bad

# EPISODE 4-magical-ngs (adv_love_2505_04_04.txt)

choicegroup 1  
0: 撫でる  
  -> No effect  
1: どかす  
  -> No effect

choicegroup 2  
0: 早く逃げて  
  -> No effect  
1: 一緒に逃げよう  
  -> No effect

choicegroup 3  
0: 買ってあげる  
  -> No effect  
1: 我慢させる  
  -> No effect

choicegroup 4  
0: 渚の飼い主だから  
  -> No effect  
1: 渚は家族だから  
  -> No effect

choicegroup 5  
0: 渚とお別れする  
  -> To EPISODE 5-magical-ngs-bad  
1: 絶対に認めない  
  -> To EPISODE 5-magical-ngs-good

# EPISODE 5-succubus-rui-good (adv_love_2505_05_01.txt)

No choicegroups.

-> Ending: ラブラブ大魔王END (love-end-love-25-0426-005-succubus-rui-happy)

# EPISODE 5-succubus-rui-bad (adv_love_2505_05_02.txt)

No choicegroups.

-> Ending: 悪魔召喚END (love-end-love-25-0426-005-succubus-rui-bad)

# EPISODE 5-succubus-chs-good (adv_love_2505_05_03.txt)

No choicegroups.

-> Ending: 人間さんと仲良しEND (love-end-love-25-0426-005-succubus-chs-happy)

# EPISODE 5-succubus-chs-bad (adv_love_2505_05_04.txt)

No choicegroups.

-> Ending: 忘却END (love-end-love-25-0426-005-succubus-chs-bad)

# EPISODE 5-magical-smr-good (adv_love_2505_05_05.txt)

choicegroup 1  
0: すみれのお兄ちゃんだから  
  -> Ending: 解けない魔法END (love-end-love-25-0426-005-magical-smr-happy)  
1: 妹を守るのは当たり前  
  -> Ending: 解けない魔法END (love-end-love-25-0426-005-magical-smr-happy)

# EPISODE 5-magical-smr-bad (adv_love_2505_05_06.txt)

No choicegroups.

-> Ending: 新しい関係END (love-end-love-25-0426-005-magical-smr-bad)

# EPISODE 5-magical-ngs-good (adv_love_2505_05_07.txt)

choicegroup 1  
0: 耳を澄ませる  
  -> No effect  
1: 周りを見回す  
  -> No effect

choicegroup 2  
0: 一緒に帰ろう  
  -> Ending: 魔法少女にゃぎさEND (love-end-love-25-0426-005-magical-ngs-happy)  
1: そばにいてほしい  
  -> Ending: 魔法少女にゃぎさEND (love-end-love-25-0426-005-magical-ngs-happy)

# EPISODE 5-magical-ngs-bad (adv_love_2505_05_08.txt)

No choicegroups.

-> Ending: 鈴の音END (love-end-love-25-0426-005-magical-ngs-bad)

# EPISODE 5-harem-happy (adv_love_2505_05_09.txt)

No choicegroups.

-> Ending: ドキドキ取り合いEND (love-end-love-25-0426-005-succubus-rui-harem-happy)

# EPISODE 5-harem-bad (adv_love_2505_05_10.txt)

choicegroup 1  
0: 魔法少女を選ぶ  
  -> Ending: 一人ぼっちの日常END (love-end-love-25-0426-005-succubus-rui-harem-bad-01)  
1: サキュバスを選ぶ  
  -> Ending: ボンバーEND (love-end-love-25-0426-005-succubus-rui-harem-bad-02)  
2: やっぱりどちらも選べない  
  -> Ending: ビンタ×４END (love-end-love-25-0426-005-succubus-rui-harem-bad-03)


# Implementation note
The two Rui patterns must match all eight respective choices exactly; they are not scores or flags that can be mixed. Also, the early part transitions occur after completing the relevant episode script, not necessarily immediately after selecting the route-determining option.
