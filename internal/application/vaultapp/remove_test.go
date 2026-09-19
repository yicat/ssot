package vaultapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// TestRemoveDocCleansDerivedAndReportsBrokenLinks 是删除用例的验收：
// 文件没了、派生行清了、断链报出来了、git 没留痕要说清原因。
func TestRemoveDocCleansDerivedAndReportsBrokenLinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/甲.md",
		"---\ntitle: 甲\nstatus: published\n---\n\n"+strings.Repeat("正文一句话。", 60))
	write(t, root, "docs/乙.md",
		"---\ntitle: 乙\nstatus: draft\n---\n\n见 [[docs/甲]]。\n")

	svc := New(root)
	// 先看影响（不动物）。
	dry, err := svc.WouldRemove("docs/甲.md")
	if err != nil {
		t.Fatal(err)
	}
	if dry.DerivedRows == 0 {
		t.Error("索引里有块，dry-run 该数出来")
	}
	if len(dry.BrokenLinks) != 1 || dry.BrokenLinks[0] != "docs/乙.md" {
		t.Errorf("该报出「乙链到甲」：%+v", dry.BrokenLinks)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "甲.md")); err != nil {
		t.Fatal("dry-run 不该真删")
	}

	// 真删。
	actor, err := vault.ParseActor("human:我")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Remove("docs/甲.md", actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.DerivedRows == 0 {
		t.Error("该报清掉了几行派生数据")
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "甲.md")); !os.IsNotExist(err) {
		t.Error("文件该被删掉")
	}
	if len(res.BrokenLinks) != 1 {
		t.Errorf("断链要报出来：%+v", res.BrokenLinks)
	}
	// 临时目录不是 git 仓库 → 必须说清「没留痕」的原因（不静默）。
	if res.Change.Committed || res.Change.VersionNote == "" {
		t.Errorf("没 git 时该给出未留痕的原因：%+v", res.Change)
	}
	// 派生层残骸清干净：检索不该再命中它。
	hits, err := svc.Search("正文一句话", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Path == "docs/甲.md" {
			t.Errorf("删了之后不该还能搜到：%+v", h)
		}
	}
	// 再删一次：该报「不存在」，而不是装作成功。
	if _, err := svc.Remove("docs/甲.md", actor); err == nil {
		t.Error("文档不存在时该报错")
	}
}

func TestRemoveRefusesNonDoc(t *testing.T) {
	root := t.TempDir()
	write(t, root, "tables/表.csv", "a,b\n1,2\n")
	svc := New(root)
	actor, err := vault.ParseActor("agent:整理")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Remove("tables/表.csv", actor); err == nil {
		t.Error("非 .md 该拒（数据表直接改文件）")
	}
	if _, err := svc.Remove("docs/不存在.md", actor); err == nil {
		t.Error("不存在的路径该报错")
	}
}

// TestMatchDocsGlob：批量删靠 glob 找文档，语义与收录范围共用一份实现。
func TestMatchDocsGlob(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/甲.md", "---\ntitle: 甲\n---\n\n甲。\n")
	write(t, root, "raw/DIR_X/一.md", "---\ntitle: 一\n---\n\n一。\n")
	write(t, root, "raw/DIR_X/二.md", "---\ntitle: 二\n---\n\n二。\n")
	write(t, root, "raw/别的/三.md", "---\ntitle: 三\n---\n\n三。\n")
	svc := New(root)

	got, err := svc.MatchDocs("raw/DIR_X/**")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "raw/DIR_X/一.md" || got[1] != "raw/DIR_X/二.md" {
		t.Errorf("glob 该匹配到 2 篇且按路径排序：%v", got)
	}
	if got, _ := svc.MatchDocs("docs/*.md"); len(got) != 1 {
		t.Errorf("`docs/*.md` 该只匹配一层：%v", got)
	}

	// ExpandArgs：glob 展开成多篇，普通路径原样留着去校验存在性。
	docs, patterns, err := svc.ExpandArgs([]string{"raw/DIR_X/**", "docs/甲.md"})
	if err != nil {
		t.Fatal(err)
	}
	if patterns != 1 || len(docs) != 3 {
		t.Errorf("该展开成 3 篇（1 个 glob）：%v patterns=%d", docs, patterns)
	}
	if _, _, err := svc.ExpandArgs([]string{"raw/不存在/**"}); err == nil {
		t.Error("glob 匹配不到东西该报错，而不是静默删 0 篇")
	}
}
