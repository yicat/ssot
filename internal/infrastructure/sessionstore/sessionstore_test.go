package sessionstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingIsEmptyNotError(t *testing.T) {
	root := t.TempDir()
	if got := Load(root); len(got) != 0 {
		t.Errorf("没文件该返回空表：%+v", got)
	}
	if got := Titles(root); len(got) != 0 {
		t.Errorf("Titles 也该空：%+v", got)
	}
	// 文件坏了同样安静返回空表（会话列表不能被标题拖垮）。
	if err := os.MkdirAll(filepath.Join(root, ".ssot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(root), []byte("{ 这不是 json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(root); len(got) != 0 {
		t.Errorf("坏文件该返回空表：%+v", got)
	}
}

func TestTouchAndTitleRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := Touch(root, "s1"); err != nil {
		t.Fatal(err)
	}
	e := Load(root)["s1"]
	if e.CreatedAt == "" || e.LastUsedAt == "" {
		t.Errorf("Touch 该写上时间：%+v", e)
	}

	// 第一次设标题：写进去。
	if err := SetTitle(root, "s1", "整理剧情目录", "first-message", false); err != nil {
		t.Fatal(err)
	}
	if got := Titles(root)["s1"]; got != "整理剧情目录" {
		t.Errorf("标题没写进：%q", got)
	}
	// 不 force：**不覆盖**（后面那句不该把名字改掉）。
	if err := SetTitle(root, "s1", "换一句话", "first-message", false); err != nil {
		t.Fatal(err)
	}
	if got := Titles(root)["s1"]; got != "整理剧情目录" {
		t.Errorf("不该被后一句覆盖：%q", got)
	}
	// force：覆盖（agent 生成更好的标题走这条）。
	if err := SetTitle(root, "s1", "剧情目录批量清理与样本保留", "agent", true); err != nil {
		t.Fatal(err)
	}
	if got := Titles(root)["s1"]; got != "剧情目录批量清理与样本保留" {
		t.Errorf("force 该覆盖：%q", got)
	}
	if Load(root)["s1"].TitleFrom != "agent" {
		t.Errorf("该记下标题是谁起的：%+v", Load(root)["s1"])
	}
}

func TestSetTitleCleansAndTruncates(t *testing.T) {
	root := t.TempDir()
	long := "第一行\n第二行   " + string(make([]rune, 60))
	for i := range []rune(long) {
		_ = i
	}
	if err := SetTitle(root, "s1", "第一行\n第二行\t很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长", "first-message", true); err != nil {
		t.Fatal(err)
	}
	got := Titles(root)["s1"]
	if len([]rune(got)) > 41 {
		t.Errorf("该截断到 40 字 + 省略号：%q", got)
	}
	for _, r := range got {
		if r == '\n' || r == '\t' {
			t.Errorf("该压成一行：%q", got)
		}
	}
	// 空标题不该写进去。
	if err := SetTitle(root, "s2", "   ", "x", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(root)["s2"]; ok {
		t.Error("空标题不该建条目")
	}
}

func TestPruneKeepsOnlyGiven(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"keep1", "keep2", "gone"} {
		if err := Touch(root, id); err != nil {
			t.Fatal(err)
		}
	}
	// keep 为空：**什么都不删**（防手滑清空）。
	if err := Prune(root, nil); err != nil {
		t.Fatal(err)
	}
	if len(Load(root)) != 3 {
		t.Errorf("keep 为空时不该删：%+v", Load(root))
	}
	if err := Prune(root, map[string]bool{"keep1": true, "keep2": true}); err != nil {
		t.Fatal(err)
	}
	all := Load(root)
	if len(all) != 2 {
		t.Fatalf("该只剩两个：%+v", all)
	}
	if _, ok := all["gone"]; ok {
		t.Error("不在 keep 里的该删掉")
	}
}

func TestDeleteOneEntry(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"a", "b"} {
		if err := SetTitle(root, id, "标题-"+id, "first-message", false); err != nil {
			t.Fatal(err)
		}
	}
	if err := Delete(root, "a"); err != nil {
		t.Fatal(err)
	}
	all := Load(root)
	if len(all) != 1 {
		t.Fatalf("该只剩 b：%+v", all)
	}
	if _, ok := all["a"]; ok {
		t.Error("a 该被删掉")
	}
	// 删不存在的、或空 id：安静返回，不改动别的条目（列表不能被它拖垮）。
	if err := Delete(root, "不存在"); err != nil {
		t.Fatal(err)
	}
	if err := Delete(root, ""); err != nil {
		t.Fatal(err)
	}
	if len(Load(root)) != 1 {
		t.Errorf("删不存在的不该动到别人：%+v", Load(root))
	}
}

func TestSortIDsByLastUsed(t *testing.T) {
	entries := map[string]Entry{
		"b": {LastUsedAt: "2026-09-19T23:00:00Z"},
		"a": {LastUsedAt: "2026-09-19T23:05:00Z"},
		"c": {LastUsedAt: "2026-09-19T23:05:00Z"}, // 同时间：按 id 升序，保证稳定
	}
	got := SortIDs(entries)
	want := []string{"a", "c", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序不对：%v（想要 %v）", got, want)
		}
	}
}
