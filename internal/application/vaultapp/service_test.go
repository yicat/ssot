package vaultapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newVault 造一个最小的 vault：两层各一篇 + 一张表，整理层引到原始层的块锚点。
func newVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "project.yml", "project: demo\ndescription: 测试用\n")
	write(t, root, "docs/式神/茨木童子.md",
		"---\ntitle: 茨木童子\n# 这行注释是人写的，程序不该把它冲掉\ntags: [式神, SSR]\nstatus: published\n---\n\n正文。\n")
	write(t, root, "docs/机制/伤害计算.md",
		"---\ntitle: 伤害计算\nstatus: draft\n---\n\n公式见下。\n依据 [[raw/灰机wiki/茨木童子#^第3段]]。\n")
	write(t, root, "raw/灰机wiki/茨木童子.md",
		"---\ntitle: 灰机wiki：茨木童子\nsource_url: https://example.com/x\n---\n\n第一段。\n第二段。\n伤害系数 1.5。 ^第3段\n")
	write(t, root, "tables/技能倍率.csv", "技能,倍率\na,1.5\n")
	return root
}

func TestListAndRead(t *testing.T) {
	svc := New(newVault(t))

	items, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("应有 3 篇文档，实际 %d：%+v", len(items), items)
	}
	byPath := map[string]vault.Status{}
	for _, it := range items {
		byPath[it.Path] = it.Status
	}
	if byPath["docs/式神/茨木童子.md"] != vault.StatusPublished {
		t.Errorf("发布态该从 front matter 读出来：%+v", byPath)
	}
	if byPath["docs/机制/伤害计算.md"] != vault.StatusDraft {
		t.Errorf("draft 该被读出来：%+v", byPath)
	}

	// 只写文件名也要能找到（像 Obsidian 那样）——但只限名字唯一的情况。
	doc, err := svc.Read("伤害计算")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "docs/机制/伤害计算.md" || doc.Title != "伤害计算" {
		t.Errorf("按文件名读错：%+v", doc)
	}

	// 同名两篇（docs 与 raw 各一篇茨木童子）时**不猜**：报歧义并列出候选。
	if _, err := svc.Read("茨木童子"); err == nil {
		t.Error("同名两篇时应报歧义，而不是随便挑一篇")
	} else if !strings.Contains(err.Error(), "同名") {
		t.Errorf("要说清是同名冲突：%v", err)
	}

	tables, err := svc.Tables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0] != "tables/技能倍率.csv" {
		t.Errorf("数据表该被列出来：%v", tables)
	}
}

// 反链要连块锚点一起给——只说「谁链了我」不够，得能看出他引的是哪一段。
func TestBacklinksCarryBlockAnchor(t *testing.T) {
	svc := New(newVault(t))

	res, err := svc.Backlinks("raw/灰机wiki/茨木童子.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backlinks) != 1 {
		t.Fatalf("应有一条反链：%+v", res.Backlinks)
	}
	b := res.Backlinks[0]
	if b.From != "docs/机制/伤害计算.md" || b.Link.Block != "第3段" {
		t.Errorf("反链该带来源与块锚点：%+v", b)
	}
}

func TestResolveBlockAnchor(t *testing.T) {
	svc := New(newVault(t))

	res, err := svc.Resolve("[[raw/灰机wiki/茨木童子#^第3段]]")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "raw/灰机wiki/茨木童子.md" {
		t.Errorf("块锚点该指到原始层那篇：%+v", res)
	}
	// 行号是**文件行号**（front matter 占 4 行，锚点落在第 8 行），不是正文内行号——
	// 否则人拿着它去编辑器里跳会跳错地方。
	if res.BlockLine != 8 {
		t.Errorf("块锚点该报文件行号 8，实际 %d", res.BlockLine)
	}
	if !strings.Contains(res.BlockText, "1.5") {
		t.Errorf("块锚点该给出那一行的原文：%q", res.BlockText)
	}
}

// 核心规则：agent 不能发布。而且**失败时文件不能被动过**。
func TestAgentCannotPublish(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	agent, _ := vault.ParseActor("agent:核验")

	before := read(t, root, "docs/机制/伤害计算.md")
	_, err := svc.SetStatus("docs/机制/伤害计算.md", vault.StatusPublished, agent)
	if err == nil {
		t.Fatal("agent 发布必须失败")
	}
	if !strings.Contains(err.Error(), "只有人") {
		t.Errorf("失败信息要说清原因：%v", err)
	}
	if after := read(t, root, "docs/机制/伤害计算.md"); after != before {
		t.Error("被拒之后文件不该被改动")
	}
}

// 人能发布，而且**只动 status 那一行**：人写的注释、键顺序都要留着。
func TestHumanPublishKeepsRestOfFrontMatter(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	human, _ := vault.ParseActor("human:我")

	change, err := svc.SetStatus("docs/式神/茨木童子.md", vault.StatusArchived, human)
	if err != nil {
		t.Fatal(err)
	}
	if change.From != vault.StatusPublished || change.To != vault.StatusArchived {
		t.Errorf("改动记录该带上前后状态：%+v", change)
	}
	if !strings.Contains(change.CommitMessage(), "Edited-By: human:我") {
		t.Errorf("提交信息该带 trailer：%q", change.CommitMessage())
	}

	after := read(t, root, "docs/式神/茨木童子.md")
	if !strings.Contains(after, "status: archived") {
		t.Errorf("status 该被改写：%q", after)
	}
	if !strings.Contains(after, "# 这行注释是人写的") || !strings.Contains(after, "tags: [式神, SSR]") {
		t.Errorf("其它行不该被动：%q", after)
	}
	if !strings.Contains(after, "正文。") {
		t.Errorf("正文不该被动：%q", after)
	}
}

// 核心规则：agent 改过一律回落 draft——哪怕原来是 published。
func TestAgentWriteFallsBackToDraft(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	agent, _ := vault.ParseActor("agent:优化")

	change, err := svc.Write("docs/式神/茨木童子.md", "改写后的正文。", agent)
	if err != nil {
		t.Fatal(err)
	}
	if change.From != vault.StatusPublished || change.To != vault.StatusDraft {
		t.Errorf("该记录 published → draft：%+v", change)
	}
	after := read(t, root, "docs/式神/茨木童子.md")
	if !strings.Contains(after, "status: draft") {
		t.Errorf("agent 写过的文档必须回落 draft：%q", after)
	}
	if !strings.Contains(after, "改写后的正文。") {
		t.Errorf("正文该被换掉：%q", after)
	}
	if !strings.Contains(after, "# 这行注释是人写的") {
		t.Errorf("front matter 该原样保留：%q", after)
	}
}

// 只写名字默认落整理层；新文档一律 draft。
func TestWriteNewDocDefaultsToDocsDraft(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	human, _ := vault.ParseActor("human:我")

	change, err := svc.Write("新页面", "内容。", human)
	if err != nil {
		t.Fatal(err)
	}
	if change.Path != "docs/新页面.md" || change.To != vault.StatusDraft {
		t.Errorf("新文档该落在 docs/ 且是 draft：%+v", change)
	}
	if !strings.Contains(read(t, root, "docs/新页面.md"), "status: draft") {
		t.Error("新文档的 front matter 该带上 status: draft")
	}
}

// 同名文档已在别处时不猜：报错，别造出第二篇同名的。
func TestWriteRefusesDuplicateNameElsewhere(t *testing.T) {
	root := newVault(t)
	svc := New(root)
	write(t, root, "raw/另有一篇.md", "---\nstatus: draft\n---\n\nx\n")

	_, err := svc.Write("另有一篇", "内容。", vault.Actor{Kind: vault.ActorHuman})
	if err == nil {
		t.Fatal("同名已在别处时应拒绝写入")
	}
	if !strings.Contains(err.Error(), "同名") {
		t.Errorf("失败信息要说清是同名：%v", err)
	}
}
