package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimal 造一个可加载的最小项目。
func minimal(t *testing.T, dir string) string {
	t.Helper()
	writeFile := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("project.yml", "project: probe\ndescription: 探针\nmetamodelVersion: 1\n")
	writeFile("units.yml", "units: []\n")
	writeFile("schema/x.schema.yml", `
entity: shikigami
metamodelVersion: 1
fields:
  - key: id
    type: number
    unit: point
    identity: true
    required: true
`)
	return dir
}

func scenarioYML(name string) string {
	return "scenario: " + name + "\ndescription: " + name + " 用途\nentity: shikigami\nrequires:\n  - shikigami.id\n"
}

func addScenario(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, "scenarios", name, name+".scenario.yml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 未指定目录时取项目列表的第一个。
func TestSessionPicksFirstProject(t *testing.T) {
	root := t.TempDir()
	minimal(t, filepath.Join(root, "alpha"))
	minimal(t, filepath.Join(root, "beta"))

	s := NewSession(root, "")
	defer s.Close()
	p, err := s.Project()
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "probe" {
		t.Errorf("应加载第一个项目，实际 %q", p.Name())
	}
	if !strings.HasSuffix(filepath.ToSlash(p.Dir), "alpha") {
		t.Errorf("应取目录名最小的那个，实际 %s", p.Dir)
	}
}

// 一个都没有时明确报「没有可用项目」，而不是拼一个不存在的路径去撞文件系统。
func TestSessionReportsNoProjects(t *testing.T) {
	s := NewSession(t.TempDir(), "")
	defer s.Close()
	_, err := s.Project()
	if err == nil {
		t.Fatal("没有项目时必须报错")
	}
	if !strings.Contains(err.Error(), "没有可用项目") {
		t.Errorf("报错要说清是「一个都没有」，实际：%v", err)
	}
}

// 切换失败时**保持原项目不变**——半加载的项目会让界面显示
// 上一个项目的数据，而标题写着新项目。
func TestOpenFailureKeepsCurrentProject(t *testing.T) {
	root := t.TempDir()
	good := minimal(t, filepath.Join(root, "good"))
	addScenario(t, good, "damage", scenarioYML("damage"))

	s := NewSession(root, "")
	defer s.Close()
	if _, err := s.Project(); err != nil {
		t.Fatal(err)
	}

	// 1. 目录不存在
	if err := s.Open(filepath.Join(root, "nope")); err == nil {
		t.Error("目录不存在时应报错")
	}
	// 2. 目录在，但没有 project.yml
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Open(empty); err == nil {
		t.Error("不是项目目录时应报错")
	}
	// 3. schema 不合法
	bad := minimal(t, filepath.Join(root, "bad"))
	if err := os.WriteFile(filepath.Join(bad, "schema", "x.schema.yml"),
		[]byte("entity: shikigami\nmetamodelVersion: 1\nfields:\n  - key: id\n    type: 不存在的类型\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Open(bad); err == nil {
		t.Error("schema 不合法时应报错")
	}

	p, err := s.Project()
	if err != nil {
		t.Fatalf("三次失败之后原项目必须仍然可用：%v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(p.Dir), "good") {
		t.Errorf("原项目不得被换掉，实际 %s", p.Dir)
	}
	if s.Scenario() != "damage" {
		t.Errorf("原场景不得被清掉，实际 %q", s.Scenario())
	}
}

// 切换项目后场景重置为新项目的第一个——留着一个属于旧项目的场景名，
// 会让所有场景级视图取错数。
func TestOpenResetsScenario(t *testing.T) {
	root := t.TempDir()
	a := minimal(t, filepath.Join(root, "a"))
	addScenario(t, a, "alpha", scenarioYML("alpha"))
	b := minimal(t, filepath.Join(root, "b"))
	addScenario(t, b, "beta", scenarioYML("beta"))

	s := NewSession(root, a)
	defer s.Close()
	if _, err := s.Project(); err != nil {
		t.Fatal(err)
	}
	if s.Scenario() != "alpha" {
		t.Fatalf("初始场景应为 alpha，实际 %q", s.Scenario())
	}
	if err := s.Open(b); err != nil {
		t.Fatal(err)
	}
	if s.Scenario() != "beta" {
		t.Errorf("切换项目后场景应重置为 beta，实际 %q", s.Scenario())
	}
}

// 项目没有场景时，当前场景为空——界面据此显示「未选择场景」。
func TestProjectWithoutScenarios(t *testing.T) {
	root := t.TempDir()
	minimal(t, filepath.Join(root, "bare"))
	s := NewSession(root, "")
	defer s.Close()
	if _, err := s.Project(); err != nil {
		t.Fatal(err)
	}
	if s.Scenario() != "" {
		t.Errorf("没有场景时应为空，实际 %q", s.Scenario())
	}
}

// 选中不存在的场景必须被拒绝，并列出可用名。
func TestSelectScenarioRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	dir := minimal(t, filepath.Join(root, "p"))
	addScenario(t, dir, "damage", scenarioYML("damage"))

	s := NewSession(root, dir)
	defer s.Close()
	if _, err := s.Project(); err != nil {
		t.Fatal(err)
	}
	err := s.SelectScenario("nope")
	if err == nil {
		t.Fatal("未知场景必须被拒绝")
	}
	if !strings.Contains(err.Error(), "damage") {
		t.Errorf("应列出可用场景名，实际：%v", err)
	}
}

// 同名场景是定义冲突：它会让「选中某场景」变成一件含糊的事。
func TestDuplicateScenarioNamesRejected(t *testing.T) {
	root := t.TempDir()
	dir := minimal(t, filepath.Join(root, "p"))
	addScenario(t, dir, "one", scenarioYML("same"))
	addScenario(t, dir, "two", scenarioYML("same"))

	s := NewSession(root, dir)
	defer s.Close()
	if _, err := s.Project(); err == nil {
		t.Fatal("场景名重复必须被拒绝")
	} else if !strings.Contains(err.Error(), "重复") {
		t.Errorf("报错应说明是重名，实际：%v", err)
	}
}

// 目录存在但没有场景声明：跳过**并给出原因**——
// 一场静默的跳过会让人以为「项目只有两个场景」。
func TestSkippedScenarioCarriesReason(t *testing.T) {
	root := t.TempDir()
	dir := minimal(t, filepath.Join(root, "p"))
	addScenario(t, dir, "ok", scenarioYML("ok"))
	if err := os.MkdirAll(filepath.Join(dir, "scenarios", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	p, err := Load(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Scenarios) != 1 {
		t.Fatalf("应有 1 个可用场景，实际 %d", len(p.Scenarios))
	}
	if len(p.ScenarioSkipped) != 1 {
		t.Fatalf("跳过项必须被记下来，实际 %d", len(p.ScenarioSkipped))
	}
	if p.ScenarioSkipped[0].Reason == "" {
		t.Error("跳过必须带原因")
	}
}
