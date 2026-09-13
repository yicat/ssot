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
	"time"

	"github.com/ngnl5/ssot/internal/domain/value"
)

// Unresolved 是一条**文本里有、但无法确定**的项。
//
// 它与「没有这个数据」是两回事，必须显式报告并进入人工队列。
type Unresolved struct {
	Artifact  string
	Subject   string
	Predicate string
	Reason    string
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
		matches := reDamageRatio.FindAllStringSubmatch(sk.Description, -1)
		switch len(matches) {
		case 0:
			// 文本里没有伤害倍率。被动与治疗/控制类技能属正常情况，
			// **不报未解决**——「没有」不是「不确定」。
			out.Stats.RatioNone++
		case 1:
			out.Stats.RatioUnique++
			add(sk.ID, "ratio", value.OfUnit(parseNum(matches[0][1]), "percent"), base+".description")
		default:
			// 多个倍率 —— 不得猜哪个是「the ratio」
			out.Stats.RatioMultiple++
			out.Unresolved = append(out.Unresolved, Unresolved{
				Artifact: artifactName, Subject: sk.ID, Predicate: "ratio",
				Reason: fmt.Sprintf("描述中出现 %d 个伤害倍率（%s），无法确定取哪一个",
					len(matches), joinMatches(matches)),
			})
		}

		// 满级倍率
		if n := len(sk.Upgrades); n == 0 {
			out.Stats.MaxNoUpgrade++
		} else {
			last := sk.Upgrades[n-1]
			anchor := fmt.Sprintf("%s.upgrades[%d].effect", base, n-1)
			ups := reUpgradeRatio.FindAllStringSubmatch(last.Effect, -1)
			switch len(ups) {
			case 0:
				out.Stats.MaxNone++
			case 1:
				out.Stats.MaxUnique++
				add(sk.ID, "ratio_max", value.OfUnit(parseNum(ups[0][1]), "percent"), anchor)
			default:
				out.Stats.MaxMultiple++
				out.Unresolved = append(out.Unresolved, Unresolved{
					Artifact: artifactName, Subject: sk.ID, Predicate: "ratio_max",
					Reason: fmt.Sprintf("满级效果中出现 %d 个伤害数值（%s），无法确定取哪一个",
						len(ups), joinMatches(ups)),
				})
			}
		}
	}
	return out, nil
}

func joinMatches(ms [][]string) string {
	var b []byte
	for i, m := range ms {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = append(b, m[1]...)
		b = append(b, '%')
	}
	return string(b)
}

func parseNum(s string) float64 {
	var n float64
	fmt.Sscanf(s, "%g", &n)
	return n
}
