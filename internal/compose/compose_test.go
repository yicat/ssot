package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 骨架级的冒烟测试：只验证「项目能被发现、能打开、失败时不留半个状态」。
//
// 这几条与方案无关（换什么设计都成立），因此留在骨架里；
// 业务规则等新方案定下来再补验收测试。

func writeProject(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// 验收：会话能按目录打开项目，名字与描述取自 project.yml。
func TestOpenProject(t *testing.T) {
	root := t.TempDir()
	dir := writeProject(t, root, "probe", "project: probe\ndescription: 探针\n")

	s := NewSession(root, "")
	defer func() { _ = s.Close() }()

	p, err := s.Project()
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "probe" || p.Description() != "探针" {
		t.Fatalf("名字与描述应取自 project.yml：%q %q", p.Name(), p.Description())
	}
	if err := s.Open(dir); err != nil {
		t.Fatal(err)
	}
	if s.Dir() != dir {
		t.Errorf("切换后当前目录应更新：%q", s.Dir())
	}
}

// 验收：「一个项目都没有」要说清楚，而不是拼一个不存在的路径去撞文件系统——
// 那会让人以为是权限或路径问题。
func TestNoProjectsIsAnExplicitError(t *testing.T) {
	s := NewSession(t.TempDir(), "")
	defer func() { _ = s.Close() }()

	_, err := s.Project()
	if err == nil {
		t.Fatal("没有项目时应报错")
	}
	if !strings.Contains(err.Error(), "没有可用项目") {
		t.Errorf("要说清是「没有」而不是别的：%v", err)
	}
}

// 验收：切换项目失败时当前项目不变——半个加载的项目会让界面显示上一个项目的数据，
// 而标题写着新项目。
func TestFailedOpenKeepsCurrentProject(t *testing.T) {
	root := t.TempDir()
	dir := writeProject(t, root, "probe", "project: probe\n")

	s := NewSession(root, dir)
	defer func() { _ = s.Close() }()
	if _, err := s.Project(); err != nil {
		t.Fatal(err)
	}

	if err := s.Open(filepath.Join(root, "不存在")); err == nil {
		t.Fatal("打开不存在的目录应失败")
	}
	if s.Dir() != dir {
		t.Errorf("失败后当前目录不该变：%q", s.Dir())
	}
	p, err := s.Project()
	if err != nil || p.Name() != "probe" {
		t.Errorf("失败后原项目仍应可用：%v %v", p, err)
	}
}
