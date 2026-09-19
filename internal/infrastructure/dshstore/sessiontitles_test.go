package dshstore

import (
	"os"
	"path/filepath"
	"testing"
)

// 造一份 DSH 的会话记录（只放我们要读的那几个字段），验「标题读得出来」。
func writeSession(t *testing.T, home, name, title, firstText string) {
	t.Helper()
	dir := filepath.Join(home, "storages", "session_projcache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":5,"record":{"rows":{` +
		`"title":{"ver":1,"val":` + quote(title) + `},` +
		`"titleInput":{"ver":3,"val":{"first":{"seq":7,"text":` + quote(firstText) + `}}}}}}`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quote(s string) string {
	b := []byte{'"'}
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\n':
			b = append(b, '\\', 'n')
		case '\\':
			b = append(b, '\\', '\\')
		default:
			b = append(b, []byte(string(r))...)
		}
	}
	return string(append(b, '"'))
}

func TestTitlesReadsDSHStorage(t *testing.T) {
	home := t.TempDir()
	// `session-<id>.json`（现在的写法）与 `<id>.json`（旧写法）都要认。
	writeSession(t, home, "session-AAA.json", "整理剧情目录", "")
	writeSession(t, home, "BBB.json", "", "把 raw/式神 里缺的字段补上")

	got := Titles(home)
	if got["AAA"] != "整理剧情目录" {
		t.Errorf("该读出 DSH 存的 title：%+v", got)
	}
	// 没有 title 时退回第一句用户消息（**老会话就靠这条**）。
	if got["BBB"] != "把 raw/式神 里缺的字段补上" {
		t.Errorf("该退回第一句：%+v", got)
	}
	if len(got) != 2 {
		t.Errorf("该只有这两条：%+v", got)
	}
}

func TestTitlesIsQuietWhenMissing(t *testing.T) {
	// 目录不存在 / DSH_HOME 为空：返回空表，**不报错**（标题是便利信息，不该拖垮会话列表）。
	if got := Titles(filepath.Join(t.TempDir(), "不存在")); len(got) != 0 {
		t.Errorf("目录不存在该返回空：%+v", got)
	}
	if got := Titles(""); len(got) != 0 {
		t.Errorf("空 DSH_HOME 该返回空：%+v", got)
	}
	// 文件结构不对（不是 JSON）也要安静跳过。
	home := t.TempDir()
	dir := filepath.Join(home, "storages")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CCC.json"), []byte("这不是 JSON"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Titles(home); len(got) != 0 {
		t.Errorf("坏文件该被跳过：%+v", got)
	}
}

func TestTitlesTruncatesAndFlattens(t *testing.T) {
	home := t.TempDir()
	long := "第一行\n第二行    很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长很长"
	writeSession(t, home, "DDD.json", long, "")
	got := Titles(home)["DDD"]
	if got == "" {
		t.Fatal("该有标题")
	}
	if len([]rune(got)) > 41 {
		t.Errorf("该截断到 40 字 + 省略号：%q", got)
	}
	for _, r := range got {
		if r == '\n' {
			t.Errorf("该压成一行：%q", got)
		}
	}
}
