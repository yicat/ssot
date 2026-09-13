package projectfile

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 验收：项目列表只包含含 project.yml 的目录。
func TestDiscoverOnlyListsRealProjects(t *testing.T) {
	root := t.TempDir()
	write(t, root, "onmyoji/project.yml", "project: onmyoji\ndescription: 阴阳师\n")
	// 空目录：不是项目
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 有别的文件但没有 project.yml：也不是项目
	write(t, root, "half-baked/units.yml", "units: []\n")
	// 普通文件混在根目录下：不该被当成项目
	write(t, root, "README.md", "x")

	refs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("只应发现 1 个项目，实际 %d：%+v", len(refs), refs)
	}
	if refs[0].Display() != "onmyoji" {
		t.Errorf("项目名应为 onmyoji，实际 %q", refs[0].Display())
	}
	if refs[0].File.Description != "阴阳师" {
		t.Errorf("描述应被读出，实际 %q", refs[0].File.Description)
	}
}

// 「一个都没有」不是「出错了」——两者在界面上必须能区分。
func TestDiscoverMissingRootIsEmptyNotError(t *testing.T) {
	refs, err := Discover(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("根目录不存在应返回空列表而不是错误：%v", err)
	}
	if len(refs) != 0 {
		t.Errorf("应返回空列表，实际 %d 项", len(refs))
	}
}

// 有 project.yml 却读不出名字：这是配置错误，必须报出来，
// 不能当成「不是项目」悄悄跳过。
func TestDiscoverReportsBrokenProjectFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "broken/project.yml", "description: 没有名字\n")
	if _, err := Discover(root); err == nil {
		t.Error("project.yml 缺名字时必须报错，不得静默跳过")
	}
}

func TestLoadRequiresProjectName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", "description: x\n")
	if _, err := Load(filepath.Join(dir, "project.yml")); err == nil {
		t.Error("缺 project 名时必须被拒绝")
	}
}
