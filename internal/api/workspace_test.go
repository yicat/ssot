package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// workspaceProject 造一个带场景与公式的最小项目。
//
// 结构与真实项目一致：**主实体是 shikigami，技能挂在它下面**。
// 把主实体写成 skill 会让「引用该指向谁」这件事失真，
// 而那正是本组测试要覆盖的地方。
func workspaceProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "probe")

	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("project.yml", "project: probe\ndescription: 探针\nmetamodelVersion: 1\n")
	write("units.yml", "units: []\n")
	write("schema/shikigami.schema.yml", `
entity: shikigami
description: 式神
metamodelVersion: 1
fields:
  - key: id
    type: number
    unit: point
    identity: true
    required: true
  - key: atk
    type: number
    unit: point
    required: true
  - key: voice
    type: text
`)
	write("schema/skill.schema.yml", `
entity: skill
description: 技能
metamodelVersion: 1
fields:
  - key: id
    type: text
    identity: true
    required: true
  - key: character_id
    type: ref
    target: shikigami
    required: true
  - key: ratio
    type: number
    unit: percent
`)
	write("scenarios/calc/calc.scenario.yml", `
scenario: calc
description: 算一下
entity: shikigami
requires:
  - shikigami.atk
inputs:
  - name: damping
    description: 衰减
    unit: fraction
    min: 0
    max: 1
refs:
  - name: ratio
    entity: skill
    via: character_id
    description: 技能倍率
formulas:
  - scale
outputs:
  - scaled
`)
	write("formulas/scale.formula.yml", `
formula: scale
version: "1"
description: 缩放
params:
  atk:
    type: number
    unit: point
  damping:
    type: number
    unit: fraction
bindings:
  atk: atk
  damping: damping
result: atk * damping
cases:
  - name: 打对折
    given:
      atk: "3082 point"
      damping: "0.5 fraction"
    expect: "1541 point"
`)
	return dir
}

// seed 写入一组断言。这里直接写库：本组测的是接口层的规则，不是准入层。
func seed(t *testing.T, dir string, as ...assertion.Assertion) {
	t.Helper()
	p, err := compose.Load(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Store.Apply(assertion.ChangeSet{Insert: as}); err != nil {
		t.Fatal(err)
	}
}

func mk(entity, subject, predicate string, v value.Value) assertion.Assertion {
	return assertion.Assertion{
		ID:     "a-" + entity + "-" + subject + "-" + predicate,
		Entity: entity, Subject: subject, Predicate: predicate,
		Value:  v,
		Source: assertion.Source{Name: "test", Tier: "semi-official"},
		Provenance: assertion.Provenance{
			Artifact: "x.json", Anchor: "a", Revision: "revid:1", CapturedAt: time.Now(),
		},
		Confidence: assertion.L2, Status: assertion.StatusPending,
	}
}

func num(s string) float64 {
	var n float64
	if _, err := fmt.Sscanf(s, "%g", &n); err != nil {
		panic(err)
	}
	return n
}

func pct(v float64) *float64 { return &v }

// seedShikigami 写一个式神及其技能。ratio 为 nil 表示该技能没有倍率。
func seedShikigami(t *testing.T, dir, sid string, skills map[string]*float64) {
	t.Helper()
	as := []assertion.Assertion{
		mk("shikigami", sid, "id", value.OfUnit(num(sid), "point")),
		mk("shikigami", sid, "atk", value.OfUnit(3082.0, "point")),
	}
	for skillID, ratio := range skills {
		as = append(as,
			mk("skill", skillID, "id", value.Of(skillID)),
			mk("skill", skillID, "character_id", value.OfUnit(num(sid), "point")),
		)
		if ratio != nil {
			as = append(as, mk("skill", skillID, "ratio", value.OfUnit(*ratio, "percent")))
		}
	}
	seed(t, dir, as...)
}

func newWS(t *testing.T, dir string) (*compose.Session, func()) {
	t.Helper()
	sess := compose.NewSession(filepath.Dir(dir), dir)
	return sess, func() { _ = sess.Close() }
}

// ── 项目与会话 ──────────────────────────────────────────────────────────────

func TestProjectsListsOnlyRealProjects(t *testing.T) {
	dir := workspaceProject(t)
	sess, done := newWS(t, dir)
	defer done()

	refs, err := NewProjectService(sess).Projects()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Name != "probe" {
		t.Fatalf("应发现 1 个项目 probe，实际 %+v", refs)
	}
}

// 「一个都没有」不是「出错了」——两者在界面上必须能区分。
func TestProjectsEmptyIsNotAnError(t *testing.T) {
	sess := compose.NewSession(t.TempDir(), "")
	defer sess.Close()
	svc := NewProjectService(sess)

	refs, err := svc.Projects()
	if err != nil {
		t.Fatalf("空项目根目录不应报错：%v", err)
	}
	if len(refs) != 0 {
		t.Errorf("应返回空列表，实际 %d 项", len(refs))
	}
	if _, err := svc.Current(); err == nil {
		t.Error("没有可用项目时 Current 必须报错，而不是给一个空会话")
	}
}

// 切换失败时会话不变——否则界面会显示上一个项目的数据，而标题写着新项目。
func TestOpenFailureKeepsSession(t *testing.T) {
	dir := workspaceProject(t)
	sess, done := newWS(t, dir)
	defer done()
	svc := NewProjectService(sess)

	before, err := svc.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("不存在的目录必须被拒绝")
	}
	after, err := svc.Current()
	if err != nil {
		t.Fatalf("失败之后会话必须仍然可用：%v", err)
	}
	if after.Dir != before.Dir {
		t.Errorf("失败不得改变会话目录：%s → %s", before.Dir, after.Dir)
	}
}

// ── 场景概览 ────────────────────────────────────────────────────────────────

func TestScenarioOverviewIsPerRequirement(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	ov, err := NewScenarioService(sess).Overview("calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Requires) != 1 || ov.Requires[0].Want != "shikigami.atk" {
		t.Fatalf("应逐项列出 requires，实际 %+v", ov.Requires)
	}
	if len(ov.Inputs) != 1 || ov.Inputs[0].Unit != "fraction" {
		t.Errorf("外部输入必须带单位，实际 %+v", ov.Inputs)
	}
	if ov.Inputs[0].Min == nil || *ov.Inputs[0].Min != 0 {
		t.Errorf("范围约束应暴露给界面，实际 %+v", ov.Inputs[0])
	}
	if len(ov.Formulas) != 1 || ov.Formulas[0].Status != "verified" {
		t.Errorf("公式状态应为 verified，实际 %+v", ov.Formulas)
	}
}

func TestScenarioOverviewRejectsUnknown(t *testing.T) {
	dir := workspaceProject(t)
	sess, done := newWS(t, dir)
	defer done()
	if _, err := NewScenarioService(sess).Overview("nope"); err == nil {
		t.Error("未知场景必须被拒绝")
	}
}

// ── 数据质量 ────────────────────────────────────────────────────────────────

func TestDataQualityPointsAtFields(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	q, err := NewDataService(sess).Quality()
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 2 {
		t.Fatalf("应有 2 个实体的质量报告，实际 %d", len(q))
	}
	var shiki EntityQuality
	for _, x := range q {
		if x.Entity == "shikigami" {
			shiki = x
		}
	}
	// schema 声明了 voice，但库里不会有 —— schema → 数据的漂移
	found := false
	for _, u := range shiki.Unused {
		if u == "voice" {
			found = true
		}
	}
	if !found {
		t.Errorf("schema 声明而数据中从未出现的字段应被列出，实际 %+v", shiki.Unused)
	}
	if len(shiki.Fields) != 3 {
		t.Errorf("应有 3 个已声明字段，实际 %d：%+v", len(shiki.Fields), shiki.Fields)
	}
	for _, f := range shiki.Fields {
		if f.Key == "atk" && f.Coverage != 1 {
			t.Errorf("atk 覆盖率应为 1，实际 %v", f.Coverage)
		}
	}
}

// 数据里出现、schema 未声明的字段是**漂移**：它会被准入层过滤掉，
// 也就是「接进来了但没入库」。
func TestDataQualityReportsUndeclaredField(t *testing.T) {
	dir := workspaceProject(t)
	seed(t, dir,
		mk("shikigami", "262", "id", value.OfUnit(262.0, "point")),
		mk("shikigami", "262", "atk", value.OfUnit(3082.0, "point")),
		mk("shikigami", "262", "tags", value.Of("联动")),
	)
	sess, done := newWS(t, dir)
	defer done()

	q, err := NewDataService(sess).Quality()
	if err != nil {
		t.Fatal(err)
	}
	var shiki EntityQuality
	for _, x := range q {
		if x.Entity == "shikigami" {
			shiki = x
		}
	}
	found := false
	for _, u := range shiki.Undeclared {
		if u == "tags" {
			found = true
		}
	}
	if !found {
		t.Errorf("数据里有而 schema 未声明的字段应被列出，实际 %+v", shiki.Undeclared)
	}
}

func TestDataSchemaExposesRefTarget(t *testing.T) {
	dir := workspaceProject(t)
	sess, done := newWS(t, dir)
	defer done()

	es, err := NewDataService(sess).Schema()
	if err != nil {
		t.Fatal(err)
	}
	var target string
	for _, e := range es {
		if e.Entity != "skill" {
			continue
		}
		for _, f := range e.Fields {
			if f.Key == "character_id" {
				target = f.Target
			}
		}
	}
	if target != "shikigami" {
		t.Errorf("ref 字段必须暴露目标实体，实际 %q", target)
	}
}

// ── 公式 ────────────────────────────────────────────────────────────────────

func TestFormulaListStatuses(t *testing.T) {
	dir := workspaceProject(t)
	sess, done := newWS(t, dir)
	defer done()

	fs, err := NewFormulaService(sess).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("应有 1 条公式，实际 %d", len(fs))
	}
	f := fs[0]
	if f.Status != "verified" {
		t.Errorf("状态应为 verified，实际 %s（%s）", f.Status, f.Error)
	}
	if len(f.Cases) != 1 || !f.Cases[0].Passed {
		t.Errorf("算例应通过，实际 %+v", f.Cases)
	}
	if f.Result == "" {
		t.Error("结果表达式必须暴露——不透明的公式没法被审阅")
	}
	if len(f.UsedBy) != 1 || f.UsedBy[0] != "calc" {
		t.Errorf("应显示被哪个场景使用，实际 %+v", f.UsedBy)
	}
}

// 一条写坏的公式不该让整页打不开，它应该被单独指出。
func TestFormulaListSurvivesBrokenFile(t *testing.T) {
	dir := workspaceProject(t)
	if err := os.WriteFile(filepath.Join(dir, "formulas", "broken.formula.yml"),
		[]byte("formula: broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, done := newWS(t, dir)
	defer done()

	fs, err := NewFormulaService(sess).List()
	if err != nil {
		t.Fatalf("一条写坏的公式不该让整页打不开：%v", err)
	}
	if len(fs) != 2 {
		t.Fatalf("应列出 2 条，实际 %d", len(fs))
	}
	for _, f := range fs {
		if f.Name == "broken" && f.Error == "" {
			t.Error("写坏的那条必须单独指出错在哪")
		}
	}
}

// ── 运行 ────────────────────────────────────────────────────────────────────

func TestRunSetupListsRefCandidates(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80), "262_03": nil})
	seedShikigami(t, dir, "375", map[string]*float64{"375_01": pct(90)})
	sess, done := newWS(t, dir)
	defer done()
	svc := NewScenarioService(sess)

	// 未选主体时没有候选
	setup, err := svc.RunSetup("calc", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(setup.Refs) != 1 || len(setup.Refs[0].Candidates) != 0 {
		t.Fatalf("未选主体时不应有候选，实际 %+v", setup.Refs)
	}
	if len(setup.Subjects) != 2 {
		t.Errorf("应列出 2 个主体，实际 %v", setup.Subjects)
	}

	// 选了主体之后，只列出属于它的技能 —— 而不是让人凭记忆敲 ID
	setup, err = svc.RunSetup("calc", "262")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(setup.Refs[0].Candidates, ",")
	if got != "262_01,262_03" {
		t.Errorf("候选应只含 character_id=262 的技能，实际 %q", got)
	}
}

func TestRunRejectsUndeclaredInput(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	_, err := NewScenarioService(sess).Run("calc", "262",
		[]RunInput{{Name: "nope", Value: "1"}}, nil)
	if err == nil {
		t.Fatal("未声明的外部输入必须被拒绝")
	}
	if !strings.Contains(err.Error(), "damping") {
		t.Errorf("报错应列出可用的输入名，实际：%v", err)
	}
}

// 单位不符必须拒绝，不做隐式换算——0.5 到底是一半还是 0.5% 正是这么来的。
func TestRunRejectsUnitMismatch(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	_, err := NewScenarioService(sess).Run("calc", "262",
		[]RunInput{{Name: "damping", Value: "0.5", Unit: "percent"}}, nil)
	if err == nil {
		t.Fatal("单位不符必须被拒绝，不得隐式换算")
	}
	if !strings.Contains(err.Error(), "不做隐式换算") {
		t.Errorf("报错应说清拒绝的原因，实际：%v", err)
	}
}

func TestRunRejectsOutOfRange(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	_, err := NewScenarioService(sess).Run("calc", "262",
		[]RunInput{{Name: "damping", Value: "2"}}, nil)
	if err == nil {
		t.Fatal("超出声明范围必须被拒绝")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Errorf("报错应说明是范围问题，实际：%v", err)
	}
}

func TestRunProducesResultWithUnverifiedRatio(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	res, err := NewScenarioService(sess).Run("calc", "262",
		[]RunInput{{Name: "damping", Value: "0.5"}},
		[]RefInput{{Name: "ratio", Subject: "262_01"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("应当算出结果，实际被拒绝：%s", res.Message)
	}
	if res.Result == "" {
		t.Error("必须有结果")
	}
	// 依赖的断言全部是 pending，因此未核验比例应为 100%
	if res.UnverifiedRatio != 1 {
		t.Errorf("未核验比例应为 1，实际 %v", res.UnverifiedRatio)
	}
	if len(res.Unverified) == 0 {
		t.Error("必须列出未核验的依赖——只给结果不给可信度就是「看起来已经核验过」")
	}
	if len(res.External) != 1 || res.External[0].Name != "damping" {
		t.Errorf("外部输入必须被标注，实际 %+v", res.External)
	}
}

// 缺外部输入时不能算出结果，也不得静默用一个默认值。
func TestRunWithoutInputIsRejected(t *testing.T) {
	dir := workspaceProject(t)
	seedShikigami(t, dir, "262", map[string]*float64{"262_01": pct(80)})
	sess, done := newWS(t, dir)
	defer done()

	res, err := NewScenarioService(sess).Run("calc", "262", nil,
		[]RefInput{{Name: "ratio", Subject: "262_01"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Error("缺外部输入时不得算出结果")
	}
	if !strings.Contains(res.Message, "damping") {
		t.Errorf("拒绝原因应指明缺哪个输入，实际：%s", res.Message)
	}
}
