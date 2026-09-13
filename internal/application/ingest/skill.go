// 技能数据的接入。
//
// 这是 L2 文本抽取——倍率写在自由文本里，不在结构化字段中。
// docs/specs/admission.spec.md 要求：**回退与推测解析不得定为 L1**，
// 因此本解析器产出的候选一律标为 ParsingText（最高 L2）。
//
// 真实数据暴露的三个难点（姑获鸟为例）：
//
//	伞剑    262_01  造成攻击80%伤害                    只有一个倍率，可抽
//	协战    262_02  30%概率（被动）                    30% 不是伤害倍率，**不得抽**
//	天翔鹤斩 262_03  造成攻击33%伤害…造成攻击88%伤害    两个倍率，**无法确定，不得猜**
//
// 因此核心规则是：**只在唯一确定时才抽**。
//
// 但必须区分两种「没抽到」（实测统计 795 个式神技能）：
//
//	文本里有但无法确定（多个候选值）  -> 计入 Unresolved，**需人工判定**
//	文本里本来就没有                   -> 正常缺失，只计入统计
//
// 后者占大多数：387 个技能没有伤害倍率，其中 156 个是被动，
// 其余 231 个是治疗、控制、增益类技能——**它们本来就不该有伤害倍率**。
// 把这两种混为一谈会得出「抽取率只有 48%，工具不行」的错误结论，
// 而事实是抽取器正确地区分了「没有」和「不能确定」。

package ingest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ngnl5/ssot/internal/domain/value"
)

// Unresolved 是一条**文本里有、但无法确定**的项。
//
// 它与「没有这个数据」是两回事，必须显式报告。
//
// 关键点：它必须**携带全部候选**（值 + 原文锚点 + 上下文），而不是只报一句
// 「有 N 个倍率」。只报数量的话，人打开界面看到的是一句抱怨，
// 还得自己回去翻原文——那就等于没把问题交出去。
type Unresolved struct {
	Entity    string
	Artifact  string
	Subject   string
	Predicate string
	// Revision 是源内容的修订标识。事项要跨修订稳定，但它必须记住
	// 「这次看到的是哪一版」——否则无法判断结论是否已经过期。
	Revision string
	// CapturedAt 是原件的采集时间，随事项一起留存，
	// 否则裁决产出的断言会把溯源指向「裁决那一刻」而不是「采集那一刻」。
	CapturedAt time.Time
	Reason     string
	// Context 是歧义所在的原文片段。
	Context string
	// Candidates 是全部说得通的取值，长度 ≥ 2。
	Candidates []Candidate
}

func (u Unresolved) String() string {
	return fmt.Sprintf("%s %s.%s：%s", u.Artifact, u.Subject, u.Predicate, u.Reason)
}

// SkillStats 是抽取的整体分布。
//
// 它回答的是「抽取器到底做了什么」，而不是「抽到了几条」——
// 没有这组数字，就无法判断 48% 的命中率是抽取不足还是数据本来如此。
type SkillStats struct {
	Skills  int
	Passive int

	RatioUnique   int // 一级倍率唯一命中
	RatioMultiple int // 一级倍率有多个候选（歧义）
	RatioNone     int // 一级倍率文本中没有

	MaxUnique    int // 满级倍率唯一命中
	MaxMultiple  int
	MaxNone      int // 文本中没有数值
	MaxNoUpgrade int // 该技能没有升级数据
}

// Add 累加另一次抽取的统计。
func (s *SkillStats) Add(o SkillStats) {
	s.Skills += o.Skills
	s.Passive += o.Passive
	s.RatioUnique += o.RatioUnique
	s.RatioMultiple += o.RatioMultiple
	s.RatioNone += o.RatioNone
	s.MaxUnique += o.MaxUnique
	s.MaxMultiple += o.MaxMultiple
	s.MaxNone += o.MaxNone
	s.MaxNoUpgrade += o.MaxNoUpgrade
}

// SkillExtract 是一次技能抽取的结果。
type SkillExtract struct {
	Candidates []Candidate
	Unresolved []Unresolved
	Stats      SkillStats
}

// 伤害倍率：仅在 `造成攻击N%伤害` 这一确定形式下抽取。
var reDamageRatio = regexp.MustCompile(`造成攻击(\d+(?:\.\d+)?)%伤害`)

// 升级倍率：`伤害增加至N%` / `伤害增至N%`。
//
// 刻意不匹配 `提升至`——那常出现在条件分支里（如「若目标生命值低于60%…提升至200%」），
// 与主倍率不是一回事。
var reUpgradeRatio = regexp.MustCompile(`伤害增(?:加)?至(\d+(?:\.\d+)?)%`)

type skillRecord struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Cost        json.Number    `json:"cost"`
	Passive     bool           `json:"passive"`
	Description string         `json:"description"`
	Upgrades    []skillUpgrade `json:"upgrades"`
}

type skillUpgrade struct {
	Level  int    `json:"level"`
	Effect string `json:"effect"`
}

type characterFile struct {
	ID     json.Number   `json:"id"`
	Name   jsonName      `json:"name"`
	Skills []skillRecord `json:"skills"`
}

type jsonName struct {
	CN string `json:"cn"`
	JP string `json:"jp"`
}

// SkillsJSON 从 Data:Character/<id>.json 中抽取技能与倍率。
func SkillsJSON(artifactName string, body []byte, revision string, capturedAt time.Time) (SkillExtract, error) {
	var out SkillExtract
	var f characterFile
	if err := json.Unmarshal(body, &f); err != nil {
		return out, fmt.Errorf("解析 %s 失败：%w", artifactName, err)
	}
	charID := f.ID.String()
	if charID == "" {
		return out, fmt.Errorf("%s 缺少 id", artifactName)
	}

	add := func(subject, predicate string, v value.Value, anchor string) {
		out.Candidates = append(out.Candidates, Candidate{
			Entity: "skill", Subject: subject, Predicate: predicate, Value: v,
			Artifact: artifactName, Anchor: anchor, Revision: revision,
			Parsing: ParsingText, // 文本抽取 -> 最高 L2
		})
	}

	for i, sk := range f.Skills {
		if sk.ID == "" {
			continue
		}
		out.Stats.Skills++
		if sk.Passive {
			out.Stats.Passive++
		}

		base := fmt.Sprintf("skills[%d]", i)
		add(sk.ID, "id", value.Of(sk.ID), base+".id")
		add(sk.ID, "character_id", value.OfUnit(parseNum(charID), "point"), base+".id")
		if sk.Name != "" {
			add(sk.ID, "name", value.Of(sk.Name), base+".name")
		}
		if sk.Cost.String() != "" {
			if n, err := sk.Cost.Float64(); err == nil {
				add(sk.ID, "cost", value.OfUnit(n, "point"), base+".cost")
			}
		}
		add(sk.ID, "passive", value.Of(sk.Passive), base+".passive")

		// 一级倍率
		matches := reDamageRatio.FindAllStringSubmatchIndex(sk.Description, -1)
		cands := ratioCandidates("skill", sk.ID, "ratio", artifactName, revision,
			base+".description", sk.Description, matches)
		switch len(cands) {
		case 0:
			// 文本里没有伤害倍率。被动与治疗/控制类技能属正常情况，
			// **不报未解决**——「没有」不是「不确定」。
			out.Stats.RatioNone++
		case 1:
			out.Stats.RatioUnique++
			add(sk.ID, "ratio", cands[0].Value, base+".description")
		default:
			// 多个**不同**的倍率 —— 不得猜哪个是「the ratio」，
			// 但必须把每一个连同上下文一起交出去。
			out.Stats.RatioMultiple++
			out.Unresolved = append(out.Unresolved, Unresolved{
				Entity: "skill", Artifact: artifactName, Subject: sk.ID, Predicate: "ratio",
				Revision: revision, CapturedAt: capturedAt,
				Reason: fmt.Sprintf("描述中出现 %d 个不同的伤害倍率（%s），无法确定取哪一个",
					len(cands), joinValues(cands)),
				Context:    sk.Description,
				Candidates: cands,
			})
		}

		// 满级倍率
		if n := len(sk.Upgrades); n == 0 {
			out.Stats.MaxNoUpgrade++
		} else {
			last := sk.Upgrades[n-1]
			anchor := fmt.Sprintf("%s.upgrades[%d].effect", base, n-1)
			ups := reUpgradeRatio.FindAllStringSubmatchIndex(last.Effect, -1)
			cands := ratioCandidates("skill", sk.ID, "ratio_max", artifactName, revision,
				anchor, last.Effect, ups)
			switch len(cands) {
			case 0:
				out.Stats.MaxNone++
			case 1:
				out.Stats.MaxUnique++
				add(sk.ID, "ratio_max", cands[0].Value, anchor)
			default:
				out.Stats.MaxMultiple++
				out.Unresolved = append(out.Unresolved, Unresolved{
					Entity: "skill", Artifact: artifactName, Subject: sk.ID, Predicate: "ratio_max",
					Revision: revision, CapturedAt: capturedAt,
					Reason: fmt.Sprintf("满级效果中出现 %d 个不同的伤害数值（%s），无法确定取哪一个",
						len(cands), joinValues(cands)),
					Context:    last.Effect,
					Candidates: cands,
				})
			}
		}
	}
	return out, nil
}

// ratioCandidates 把正则命中变成候选，并按**取值**去重。
//
// 关键区分：歧义是「有多个不同的取值」，不是「命中了很多次」。
// 实测踩过：「每次造成攻击50%伤害，若…则额外造成攻击50%伤害」命中两次、
// 取值相同——那不是歧义，那就是 50%。按命中次数判定会让这类技能
// 永远卡在待判定里，白白消耗人力。
//
// 去重后保留**首次出现**的位置，锚点因此指向主叙述那一次。
func ratioCandidates(entity, subject, predicate, artifact, revision, anchorBase, text string, matches [][]int) []Candidate {
	var out []Candidate
	seen := map[float64]bool{}
	for k, m := range matches {
		n := parseNum(text[m[2]:m[3]])
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, Candidate{
			Entity: entity, Subject: subject, Predicate: predicate,
			Value:    value.OfUnit(n, "percent"),
			Artifact: artifact,
			Anchor:   fmt.Sprintf("%s#m%d", anchorBase, k),
			Context:  contextAround(text, m[0], m[1]),
			Parsing:  ParsingText,
			Revision: revision,
		})
	}
	return out
}

// contextAround 返回匹配位置周围的原文片段。
//
// 分两级收窄，因为这两件事都要满足：
//
//	只给一个数字        -> 人无法判断
//	给整段技能描述      -> 几个候选的上下文长得一模一样，人还是无法判断
//
// 因此先按句读（。；\n 等）取整句；整句超过 40 字时再按逗号收到分句。
// 实测效果：「每次造成攻击33%伤害，最后对敌方目标劈斩造成攻击88%伤害」
// 会被切成「每次造成攻击33%伤害，」与「最后对敌方目标劈斩造成攻击88%伤害。」——
// 两个候选一眼可辨。
func contextAround(text string, start, end int) string {
	rs := []rune(text)
	lo := utf8.RuneCountInString(text[:start])
	hi := utf8.RuneCountInString(text[:end])

	const (
		clauseLen = 40  // 超过这个长度就按逗号再收一次
		maxLen    = 120 // 硬上限
	)
	left, right := 0, len(rs)
	for i := lo - 1; i >= 0; i-- {
		if isBreak(rs[i]) {
			left = i + 1
			break
		}
	}
	for i := hi; i < len(rs); i++ {
		if isBreak(rs[i]) {
			right = i + 1
			break
		}
	}
	if right-left > clauseLen {
		if l, ok := lastComma(rs, left, lo); ok {
			left = l
		}
		if r, ok := firstComma(rs, hi, right); ok {
			right = r
		}
	}
	if right-left > maxLen {
		pad := (maxLen - (hi - lo)) / 2
		left = max(lo-pad, left)
		right = min(left+maxLen, len(rs))
		if right < hi {
			right = hi
			left = max(right-maxLen, 0)
		}
	}
	return strings.TrimSpace(string(rs[left:right]))
}

// lastComma 返回 [from, before) 内最后一个逗号之后的位置。
func lastComma(rs []rune, from, before int) (int, bool) {
	for i := before - 1; i >= from; i-- {
		if isComma(rs[i]) {
			return i + 1, true
		}
	}
	return 0, false
}

// firstComma 返回 [from, before) 内第一个逗号之后的位置。
func firstComma(rs []rune, from, before int) (int, bool) {
	for i := from; i < before; i++ {
		if isComma(rs[i]) {
			return i + 1, true
		}
	}
	return 0, false
}

func isComma(r rune) bool { return r == '，' || r == ',' }

func isBreak(r rune) bool { return r == '。' || r == '；' || r == '\n' || r == '！' || r == '？' }

// joinValues 把候选数值连成一行，供人读的「原因」使用。
func joinValues(cands []Candidate) string {
	var b []byte
	for i, c := range cands {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = append(b, fmt.Sprintf("%v%%", c.Value.Data)...)
	}
	return string(b)
}

func parseNum(s string) float64 {
	var n float64
	fmt.Sscanf(s, "%g", &n)
	return n
}
