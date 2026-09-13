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
	bad := "scenario: x\nrequires: [游戏理解]\n"
	if _, err := parseSpec(bad); err == nil {
		t.Error("无法判定的 requires 项必须被拒绝")
	}
}