package vaultapp

import (
	"testing"
)

// 检索的 limit 与「一共命中多少篇」必须对得上：这是调用方（agent / CLI / 界面）
// 判断「我拿全了没有」的唯一依据（`OPEN.md` #33）。这一层测的是**接线**：
// 用例层不许自己夹 limit、不许把总数算成别的口径。
func TestSearchLimitAndTotalAgree(t *testing.T) {
	root := t.TempDir()
	// 命中 4 篇，其中 1 篇只在**标题**里有那个词（两个口子的判据必须一致：
	// 标题 OR 正文）；再写 2 篇不命中的，保证「没命中」也被覆盖。
	write(t, root, "docs/甲.md", "---\ntitle: 甲\n---\n\n增益 出现在正文。\n")
	write(t, root, "docs/乙.md", "---\ntitle: 乙\n---\n\n增益 也出现。\n")
	write(t, root, "docs/丙.md", "---\ntitle: 增益手册\n---\n\n正文里没有那个词。\n")
	write(t, root, "docs/丁.md", "---\ntitle: 增益速查\n---\n\n增益 标题与正文都有。\n")
	write(t, root, "raw/戊.md", "---\ntitle: 戊\n---\n\n无关内容。\n")
	write(t, root, "raw/己.md", "---\ntitle: 己\n---\n\n无关内容。\n")

	svc := New(root)

	total, err := svc.CountMatches("增益")
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("一共该命中 4 篇（标题算、正文也算），拿到 %d", total)
	}

	// 返回条数 = min(limit, total)。
	for _, limit := range []int{1, 2, 3, 4, 5, 100} {
		hits, err := svc.Search("增益", limit)
		if err != nil {
			t.Fatalf("limit=%d 报错：%v", limit, err)
		}
		want := limit
		if want > total {
			want = total
		}
		if len(hits) != want {
			t.Errorf("limit=%d 该返回 %d 条，拿到 %d 条", limit, want, len(hits))
		}
	}

	// 不变式：把 limit 开到足够大，条数正好等于 total——两个口子说的是同一件事。
	hits, err := svc.Search("增益", total+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != total {
		t.Errorf("取全时条数该等于 total：%d vs %d", len(hits), total)
	}

	// 没命中：0 条、总数 0，都不是错误。
	if n, err := svc.CountMatches("没有这个词"); err != nil || n != 0 {
		t.Errorf("没命中该是 0 且不报错：n=%d err=%v", n, err)
	}
	if hits, err := svc.Search("没有这个词", 10); err != nil || len(hits) != 0 {
		t.Errorf("没命中该返回空列表：len=%d err=%v", len(hits), err)
	}

	// 空搜索词：两个口子都明确报错（不许静默变成「全库」）。
	if _, err := svc.Search("   ", 10); err == nil {
		t.Error("空搜索词该报错")
	}
	if _, err := svc.CountMatches("   "); err == nil {
		t.Error("空搜索词的计数该报错")
	}
}
