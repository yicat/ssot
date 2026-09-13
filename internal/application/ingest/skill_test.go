package ingest

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
)

// 真实数据结构（姑获鸟，Data:Character/262.json）。
// 三个技能分别代表三种情形：可抽、不得抽、无法确定。
const guhuoniao = `{
  "id": 262,
  "name": {"cn": "姑获鸟"},
  "skills": [
    {"id":"262_01","name":"伞剑","cost":0,
     "description":"以肉眼无法捕捉的速度迅速拔出藏在腰间的伞挥砍敌方目标，造成攻击80%伤害并无视20%防御。",
     "upgrades":[{"level":2,"effect":"伤害增加至84%"},
                 {"level":3,"effect":"伤害增加至88%"},
                 {"level":4,"effect":"伤害增加至92%"},
                 {"level":5,"effect":"伤害增加至96%"}]},
    {"id":"262_02","name":"协战","cost":-1,"passive":true,
     "description":"30%概率[tips_9]。<br>若自身总攻击是友方非召唤物单位中最高时，自身获得[buff_2622_1]。",
     "upgrades":[]},
    {"id":"262_03","name":"天翔鹤斩","cost":3,
     "description":"以伞为剑，划出凌冽的剑气攻击敌方全体3次，每次造成攻击33%伤害，最后对敌方目标劈斩造成攻击88%伤害。",
     "upgrades":[{"level":5,"effect":"全体伤害增至41%，劈斩伤害增至108%。若目标生命值低于60%则是最后1个目标，则劈斩伤害提升至200%"}]}
  ]
}`

// 多次命中但取值相同 —— **不是歧义**，就是那个值。
//
// 实测踩过：按「命中次数」判定歧义，会让「每次造成攻击50%伤害，若…则额外
// 造成攻击50%伤害」这类技能永远卡在待判定里，白白消耗人力。
func TestRepeatedSameValueIsNotAmbiguous(t *testing.T) {
	const same = `{
	  "id": 302,
	  "name": {"cn": "同类"},
	  "skills": [
	    {"id":"302_03","name":"两段同值","cost":3,
	     "description":"对敌方目标造成攻击50%伤害；若目标已被减速，则追加造成攻击50%伤害。",
	     "upgrades":[]}
	  ]
	}`
	ex, err := SkillsJSON("Data:Character/302.json", []byte(same), "revid:1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	n, ok := valueOf(ex, "302_03", "ratio")
	if !ok {
		t.Fatalf("命中两次但取值相同时应正常抽出，实际未解决项 %+v", ex.Unresolved)
	}
	if n != 50 {
		t.Errorf("倍率应为 50，实际 %v", n)
	}
	for _, u := range ex.Unresolved {
		if u.Subject == "302_03" && u.Predicate == "ratio" {
			t.Error("取值相同不得记为歧义")
		}
	}
	if ex.Stats.RatioMultiple != 0 {
		t.Errorf("多值歧义计数应为 0，实际 %d", ex.Stats.RatioMultiple)
	}
	if ex.Stats.RatioUnique != 1 {
		t.Errorf("唯一命中计数应为 1，实际 %d", ex.Stats.RatioUnique)
	}
	// 锚点指向**首次出现**的那一次，也就是主叙述
	for _, c := range ex.Candidates {
		if c.Subject == "302_03" && c.Predicate == "ratio" {
			if c.Anchor != "skills[0].description" {
				t.Errorf("锚点应指向主叙述那一次，实际 %s", c.Anchor)
			}
		}
	}
}

func extract(t *testing.T) SkillExtract {
	t.Helper()
	ex, err := SkillsJSON("Data:Character/262.json", []byte(guhuoniao), "revid:8053", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func valueOf(ex SkillExtract, subject, predicate string) (float64, bool) {
	for _, c := range ex.Candidates {
		if c.Subject == subject && c.Predicate == predicate {
			n, err := c.Value.Number()
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// 唯一确定时才抽：伞剑的 80% 可以抽。
func TestExtractsUnambiguousRatio(t *testing.T) {
	ex := extract(t)
	n, ok := valueOf(ex, "262_01", "ratio")
	if !ok {
		t.Fatal("伞剑的一级倍率应被抽出")
	}
	if n != 80 {
		t.Errorf("伞剑一级倍率应为 80，实际 %v", n)
	}
	// 满级倍率取自最后一条 upgrade（96%），
	// 且刻意不匹配「提升至200%」那种条件分支
	max, ok := valueOf(ex, "262_01", "ratio_max")
	if !ok {
		t.Fatal("伞剑的满级倍率应被抽出")
	}
	if max != 96 {
		t.Errorf("伞剑满级倍率应为 96，实际 %v", max)
	}
}

// 被动技能的「30%概率」不是伤害倍率 —— 不得抽取。
func TestPassivePercentageIsNotExtracted(t *testing.T) {
	ex := extract(t)
	if _, ok := valueOf(ex, "262_02", "ratio"); ok {
		t.Error("被动技能的 30%概率 不得被当作伤害倍率")
	}
	if ex.Stats.Passive != 1 {
		t.Errorf("应统计到 1 个被动技能，实际 %d", ex.Stats.Passive)
	}
}

// 多个倍率时**不得猜**，必须记为未解决并给出候选值。
func TestMultipleRatiosAreUnresolvedNotGuessed(t *testing.T) {
	ex := extract(t)
	if _, ok := valueOf(ex, "262_03", "ratio"); ok {
		t.Error("存在两个伤害倍率时不得猜一个")
	}
	found := false
	for _, u := range ex.Unresolved {
		if u.Subject == "262_03" && u.Predicate == "ratio" {
			found = true
			// 报告里应能看到候选值，便于人工判定
			if u.Reason == "" {
				t.Error("未解决项必须说明原因")
			}
		}
	}
	if !found {
		t.Errorf("多倍率情形应记为未解决，实际 %+v", ex.Unresolved)
	}
}

// 未解决项必须携带**全部候选**（值 + 锚点 + 上下文），而不是只报一句数量。
//
// 只报数量的话，人打开界面看到的是一句抱怨，还得自己回去翻原文——
// 那就等于没把问题交出去。
func TestUnresolvedCarriesAllCandidatesWithContext(t *testing.T) {
	ex := extract(t)
	var target *Unresolved
	for i := range ex.Unresolved {
		if ex.Unresolved[i].Subject == "262_03" && ex.Unresolved[i].Predicate == "ratio" {
			target = &ex.Unresolved[i]
		}
	}
	if target == nil {
		t.Fatal("应有 262_03 的一级倍率歧义项")
	}
	if len(target.Candidates) < 2 {
		t.Fatalf("歧义项必须携带全部候选，实际 %d 个", len(target.Candidates))
	}
	seen := map[string]bool{}
	for _, c := range target.Candidates {
		if c.Anchor == "" {
			t.Error("候选缺少原文锚点")
		}
		if c.Context == "" {
			t.Errorf("候选 %s 缺少上下文片段——只给一个数字人无法判断", c.Value.String())
		}
		if !strings.Contains(c.Context, "伤害") {
			t.Errorf("上下文里应含判定所需的原文，实际 %q", c.Context)
		}
		if c.Parsing != ParsingText {
			t.Errorf("候选解析方式应为 text，实际 %s", c.Parsing)
		}
		k := c.Value.String()
		if seen[k] {
			t.Errorf("候选 %s 重复出现", k)
		}
		seen[k] = true
	}
	if target.Context == "" {
		t.Error("歧义项应带原文片段")
	}
}

// 升级文本里出现多个数值（含条件分支的「提升至200%」）时同样不得猜。
func TestMultipleUpgradeValuesAreUnresolved(t *testing.T) {
	ex := extract(t)
	if _, ok := valueOf(ex, "262_03", "ratio_max"); ok {
		t.Error("满级效果含多个数值时不得猜")
	}
}

// 抽取自文本，因此最高只能是 L2 —— 回退与推测不得定为 L1。
func TestSkillCandidatesAreL2AtMost(t *testing.T) {
	ex := extract(t)
	if len(ex.Candidates) == 0 {
		t.Fatal("应有候选产出")
	}
	for _, c := range ex.Candidates {
		if c.Parsing != ParsingText {
			t.Errorf("%s 的解析方式应为 text，实际 %s", c.Predicate, c.Parsing)
		}
		if c.Parsing.MaxConfidence() == assertion.L1 {
			t.Errorf("%s 不得定为 L1", c.Predicate)
		}
	}
}

// 溯源必须指回原文位置。
func TestCandidatesCarryAnchor(t *testing.T) {
	ex := extract(t)
	for _, c := range ex.Candidates {
		if c.Artifact == "" || c.Anchor == "" || c.Revision == "" {
			t.Errorf("%s.%s 缺少溯源（artifact=%q anchor=%q revision=%q）",
				c.Subject, c.Predicate, c.Artifact, c.Anchor, c.Revision)
		}
	}
	for _, c := range ex.Candidates {
		if c.Predicate == "ratio" && c.Subject == "262_01" {
			if c.Anchor != "skills[0].description" {
				t.Errorf("倍率的锚点应指向 description，实际 %q", c.Anchor)
			}
		}
	}
}

// 「文本里没有」不是「无法确定」—— 统计要分开，未解决清单里不该出现。
func TestAbsenceIsNotUnresolved(t *testing.T) {
	ex := extract(t)
	for _, u := range ex.Unresolved {
		if u.Subject == "262_02" {
			t.Errorf("被动技能没有倍率属正常缺失，不应进未解决清单：%s", u)
		}
	}
	if ex.Stats.RatioNone == 0 {
		t.Error("应有技能被统计为「文本无倍率」")
	}
	if ex.Stats.RatioUnique != 1 {
		t.Errorf("应有 1 个技能唯一命中倍率，实际 %d", ex.Stats.RatioUnique)
	}
}

func hasCand(ex SkillExtract, subject, predicate string) bool {
	for _, c := range ex.Candidates {
		if c.Subject == subject && c.Predicate == predicate {
			return true
		}
	}
	return false
}

// 引用与身份字段必须齐全，否则准入会因缺必填而拒绝整条记录。
func TestMandatoryFieldsPresent(t *testing.T) {
	ex := extract(t)
	for _, sk := range []string{"262_01", "262_02", "262_03"} {
		for _, p := range []string{"id", "character_id", "name", "cost", "passive"} {
			if !hasCand(ex, sk, p) {
				t.Errorf("%s 缺少 %s", sk, p)
			}
		}
	}
	n, ok := valueOf(ex, "262_01", "character_id")
	if !ok || n != 262 {
		t.Errorf("character_id 应为 262，实际 %v (ok=%v)", n, ok)
	}
}
