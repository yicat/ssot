package vault

import (
	"strings"
	"testing"
)

func TestCountOccurrences(t *testing.T) {
	if n := CountOccurrences("伤害计算与伤害减免", "伤害"); n != 2 {
		t.Errorf("中文该数对：%d", n)
	}
	if n := CountOccurrences("Damage and damage", "damage"); n != 2 {
		t.Errorf("大小写不敏感：%d", n)
	}
	if n := CountOccurrences("abc", ""); n != 0 {
		t.Errorf("空词该是 0：%d", n)
	}
}

// 片段按 rune 切：按字节切会把汉字切碎，显示出来是乱码。
func TestSnippetIsRuneSafe(t *testing.T) {
	text := "前面前面前面前面前面伤害系数为 263%后面后面后面后面后面"
	got := Snippet(text, "伤害", 12)
	if !strings.Contains(got, "伤害") {
		t.Errorf("片段该包住命中词：%q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("出现了替换字符，说明按字节切坏了：%q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Errorf("两端截断了该有省略号：%q", got)
	}

	// 找不到命中词时给开头一段——至少让人看到这篇在讲什么。
	head := Snippet(text, "不存在", 6)
	if !strings.HasPrefix(head, "前面前面") {
		t.Errorf("找不到时该退回开头：%q", head)
	}

	// 换行会被压成空格：单行展示里换行会把版面弄乱。
	if s := Snippet("甲\n乙\n丙", "乙", 10); strings.Contains(s, "\n") {
		t.Errorf("换行该被压平：%q", s)
	}
}

// 排序必须是确定的：标题命中优先 → 出现次数多优先 → 路径稳定。
func TestRankHitsIsDeterministic(t *testing.T) {
	hits := []Hit{
		{Path: "docs/b.md", Occurrences: 5},
		{Path: "docs/c.md", TitleMatch: true, Occurrences: 1},
		{Path: "docs/a.md", Occurrences: 5},
	}
	RankHits(hits)
	want := []string{"docs/c.md", "docs/a.md", "docs/b.md"}
	for i, w := range want {
		if hits[i].Path != w {
			t.Fatalf("第 %d 位应是 %s，实际 %s（顺序：%+v）", i, w, hits[i].Path, hits)
		}
	}
}
