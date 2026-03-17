package idlechat

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	storyChunkMaxRunes = 90
	storyChunkMinRunes = 28
	storyStageMaxRetries = 3
	storySourceMaxAttempts = 3
)

type StorySource struct {
	ID           string
	Title        string
	SourceLabel  string
	Kind         string
	Language     string
	PublicDomain bool
	Text         string
}

type StoryRewritePlan struct {
	SourceTitle  string
	RewriteStyle string
	StoryTitle   string
	Premise      string
	Setting      string
	Viewpoint    string
	Tone         string
	Hook         string
	EndingShape  string
	EndingFlavor string
	CoreMotifs   []string
	MotifMap     []string
}

type StoryBeat struct {
	ID                string
	Label             string
	Canonical         []string
	Required          bool
	AllowedSubstitute []string
}

type StorySkeleton struct {
	ID                  string
	SourceTitle         string
	CanonicalMotifs     []string
	RequiredBeats       []StoryBeat
	RoleConstraints     []string
	TabooOrRule         string
	RewardPunishment    string
	EmotionalAftertaste string
	RecognitionCues     []string
}

type StorySourceAnalysis struct {
	CoreMotifs          []string
	RoleMap             []string
	TabooOrRule         string
	RewardAndPunish     string
	EmotionalAftertaste string
	Skeleton            StorySkeleton
}

type StoryBeatPlan struct {
	Opening   string
	Deviation string
	Reversal  string
	Landing   string
}

type StoryTwist struct {
	Style           string
	VisibleTwist    string
	Setting         string
	Viewpoint       string
	ImmediateChange string
	ConflictShift   string
	EndingShape     string
	StoryTitle      string
	Hook            string
	Tone            string
	EndingFlavor    string
}

type StoryAdaptationPlan struct {
	SkeletonID      string
	RewriteStyle    string
	BeatMappings    []string
	MotifMappings   []string
	RoleRemap       []string
	EndingFlavor    string
	RecognitionCues []string
}

var storyRewriteStyles = []string{"role_shift", "view_shift", "value_shift"}
var storyRandIntn = rand.Intn

var storyCorpus = []StorySource{
	{ID: "momotaro", Title: "桃太郎", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("momotaro")},
	{ID: "urashima", Title: "浦島太郎", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("urashima")},
	{ID: "kaguya", Title: "竹取物語", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("kaguya")},
	{ID: "issun", Title: "一寸法師", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("issun")},
	{ID: "hanasaka", Title: "花咲かじいさん", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("hanasaka")},
	{ID: "shitakiri", Title: "舌切り雀", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("shitakiri")},
	{ID: "kasajizo", Title: "笠地蔵", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("kasajizo")},
	{ID: "kintaro", Title: "金太郎", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("kintaro")},
	{ID: "sarukani", Title: "さるかに合戦", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("sarukani")},
	{ID: "tsuru", Title: "鶴の恩返し", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("tsuru")},
	{ID: "kobutori", Title: "こぶとりじいさん", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("kobutori")},
	{ID: "bunbuku", Title: "ぶんぶく茶釜", SourceLabel: "日本昔話", Kind: "folktale", Language: "ja", PublicDomain: true, Text: storySourceText("bunbuku")},
	{ID: "redriding", Title: "赤ずきん", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("redriding")},
	{ID: "cinderella", Title: "シンデレラ", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("cinderella")},
	{ID: "snowwhite", Title: "白雪姫", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("snowwhite")},
	{ID: "hansel", Title: "ヘンゼルとグレーテル", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("hansel")},
	{ID: "bremen", Title: "ブレーメンの音楽隊", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("bremen")},
	{ID: "puss", Title: "長靴をはいた猫", SourceLabel: "ペロー童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("puss")},
	{ID: "threepigs", Title: "三匹の子ぶた", SourceLabel: "イギリス童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("threepigs")},
	{ID: "beauty", Title: "美女と野獣", SourceLabel: "童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("beauty")},
	{ID: "aladdin", Title: "アラジンと魔法のランプ", SourceLabel: "アラビアンナイト", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("aladdin")},
	{ID: "ali40", Title: "アリババと四十人の盗賊", SourceLabel: "アラビアンナイト", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("ali40")},
	{ID: "emperors", Title: "裸の王様", SourceLabel: "アンデルセン童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("emperors")},
	{ID: "matchgirl", Title: "マッチ売りの少女", SourceLabel: "アンデルセン童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("matchgirl")},
	{ID: "littlemermaid", Title: "人魚姫", SourceLabel: "アンデルセン童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("littlemermaid")},
	{ID: "sleepingbeauty", Title: "眠れる森の美女", SourceLabel: "童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("sleepingbeauty")},
	{ID: "frogprince", Title: "かえるの王さま", SourceLabel: "グリム童話", Kind: "fairytale", Language: "ja", PublicDomain: true, Text: storySourceText("frogprince")},
}

func storySourceText(id string) string {
	switch id {
	case "momotaro":
		return "昔々、子どものいない老夫婦が川で流れてきた大きな桃を拾い、家へ持ち帰りました。桃を割ると中から元気な男の子が現れ、夫婦はその子を桃太郎と名づけて大切に育てます。桃太郎は力強くまっすぐに育ち、村人たちが鬼ヶ島の鬼に苦しめられ、米や宝を奪われていることを知りました。鬼を退治して村を救うと決めた桃太郎は、おばあさんに作ってもらったきびだんごを持って旅に出ます。道中で出会った犬、猿、雉はきびだんごを受け取り、桃太郎の家来となって鬼ヶ島行きに加わりました。鬼ヶ島では鬼たちが酒盛りをして油断しており、桃太郎たちは力だけでなく役割分担と知恵も使って城へ攻め込みます。桃太郎たちに打ち負かされた鬼たちは降参し、これまで村々から奪った宝や米俵を返しました。桃太郎は仲間たちとともに宝を持ち帰り、老夫婦と村人に迎えられ、村には再び平和な暮らしが戻りました。"
	case "urashima":
		return "昔々、浦島太郎という心優しい若者が海辺を歩いていると、子どもたちにいじめられている亀を見つけました。太郎は亀を助けて海へ返し、そのことはすぐに忘れたつもりで普段の暮らしへ戻ります。ところが数日後、助けた亀が再び現れ、礼をしたいと言って太郎を背に乗せて海の底の竜宮城へ案内しました。竜宮城では乙姫が太郎を温かく迎え、珍しい舞や美しい景色、豪華な食事でもてなし、太郎は夢の中のような時間を過ごします。けれど楽しい日々が続くほど、太郎の胸には故郷の浜辺や家族のことが浮かび、やがて帰りたいと願うようになりました。乙姫は別れを惜しみながら玉手箱を渡し、どんなことがあっても決して開けてはいけないと固く言い含めます。太郎が海辺へ戻ると、見慣れたはずの村は姿を変え、知っている人も家も見当たらず、長い歳月が流れたことを思い知らされました。驚きと不安に耐えきれなくなった太郎が玉手箱を開けると白い煙が立ちのぼり、その煙を浴びた太郎は一気に年老いてしまいました。"
	case "kaguya":
		return "竹取の翁が山で竹を切っていると、一本の竹の中が不思議な光で満ちているのを見つけました。竹を割ると中には手のひらに乗るほど小さな美しい姫がいて、翁はその子を家へ連れて帰り、おばあさんとともに娘として育てます。姫はかぐや姫と名づけられ、月日のうちに目を見張るほどの美しさを持つ娘へ成長しました。噂を聞きつけた多くの貴公子が求婚に訪れますが、かぐや姫は蓬莱の玉の枝や火鼠の皮衣など、誰にも本当に持ち帰れない難題を出して相手の心の浅さや偽りを見抜きます。帝さえも姫に心を寄せますが、姫はどの求婚も受け入れず、地上に長くは留まれない運命を胸に秘めていました。やがて満月の夜が近づくと、かぐや姫は自分が月の都の者であり、迎えが来れば必ず帰らねばならないと翁たちへ打ち明けます。翁や帝は兵を置いて守ろうとしますが、月から来た使者たちの前では人の力は及ばず、かぐや姫を引き留めることはできませんでした。姫は地上で暮らした日々への情を涙ながらに残し、別れの文と不死の薬を託して月へ帰っていきました。"
	case "issun":
		return "子どものいない夫婦のもとに授かったのは、一寸ほどの小さな男の子でした。夫婦はその子を大切に育て、一寸法師は小さい体でも立派な働きをしたいと願うようになります。やがて一寸法師は都へ出ることを決め、針を刀に、椀を舟に、箸を櫂にして川を下って旅立ちました。都に着いた一寸法師はある屋敷で働き者として認められ、姫のそばで身の回りの世話をする役目を与えられます。ある日、姫が外出した帰り道で鬼が現れて姫をさらおうとし、一寸法師は小さな体を武器に鬼へ立ち向かいました。一寸法師は鬼に飲み込まれてもあきらめず、腹の中や口の中で針の刀を振るって暴れ、鬼を苦しめて退散させます。鬼が逃げるときに落としていった打ち出の小槌を手に入れた一寸法師は、その力でついに普通の大きさの立派な若者になりました。小さな体で姫を守り抜いた勇気と機転が認められ、一寸法師は幸せな将来を約束されます。"
	case "hanasaka":
		return "心優しい老夫婦は、犬のポチを本当の家族のようにかわいがって慎ましく暮らしていました。ある日、ポチが庭の一角をしきりに掘るので、おじいさんがそこを掘ってみると、土の中からたくさんの宝が出てきます。その話を聞いた隣の欲深い夫婦は、同じように宝を得ようとポチを借りて無理やり掘らせました。けれどポチが示した場所から出てきたのはごみや汚れたものばかりで、腹を立てた欲深い夫婦はポチをひどい目に遭わせ、ついには死なせてしまいます。老夫婦は深く悲しみ、せめて供養したいとポチを埋めて木を植え、その木で臼を作りました。するとその臼からも宝がこぼれ落ちますが、それを知った隣夫婦が臼を奪うと、今度は汚いものしか出ず、怒って臼を焼き捨ててしまいました。老夫婦は焼け跡の灰を持ち帰り、枯れ木にまいてみると不思議にも花が咲き、春でもないのに見事な景色が広がります。その噂を聞いた殿様も喜び、老夫婦は善い心の報いを受け、欲深い夫婦は最後まで恥をさらすことになりました。"
	case "shitakiri":
		return "優しいおじいさんは、家に来る雀をかわいがり、毎日えさをやって大切にしていました。ところが欲深く気の荒いおばあさんは、その雀が用意していた糊を食べてしまったことに腹を立て、雀の舌を切って追い出してしまいます。帰宅したおじいさんは雀がいなくなった理由を知って悲しみ、山や野を歩き回ってようやく雀の家を探し当てました。雀たちはおじいさんを恨むどころか温かく迎え、食事や踊りでもてなし、別れに大小二つのつづらを差し出します。おじいさんは欲張らず、小さいつづらだけを持ち帰ると、その中には宝や美しい品々が入っていました。これを見たおばあさんは自分も宝を手に入れようと雀の家へ押しかけ、礼も節度もなく大きいつづらを持ち帰ります。我慢できず途中でつづらを開けると、中から恐ろしい化け物や妖怪が飛び出し、おばあさんは逃げ帰るしかありませんでした。こうして雀への優しさと欲深さの違いがはっきり現れ、おじいさんは静かな幸福を得て、おばあさんは自分の行いの報いを受けました。"
	case "kasajizo":
		return "年の暮れ、貧しい老夫婦は正月の支度もできないほど苦しい暮らしをしていました。おじいさんは少しでも米を買おうと、丹精して編んだ笠を町へ売りに行きますが、寒さの厳しい日で一つも売れません。肩を落として雪道を帰る途中、おじいさんは雪をかぶって寒そうに立つ六体のお地蔵さまを見つけました。気の毒に思ったおじいさんは売れ残った笠を一つずつ地蔵にかぶせ、笠が足りない一体には自分の手ぬぐいを巻いてあげます。何の見返りも求めず家へ戻ると、おばあさんもその話を聞いて、おじいさんらしいことをしたと静かにうなずきました。夜が更けるころ、戸口の外から大きな音がして、夫婦が外へ出るとたくさんの米俵や薪、魚や餅が積み上げられています。それはお礼に訪れた地蔵たちが運んできた贈り物でした。老夫婦は思いがけず豊かな正月を迎え、人に見えないところで行った親切が確かな形で返ってきたことを喜びます。"
	case "kintaro":
		return "足柄山の奥で育った金太郎は、赤い顔と並外れた力を持つ元気な子どもでした。母と二人で山に住みながら、熊や鹿、猿、兎たちと友だちになり、毎日のように力比べや相撲をして遊びます。大木をまたいで橋にしたり、大きな岩を動かしたりする怪力は、山の動物たちさえ驚くほどでしたが、金太郎は乱暴ではなく、弱い者を助ける優しさも持っていました。ある日、山へ入った武士がそんな金太郎の噂を聞きつけ、実際にその力と度胸を見て強く感心します。武士は金太郎に、都へ出て立派な武士になる道があると語り、母にもその将来を託してほしいと願いました。母は別れを惜しみながらも、山の中だけに留めておくより広い世界で力を役立ててほしいと考え、金太郎を送り出します。こうして金太郎は都へ向かい、武士として修行を重ね、やがて坂田金時として名を知られる存在になりました。山で培った豪胆さ、仲間を思う気持ち、体の強さは、その後も彼の土台として生き続けます。"
	case "sarukani":
		return "ある日、蟹はおいしそうな握り飯を持っていましたが、猿は口先だけでうまく言いくるめ、その握り飯とまだ食べられない柿の種を交換させてしまいます。蟹はだまされたと気づかず、もらった柿の種をまじめに植え、水をやり、長い時間をかけて大事に育てました。やがて木は大きく育ち、枝にはたくさんの柿が実りますが、木に登れない蟹は熟れた実を取れません。そこへ猿が現れ、自分が取ってやると言って木に登ると、よく熟れた実は自分で食べ、まだ固い青柿だけを蟹へ投げつけました。青柿は蟹の体に当たり、親蟹は深い傷を負って死んでしまいます。残された子蟹たちは栗、蜂、臼など親しい仲間に事情を話し、猿へ報いを与えるため力を合わせることにしました。仲間たちはそれぞれの得意な方法で猿を待ち伏せし、やけど、刺し傷、打撃を次々と与えて、猿はとうとう自分の悪事を思い知らされます。だましたこと、働きを横取りしたこと、弱い相手を傷つけたことの報いが、最後に猿へまとめて返ってきたのでした。"
	case "tsuru":
		return "雪の降る日、貧しい男は罠にかかった一羽の鶴を見つけ、かわいそうに思って助けてやりました。しばらくして男の家を一人の美しい娘が訪れ、行くあてもないので置いてほしいと頼みます。男と妻は娘を迎え入れ、三人で慎ましく暮らし始めますが、娘は家計を助けるため、自分が機を織るかわりに決して部屋をのぞかないでほしいと願いました。娘が織り上げた布は見事で高く売れ、夫婦の暮らしは次第に楽になります。けれど何度も布を織るうちに娘は弱っていき、夫婦は心配しながらも、どうしてそこまでやせ細るのか知りたくなりました。我慢できなくなった夫婦が約束を破って部屋をのぞくと、そこには助けたあの鶴が自分の羽を抜きながら布を織る姿がありました。正体を見られた鶴は約束が破られたことを悲しみ、これ以上ここにはいられないと言って空へ飛び去ります。夫婦のもとには美しい布と、守れなかった約束への後悔だけが残りました。"
	case "kobutori":
		return "顔に大きなこぶを持つおじいさんが山で木を切っていると、急な雨に降られて山小屋へ逃げ込みました。夜が深まるころ、外から太鼓や笛のようなにぎやかな音が聞こえ、小屋の外をのぞくと、鬼たちが火を囲んで宴を開いています。おじいさんは恐ろしく思いながらも、その踊りや歌に心を奪われ、しまいには自分も思わず輪の中へ出て陽気に踊り出しました。鬼たちはおじいさんの踊りを大いに気に入り、また来て踊ってくれと頼み、その約束のしるしとして顔のこぶを取ってしまいます。身軽になって帰ったおじいさんは大喜びし、その話はたちまち近所へ広まりました。それを聞いた隣の欲深いおじいさんは、自分もこぶを取ってもらおうと、同じ山小屋へ出かけて鬼の宴を待ちます。ところが無理に踊っても楽しさがなく、鬼たちを喜ばせるどころか怒らせてしまい、返されたこぶにもう一つ余計なこぶまでつけられて帰ることになりました。気楽に楽しんだ者と、欲だけでまねをした者との差が、はっきりした形で表れた話です。"
	case "bunbuku":
		return "貧しい男が一匹の狸を助けると、狸は恩返しとして自分が茶釜に化けるので寺へ売って金にしてほしいと申し出ました。男は半信半疑ながらも狸の言うとおりにし、寺へ茶釜を売ってわずかな金を得ます。けれど寺の和尚がその茶釜を火にかけると、熱さにたえられなくなった狸は手足や尻尾を出して大騒ぎし、そのまま寺を飛び出して逃げ帰ってきました。男はあきれながらも狸を責めず、二人で別の方法を考えることにします。そこで狸は半分茶釜、半分狸の姿のまま綱渡りや芸を見せる見世物になることを思いつきました。珍しい茶釜の芸はたちまち評判となり、人々が押し寄せ、男はようやく安定した暮らしを立てられるようになります。恩返しを終えた狸は再び自由に戻り、男もまたその茶釜を大切に祭って忘れませんでした。困っていた者が、助けた命と知恵によって救われる、少しおかしく温かい話です。"
	case "redriding":
		return "赤い頭巾をかぶった少女は、病気で寝ているおばあさんにお菓子と食べ物を届けるよう母に頼まれました。母は寄り道をせず、まっすぐ森の道を行くよう言い聞かせますが、少女は途中で出会った狼へ気軽に行き先を話してしまいます。狼は親切そうに花を摘んで行ったらどうかと勧め、少女が道草をくっているあいだに自分は先回りしておばあさんの家へ向かいました。狼は家にいたおばあさんを飲み込み、その衣服を着て寝床に入り、訪ねてくる赤ずきんを待ち構えます。やがて家へ来た少女は、耳も目も口もいつものおばあさんと違うことに不安を覚えながらも、とうとう狼に食べられてしまいました。そこへ様子のおかしさに気づいた狩人が現れ、狼の腹を裂いて、おばあさんと赤ずきんを助け出します。二人は命を取りとめ、赤ずきんはもう二度と見知らぬ相手に簡単に心を許さないと心に決めました。森の中の寄り道と油断が、命の危険につながることを強く印象づける話です。"
	case "cinderella":
		return "母を亡くした娘は、再婚した父の家で継母と二人の義姉に召使いのように扱われ、灰だらけの台所で働かされる毎日を送っていました。娘はシンデレラと呼ばれ、きれいな服も自由も持たず、家の中で一番低い場所に押し込められていました。やがて王子の花嫁選びの舞踏会が開かれると、義姉たちは着飾って城へ向かいますが、シンデレラだけは行くことを許されません。それでも不思議な助けによって美しい衣装と馬車を得たシンデレラは、夜更けまでという条件つきで舞踏会へ向かいました。王子は名も知らぬ美しい娘に心を奪われ、どの娘よりも彼女と踊りますが、シンデレラは正体が明かされる前に急いで立ち去ります。その際に落とした片方の靴だけが城に残り、王子はその靴に足が合う娘を国中探し始めました。継母たちは娘たちを無理に靴へ合わせようとしますが、最後に試したシンデレラの足にだけ靴はぴたりと合います。こうしてシンデレラは虐げられた暮らしから抜け出し、自分にふさわしい新しい人生を手に入れました。"
	case "snowwhite":
		return "雪のように白い肌を持つ姫は、その美しさのために継母である王妃から激しく妬まれていました。王妃は魔法の鏡に誰が一番美しいかを尋ね続け、自分より白雪姫が美しいと知ると、ついに家来へ命じて姫を森で殺させようとします。家来は哀れに思って姫を逃がし、白雪姫は深い森をさまよった末に七人の小人の家へたどり着きました。小人たちは姫を迎え入れますが、王妃は姫が生きていると知ると、櫛や紐や毒りんごを使って何度も命を狙います。最後に毒りんごを口にした白雪姫は深い眠りに落ち、小人たちは死んだと思ってガラスの棺に納め、大切に守り続けました。時がたち、通りかかった王子がその姿に心を奪われ、棺ごと城へ運ばせようとしたとき、喉に詰まっていたりんごのかけらが取れて白雪姫は目を覚まします。白雪姫は再び命を取り戻し、王子と結ばれました。美しさへの嫉妬が何度も殺意へ変わる一方で、最後には救いへ着地する話です。"
	case "hansel":
		return "貧しさに苦しむ家で、兄ヘンゼルと妹グレーテルは、もう子どもを養えないという大人たちの話を聞いてしまいます。兄妹は森へ置き去りにされる運命を知り、最初は白い小石を落として帰り道を覚え、家へ戻ることに成功しました。けれど二度目は小石が使えず、パンくずを落としても鳥に食べられてしまい、兄妹は本当に深い森で迷ってしまいます。飢えと不安の中でさまよう二人は、やがて壁も屋根もお菓子でできた家を見つけ、夢中で食べ始めました。しかしそこに住んでいた老婆は優しそうに見えて、実は子どもを太らせて食べる魔女で、兄を檻へ閉じ込め、妹をこき使い始めます。魔女が兄を焼こうとしたとき、妹はとっさに機転を利かせ、逆に魔女をかまどへ押し込めました。兄妹は家の中にあった宝石や宝を持って森を抜け、ようやく家へ帰ります。飢えと捨てられる不安、甘い誘惑、知恵による逆転がはっきり続く話です。"
	case "bremen":
		return "年を取って働けなくなったロバは、このままでは主人に見捨てられると悟り、ブレーメンで音楽隊になるつもりで旅に出ました。道中で同じように役に立たないとされた犬、猫、雄鶏に出会い、それぞれが行き場を失っていることを知ります。ロバは仲間になろうと誘い、四匹は歌や鳴き声を武器に新しい暮らしを探して一緒に進むことにしました。夜になって森の中で明かりのついた家を見つけると、そこは盗賊たちが酒盛りをしている家でした。四匹は窓の外でロバの上に犬、猫、雄鶏と重なり、一斉に鳴き声を上げて飛び込みます。盗賊たちは化け物が現れたと思って逃げ出し、戻ってきても、猫の爪や犬の牙、ロバの蹴り、雄鶏の声におびえて再び逃げ去りました。四匹はその家を自分たちの住みかに決め、結局ブレーメンへ行かなくても満足できる居場所を得ます。役立たずと見なされた者同士が力を合わせ、新しい共同生活を見つける話です。"
	case "puss":
		return "粉ひき屋の死後、末息子に残された遺産はたった一匹の猫だけでした。息子は絶望しますが、その猫は長靴と袋を用意してくれれば主人を立派な身分にしてみせると自信たっぷりに言います。猫は野うさぎや鳥を捕まえては、架空のカラバ侯爵からの贈り物だと王様へ届け、少しずつ主人の名を王宮へ売り込みました。さらに主人を川へ入らせておいて盗まれたふりをし、通りかかった王様に立派な衣服を与えさせ、侯爵らしい姿へと仕立てます。猫は先回りして農民たちにこの土地はすべてカラバ侯爵のものだと言わせ、主人が大地主であるかのような舞台を整えました。最後に城を持つ人食い鬼のところへ乗り込み、変身自慢を逆手に取って鬼を小さな鼠へ変えさせ、そのまま食べてしまいます。こうして主人は本当に城の主となり、王女とも結ばれました。知恵と演出で主人の運命をひっくり返す猫の働きが話の中心です。"
	case "threepigs":
		return "三匹の子ぶたは親元を離れ、それぞれ自分の家を建てて暮らすことになりました。早く遊びたい二匹は、簡単に作れる藁の家と木の家を選び、手間を惜しんで仕事を終わらせます。けれど末の一匹は時間がかかっても丈夫なれんがの家を建てるべきだと考え、黙々と働き続けました。そこへ狼が現れ、藁の家と木の家の前で中へ入れろと脅し、断られると息を吹きかけて簡単に吹き飛ばしてしまいます。逃げ出した二匹はれんがの家へ駆け込み、三匹はそこで身を寄せて狼を防ぐことになりました。狼は戸を破れないと知ると煙突から入ろうとしますが、家の中で待ち構えていた子ぶたは大鍋の湯を沸かしておきます。煙突を降りてきた狼は熱湯へ落ち、とうとう退散するか命を失うしかありませんでした。手間を惜しまない備えが、最後に命を守る話です。"
	case "beauty":
		return "商人の父は不運が重なって財産を失い、家族は豊かな暮らしから一転して苦しい生活へ落ちます。末娘の美女だけは不満を言わず父を助けますが、ある日父が帰り道に迷い込み、不思議な城で世話になった礼も分からぬまま、一本のばらを持ち帰ろうとして野獣を怒らせてしまいました。野獣は命を助けるかわりに娘の一人を城へよこせと言い、父を思う美女は自ら身代わりになる決意をします。城で暮らし始めると、野獣は恐ろしい外見に反して乱暴ではなく、毎晩食事の席で結婚してくれないかと尋ねるだけでした。美女は最初こそ恐れますが、野獣の誠実さ、孤独、思いやりを知るにつれて、その姿の奥にある心へ目を向けるようになります。やがて父を見舞うため一時帰宅した美女は、夢の中で野獣が悲しげに弱っていく様子を見て、自分が帰らなければならないと悟ります。急いで城へ戻った美女が、野獣を愛していると告げた瞬間、野獣にかけられていた呪いは解け、彼は本来の王子の姿へ戻りました。外見ではなく真心を見ることが救いにつながる話です。"
	case "aladdin":
		return "貧しい若者アラジンは遊び好きで、母と二人ぎりぎりの暮らしをしていました。そこへ遠い親類を名乗る魔法使いが現れ、アラジンをうまくだまして宝の眠る洞窟へ連れて行きます。魔法使いは洞窟の奥にある古びたランプを取ってこいと命じますが、先にランプを渡せと言う態度に不信を抱いたアラジンは従わず、洞窟へ閉じ込められてしまいました。絶望の中で指輪をこすったアラジンは精霊の力によって地上へ戻り、さらにランプをこするともっと強い魔人が現れて願いをかなえてくれることを知ります。アラジンはその力で富を得て、壮麗な宮殿を建て、美しい王女と結婚するところまでたどり着きました。けれど魔法使いは再び現れ、古いランプを新しいものと交換させる trick を使ってランプを奪い、宮殿ごと王女を遠くへ移してしまいます。アラジンは指輪の精霊の助けと自分の勇気で魔法使いのもとへ乗り込み、王女と協力してランプを取り返しました。だまされて落ちた若者が、魔法の力を使いこなしながら最後は自分の知恵で運命を奪い返す話です。"
	case "ali40":
		return "貧しい木こりのアリババは森で仕事をしている最中、馬に乗った盗賊たちが大きな岩の前に集まるのを偶然目撃しました。盗賊の頭領が『開けゴマ』と唱えると岩が開き、中には山のような宝が隠されていました。盗賊が去ったあと、アリババは恐る恐る同じ言葉を使って洞窟へ入り、必要な分だけ宝を持ち帰ります。その話を聞いた兄は欲を出して一人で洞窟へ向かいますが、合言葉を忘れて閉じ込められ、戻ってきた盗賊に殺されてしまいました。盗賊たちは宝を持ち去った者を探してアリババの家を突き止めようとし、扉に印をつけるなどして何度も襲撃の機会をうかがいます。しかし召使いのモルジアナはそのたびに印を見破り、油壺に隠れてきた盗賊を退け、頭領の変装も見抜きました。最後には宴の席で頭領まで討ち取り、アリババの家と秘密を守り抜きます。偶然得た宝そのものより、モルジアナの知恵と警戒心が一家を救う話です。"
	case "emperors":
		return "新しい服に目がない王様は、国の仕事より着飾ることに熱心で、自分を美しく見せるものなら何でも欲しがっていました。そこへ二人の詐欺師がやって来て、愚かな者や役に立たない者には見えない特別な布を織れると吹き込みます。王様はその言葉に飛びつき、大金を与えて布を織らせますが、もちろん機の上には何もありません。家来や大臣たちは布が見えないと言えば自分が愚かだと思われるのを恐れ、見えない布を見えるふりで褒めそやしました。王様自身も同じ恐れから真実を口にできず、ついには何も身につけないまま新しい服をまとった気になって行進へ出ます。沿道の民衆もまた、周囲に合わせて立派な服だと口にしますが、一人の子どもだけは恐れを知らず『王様は裸だ』と叫びました。その声はたちまち人々へ広がり、皆がようやく目の前の真実を認め始めます。王様は恥を感じながらも行進をやめられず、見栄と同調圧力の愚かさだけがその場に残りました。"
	case "matchgirl":
		return "大みそかの冷え切った夜、貧しい少女は裸足同然で雪の街を歩きながらマッチを売っていました。けれど誰も立ち止まらず、一箱も売れないまま時間だけが過ぎていきます。家へ帰れば父に叱られるのが怖く、少女は石造りの家の間の狭い路地へ身を寄せて寒さに震えました。少しでも暖を取りたくなった少女が一本マッチをすると、壁に映る火は大きな暖炉に見え、消えた瞬間にそのぬくもりも消えてしまいます。二本目ではごちそうの並ぶ食卓、三本目では輝くクリスマスツリーが現れ、どれも手を伸ばした途端に闇へ消えました。最後に見えたのは、誰よりも優しかった亡き祖母の姿で、少女はその幻を失いたくなくて残りのマッチをすべて燃やします。祖母は少女を抱きかかえるように連れていき、少女は凍える路地で静かに息を引き取りました。翌朝、人々は笑みを浮かべた少女の亡骸を見つけますが、彼女が最後にどれほど温かなものを見たかを知る者はいませんでした。"
	case "littlemermaid":
		return "海の底の王国に暮らす人魚姫は、人間の世界へ強いあこがれを抱き、十五歳になると海の上へ出られる日を待ち続けていました。ある嵐の夜、姫は難破した船から海へ投げ出された王子を見つけ、波の中から岸まで運んで命を救います。けれど王子が目を覚ます前に人目を避けて姿を消したため、王子は自分を助けたのが誰かを知りません。王子への思いを忘れられなくなった姫は、人間の足と引き換えに自分の美しい声を差し出すという、海の魔女の危険な取り引きに応じます。足を得た姫は激しい痛みに耐えて王子のそばにいますが、声を失っているため、自分こそ命の恩人だと伝えることができません。王子は姫へ親しみを持ちながらも、岸辺で自分を救ったと思い込んでいる別の姫と結婚してしまいます。姫には王子を殺して海へ帰れば助かる道も示されますが、姫はその選択を拒み、王子を傷つけることなく自ら海の泡となりました。報われない恋と自己犠牲の末に、姫の魂はより高い存在へ昇っていくという、痛みと清らかさの両方を残す話です。"
	case "sleepingbeauty":
		return "待望の姫が生まれた国では盛大な祝いが開かれ、王と妃は妖精たちを招いて娘に祝福を授けてもらうことにしました。けれど招かれなかった年老いた魔女は怒り、姫が十五歳になった日に糸車の針で指を刺して死ぬという呪いをかけます。最後に残っていた妖精は呪いを完全には消せず、死ではなく百年の眠りへ変えることで姫を救いました。王は国中の糸車を焼かせますが、運命の日に姫は塔の奥でひっそり糸を紡ぐ老婆を見つけ、知らずに針へ触れてしまいます。姫が眠りに落ちると、城の家来も王も妃も犬も馬も火までが一斉に眠り、城全体が深い沈黙へ包まれました。城の周囲にはいばらが伸び広がり、年月のうちに誰も近づけない森のようになります。百年後、その時が満ちた日に一人の王子が城へたどり着き、眠る姫の前へ進みました。王子が姫へ口づけすると城中の眠りが解け、人々は止まっていた時間を再び動かし始めます。"
	case "frogprince":
		return "お姫さまは城の近くの森で金のまりをついて遊ぶのが好きでしたが、ある日そのまりを深い井戸へ落としてしまいます。泣いている姫の前へ一匹の醜い蛙が現れ、まりを取ってきてやる代わりに友だちになり、食卓も寝台も共にしてほしいと約束を求めました。姫はまりを取り戻したい一心で約束しますが、まりを返してもらうとすぐ城へ逃げ帰り、蛙のことなど忘れようとします。ところが蛙は約束どおり城まで追ってきて戸をたたき、王様の前で姫に約束を守るよう迫りました。王様に、人は交わした約束を軽く扱ってはならないと諭された姫は、しぶしぶ蛙を食卓へ上げ、やがて同じ部屋で過ごすことになります。嫌悪と怒りの末に、姫が蛙を壁へ投げつけるか、真剣に受け入れるかした瞬間、蛙にかけられていた呪いが解けて若い王子の姿に戻りました。王子は魔法で蛙にされていた事情を語り、姫は初めて外見だけで相手を判断していた自分に向き合います。約束を守ることと、嫌悪の向こうにある真実を見ることが結びついた話です。"
	default:
		return ""
	}
}

func (o *IdleChatOrchestrator) RunStorySession() {
	sessionID := fmt.Sprintf("story-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)

	o.mu.Lock()
	o.chatActive = true
	o.sessionMode = "story"
	o.mu.Unlock()

	style := chooseStoryRewriteStyle(o.GetHistory(12))
	type storySuccess struct {
		source       StorySource
		plan         StoryRewritePlan
		draftText    string
		revisionNote string
		storyText    string
	}
	var result storySuccess
	var ok bool
	usedSources := make(map[string]struct{}, storySourceMaxAttempts)
	for sourceAttempt := 0; sourceAttempt < storySourceMaxAttempts; sourceAttempt++ {
		source := o.selectStorySourceExcluding(usedSources)
		usedSources[source.Title] = struct{}{}
		analysis := analyzeStorySource(source)
		plan, err := o.retryStoryRewritePlan(source, analysis, style)
		if err != nil {
			log.Printf("[Story] rewrite plan failed after retries (%s): %v", source.Title, err)
			continue
		}
		beatPlan, err := o.retryStoryBeatPlan(source, analysis, plan)
		if err != nil {
			log.Printf("[Story] beat plan failed after retries (%s): %v", source.Title, err)
			continue
		}
		adaptation := buildStoryAdaptationPlan(analysis.Skeleton, plan, beatPlan)
		draftText, err := o.retryStoryDraft(source, analysis, plan, adaptation, beatPlan)
		if err != nil {
			log.Printf("[Story] draft failed after retries (%s): %v", source.Title, err)
			continue
		}
		if storyDraftMatchesSourceRetelling(source, draftText) {
			result = storySuccess{
				source:       source,
				plan:         plan,
				draftText:    draftText,
				revisionNote: "第1稿が元話の骨格を十分に保っていたため、そのまま採用した。",
				storyText:    draftText,
			}
			ok = true
			break
		}
		storyText, revisionNote, err := o.retryStoryRevision(source, analysis, plan, adaptation, beatPlan, draftText)
		if err != nil {
			log.Printf("[Story] revision failed after retries (%s): %v", source.Title, err)
			candidate := strings.TrimSpace(draftText)
			if candidate == "" || !storyNarrativeLooksLikeProse(candidate) || !storySatisfiesSkeleton(candidate, analysis.Skeleton, adaptation) {
				candidate = repairStoryDraft(source, analysis, plan, adaptation, beatPlan, draftText)
			}
			if strings.TrimSpace(candidate) == "" || !storyNarrativeLooksLikeProse(candidate) {
				continue
			}
			storyText = candidate
			revisionNote = "改稿が不安定だったため、第1稿を整文して採用した。"
		}
		result = storySuccess{
			source:       source,
			plan:         plan,
			draftText:    draftText,
			revisionNote: revisionNote,
			storyText:    storyText,
		}
		ok = true
		break
	}
	if !ok {
		log.Printf("[Story] story generation failed for %d sources, falling back to normal chat", storySourceMaxAttempts)
		o.mu.Lock()
		o.sessionMode = "idle"
		o.currentTopic = ""
		o.mu.Unlock()
		o.runChatSession(StrategySingleGenre)
		o.mu.Lock()
		o.chatActive = false
		o.sessionMode = ""
		o.currentTopic = ""
		o.sessionContext = ""
		o.lastActivity = time.Now()
		o.mu.Unlock()
		return
	}

	currentTopic := fmt.Sprintf("元: %s / 改題: %s / 方式: %s", result.source.Title, result.plan.StoryTitle, result.plan.RewriteStyle)
	o.mu.Lock()
	o.currentTopic = currentTopic
	o.mu.Unlock()

	transcript := make([]string, 0, 12)
	intro := fmt.Sprintf("今夜の物語です。元になったのは『%s』。%s。", result.source.Title, result.plan.Hook)
	for _, chunk := range splitStoryNarration(intro, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	titleLine := fmt.Sprintf("改題は『%s』。", result.plan.StoryTitle)
	for _, chunk := range splitStoryNarration(titleLine, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	for _, chunk := range splitStoryNarration(result.storyText, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}
	closing := fmt.Sprintf("元の『%s』を下敷きにした、%sの物語でした。", result.source.Title, rewriteStyleLabel(result.plan.RewriteStyle))
	for _, chunk := range splitStoryNarration(closing, storyChunkMaxRunes) {
		o.emitStoryChunk(sessionID, chunk)
		transcript = append(transcript, "mio: "+chunk)
	}

	endedAt := time.Now().In(jst)
	o.saveStorySummary(sessionID, result.source, result.plan, result.draftText, result.revisionNote, result.storyText, transcript, startedAt, endedAt)

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.sessionContext = ""
	o.lastActivity = time.Now()
	o.mu.Unlock()
}

func (o *IdleChatOrchestrator) retryStoryRewritePlan(source StorySource, analysis StorySourceAnalysis, style string) (StoryRewritePlan, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		plan, err := o.generateStoryRewritePlan(source, analysis, style)
		if err == nil {
			return plan, nil
		}
		lastErr = err
		log.Printf("[Story] rewrite plan retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	return StoryRewritePlan{}, fmt.Errorf("rewrite plan retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) retryStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) (StoryBeatPlan, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		beatPlan, err := o.generateStoryBeatPlan(source, analysis, plan)
		if err == nil {
			return beatPlan, nil
		}
		lastErr = err
		log.Printf("[Story] beat plan retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	return StoryBeatPlan{}, fmt.Errorf("beat plan retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) retryStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		draftText, err := o.generateStoryDraft(source, analysis, plan, adaptation, beatPlan)
		if err == nil {
			return draftText, nil
		}
		lastErr = err
		log.Printf("[Story] draft retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
		log.Printf("[Story] draft retries exhausted (%s), using deterministic fallback", source.Title)
		return fallback, nil
	}
	if fallback := safeStoryRetelling(source, plan); fallback != "" {
		log.Printf("[Story] draft retries exhausted (%s), using source retelling fallback", source.Title)
		return fallback, nil
	}
	return "", fmt.Errorf("draft retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) retryStoryRevision(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) (string, string, error) {
	var lastErr error
	for attempt := 0; attempt < storyStageMaxRetries; attempt++ {
		storyText, revisionNote, err := o.reviseStoryNarrative(source, analysis, plan, adaptation, beatPlan, draftText)
		if err == nil {
			return storyText, revisionNote, nil
		}
		lastErr = err
		log.Printf("[Story] revision retry %d/%d failed (%s): %v", attempt+1, storyStageMaxRetries, source.Title, err)
	}
	return "", "", fmt.Errorf("revision retries exhausted: %w", lastErr)
}

func (o *IdleChatOrchestrator) emitStoryChunk(sessionID, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	msg := domaintransport.NewMessage("mio", "user", sessionID, "", content)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	ttsDone := o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "user",
		Content:   content,
		SessionID: sessionID,
	})
	o.waitForTTSDone(ttsDone)
	o.waitBreak(speakerBreak)
}

func (o *IdleChatOrchestrator) saveStorySummary(sessionID string, source StorySource, plan StoryRewritePlan, draftText, revisionNote, storyText string, transcript []string, startedAt, endedAt time.Time) {
	summary := fmt.Sprintf("元作品: %s\n改変方式: %s\n改題: %s\n導入: %s\n余韻: %s\nモチーフ: %s\n改稿: %s", source.Title, rewriteStyleLabel(plan.RewriteStyle), plan.StoryTitle, plan.Premise, plan.EndingFlavor, strings.Join(plan.MotifMap, " / "), revisionNote)
	record := SessionSummary{
		SessionID:         sessionID,
		Title:             fmt.Sprintf("%d月%d日の%sの物語まとめ", endedAt.Month(), endedAt.Day(), truncate(plan.StoryTitle, 24)),
		Topic:             fmt.Sprintf("元: %s / 改題: %s / 方式: %s", source.Title, plan.StoryTitle, plan.RewriteStyle),
		Strategy:          TopicStrategy(fmt.Sprintf("story:%s", plan.RewriteStyle)),
		Summary:           summary,
		SourceTitle:       source.Title,
		RewriteStyle:      plan.RewriteStyle,
		StoryTitle:        plan.StoryTitle,
		StoryText:         storyText,
		StoryDraftText:    draftText,
		StoryRevisionNote: revisionNote,
		StoryEndingFlavor: plan.EndingFlavor,
		StartedAt:         startedAt.Format(time.RFC3339),
		EndedAt:           endedAt.Format(time.RFC3339),
		Turns:             len(transcript),
		TopicProvider:     "shiro",
		SummaryProvider:   "shiro",
		Transcript:        append([]string(nil), transcript...),
	}
	o.mu.Lock()
	o.history = append(o.history, record)
	if len(o.history) > 200 {
		o.history = o.history[len(o.history)-200:]
	}
	store := o.topicStore
	o.mu.Unlock()
	if store != nil {
		if err := store.Append(record); err != nil {
			log.Printf("[Story] topic store append failed: %v", err)
		}
	}
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.summary",
		From:      "shiro",
		To:        "story_summary",
		Content:   record.Title + "\n" + summary,
		SessionID: sessionID,
	})
}

func chooseStoryRewriteStyle(history []SessionSummary) string {
	candidates := append([]string(nil), storyRewriteStyles...)
	if len(history) > 0 {
		last := strings.TrimSpace(history[0].RewriteStyle)
		if last == "" {
			if s := strings.TrimSpace(string(history[0].Strategy)); strings.HasPrefix(s, "story:") {
				last = strings.TrimPrefix(s, "story:")
			}
		}
		if last != "" {
			filtered := candidates[:0]
			for _, c := range candidates {
				if c != last {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			}
		}
	}
	return candidates[storyRandIntn(len(candidates))]
}

func (o *IdleChatOrchestrator) selectStorySource() StorySource {
	return o.selectStorySourceExcluding(nil)
}

func (o *IdleChatOrchestrator) selectStorySourceExcluding(excluded map[string]struct{}) StorySource {
	history := o.GetHistory(12)
	blocked := make(map[string]struct{}, 4)
	for _, item := range history {
		if strings.TrimSpace(item.SourceTitle) == "" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(string(item.Strategy)), "story:") {
			continue
		}
		blocked[item.SourceTitle] = struct{}{}
		if len(blocked) >= 4 {
			break
		}
	}
	pool := make([]StorySource, 0, len(storyCorpus))
	for _, item := range storyCorpus {
		if excluded != nil {
			if _, skip := excluded[item.Title]; skip {
				continue
			}
		}
		if _, ok := blocked[item.Title]; ok {
			continue
		}
		pool = append(pool, item)
	}
	if len(pool) == 0 {
		for _, item := range storyCorpus {
			if excluded != nil {
				if _, skip := excluded[item.Title]; skip {
					continue
				}
			}
			pool = append(pool, item)
		}
	}
	if len(pool) == 0 {
		pool = append(pool, storyCorpus...)
	}
	return pool[storyRandIntn(len(pool))]
}

func (o *IdleChatOrchestrator) generateStoryRewritePlan(source StorySource, analysis StorySourceAnalysis, style string) (StoryRewritePlan, error) {
	return groundStoryRewritePlan(source, analysis, fallbackStoryRewritePlan(source, analysis, style)), nil
}

func (o *IdleChatOrchestrator) generateStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) (StoryBeatPlan, error) {
	return groundedStoryBeatPlan(source, analysis, plan), nil
}

func (o *IdleChatOrchestrator) generateStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	return o.generateStoryDraftByBeats(source, analysis, plan, adaptation, beatPlan)
}

func (o *IdleChatOrchestrator) generateStoryDraftByBeats(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) (string, error) {
	openingSeed := storyOpeningSeed(source, plan)
	type beatSection struct {
		label   string
		content string
	}
	sections := []beatSection{
		{label: "導入", content: beatPlan.Opening},
		{label: "逸脱", content: beatPlan.Deviation},
		{label: "反転", content: beatPlan.Reversal},
		{label: "着地", content: beatPlan.Landing},
	}
	paragraphs := make([]string, 0, len(sections))
	for i, section := range sections {
		context := ""
		if len(paragraphs) > 0 {
			context = paragraphs[len(paragraphs)-1]
		}
		messages := []llm.Message{
			{Role: "system", Content: "あなたは朗読短編作家です。指定された場面だけを、聞き取りやすい日本語の短い段落で書いてください。"},
			{Role: "user", Content: fmt.Sprintf(`元作品: %s
元話の要約本文:
%s
改題: %s
改変方式: %s
舞台: %s
視点: %s
今回書く場面: %s
この場面の役割: %s
前の段落:
%s
必須モチーフ: %s
認識手がかり: %s

要件:
- この場面だけを2〜4文で書く
- 会社名、開発計画、ランキング制度、観光客、SNS、スマホ、モデル、広告の話にしない
- 比喩は多用しない
- 新しい固有名詞を増やさない
- 人物の行動か対話で場面を進める
- 元話の骨格が分かる出来事を書く
- 教訓のまとめ、抽象的な総括、象徴の説明を書かない
- 前の段落で書いた出来事や文を言い直さない
- %s
- 出力は本文だけ`, source.Title, source.Text, plan.StoryTitle, plan.RewriteStyle, plan.Setting, plan.Viewpoint, section.label, section.content, context, strings.Join(plan.CoreMotifs, " / "), strings.Join(adaptation.RecognitionCues, " / "), beatInstruction(i, openingSeed))},
		}
		resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   280,
			Temperature: 0.2,
		})
		if err != nil {
			return "", fmt.Errorf("beat draft failed: %w", err)
		}
		paragraph := normalizeStoryNarrative(resp.Content)
		if paragraph == "" || storyHasOutlineLanguage(paragraph) || storyHasOverblownSetting(paragraph) || storyParagraphLooksAtmospheric(paragraph) || storyParagraphRepeatsContext(context, paragraph) {
			return "", fmt.Errorf("empty story draft")
		}
		paragraphs = append(paragraphs, paragraph)
	}
	story := strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
	if !storyNarrativeLooksLikeProse(story) || !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("empty story draft")
	}
	return story, nil
}

func storyParagraphLooksAtmospheric(paragraph string) bool {
	sentences := splitStorySentences(paragraph)
	if len(sentences) >= 2 && storyStartsWithAtmosphere(strings.TrimSpace(sentences[1])) {
		return true
	}
	if strings.Count(paragraph, "まるで") > 1 {
		return true
	}
	return false
}

func storyParagraphRepeatsContext(context, paragraph string) bool {
	context = strings.TrimSpace(context)
	paragraph = strings.TrimSpace(paragraph)
	if context == "" || paragraph == "" {
		return false
	}
	contextSentences := splitStorySentences(context)
	paragraphSentences := splitStorySentences(paragraph)
	seen := make(map[string]struct{}, len(contextSentences))
	for _, sentence := range contextSentences {
		seen[storySignature(sentence)] = struct{}{}
	}
	repeats := 0
	for _, sentence := range paragraphSentences {
		if _, ok := seen[storySignature(sentence)]; ok {
			repeats++
		}
	}
	return repeats > 0
}

func beatInstruction(index int, openingSeed string) string {
	if index == 0 {
		return "第1文は必ずこの文をそのまま使う: " + openingSeed
	}
	return "前の段落を受けて自然につなげる"
}

func (o *IdleChatOrchestrator) reviseStoryNarrative(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) (string, string, error) {
	openingSeed := storyOpeningSeed(source, plan)
	messages := []llm.Message{
		{Role: "system", Content: "あなたは朗読短編の編集者です。第1稿の面白さを残しつつ、因果、余韻、読後感を整えた第2稿に直してください。"},
		{Role: "user", Content: fmt.Sprintf(`次の第1稿を改稿して、第2稿を作ってください。

元作品: %s
元話の要約本文:
%s
改題: %s
改変方式: %s
余韻: %s
必須モチーフ: %s
必須イベント順: %s
認識手がかり: %s
禁忌/約束: %s
報酬と罰: %s
ビート:
- 導入: %s
- 逸脱: %s
- 反転: %s
- 着地: %s

第1稿:
%s

改稿方針:
- 第1文は次の文を一字一句変えずに保つ: %s
- 第1稿の良い飛躍は消しすぎない
- 因果が飛ぶ箇所だけを補う
- 結末で %s が残るようにする
- ひねりは「%s」という一点に絞り、それ以外は元話の骨格へ戻しすぎるくらいでよい
- 必須モチーフの位置を聞き取りやすくする
- 必須イベント順と認識手がかりを落とさない
- 導入 -> 逸脱 -> 反転 -> 余韻 が感じられるように整える
- 説明臭くしない
- 元話の骨格を、事件と場面として再演する
- 4〜8段落相当の短編として落ち着かせる
- 各段落で誰かの行動、対話、決断のどれかを必ず進める
- 必須イベントごとに少なくとも1つ、目に見える場面が残るようにする
- 「元の『%s』で禁じられていた」などの設計説明文を本文に入れない
- 「最初の違和感として立ち上がる」「ここで〜が意外な意味に変わる」「最後に残るのは〜だ」を本文に入れない
- 新しい固有名詞をむやみに増やさない
- 舞台は現代の地続きの世界に固定する
- 年号を出すなら現在に近いものだけにし、未来年代や時代跳躍を入れない
- 巨大企業、AI支配、世界規模の陰謀へ話を膨らませず、生活圏の事件として整える
- SNS、観光客、スマホ、会員制施設、権限トークンのような手癖の現代化を避ける
- 会社名、開発計画、ランキング制度、不動産会社、プロジェクト名を新しく作らない
- 比喩を減らし、冒頭2文のどちらかで必ず人物の行動か対話を始める
- 第1文は必ず人物の行動か対話で始める
- 幼少期の思い出、象徴的な回想、説明のための脇道を新しく足さない
- 教訓の言い直し、象徴の説明、抽象的な総括で終わらせない
- 一人称か三人称の自然な物語文にし、二人称で説教や勧誘をしない
- 出力は次の形式だけ
REVISION_NOTE:
STORY:`, source.Title, source.Text, plan.StoryTitle, plan.RewriteStyle, plan.EndingFlavor, strings.Join(plan.CoreMotifs, " / "), strings.Join(storyBeatLabels(analysis.Skeleton.RequiredBeats), " -> "), strings.Join(analysis.Skeleton.RecognitionCues, " / "), analysis.TabooOrRule, analysis.RewardAndPunish, beatPlan.Opening, beatPlan.Deviation, beatPlan.Reversal, beatPlan.Landing, draftText, openingSeed, plan.EndingFlavor, plan.Premise, source.Title)},
	}
	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   1400,
		Temperature: 0.25,
	})
	if err != nil {
		return "", "", err
	}
	revisionNote, story := parseStoryRevision(resp.Content)
	story = normalizeStoryNarrative(story)
	if story == "" {
		log.Printf("[Story] revision rejected (%s): empty response", source.Title)
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storyNarrativeLooksSettled(story, draftText, plan, beatPlan) {
		log.Printf("[Story] revision rejected (%s): settling check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storyNarrativeLooksLikeProse(story) {
		log.Printf("[Story] revision rejected (%s): prose check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		log.Printf("[Story] revision rejected (%s): skeleton check failed: %s", source.Title, storyLogSnippet(story))
		return "", "", fmt.Errorf("invalid revised story")
	}
	if revisionNote == "" {
		revisionNote = fallbackStoryRevisionNote(plan, beatPlan)
	}
	return story, revisionNote, nil
}

func parseStoryRewritePlan(sourceTitle, style, raw string, twist StoryTwist, coreMotifs []string) StoryRewritePlan {
	plan := StoryRewritePlan{
		SourceTitle:  sourceTitle,
		RewriteStyle: normalizeStoryRewriteStyle(style),
		StoryTitle:   twist.StoryTitle,
		Premise:      twist.VisibleTwist,
		Setting:      twist.Setting,
		Viewpoint:    twist.Viewpoint,
		Tone:         twist.Tone,
		Hook:         twist.Hook,
		EndingShape:  twist.EndingShape,
		EndingFlavor: twist.EndingFlavor,
		CoreMotifs:   append([]string(nil), coreMotifs...),
		MotifMap:     defaultStoryMotifMap(normalizeStoryRewriteStyle(style), coreMotifs),
	}
	for _, line := range strings.Split(raw, "\n") {
		line = normalizeStoryFieldLine(line)
		switch {
		case strings.HasPrefix(line, "STORY_TITLE:"):
			plan.StoryTitle = strings.TrimSpace(strings.TrimPrefix(line, "STORY_TITLE:"))
		case strings.HasPrefix(line, "改題:"):
			plan.StoryTitle = strings.TrimSpace(strings.TrimPrefix(line, "改題:"))
		case strings.HasPrefix(line, "PREMISE:"):
			plan.Premise = strings.TrimSpace(strings.TrimPrefix(line, "PREMISE:"))
		case strings.HasPrefix(line, "前提:"):
			plan.Premise = strings.TrimSpace(strings.TrimPrefix(line, "前提:"))
		case strings.HasPrefix(line, "SETTING:"):
			plan.Setting = strings.TrimSpace(strings.TrimPrefix(line, "SETTING:"))
		case strings.HasPrefix(line, "舞台:"):
			plan.Setting = strings.TrimSpace(strings.TrimPrefix(line, "舞台:"))
		case strings.HasPrefix(line, "VIEWPOINT:"):
			plan.Viewpoint = strings.TrimSpace(strings.TrimPrefix(line, "VIEWPOINT:"))
		case strings.HasPrefix(line, "視点:"):
			plan.Viewpoint = strings.TrimSpace(strings.TrimPrefix(line, "視点:"))
		case strings.HasPrefix(line, "TONE:"):
			plan.Tone = strings.TrimSpace(strings.TrimPrefix(line, "TONE:"))
		case strings.HasPrefix(line, "トーン:"):
			plan.Tone = strings.TrimSpace(strings.TrimPrefix(line, "トーン:"))
		case strings.HasPrefix(line, "HOOK:"):
			plan.Hook = strings.TrimSpace(strings.TrimPrefix(line, "HOOK:"))
		case strings.HasPrefix(line, "導入:"):
			plan.Hook = strings.TrimSpace(strings.TrimPrefix(line, "導入:"))
		case strings.HasPrefix(line, "ENDING:"):
			plan.EndingShape = strings.TrimSpace(strings.TrimPrefix(line, "ENDING:"))
		case strings.HasPrefix(line, "結末:"):
			plan.EndingShape = strings.TrimSpace(strings.TrimPrefix(line, "結末:"))
		case strings.HasPrefix(line, "ENDING_FLAVOR:"):
			plan.EndingFlavor = normalizeStoryEndingFlavor(strings.TrimSpace(strings.TrimPrefix(line, "ENDING_FLAVOR:")))
		case strings.HasPrefix(line, "余韻:"):
			plan.EndingFlavor = normalizeStoryEndingFlavor(strings.TrimSpace(strings.TrimPrefix(line, "余韻:")))
		case strings.HasPrefix(line, "CORE_MOTIFS:"):
			if motifs := parseStoryList(strings.TrimSpace(strings.TrimPrefix(line, "CORE_MOTIFS:"))); len(motifs) > 0 {
				plan.CoreMotifs = motifs
			}
		case strings.HasPrefix(line, "必須モチーフ:"):
			if motifs := parseStoryList(strings.TrimSpace(strings.TrimPrefix(line, "必須モチーフ:"))); len(motifs) > 0 {
				plan.CoreMotifs = motifs
			}
		case strings.HasPrefix(line, "MOTIF_MAP:"):
			if motifs := parseStoryList(strings.TrimSpace(strings.TrimPrefix(line, "MOTIF_MAP:"))); len(motifs) > 0 {
				plan.MotifMap = motifs
			}
		case strings.HasPrefix(line, "モチーフ:"):
			if motifs := parseStoryList(strings.TrimSpace(strings.TrimPrefix(line, "モチーフ:"))); len(motifs) > 0 {
				plan.MotifMap = motifs
			}
		case strings.HasPrefix(line, "STYLE:"):
			if v := strings.TrimSpace(strings.TrimPrefix(line, "STYLE:")); v != "" {
				plan.RewriteStyle = normalizeStoryRewriteStyle(v)
			}
		case strings.HasPrefix(line, "方式:"):
			if v := strings.TrimSpace(strings.TrimPrefix(line, "方式:")); v != "" {
				plan.RewriteStyle = normalizeStoryRewriteStyle(v)
			}
		}
	}
	rewriteLabels := []string{
		"STORY_TITLE:", "改題:",
		"PREMISE:", "前提:",
		"SETTING:", "舞台:",
		"VIEWPOINT:", "視点:",
		"TONE:", "トーン:",
		"HOOK:", "導入:",
		"ENDING:", "結末:",
		"ENDING_FLAVOR:", "余韻:",
		"CORE_MOTIFS:", "必須モチーフ:",
		"MOTIF_MAP:", "モチーフ:",
		"STYLE:", "方式:",
	}
	if v := extractStoryField(raw, []string{"STORY_TITLE:", "改題:"}, rewriteLabels); v != "" {
		plan.StoryTitle = v
	}
	if v := extractStoryField(raw, []string{"PREMISE:", "前提:"}, rewriteLabels); v != "" {
		plan.Premise = v
	}
	if v := extractStoryField(raw, []string{"SETTING:", "舞台:"}, rewriteLabels); v != "" {
		plan.Setting = v
	}
	if v := extractStoryField(raw, []string{"VIEWPOINT:", "視点:"}, rewriteLabels); v != "" {
		plan.Viewpoint = v
	}
	if v := extractStoryField(raw, []string{"TONE:", "トーン:"}, rewriteLabels); v != "" {
		plan.Tone = v
	}
	if v := extractStoryField(raw, []string{"HOOK:", "導入:"}, rewriteLabels); v != "" {
		plan.Hook = v
	}
	if v := extractStoryField(raw, []string{"ENDING:", "結末:"}, rewriteLabels); v != "" {
		plan.EndingShape = v
	}
	if v := extractStoryField(raw, []string{"ENDING_FLAVOR:", "余韻:"}, rewriteLabels); v != "" {
		plan.EndingFlavor = normalizeStoryEndingFlavor(v)
	}
	if v := extractStoryField(raw, []string{"CORE_MOTIFS:", "必須モチーフ:"}, rewriteLabels); v != "" {
		if motifs := parseStoryList(v); len(motifs) > 0 {
			plan.CoreMotifs = motifs
		}
	}
	if v := extractStoryField(raw, []string{"MOTIF_MAP:", "モチーフ:"}, rewriteLabels); v != "" {
		if motifs := parseStoryList(v); len(motifs) > 0 {
			plan.MotifMap = motifs
		}
	}
	if v := extractStoryField(raw, []string{"STYLE:", "方式:"}, rewriteLabels); v != "" {
		plan.RewriteStyle = normalizeStoryRewriteStyle(v)
	}
	return plan
}

func fallbackStoryRewritePlan(source StorySource, analysis StorySourceAnalysis, style string) StoryRewritePlan {
	twist := chooseStoryTwist(source, style)
	return StoryRewritePlan{
		SourceTitle:  source.Title,
		RewriteStyle: style,
		StoryTitle:   twist.StoryTitle,
		Premise:      twist.VisibleTwist,
		Setting:      twist.Setting,
		Viewpoint:    twist.Viewpoint,
		Tone:         twist.Tone,
		Hook:         twist.Hook,
		EndingShape:  twist.EndingShape,
		EndingFlavor: twist.EndingFlavor,
		CoreMotifs:   append([]string(nil), analysis.CoreMotifs...),
		MotifMap:     defaultStoryMotifMap(normalizeStoryRewriteStyle(style), analysis.CoreMotifs),
	}
}

func fallbackStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) StoryBeatPlan {
	return StoryBeatPlan{
		Opening:   fmt.Sprintf("%s。%sが最初の違和感として立ち上がる。", plan.Hook, analysis.TabooOrRule),
		Deviation: fmt.Sprintf("%s。ここで%sが意外な意味に変わる。", plan.Premise, firstStoryMotifLabel(plan.MotifMap)),
		Reversal:  fmt.Sprintf("%s。その飛躍が、%sによって因果として結び直される。", plan.EndingShape, analysis.RewardAndPunish),
		Landing:   fmt.Sprintf("最後に残るのは%sだ。", plan.EndingFlavor),
	}
}

func groundStoryRewritePlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) StoryRewritePlan {
	if plan.RewriteStyle == "" {
		plan.RewriteStyle = "role_shift"
	}
	plan.StoryTitle = defaultGroundedStoryTitle(source, plan.RewriteStyle)
	plan.Premise = defaultGroundedStoryPremise(source, plan.RewriteStyle)
	plan.Setting = defaultGroundedStorySetting(source)
	plan.Viewpoint = defaultGroundedStoryViewpoint(plan.RewriteStyle)
	plan.Hook = defaultGroundedStoryHook(source, plan.RewriteStyle)
	plan.Tone = "生活圏の手触りを残す静かな短編"
	if len(plan.CoreMotifs) == 0 {
		plan.CoreMotifs = append([]string(nil), analysis.CoreMotifs...)
	}
	if len(plan.MotifMap) == 0 {
		plan.MotifMap = defaultStoryMotifMap(plan.RewriteStyle, plan.CoreMotifs)
	}
	return plan
}

func storyTextNeedsGrounding(text string) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	for _, token := range []string{
		"AI", "SNS", "スマホ", "観光客", "巨大企業", "企業", "高層", "地下", "トークン", "権限",
		"量子", "ニューヨーク", "LED", "会員制", "保養施設", "リゾート", "アプリ", "アルゴリズム",
		"近未来", "未来都市", "システム", "ソリューション", "データ入力", "ホテル宴会場",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func defaultGroundedStoryTitle(source StorySource, style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "view_shift":
		return source.Title + "のそばにいた人"
	case "value_shift":
		return source.Title + "の裏返し"
	default:
		return "今の町の" + source.Title
	}
}

func defaultGroundedStoryPremise(source StorySource, style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "view_shift":
		return fmt.Sprintf("『%s』を、主役のすぐ近くにいた人物の視点から語り直し、元話の進行は保ったまま見え方だけをずらす。", source.Title)
	case "value_shift":
		return fmt.Sprintf("『%s』の報いと救いの意味を少しだけ反転させ、元話の骨格はそのままに今の生活圏で起きる事件として描く。", source.Title)
	default:
		return fmt.Sprintf("『%s』を、現代の町で起きる身近な事件として置き換え、役目だけを少しずらして語る。", source.Title)
	}
}

func defaultGroundedStoryHook(source StorySource, style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "view_shift":
		return fmt.Sprintf("『%s』は、近くで見ていた人の目には別の物語に見えていた。", source.Title)
	case "value_shift":
		return fmt.Sprintf("『%s』で当然とされた救いや報いを、少し疑ってみる。", source.Title)
	default:
		return fmt.Sprintf("『%s』の出来事が、今の町で起きたらどう見えるか。", source.Title)
	}
}

func defaultGroundedStoryViewpoint(style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "view_shift":
		return "主役のすぐそばにいた人物の一人称"
	default:
		return "出来事の渦中にいる人物の近接三人称"
	}
}

func defaultGroundedStorySetting(source StorySource) string {
	switch source.ID {
	case "momotaro":
		return "川沿いの町とその外れ"
	case "urashima":
		return "海辺の町と港の近く"
	case "kaguya":
		return "町外れの竹林と古い家"
	case "issun":
		return "下町の長屋と小さな店"
	case "hanasaka":
		return "庭のある古い家と近所の道"
	case "shitakiri":
		return "町外れの家と小さな鳥小屋"
	case "kasajizo":
		return "雪の積もる町はずれの道"
	case "kintaro":
		return "山あいの集落"
	case "sarukani":
		return "畑の残る町はずれ"
	case "tsuru":
		return "雪の降る田舎町の小さな家"
	case "kobutori":
		return "山裾の家と夜の小屋"
	case "bunbuku":
		return "古道具屋のある商店街"
	case "redriding":
		return "町外れの林道と祖母の家"
	case "cinderella":
		return "町なかの家と公民館の祝賀会"
	case "snowwhite":
		return "山あいの町と共同住宅"
	case "hansel":
		return "町外れの林道と菓子店"
	case "bremen":
		return "街道沿いの町と古い家"
	case "puss":
		return "河原の町と古い屋敷"
	case "threepigs":
		return "郊外の住宅地"
	case "beauty":
		return "町はずれの古い洋館"
	case "aladdin":
		return "市場通りと古い倉庫"
	case "ali40":
		return "乾いた商店街と外れの倉庫"
	case "hadaka":
		return "祭りを控えた町役場"
	case "match":
		return "年の瀬の商店街"
	case "littlemermaid":
		return "海辺の町と防波堤"
	case "sleepingbeauty":
		return "古い屋敷と長く閉ざされた部屋"
	case "frogprince":
		return "池のある古い家"
	default:
		return "現代の地方都市とその周辺"
	}
}

func groundedStoryBeatPlan(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan) StoryBeatPlan {
	labels := storyBeatLabels(analysis.Skeleton.RequiredBeats)
	opening := firstOrFallback(labels, 0, "導入")
	deviation := firstOrFallback(labels, 1, "逸脱")
	reversal := firstOrFallback(labels, 2, "反転")
	landing := firstOrFallback(labels, 3, "着地")

	return StoryBeatPlan{
		Opening:   fmt.Sprintf("%sを、%sで始める。", opening, plan.Setting),
		Deviation: fmt.Sprintf("%sが起こり、%sというひねりが見える。", deviation, plan.RewriteStyleLabel()),
		Reversal:  fmt.Sprintf("%sによって、%sという元話の骨格をはっきり見せる。", reversal, joinSome(plan.CoreMotifs, 2)),
		Landing:   fmt.Sprintf("%sで終え、最後に%sが残る。", landing, plan.EndingFlavor),
	}
}

func (p StoryRewritePlan) RewriteStyleLabel() string {
	return rewriteStyleLabel(p.RewriteStyle)
}

func firstOrFallback(items []string, idx int, fallback string) string {
	if idx >= 0 && idx < len(items) && strings.TrimSpace(items[idx]) != "" {
		return items[idx]
	}
	return fallback
}

func joinSome(items []string, max int) string {
	if len(items) == 0 {
		return "元話の手がかり"
	}
	if len(items) > max {
		items = items[:max]
	}
	return strings.Join(items, "と")
}

func storyOpeningSeed(source StorySource, plan StoryRewritePlan) string {
	switch source.ID {
	case "momotaro":
		return "その夜、桃太郎は川沿いの倉庫で小さな包みを仲間に配った。"
	case "urashima":
		return "浦島は海辺の道で、子どもに囲まれていた小さな亀を助けた。"
	case "kaguya":
		return "翁は町外れの竹林で、ひときわ光る竹を見つけて立ち止まった。"
	case "issun":
		return "一寸ほどの背丈しかない若者は、椀のように小さな舟を押して町へ向かった。"
	case "hanasaka":
		return "おじいさんは庭の土を掘る犬の前にしゃがみこみ、何が出るのか息をひそめた。"
	case "shitakiri":
		return "おじいさんは傷ついた雀を手のひらにのせ、誰にも見つからないよう家へ連れ帰った。"
	case "kasajizo":
		return "おじいさんは売れ残った笠を背負い、雪の道で立ち止まった。"
	case "kintaro":
		return "金太郎は山道の真ん中で丸太を持ち上げ、動物たちを笑わせた。"
	case "sarukani":
		return "蟹は握り飯を抱えたまま、猿に差し出された柿の種を見つめた。"
	case "tsuru":
		return "男は雪の畑で羽を傷めた鶴を見つけ、ためらいながら縄をほどいた。"
	case "kobutori":
		return "こぶを気にする老人は、雨をよけて古い小屋へ駆け込んだ。"
	case "bunbuku":
		return "古道具屋の主人は、寺から預かった茶釜を店の奥へ運びこんだ。"
	case "redriding":
		return "赤い頭巾の娘は、包みを抱えて祖母の家へ向かう林道に入った。"
	case "cinderella":
		return "娘は灰だらけの手を洗い、公民館の明かりを遠くから見上げた。"
	case "snowwhite":
		return "娘は追い立てられるように町を離れ、山あいの家の戸をたたいた。"
	case "hansel":
		return "兄妹は細い道へ入り、帰り道の目印に白い小石を落としていった。"
	case "bremen":
		return "年老いたロバは荷を降ろされる前に家を出て、街道を歩き始めた。"
	case "puss":
		return "猫は主人の前に立ち、まず一足の長靴を買ってくれと頼んだ。"
	case "threepigs":
		return "三人のきょうだいは町はずれの空き地に立ち、それぞれ別の家を建て始めた。"
	case "beauty":
		return "娘は父の代わりに、町はずれの古い洋館の門をくぐった。"
	case "aladdin":
		return "若者は市場通りの裏で声をかけてきた男に連れられ、古い倉庫へ入った。"
	case "ali40":
		return "木こりの男は荷を背負って歩く途中で、岩陰に人影が集まるのを見つけた。"
	case "hadaka":
		return "仕立て屋たちは町役場へ呼ばれ、誰にも見えない布の話を始めた。"
	case "match":
		return "少女は年の瀬の商店街で足を止め、売れ残った細い箱を抱え直した。"
	case "littlemermaid":
		return "海辺の娘は防波堤の向こうを見つめ、沖から戻る船を待っていた。"
	case "sleepingbeauty":
		return "若者は長く閉ざされていた部屋の扉を押し開け、中の静けさに息をのんだ。"
	case "frogprince":
		return "娘は池のほとりで金のまりを落とし、水面をのぞきこんだ。"
	default:
		return fmt.Sprintf("%sは%sで足を止め、これから起こる出来事の気配を見つけた。", source.Title, defaultGroundedStorySetting(source))
	}
}

func fallbackStoryNarrative(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, beatPlan StoryBeatPlan) string {
	return deterministicStoryDraft(source, analysis, plan, buildStoryAdaptationPlan(analysis.Skeleton, plan, beatPlan), beatPlan)
}

func repairStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan, draftText string) string {
	repaired := stripStoryMetaLeak(draftText)
	repaired = strings.ReplaceAll(repaired, "。。", "。")
	repaired = strings.TrimSpace(repaired)
	if repaired == "" {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return stripStoryMetaLeak(fallback)
		}
		return stripStoryMetaLeak(safeStoryRetelling(source, plan))
	}
	if !strings.Contains(repaired, firstToken(plan.EndingFlavor)) && strings.TrimSpace(beatPlan.Landing) != "" {
		if !strings.HasSuffix(repaired, "。") {
			repaired += "。"
		}
		repaired = strings.TrimSpace(repaired + " " + beatPlan.Landing)
	}
	if !storySatisfiesSkeleton(repaired, analysis.Skeleton, adaptation) {
		if fallback := deterministicStoryDraft(source, analysis, plan, adaptation, beatPlan); fallback != "" {
			return stripStoryMetaLeak(fallback)
		}
		return stripStoryMetaLeak(safeStoryRetelling(source, plan))
	}
	return repaired
}

func storyDraftMatchesSourceRetelling(source StorySource, draftText string) bool {
	draftText = normalizeStoryNarrative(draftText)
	sourceText := normalizeStoryNarrative(source.Text)
	if draftText == "" || sourceText == "" {
		return false
	}
	sentences := splitStorySentences(sourceText)
	hits := 0
	for i := 0; i < len(sentences) && i < 3; i++ {
		if strings.Contains(draftText, sentences[i]) {
			hits++
		}
	}
	return hits >= 2
}

func deterministicStoryDraft(source StorySource, analysis StorySourceAnalysis, plan StoryRewritePlan, adaptation StoryAdaptationPlan, beatPlan StoryBeatPlan) string {
	opening := storyOpeningSeed(source, plan)
	motif0 := storyMappedMotif(plan.MotifMap, 0, firstStoryMotifLabel(plan.MotifMap))
	motif1 := storyMappedMotif(plan.MotifMap, 1, motif0)
	motif2 := storyMappedMotif(plan.MotifMap, 2, motif1)
	baseSentences := splitStorySentences(normalizeStoryNarrative(source.Text))
	if len(baseSentences) == 0 {
		baseSentences = []string{opening}
	}
	paragraphs := []string{
		opening + " " + fmt.Sprintf("その場では、%sと%sの名がひそやかに広まり始めていた。", motif0, motif1),
		storyDeterministicParagraph(baseSentences, 1),
		storyDeterministicParagraph(baseSentences, 2),
		storyDeterministicParagraph(baseSentences, 3) + " " + fmt.Sprintf("あとに残ったのは、%sに近い静けさだった。", plan.EndingFlavor),
	}
	story := normalizeStoryNarrative(strings.Join(paragraphs, "\n\n"))
	if !storyNarrativeLooksLikeProse(story) {
		return ""
	}
	if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
		story = normalizeStoryNarrative(story + "\n\n" + fmt.Sprintf("%s、%s、%sは順番どおりにそこへ現れた。", motif0, motif1, motif2))
		if !storySatisfiesSkeleton(story, analysis.Skeleton, adaptation) {
			return ""
		}
	}
	return story
}

func storyDeterministicParagraph(sentences []string, idx int) string {
	if idx < len(sentences) {
		return strings.TrimSpace(sentences[idx])
	}
	return strings.TrimSpace(sentences[len(sentences)-1])
}

func safeStoryRetelling(source StorySource, plan StoryRewritePlan) string {
	opening := storyOpeningSeed(source, plan)
	body := normalizeStoryNarrative(source.Text)
	if body == "" {
		return ""
	}
	return normalizeStoryNarrative(opening + "\n\n" + body + "\n\n" + fmt.Sprintf("そのあとに残ったのは、%sに近い静けさだった。", plan.EndingFlavor))
}

func storyMappedMotif(motifMap []string, idx int, fallback string) string {
	if idx >= 0 && idx < len(motifMap) {
		if token := firstToken(motifMap[idx]); token != "" {
			return token
		}
	}
	return fallback
}

func rewriteStyleLabel(style string) string {
	switch normalizeStoryRewriteStyle(style) {
	case "role_shift":
		return "役割転換"
	case "view_shift":
		return "視点変更"
	case "value_shift":
		return "価値反転"
	default:
		return style
	}
}

func normalizeStoryRewriteStyle(style string) string {
	switch strings.TrimSpace(style) {
	case "role_shift", "what_if", "if", "役割転換", "もしも転換":
		return "role_shift"
	case "view_shift", "視点変更":
		return "view_shift"
	case "value_shift", "価値反転":
		return "value_shift"
	default:
		return strings.TrimSpace(style)
	}
}

func normalizeStoryEndingFlavor(flavor string) string {
	switch strings.TrimSpace(flavor) {
	case "報い", "救い", "喪失", "皮肉":
		return strings.TrimSpace(flavor)
	default:
		return "余韻"
	}
}

func fallbackStorySetting(style string) string {
	switch strings.TrimSpace(style) {
	case "view_shift":
		return "同じ事件を横から見ている地域コミュニティ"
	case "value_shift":
		return "善意と損得が衝突する商店街"
	default:
		return "深夜の物流と生活が交差する町"
	}
}

func fallbackStoryViewpoint(style string) string {
	switch strings.TrimSpace(style) {
	case "view_shift":
		return "元の脇役の一人称"
	case "value_shift":
		return "正しさを信じていた当事者の一人称"
	default:
		return "役目を押しつけられた当事者"
	}
}

func chooseStoryTwist(source StorySource, style string) StoryTwist {
	style = normalizeStoryRewriteStyle(style)
	switch style {
	case "view_shift":
		return chooseViewShiftTwist(source)
	case "value_shift":
		return chooseValueShiftTwist(source)
	default:
		return chooseRoleShiftTwist(source)
	}
}

func chooseRoleShiftTwist(source StorySource) StoryTwist {
	return StoryTwist{
		Style:           "role_shift",
		VisibleTwist:    defaultGroundedStoryPremise(source, "role_shift"),
		Setting:         defaultGroundedStorySetting(source),
		Viewpoint:       "元話の主役の役目を少し引き受け直した人物の近接三人称",
		ImmediateChange: "冒頭で、元話と同じ困りごとが今の町の暮らしの問題として起きる",
		ConflictShift:   "元話の敵対や試練を、人間どうしの誤解や圧力へ置き換える",
		EndingShape:     "元話の結末を保ちつつ、今の生活の痛みが少し残る終わり",
		EndingFlavor:    "救い",
		StoryTitle:      defaultGroundedStoryTitle(source, "role_shift"),
		Hook:            defaultGroundedStoryHook(source, "role_shift"),
		Tone:            "生活圏の手触りがある静かな現代短編",
	}
}

func chooseValueShiftTwist(source StorySource) StoryTwist {
	return StoryTwist{
		Style:           "value_shift",
		VisibleTwist:    defaultGroundedStoryPremise(source, "value_shift"),
		Setting:         defaultGroundedStorySetting(source),
		Viewpoint:       "正しさを信じていた当事者の一人称",
		ImmediateChange: "冒頭で、元話では善意や報酬に見えたものが少し重たい意味を帯びる",
		ConflictShift:   "出来事の順番は同じまま、何が救いで何が負債かの見え方をずらす",
		EndingShape:     "元話の結末に、報いや皮肉の手触りを少し強く残す終わり",
		EndingFlavor:    "皮肉",
		StoryTitle:      defaultGroundedStoryTitle(source, "value_shift"),
		Hook:            defaultGroundedStoryHook(source, "value_shift"),
		Tone:            "生活の痛みがにじむ現代短編",
	}
}

func chooseViewShiftTwist(source StorySource) StoryTwist {
	return StoryTwist{
		Style:           "view_shift",
		VisibleTwist:    defaultGroundedStoryPremise(source, "view_shift"),
		Setting:         defaultGroundedStorySetting(source),
		Viewpoint:       "主役のすぐそばにいた人物の一人称",
		ImmediateChange: "冒頭で、主役のすぐ近くにいた者だけが知る違和感を明かす",
		ConflictShift:   "元話の大きな出来事を、脇にいた人の迷いや観察として捉え直す",
		EndingShape:     "元話の結末は保ちつつ、見ていた者の痛みや救いが残る終わり",
		EndingFlavor:    "喪失",
		StoryTitle:      defaultGroundedStoryTitle(source, "view_shift"),
		Hook:            defaultGroundedStoryHook(source, "view_shift"),
		Tone:            "近くで見ていた人の息づかいが残る語り",
	}
}

func analyzeStorySource(source StorySource) StorySourceAnalysis {
	skeleton := storySkeleton(source)
	return StorySourceAnalysis{
		CoreMotifs:          skeleton.CanonicalMotifs,
		RoleMap:             skeleton.RoleConstraints,
		TabooOrRule:         skeleton.TabooOrRule,
		RewardAndPunish:     skeleton.RewardPunishment,
		EmotionalAftertaste: skeleton.EmotionalAftertaste,
		Skeleton:            skeleton,
	}
}

func buildStoryAdaptationPlan(skeleton StorySkeleton, plan StoryRewritePlan, beatPlan StoryBeatPlan) StoryAdaptationPlan {
	beatMappings := []string{
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 0), beatPlan.Opening),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 1), beatPlan.Deviation),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 2), beatPlan.Reversal),
		fmt.Sprintf("%s=>%s", labelOrBeatID(skeleton.RequiredBeats, 3), beatPlan.Landing),
	}
	return StoryAdaptationPlan{
		SkeletonID:      skeleton.ID,
		RewriteStyle:    plan.RewriteStyle,
		BeatMappings:    beatMappings,
		MotifMappings:   append([]string(nil), plan.MotifMap...),
		RoleRemap:       append([]string(nil), skeleton.RoleConstraints...),
		EndingFlavor:    plan.EndingFlavor,
		RecognitionCues: append([]string(nil), skeleton.RecognitionCues...),
	}
}

func labelOrBeatID(beats []StoryBeat, idx int) string {
	if idx >= 0 && idx < len(beats) && strings.TrimSpace(beats[idx].Label) != "" {
		return beats[idx].Label
	}
	switch idx {
	case 0:
		return "導入"
	case 1:
		return "逸脱"
	case 2:
		return "反転"
	default:
		return "着地"
	}
}

func storyBeatLabels(beats []StoryBeat) []string {
	out := make([]string, 0, len(beats))
	for _, beat := range beats {
		if strings.TrimSpace(beat.Label) == "" {
			continue
		}
		out = append(out, beat.Label)
	}
	return out
}

func parseStoryBeatPlan(raw string, plan StoryRewritePlan) StoryBeatPlan {
	beatPlan := StoryBeatPlan{
		Opening:   plan.Hook,
		Deviation: plan.Premise,
		Reversal:  plan.EndingShape,
		Landing:   "最後に残るのは" + plan.EndingFlavor,
	}
	for _, line := range strings.Split(raw, "\n") {
		line = normalizeStoryFieldLine(line)
		switch {
		case strings.HasPrefix(line, "OPENING:"):
			beatPlan.Opening = strings.TrimSpace(strings.TrimPrefix(line, "OPENING:"))
		case strings.HasPrefix(line, "導入:"):
			beatPlan.Opening = strings.TrimSpace(strings.TrimPrefix(line, "導入:"))
		case strings.HasPrefix(line, "DEVIATION:"):
			beatPlan.Deviation = strings.TrimSpace(strings.TrimPrefix(line, "DEVIATION:"))
		case strings.HasPrefix(line, "逸脱:"):
			beatPlan.Deviation = strings.TrimSpace(strings.TrimPrefix(line, "逸脱:"))
		case strings.HasPrefix(line, "REVERSAL:"):
			beatPlan.Reversal = strings.TrimSpace(strings.TrimPrefix(line, "REVERSAL:"))
		case strings.HasPrefix(line, "反転:"):
			beatPlan.Reversal = strings.TrimSpace(strings.TrimPrefix(line, "反転:"))
		case strings.HasPrefix(line, "LANDING:"):
			beatPlan.Landing = strings.TrimSpace(strings.TrimPrefix(line, "LANDING:"))
		case strings.HasPrefix(line, "着地:"):
			beatPlan.Landing = strings.TrimSpace(strings.TrimPrefix(line, "着地:"))
		}
	}
	beatLabels := []string{"OPENING:", "導入:", "DEVIATION:", "逸脱:", "REVERSAL:", "反転:", "LANDING:", "着地:"}
	if v := extractStoryField(raw, []string{"OPENING:", "導入:"}, beatLabels); v != "" {
		beatPlan.Opening = v
	}
	if v := extractStoryField(raw, []string{"DEVIATION:", "逸脱:"}, beatLabels); v != "" {
		beatPlan.Deviation = v
	}
	if v := extractStoryField(raw, []string{"REVERSAL:", "反転:"}, beatLabels); v != "" {
		beatPlan.Reversal = v
	}
	if v := extractStoryField(raw, []string{"LANDING:", "着地:"}, beatLabels); v != "" {
		beatPlan.Landing = v
	}
	return beatPlan
}

func parseStoryRevision(raw string) (string, string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	note := ""
	story := raw
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return "", strings.TrimSpace(raw)
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "REVISION_NOTE:") {
		note = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "REVISION_NOTE:"))
		story = strings.Join(lines[1:], "\n")
		if strings.HasPrefix(strings.TrimSpace(story), "STORY:") {
			story = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(story), "STORY:"))
		}
	}
	return note, strings.TrimSpace(story)
}

func normalizeStoryNarrative(story string) string {
	story = strings.ReplaceAll(story, "\r\n", "\n")
	story = strings.ReplaceAll(story, "\r", "\n")
	story = strings.ReplaceAll(story, "REVISION_NOTE:", "")
	story = strings.ReplaceAll(story, "STORY:", "")
	lines := strings.Split(story, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(kept) > 0 && kept[len(kept)-1] != "" {
				kept = append(kept, "")
			}
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "*   **") || strings.HasPrefix(line, "- ") {
			break
		}
		if strings.HasPrefix(line, "わかりました。") || strings.HasPrefix(line, "以下に、") || strings.HasPrefix(line, "いかがでしょうか") {
			continue
		}
		if strings.HasPrefix(line, "（余韻）") || strings.HasPrefix(line, "(余韻)") || strings.HasPrefix(line, "余韻:") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	out = stripStoryMetaSentences(out)
	out = dedupeStoryParagraphs(out)
	out = dedupeStorySentences(out)
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out
}

func fallbackStoryRevisionNote(plan StoryRewritePlan, beatPlan StoryBeatPlan) string {
	return fmt.Sprintf("逸脱を残しつつ、%s から %s へ因果が通るよう整えた。", truncate(beatPlan.Deviation, 18), plan.EndingFlavor)
}

func storyNarrativeLooksSettled(story, draft string, plan StoryRewritePlan, beatPlan StoryBeatPlan) bool {
	if storyHasMetaLeak(story) {
		return false
	}
	tail := story
	if utf8.RuneCountInString(tail) > 160 {
		r := []rune(tail)
		tail = string(r[len(r)-160:])
	}
	if !strings.Contains(tail, firstToken(plan.EndingFlavor)) && !strings.Contains(tail, firstToken(beatPlan.Landing)) {
		return false
	}
	return true
}

func storyNarrativeLooksLikeProse(story string) bool {
	story = strings.TrimSpace(story)
	if utf8.RuneCountInString(story) < 160 {
		return false
	}
	if !storyOpensWithActionOrDialogue(story) {
		return false
	}
	if storyHasOutlineLanguage(story) {
		return false
	}
	if storyHasOverblownSetting(story) {
		return false
	}
	if storyHasDistractingDigression(story) {
		return false
	}
	sentenceCount := strings.Count(story, "。") + strings.Count(story, "！") + strings.Count(story, "？")
	if sentenceCount < 3 {
		return false
	}
	actionHits := 0
	for _, token := range []string{"言っ", "聞い", "見", "向か", "渡し", "開け", "閉め", "走", "歩", "置", "差し出", "隠", "助け", "届", "待っ", "座っ"} {
		if strings.Contains(story, token) {
			actionHits++
		}
	}
	return actionHits >= 1
}

func storyHasOverblownSetting(story string) bool {
	patterns := []string{
		"AI開発部", "量子コンピューター", "未来テック", "2040", "2041", "2042", "2043", "2044", "2045",
		"巨大企業", "世界最大手", "社会の神経回路", "ご招待ありがとうございます", "会員限定リゾート",
		"大規模言語モデル", "量子", "システム部門の地下室", "世界規模", "未来都市", "近未来",
		"SNS", "いいね", "観光客", "スマホ", "会員制", "保養施設", "トークン", "権限", "高層", "地下保守",
		"不動産会社", "株式会社", "開発計画", "プロジェクト", "ランキング", "評価システム", "商業施設", "アプリ",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	if strings.Count(story, "まるで") >= 5 {
		return true
	}
	head := story
	if utf8.RuneCountInString(head) > 60 {
		head = string([]rune(head)[:60])
	}
	if strings.Contains(head, "あなたは") {
		return true
	}
	return false
}

func storyHasDistractingDigression(story string) bool {
	patterns := []string{
		"幼い頃",
		"子どもの頃",
		"思い出した",
		"思い出す",
		"記憶のよう",
		"象徴している",
		"物語の一部だった",
		"結局のところ",
		"最も恐ろしい",
		"悪だった",
		"唯一無二の宝",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func storyOpensWithActionOrDialogue(story string) bool {
	firstSentence := firstStorySentences(story, 1)
	if storyStartsWithAtmosphere(firstSentence) {
		return false
	}
	head := firstStorySentences(story, 2)
	if head == "" {
		return false
	}
	if strings.Count(head, "まるで") > 1 {
		return false
	}
	if strings.Contains(head, "「") || strings.Contains(head, "『") || strings.Contains(head, "“") {
		return true
	}
	for _, token := range []string{
		"行っ", "来", "入", "出", "渡", "開", "閉", "運", "置", "持", "返", "逃", "走", "見",
		"拾", "渡し", "呼", "言", "頼", "座", "立", "向か", "届け", "売", "買", "隠", "探",
		"助け", "差し出", "断", "受け取",
	} {
		if strings.Contains(head, token) {
			return true
		}
	}
	return false
}

func storyStartsWithAtmosphere(sentence string) bool {
	sentence = strings.TrimSpace(sentence)
	for _, prefix := range []string{"雨", "風", "雪", "夜", "朝", "夕", "月", "光", "薄明かり", "霧", "静けさ"} {
		if strings.HasPrefix(sentence, prefix) {
			return true
		}
	}
	return false
}

func firstStorySentences(story string, limit int) string {
	parts := splitStorySentences(story)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > limit {
		parts = parts[:limit]
	}
	return strings.Join(parts, "")
}

func splitStorySentences(story string) []string {
	var (
		sentences []string
		buf       strings.Builder
	)
	for _, r := range story {
		buf.WriteRune(r)
		switch r {
		case '。', '！', '？', '\n':
			part := strings.TrimSpace(buf.String())
			if part != "" {
				sentences = append(sentences, part)
			}
			buf.Reset()
		}
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func storyHasOutlineLanguage(story string) bool {
	patterns := []string{
		"どうひねったか",
		"よく分からないけど",
		"物語の始まりを予感",
		"最初の違和感として立ち上がる",
		"ここで",
		"意外な意味に変わる",
		"因果として結び直される",
		"最後に残るのは",
		"という感触だった",
		"導入:",
		"逸脱:",
		"反転:",
		"着地:",
		"要件:",
		"改稿方針:",
		"REVISION_NOTE:",
		"STORY:",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func stripStoryMetaSentences(story string) string {
	sentences := splitStorySentences(story)
	if len(sentences) == 0 {
		return strings.TrimSpace(story)
	}
	filtered := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		if strings.Contains(sentence, "どうひねったか") ||
			strings.Contains(sentence, "よく分からないけど") ||
			strings.Contains(sentence, "物語の始まりを予感") {
			continue
		}
		filtered = append(filtered, sentence)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func dedupeStoryParagraphs(story string) string {
	parts := strings.Split(strings.TrimSpace(story), "\n\n")
	if len(parts) == 0 {
		return strings.TrimSpace(story)
	}
	seen := make(map[string]struct{}, len(parts))
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := storySignature(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, part)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func dedupeStorySentences(story string) string {
	sentences := splitStorySentences(story)
	if len(sentences) == 0 {
		return strings.TrimSpace(story)
	}
	seen := make(map[string]int, len(sentences))
	kept := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		key := storySignature(sentence)
		if seen[key] >= 1 {
			continue
		}
		seen[key]++
		kept = append(kept, sentence)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func storySignature(text string) string {
	replacer := strings.NewReplacer(
		" ", "", "　", "", "\n", "", "。", "", "、", "", "！", "", "？", "",
		"「", "", "」", "", "（", "", "）", "", "(", "", ")", "", "『", "", "』", "",
	)
	return replacer.Replace(strings.TrimSpace(text))
}

func storyLogSnippet(story string) string {
	story = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(story, "\n", " "), "\r", " "))
	runes := []rune(story)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return story
}

func normalizeStoryFieldLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "#-* ")
	line = strings.ReplaceAll(line, "**", "")
	return strings.TrimSpace(line)
}

func extractStoryField(raw string, starts []string, labels []string) string {
	clean := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	clean = strings.ReplaceAll(clean, "**", "")
	for _, start := range starts {
		idx := strings.Index(clean, start)
		if idx < 0 {
			continue
		}
		rest := clean[idx+len(start):]
		end := len(rest)
		for _, label := range labels {
			if label == start {
				continue
			}
			if j := strings.Index(rest, label); j >= 0 && j < end {
				end = j
			}
		}
		value := strings.TrimSpace(rest[:end])
		value = strings.Trim(value, "-*# ")
		if value != "" {
			return value
		}
	}
	return ""
}

func storyHasMetaLeak(story string) bool {
	patterns := []string{
		"元の『",
		"元作品",
		"禁じられていたのは",
		"ここではそれが別の形",
		"読後感だった",
		"改変方式",
		"必須モチーフ",
		"報酬と罰",
	}
	for _, pattern := range patterns {
		if strings.Contains(story, pattern) {
			return true
		}
	}
	return false
}

func stripStoryMetaLeak(story string) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(story, "\r\n", "\n"), "\r", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if storyHasMetaLeak(line) {
			continue
		}
		kept = append(kept, line)
	}
	clean := strings.TrimSpace(strings.Join(kept, "\n"))
	for strings.Contains(clean, "。。") {
		clean = strings.ReplaceAll(clean, "。。", "。")
	}
	return clean
}

func storySatisfiesSkeleton(story string, skeleton StorySkeleton, adaptation StoryAdaptationPlan) bool {
	if strings.TrimSpace(story) == "" {
		return false
	}
	if !storyHasRecognitionCues(story, skeleton) {
		return false
	}
	if storyViolatesBeatOrder(story, skeleton) {
		return false
	}
	if len(adaptation.MotifMappings) > 0 && !storyContainsMappedMotifs(story, adaptation.MotifMappings) {
		return false
	}
	return true
}

func storyHasRecognitionCues(story string, skeleton StorySkeleton) bool {
	if len(skeleton.RecognitionCues) == 0 {
		return true
	}
	hits := 0
	for _, cue := range skeleton.RecognitionCues {
		if cue != "" && strings.Contains(story, cue) {
			hits++
		}
	}
	need := 2
	if len(skeleton.RecognitionCues) < need {
		need = len(skeleton.RecognitionCues)
	}
	if need == 0 {
		return true
	}
	return hits >= need
}

func storyViolatesBeatOrder(story string, skeleton StorySkeleton) bool {
	if len(skeleton.RequiredBeats) == 0 {
		return false
	}
	prev := -1
	matched := 0
	for _, beat := range skeleton.RequiredBeats {
		pos := firstCuePosition(story, append([]string{beat.Label}, append(beat.Canonical, beat.AllowedSubstitute...)...))
		if pos < 0 {
			continue
		}
		matched++
		if pos < prev {
			return true
		}
		prev = pos
	}
	minMatched := 2
	if len(skeleton.RequiredBeats) < minMatched {
		minMatched = len(skeleton.RequiredBeats)
	}
	return matched < minMatched
}

func firstCuePosition(story string, cues []string) int {
	best := -1
	for _, cue := range cues {
		cue = strings.TrimSpace(cue)
		if cue == "" {
			continue
		}
		if idx := strings.Index(story, cue); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	return best
}

func storyContainsMappedMotifs(story string, mappings []string) bool {
	hits := 0
	for _, mapping := range mappings {
		if token := firstToken(mapping); token != "" && strings.Contains(story, token) {
			hits++
		}
	}
	return hits >= minStoryMotifHits(len(mappings))
}

func firstStoryMotifLabel(motifMap []string) string {
	if len(motifMap) == 0 {
		return "元話の核"
	}
	return firstToken(motifMap[0])
}

func storyNarrativeLooksTwisted(story string, plan StoryRewritePlan) bool {
	return !storyHasOutlineLanguage(story)
}

func storySignalTokens(values ...string) []string {
	seen := make(map[string]struct{}, 16)
	out := make([]string, 0, 16)
	replacer := strings.NewReplacer(
		"。", " ", "、", " ", "・", " ", "「", " ", "」", " ", "『", " ", "』", " ", "（", " ", "）", " ",
		":", " ", "：", " ", "/", " ", "／", " ", "(", " ", ")", " ", "\n", " ", "\t", " ", "の", " ",
	)
	stop := map[string]struct{}{
		"現代": {}, "地続き": {}, "世界": {}, "主人公": {}, "物語": {}, "話": {}, "一人称": {}, "三人称": {},
		"近接": {}, "視点": {}, "舞台": {}, "終わり": {}, "最後": {}, "まま": {}, "こと": {}, "として": {},
	}
	for _, value := range values {
		clean := replacer.Replace(strings.TrimSpace(value))
		for _, part := range strings.Fields(clean) {
			part = strings.TrimSpace(part)
			if utf8.RuneCountInString(part) < 2 {
				continue
			}
			if _, skip := stop[part]; skip {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
			if short := firstToken(part); short != "" && short != part && utf8.RuneCountInString(short) >= 2 {
				if _, ok := stop[short]; !ok {
					if _, dup := seen[short]; !dup {
						seen[short] = struct{}{}
						out = append(out, short)
					}
				}
			}
		}
	}
	return out
}

func storyContainsMotifEcho(story string, plan StoryRewritePlan) bool {
	if len(plan.MotifMap) == 0 {
		return true
	}
	hits := 0
	for _, motif := range plan.MotifMap {
		token := firstToken(motif)
		if token != "" && strings.Contains(story, token) {
			hits++
		}
	}
	return hits >= minStoryMotifHits(len(plan.MotifMap))
}

func minStoryMotifHits(n int) int {
	if n <= 1 {
		return n
	}
	if n == 2 {
		return 2
	}
	return 2
}

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "=>") {
		parts := strings.SplitN(s, "=>", 2)
		s = strings.TrimSpace(parts[1])
	}
	for _, sep := range []string{"、", " ", "の", "と"} {
		if idx := strings.Index(s, sep); idx > 0 {
			return strings.TrimSpace(s[:idx])
		}
	}
	return s
}

func parseStoryList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seps := []string{" / ", "／", ";", "；", ","}
	parts := []string{raw}
	for _, sep := range seps {
		if strings.Contains(raw, sep) {
			parts = strings.Split(raw, sep)
			break
		}
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func defaultStoryMotifMap(style string, motifs []string) []string {
	out := make([]string, 0, len(motifs))
	for _, motif := range motifs {
		out = append(out, motif+"=>"+transformMotif(style, motif))
	}
	return out
}

func transformMotif(style, motif string) string {
	style = normalizeStoryRewriteStyle(style)
	switch style {
	case "view_shift":
		switch motif {
		case "舌を切る":
			return "声を奪われた理由"
		case "小さいつづら":
			return "控えめな贈り物"
		case "大きいつづら":
			return "欲の大きい選択肢"
		case "玉手箱":
			return "開けるなと言われた包み"
		case "時間のずれ":
			return "待っていた側の空白"
		case "亀を助ける":
			return "見捨てずに庇った相手"
		}
	case "value_shift":
		switch motif {
		case "舌を切る":
			return "善意の名で発言権を奪う処置"
		case "小さいつづら":
			return "控えめだが自由のある謝礼"
		case "大きいつづら":
			return "豪華だが断れない支援"
		case "玉手箱":
			return "開けば借りを負う封筒"
		case "時間のずれ":
			return "戻った時に生まれる社会的な空白"
		case "亀を助ける":
			return "助けた後に責任まで背負う相手"
		}
	default:
		switch motif {
		case "舌を切る":
			return "言葉を奪う処分"
		case "小さいつづら":
			return "小さな箱"
		case "大きいつづら":
			return "大きな箱"
		case "玉手箱":
			return "禁を破る箱"
		case "時間のずれ":
			return "帰還後の時間差"
		case "亀を助ける":
			return "弱った相手を助ける"
		}
	}
	return motif
}

func storySkeleton(source StorySource) StorySkeleton {
	if spec, ok := storySpecForSource(source); ok {
		return spec.Skeleton
	}
	switch source.ID {
	case "redriding":
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     []string{"赤い頭巾", "おばあさんへの届け物", "狼の先回り", "おばあさんに化ける", "見抜けない会話", "救出"},
			RoleConstraints:     []string{"届け物を持つ主人公", "先回りする捕食者", "訪問先のおばあさん", "救出に関わる第三者"},
			TabooOrRule:         "道を外れて誘惑に乗らないこと",
			RewardPunishment:    "油断は危機を招き、見抜く力が生還につながる",
			EmotionalAftertaste: "助かっても無垢ではいられない",
			RecognitionCues:     []string{"赤い頭巾", "おばあさん", "狼", "先回り", "化ける"},
			RequiredBeats: []StoryBeat{
				{ID: "delivery", Label: "届け物の出発", Canonical: []string{"届け物", "おばあさん", "届け"}, Required: true, AllowedSubstitute: []string{"荷物", "訪問先", "祖母"}},
				{ID: "detour", Label: "道中の足止め", Canonical: []string{"道草", "引き止め"}, Required: true, AllowedSubstitute: []string{"寄り道", "足止め"}},
				{ID: "overtake", Label: "先回り", Canonical: []string{"先回り", "待ち伏せ"}, Required: true, AllowedSubstitute: []string{"先に着く"}},
				{ID: "disguise", Label: "変装と誤認", Canonical: []string{"化ける", "化けて", "見抜けない"}, Required: true, AllowedSubstitute: []string{"変装", "違和感"}},
				{ID: "rescue", Label: "危機と救出", Canonical: []string{"飲み込まれる", "救出"}, Required: true, AllowedSubstitute: []string{"閉じ込める", "助け出す"}},
			},
		}
	case "shitakiri":
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     []string{"舌を切る", "小さいつづら", "大きいつづら", "欲深さの報い"},
			RoleConstraints:     []string{"優しい与え手", "欲深い対比役", "恩返しする雀", "選択を迫る贈り物"},
			TabooOrRule:         "相手の尊厳を傷つけず、欲で選ばないこと",
			RewardPunishment:    "控えめさは報われ、欲深さは自滅に変わる",
			EmotionalAftertaste: "欲と慎みの差が静かに痛む",
			RecognitionCues:     []string{"雀", "舌", "小さいつづら", "大きいつづら"},
			RequiredBeats: []StoryBeat{
				{ID: "hurt", Label: "尊厳を傷つける", Canonical: []string{"舌を切る", "傷つける"}, Required: true, AllowedSubstitute: []string{"声を奪う"}},
				{ID: "hospitality", Label: "恩返しのもてなし", Canonical: []string{"もてなし", "恩返し"}, Required: true, AllowedSubstitute: []string{"招待", "返礼"}},
				{ID: "small", Label: "小さい選択", Canonical: []string{"小さいつづら", "小さな箱"}, Required: true, AllowedSubstitute: []string{"控えめな選択"}},
				{ID: "large", Label: "大きい選択", Canonical: []string{"大きいつづら", "大きな箱"}, Required: true, AllowedSubstitute: []string{"欲の大きい選択"}},
				{ID: "punish", Label: "欲の報い", Canonical: []string{"報い", "化け物"}, Required: true, AllowedSubstitute: []string{"破綻", "自滅"}},
			},
		}
	case "urashima":
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     []string{"亀を助ける", "竜宮城", "玉手箱", "時間のずれ"},
			RoleConstraints:     []string{"助ける若者", "導く相手", "禁を託す存在", "帰還後の世界"},
			TabooOrRule:         "託された禁を破らないこと",
			RewardPunishment:    "好奇心の代償が、帰還後に一気に押し寄せる",
			EmotionalAftertaste: "取り返せない時間が胸に残る",
			RecognitionCues:     []string{"亀", "竜宮城", "玉手箱", "時間"},
			RequiredBeats: []StoryBeat{
				{ID: "rescue", Label: "弱い相手を助ける", Canonical: []string{"助ける", "亀"}, Required: true, AllowedSubstitute: []string{"庇う"}},
				{ID: "invite", Label: "別世界へ招かれる", Canonical: []string{"竜宮城", "招かれる"}, Required: true, AllowedSubstitute: []string{"奥の空間", "歓待"}},
				{ID: "taboo", Label: "禁を託される", Canonical: []string{"玉手箱", "開けるな"}, Required: true, AllowedSubstitute: []string{"封印", "触れるな"}},
				{ID: "return", Label: "帰還と時間差", Canonical: []string{"帰る", "時間のずれ"}, Required: true, AllowedSubstitute: []string{"戻る", "空白"}},
			},
		}
	case "snowwhite":
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     []string{"嫉妬", "追放", "毒りんご", "七人の小人", "ガラスの棺", "再生"},
			RoleConstraints:     []string{"嫉妬される主役", "執着する対立役", "匿う共同体", "再生を見届ける証人"},
			TabooOrRule:         "嫉妬に従って他者を排除しないこと",
			RewardPunishment:    "執着は孤立へ向かい、連帯は再生を呼ぶ",
			EmotionalAftertaste: "救われても傷は残る",
			RecognitionCues:     []string{"毒りんご", "小人", "棺", "嫉妬"},
			RequiredBeats: []StoryBeat{
				{ID: "envy", Label: "嫉妬と追放", Canonical: []string{"嫉妬", "追放"}, Required: true, AllowedSubstitute: []string{"排除", "逃亡"}},
				{ID: "shelter", Label: "匿われる", Canonical: []string{"小人", "匿う"}, Required: true, AllowedSubstitute: []string{"共同体", "保護"}},
				{ID: "poison", Label: "毒の贈り物", Canonical: []string{"毒りんご", "毒"}, Required: true, AllowedSubstitute: []string{"甘い罠"}},
				{ID: "sleep", Label: "仮死と保存", Canonical: []string{"棺", "眠る"}, Required: true, AllowedSubstitute: []string{"止まる", "保管"}},
				{ID: "revive", Label: "再生", Canonical: []string{"目を覚ます", "再生"}, Required: true, AllowedSubstitute: []string{"戻る", "息を吹き返す"}},
			},
		}
	case "aladdin":
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     []string{"洞窟でだまされる", "魔法のランプ", "魔人", "王女との結婚", "ランプを奪われる", "機転で奪還する"},
			RoleConstraints:     []string{"だまされる若者", "命令に従う魔人", "力を奪う魔法使い", "奇跡を受け取る王女"},
			TabooOrRule:         "力を得ても、それを他者支配の道具にしないこと",
			RewardPunishment:    "願いで得たものは、奪われたときに本当の価値を問われる",
			EmotionalAftertaste: "まぶしい成功の裏に、使われる側の息苦しさが残る",
			RecognitionCues:     []string{"ランプ", "魔人", "魔法使い", "王女"},
			RequiredBeats: []StoryBeat{
				{ID: "trick", Label: "だまされて力に触れる", Canonical: []string{"だまされる", "洞窟"}, Required: true, AllowedSubstitute: []string{"誘い込まれる"}},
				{ID: "gain", Label: "ランプと魔人を得る", Canonical: []string{"ランプ", "魔人"}, Required: true, AllowedSubstitute: []string{"願いの力"}},
				{ID: "rise", Label: "成り上がりと結婚", Canonical: []string{"王女", "結婚"}, Required: true, AllowedSubstitute: []string{"祝宴", "上昇"}},
				{ID: "loss", Label: "力を奪われる", Canonical: []string{"奪われる", "失う"}, Required: true, AllowedSubstitute: []string{"横取り"}},
				{ID: "recover", Label: "機転で奪還する", Canonical: []string{"奪還", "取り戻す"}, Required: true, AllowedSubstitute: []string{"出し抜く"}},
			},
		}
	default:
		motifs := storyCoreMotifs(source)
		return StorySkeleton{
			ID:                  source.ID,
			SourceTitle:         source.Title,
			CanonicalMotifs:     motifs,
			RoleConstraints:     storyRoleMap(source),
			TabooOrRule:         storyTabooOrRule(source),
			RewardPunishment:    storyRewardAndPunishment(source),
			EmotionalAftertaste: storyEmotionalAftertaste(source),
			RecognitionCues:     genericRecognitionCues(motifs),
			RequiredBeats:       genericRequiredBeats(motifs),
		}
	}
}

func genericRecognitionCues(motifs []string) []string {
	out := make([]string, 0, len(motifs))
	for _, motif := range motifs {
		if token := firstToken(motif); token != "" {
			out = append(out, token)
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func genericRequiredBeats(motifs []string) []StoryBeat {
	labels := []string{"導入", "逸脱", "反転", "着地"}
	beats := make([]StoryBeat, 0, len(labels))
	for i, label := range labels {
		canonical := []string{}
		if i < len(motifs) {
			canonical = append(canonical, firstToken(motifs[i]))
		}
		beats = append(beats, StoryBeat{
			ID:        fmt.Sprintf("generic_%d", i),
			Label:     label,
			Canonical: canonical,
			Required:  true,
		})
	}
	return beats
}

func storyRoleMap(source StorySource) []string {
	switch source.ID {
	case "shitakiri":
		return []string{"優しい与え手", "欲深い対比役", "恩返しする雀", "選択を迫る贈り物"}
	case "snowwhite":
		return []string{"嫉妬される主役", "執着する対立役", "匿う共同体", "再生を見届ける証人"}
	case "urashima":
		return []string{"助ける若者", "導く相手", "禁を託す存在", "帰還後の世界"}
	case "aladdin":
		return []string{"だまされる若者", "命令に従う魔人", "力を奪う魔法使い", "奇跡を受け取る王女"}
	default:
		return []string{"主人公", "対立役", "援助者", "証人"}
	}
}

func storyTabooOrRule(source StorySource) string {
	switch source.ID {
	case "shitakiri":
		return "相手の尊厳を傷つけず、欲で選ばないこと"
	case "snowwhite":
		return "嫉妬に従って他者を排除しないこと"
	case "urashima":
		return "託された禁を破らないこと"
	case "aladdin":
		return "力を得ても、それを他者支配の道具にしないこと"
	default:
		return "与えられた約束を破らないこと"
	}
}

func storyRewardAndPunishment(source StorySource) string {
	switch source.ID {
	case "shitakiri":
		return "控えめさは報われ、欲深さは自滅に変わる"
	case "snowwhite":
		return "執着は孤立へ向かい、連帯は再生を呼ぶ"
	case "urashima":
		return "好奇心の代償が、帰還後に一気に押し寄せる"
	case "aladdin":
		return "願いで得たものは、奪われたときに本当の価値を問われる"
	default:
		return "選択の意味が最後に報いか罰として返る"
	}
}

func storyEmotionalAftertaste(source StorySource) string {
	switch source.ID {
	case "shitakiri":
		return "欲と慎みの差が静かに痛む"
	case "snowwhite":
		return "救われても傷は残る"
	case "urashima":
		return "取り返せない時間が胸に残る"
	case "aladdin":
		return "まぶしい成功の裏に、使われる側の息苦しさが残る"
	default:
		return "少し苦みのある余韻"
	}
}

func storyCoreMotifs(source StorySource) []string {
	switch source.ID {
	case "momotaro":
		return []string{"桃から生まれる", "きびだんご", "鬼退治"}
	case "urashima":
		return []string{"亀を助ける", "竜宮城", "玉手箱", "時間のずれ"}
	case "kaguya":
		return []string{"光る竹", "無理難題の求婚", "月へ帰る"}
	case "issun":
		return []string{"小さな体", "針の刀", "打ち出の小槌"}
	case "hanasaka":
		return []string{"犬が宝を示す", "灰で花を咲かせる", "欲深さの報い"}
	case "shitakiri":
		return []string{"舌を切る", "小さいつづら", "大きいつづら", "欲深さの報い"}
	case "kasajizo":
		return []string{"売れ残りの笠", "地蔵に贈る", "年の暮れの贈り物"}
	case "kintaro":
		return []string{"怪力", "山の動物たち", "都へ見いだされる"}
	case "sarukani":
		return []string{"柿の種", "親の仇討ち", "栗と蜂と臼"}
	case "tsuru":
		return []string{"助けた鶴", "機織り", "のぞいてはいけない"}
	case "kobutori":
		return []string{"鬼の宴", "踊り", "こぶを取る"}
	case "bunbuku":
		return []string{"狸が茶釜に化ける", "寺から逃げる", "見世物になる"}
	case "redriding":
		return []string{"赤い頭巾", "狼の先回り", "おばあさんに化ける"}
	case "cinderella":
		return []string{"灰まみれの娘", "舞踏会", "片方の靴"}
	case "snowwhite":
		return []string{"毒りんご", "七人の小人", "ガラスの棺"}
	case "hansel":
		return []string{"森に置き去り", "お菓子の家", "魔女とかまど"}
	case "bremen":
		return []string{"年老いた動物", "音楽隊の旅", "盗賊の家"}
	case "puss":
		return []string{"長靴の猫", "主人を侯爵に仕立てる", "人食い鬼をだます"}
	case "threepigs":
		return []string{"藁の家", "木の家", "れんがの家", "狼が吹き飛ばす"}
	case "beauty":
		return []string{"野獣の城", "毎夜の求婚", "愛で呪いが解ける"}
	case "aladdin":
		return []string{"洞窟でだまされる", "魔法のランプ", "魔人", "王女との結婚", "ランプを奪われる", "機転で奪還する"}
	case "ali40":
		return []string{"開けゴマ", "宝の洞窟", "召使いの知恵"}
	case "emperors":
		return []string{"見えない服", "誰も真実を言えない", "子どもの一言"}
	case "matchgirl":
		return []string{"マッチをする", "幻を見る", "祖母に抱かれる"}
	case "littlemermaid":
		return []string{"声と引き換えに足", "王子への恋", "泡になる"}
	case "sleepingbeauty":
		return []string{"糸車の針", "百年の眠り", "口づけで目覚める"}
	case "frogprince":
		return []string{"井戸に落ちたまり", "蛙との約束", "王子に戻る"}
	default:
		return []string{source.Title}
	}
}

func splitStoryNarration(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = storyChunkMaxRunes
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out []string
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		for utf8.RuneCountInString(para) > maxRunes {
			idx := bestStorySplitIndex(para, maxRunes)
			head := strings.TrimSpace(para[:idx])
			if head != "" {
				out = append(out, head)
			}
			para = strings.TrimSpace(para[idx:])
		}
		if para != "" {
			out = append(out, para)
		}
	}
	return out
}

func bestStorySplitIndex(s string, maxRunes int) int {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return len(s)
	}
	limit := maxRunes
	if limit < storyChunkMinRunes {
		limit = storyChunkMinRunes
	}
	best := -1
	for i := limit - 1; i >= storyChunkMinRunes-1 && i < len(runes); i-- {
		switch runes[i] {
		case '。', '！', '？', '!', '?':
			return len(string(runes[:i+1]))
		case '、', '，', ',', '」':
			if best < 0 {
				best = len(string(runes[:i+1]))
			}
		}
	}
	if best > 0 {
		return best
	}
	return len(string(runes[:maxRunes]))
}
