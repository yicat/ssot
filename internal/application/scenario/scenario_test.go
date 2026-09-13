package scenario

import (
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// fakeStore 模拟断言库的统计能力。
type fakeStore struct {
	predCount map[string]int
	subjects  int
}

func (f fakeStore) PredicateCount(entity, predicate string) (int, error) {
	return f.predCount[predicate], nil
}
func (f fakeStore) SubjectCount(entity string) (int, error) { return f.subjects, nil }

// fakeReader 模拟按主体取断言。
type fakeReader struct {
	byPred map[string]assertion.Assertion
}

func (f fakeReader) BySubject(entity, subject string) ([]assertion.Assertion, error) {
	out := make([]assertion.Assertion, 0, len(f.byPred))
	for _, a := range f.byPred {
		out = append(out, a)
	}
	return out, nil
}

func mkAssertion(pred string, v value.Value, st assertion.Status) assertion.Assertion {
	return assertion.Assertion{
		ID: "id-" + pred, Entity: "shikigami", Subject: "262", Predicate: pred, Value: v,
		Source: assertion.Source{Name: "test"},
		Provenance: assertion.Provenance{
			Artifact: "a.json", Anchor: "x", Revision: "1", CapturedAt: time.Now(),
		},
		Confidence: assertion.L1, Status: st,
	}
}

const dmgFormula = `
formula: damage
version: "1"
params:
  atk:           {type: number, unit: point}
  ratio:         {type: number, unit: percent}
  crit_factor:   {type: number, unit: fraction}
  def_reduction: {type: number, unit: fraction}
bindings:
  atk: atk
  ratio: ratio
  crit_factor: crit_factor
  def_reduction: def_reduction
result: atk * ratio * crit_factor * def_reduction
cases:
  - name: 基本
    given:
      atk: "1000 point"
      ratio: "100 percent"
      crit_factor: "1 fraction"
      def_reduction: "1 fraction"
    expect: "1000 point"
`

func spec() Spec {
	return Spec{
		Name:     "damage-calc",
		Requires: []string{"shikigami.atk", "shikigami.crit_factor"},
		Inputs: []Input{
			{Name: "ratio", Description: "技能倍率"},
			{Name: "def_reduction", Description: "防御减免"},
		},
		Formulas: []string{"damage"},
	}
}

func loadFormula(t *testing.T) *formula.Formula {
	t.Helper()
	f, err := formula.Parse(dmgFormula, unit.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// requires 缺失 -> 不可运行，且必须列出缺什么。
func TestMissingRequirementBlocksRun(t *testing.T) {
	st := fakeStore{predCount: map[string]int{"atk": 268}, subjects: 268} // 缺 crit_factor
	rep, err := Check(spec(), st, map[string]*formula.Formula{"damage": loadFormula(t)}, unit.Default())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Runnable() {
		t.Fatal("requires 缺失时不得判定为可运行")
	}
	missing := rep.Missing()
	if len(missing) == 0 || !strings.Contains(strings.Join(missing, " "), "crit_factor") {
		t.Errorf("缺失清单应指出 crit_factor，实际 %v", missing)
	}
}

func TestAllRequirementsSatisfied(t *testing.T) {
	st := fakeStore{predCount: map[string]int{"atk": 268, "crit_factor": 268}, subjects: 268}
	rep, err := Check(spec(), st, map[string]*formula.Formula{"damage": loadFormula(t)}, unit.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Runnable() {
		t.Fatalf("全部满足时应可运行，缺失 %v", rep.Missing())
	}
	for _, q := range rep.Requirements {
		if q.Status != Satisfied {
			t.Errorf("%s 应为 satisfied，实际 %s", q.Want, q.Status)
		}
	}
}

// 公式算例未通过 -> 场景不可运行。
func TestFailedFormulaBlocksScenario(t *testing.T) {
	bad := strings.Replace(dmgFormula, `expect: "1000 point"`, `expect: "1 point"`, 1)
	f, err := formula.Parse(bad, unit.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	st := fakeStore{predCount: map[string]int{"atk": 1, "crit_factor": 1}, subjects: 1}
	rep, err := Check(spec(), st, map[string]*formula.Formula{"damage": f}, unit.Default())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Runnable() {
		t.Error("公式算例未通过时场景不得运行")
	}
}

// 外部输入与库中数据是两回事：inputs 不该被当成缺失的 requires。
func TestInputsAreDeclaredSeparately(t *testing.T) {
	s := spec()
	if !s.IsInput("ratio") {
		t.Error("ratio 应被识别为声明的外部输入")
	}
	if s.IsInput("atk") {
		t.Error("atk 是库中数据，不是外部输入")
	}
}

// 库中数据 + 外部输入共同求值。
func TestExecuteCombinesStoreAndInputs(t *testing.T) {
	r := fakeReader{byPred: map[string]assertion.Assertion{
		"atk":         mkAssertion("atk", value.OfUnit(float64(1000), "point"), assertion.StatusVerified),
		"crit_factor": mkAssertion("crit_factor", value.OfUnit(float64(1), "fraction"), assertion.StatusVerified),
	}}
	in := RunInput{Entity: "shikigami", Subject: "262", Values: map[string]string{
		"ratio":         "100 percent",
		"def_reduction": "1 fraction",
	}}
	out, err := Execute(spec(), loadFormula(t), r, unit.Default(), in, formula.StatusVerified)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result.Num != 1000 {
		t.Errorf("结果应为 1000，实际 %v", out.Result)
	}
	// 外部输入必须被标注为未核验
	if len(out.Unverified) != 2 {
		t.Errorf("两项外部输入都应标注未核验，实际 %v", out.Unverified)
	}
	joined := strings.Join(out.Notes, " ")
	if !strings.Contains(joined, "未核验") {
		t.Errorf("产出注释应说明未核验，实际 %v", out.Notes)
	}
}

// 输入为未知时拒绝产出 —— 不以不完整数据出方案。
func TestUnknownInputRefusesToProduce(t *testing.T) {
	r := fakeReader{byPred: map[string]assertion.Assertion{
		"atk":         mkAssertion("atk", value.UnknownValue(), assertion.StatusPending),
		"crit_factor": mkAssertion("crit_factor", value.OfUnit(float64(1), "fraction"), assertion.StatusPending),
	}}
	in := RunInput{Entity: "shikigami", Subject: "262", Values: map[string]string{
		"ratio": "100 percent", "def_reduction": "1 fraction",
	}}
	if _, err := Execute(spec(), loadFormula(t), r, unit.Default(), in, formula.StatusVerified); err == nil {
		t.Error("输入为未知时必须拒绝产出")
	}
}

// 未核验的库中数据仍可用于产出，但必须标注。
func TestUnverifiedStoreDataIsAnnotated(t *testing.T) {
	r := fakeReader{byPred: map[string]assertion.Assertion{
		"atk":         mkAssertion("atk", value.OfUnit(float64(1000), "point"), assertion.StatusPending),
		"crit_factor": mkAssertion("crit_factor", value.OfUnit(float64(1), "fraction"), assertion.StatusVerified),
	}}
	in := RunInput{Entity: "shikigami", Subject: "262", Values: map[string]string{
		"ratio": "100 percent", "def_reduction": "1 fraction",
	}}
	out, err := Execute(spec(), loadFormula(t), r, unit.Default(), in, formula.StatusVerified)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(out.Notes, " "), "pending") {
		t.Errorf("应标注库中数据未核验，实际 %v", out.Notes)
	}
}

// 公式无算例时，产出必须标注。
func TestUnverifiedFormulaIsAnnotated(t *testing.T) {
	r := fakeReader{byPred: map[string]assertion.Assertion{
		"atk":         mkAssertion("atk", value.OfUnit(float64(1000), "point"), assertion.StatusVerified),
		"crit_factor": mkAssertion("crit_factor", value.OfUnit(float64(1), "fraction"), assertion.StatusVerified),
	}}
	in := RunInput{Entity: "shikigami", Subject: "262", Values: map[string]string{
		"ratio": "100 percent", "def_reduction": "1 fraction",
	}}
	out, err := Execute(spec(), loadFormula(t), r, unit.Default(), in, formula.StatusUnverified)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(out.Notes, " "), "未验证") {
		t.Errorf("公式未验证时必须标注，实际 %v", out.Notes)
	}
}

func TestLoadRejectsUnjudgeableRequirement(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\nrequires: [游戏理解]\n"
	if _, err := parseSpec(bad); err == nil {
		t.Error("无法判定的 requires 项必须被拒绝")
	}
}

// 外部输入必须声明单位：`def_reduction=0.5` 到底是一半还是 0.5%，
// 光看数字无从判断。没有量纲的输入框就是一个歧义制造机。
func TestInputRequiresUnit(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\ninputs:\n  - name: def_reduction\n    description: 防御减免\n"
	if _, err := parseSpec(bad); err == nil {
		t.Fatal("外部输入缺 unit 必须被拒绝")
	} else if !strings.Contains(err.Error(), "unit") {
		t.Errorf("报错应指明缺的是 unit，实际：%v", err)
	}

	// 声明了单位即可加载
	ok := "scenario: x\nentity: shikigami\ninputs:\n  - name: def_reduction\n    unit: fraction\n    min: 0\n    max: 1\n"
	s, err := parseSpec(ok)
	if err != nil {
		t.Fatalf("声明了单位应当可加载：%v", err)
	}
	if len(s.Inputs) != 1 || s.Inputs[0].Unit != "fraction" {
		t.Errorf("单位应被读出，实际 %+v", s.Inputs)
	}
	if s.Inputs[0].Min == nil || *s.Inputs[0].Min != 0 {
		t.Errorf("min 应被读出，实际 %+v", s.Inputs[0].Min)
	}
}

// 声明了范围却不合法：min > max 必须在加载期就被拒绝。
func TestInputRangeMustBeSane(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\ninputs:\n  - name: a\n    unit: fraction\n    min: 2\n    max: 1\n"
	if _, err := parseSpec(bad); err == nil {
		t.Error("min 大于 max 必须被拒绝——声明了不校验比不声明更糟")
	}
}

// 主实体必须写清楚：在代码里写死会让换主实体变成改程序。
func TestSpecRequiresPrimaryEntity(t *testing.T) {
	if _, err := parseSpec("scenario: x\nrequires: [shikigami.id]\n"); err == nil {
		t.Fatal("缺 entity 必须被拒绝")
	} else if !strings.Contains(err.Error(), "entity") {
		t.Errorf("报错应指明缺的是 entity，实际：%v", err)
	}
}

// 引用必须声明 via：没有它就无法列出候选，界面只能让人凭记忆敲 ID。
func TestRefDeclRequiresVia(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\nrefs:\n  - name: ratio\n    entity: skill\n"
	if _, err := parseSpec(bad); err == nil {
		t.Fatal("引用缺 via 必须被拒绝")
	} else if !strings.Contains(err.Error(), "via") {
		t.Errorf("报错应指明缺的是 via，实际：%v", err)
	}

	ok := "scenario: x\nentity: shikigami\nrefs:\n  - name: ratio\n    entity: skill\n    via: character_id\n"
	s, err := parseSpec(ok)
	if err != nil {
		t.Fatalf("声明完整时应当可加载：%v", err)
	}
	r, found := s.RefByName("ratio")
	if !found || r.Entity != "skill" || r.Via != "character_id" {
		t.Errorf("引用应被读出，实际 %+v", r)
	}
}

// 同一个绑定路径只能有一个来源：既声明为外部输入又声明为引用会让人无从判断。
func TestRefAndInputCannotShareName(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\n" +
		"inputs:\n  - name: ratio\n    unit: percent\n" +
		"refs:\n  - name: ratio\n    entity: skill\n    via: character_id\n"
	if _, err := parseSpec(bad); err == nil {
		t.Error("引用与外部输入同名必须被拒绝")
	}
}

// 引用名重复会让「取哪一个」变成一件含糊的事。
func TestRefNameMustBeUnique(t *testing.T) {
	bad := "scenario: x\nentity: shikigami\n" +
		"refs:\n  - name: ratio\n    entity: skill\n    via: character_id\n" +
		"  - name: ratio\n    entity: skill\n    via: character_id\n"
	if _, err := parseSpec(bad); err == nil {
		t.Error("引用名重复必须被拒绝")
	}
}

// requires 未满足时 Run 拒绝执行——不得用不完整数据产出方案。
func TestRunRefusesWhenRequirementsMissing(t *testing.T) {
	spec := Spec{Name: "x", Entity: "shikigami", Requires: []string{"shikigami.atk"}}
	out, rep, err := Run(spec, nil, missingStore{}, nil, unit.Default(),
		RunInput{Entity: "shikigami", Subject: "262"})
	if err == nil {
		t.Fatal("requires 未满足时必须拒绝运行")
	}
	if !strings.Contains(err.Error(), "拒绝运行") {
		t.Errorf("报错应说清是拒绝运行，实际：%v", err)
	}
	if out.Scenario != "" {
		t.Errorf("被拒绝时不得产出结果，实际场景 %q", out.Scenario)
	}
	if len(rep.Missing()) == 0 {
		t.Error("拒绝时必须同时给出缺失清单，否则人不知道该补什么")
	}
}

// missingStore 报告「该谓词完全没有数据」。
type missingStore struct{}

func (missingStore) PredicateCount(string, string) (int, error) { return 0, nil }
func (missingStore) SubjectCount(string) (int, error)           { return 100, nil }
