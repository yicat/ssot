package vaultfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// 没有 front matter：整份当正文，状态默认 draft（未核验是默认状态，不是缺失），
// 标题退回文件名（界面上永远要有可读的标识）。
func TestParseDocWithoutFrontMatter(t *testing.T) {
	doc := parseDoc("docs/式神/茨木童子.md", "只有正文。\n")
	if doc.Status != vault.StatusDraft {
		t.Errorf("没有 status 该是 draft：%q", doc.Status)
	}
	if doc.Body != "只有正文。\n" {
		t.Errorf("正文该原样保留：%q", doc.Body)
	}
	if doc.Title != "茨木童子" {
		t.Errorf("缺标题时退回文件名：%q", doc.Title)
	}
	if doc.Layer != vault.LayerDocs {
		t.Errorf("层该从路径首段判断：%q", doc.Layer)
	}
}

// front matter 只有开头没有结尾：整份当正文。
// 半个 front matter 若被吞掉，正文会凭空少一段——那比「元信息没解析出来」严重得多。
func TestParseDocUnterminatedFrontMatterIsBody(t *testing.T) {
	raw := "---\nstatus: published\n正文没有结束线\n"
	doc := parseDoc("docs/a.md", raw)
	if doc.Body != raw {
		t.Errorf("没结束的 front matter 该整份当正文：%q", doc.Body)
	}
	if doc.Status != vault.StatusDraft {
		t.Errorf("不该把没闭合的 status 读进来：%q", doc.Status)
	}
}

func TestParseDocToleratesBOMAndBadStatus(t *testing.T) {
	doc := parseDoc("docs/b.md", "\ufeff---\nstatus: published\n---\n\n正文\n")
	if doc.Status != vault.StatusPublished {
		t.Errorf("带 BOM 也要能解析：%q", doc.Status)
	}
	// status 写错时退回 draft，但正文要保住。
	doc = parseDoc("docs/c.md", "---\nstatus: 瞎写\n---\n\n正文\n")
	if doc.Status != vault.StatusDraft || !strings.Contains(doc.Body, "正文") {
		t.Errorf("status 非法该退回 draft 且保住正文：%+v", doc)
	}
}

func TestSetStatusLine(t *testing.T) {
	// 原本没有 front matter：补一个最小的，正文留在后面。
	out, err := setStatusLine("正文。\n", vault.StatusDraft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "---\nstatus: draft\n---\n正文。\n") {
		t.Errorf("该补出最小 front matter 且不动正文：%q", out)
	}

	// 有 status：只换那一行，其它行（含人写的注释）原样。
	in := "---\ntitle: 甲\n# 人写的注释\nstatus: draft\ntags: [x]\n---\n\n正文\n"
	out, err = setStatusLine(in, vault.StatusPublished)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "status: published") || !strings.Contains(out, "# 人写的注释") ||
		!strings.Contains(out, "tags: [x]") || !strings.Contains(out, "title: 甲") {
		t.Errorf("只该改 status 那一行：%q", out)
	}

	// 有 front matter 但没 status：插在结束线之前。
	in = "---\ntitle: 乙\n---\n\n正文\n"
	out, err = setStatusLine(in, vault.StatusDraft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "title: 乙\nstatus: draft\n---") {
		t.Errorf("status 该插在 front matter 里：%q", out)
	}

	// front matter 没结束：拒绝，免得把正文改坏。
	if _, err := setStatusLine("---\nstatus: draft\n正文\n", vault.StatusDraft); err == nil {
		t.Error("front matter 没结束时应拒绝改写")
	}
}

func TestWriteBodyKeepsFrontMatterAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := "---\ntitle: 甲\n# 注释\nstatus: published\n---\n\n旧正文\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.WriteBody("docs/a.md", "新正文\n"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "docs", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "# 注释") || !strings.Contains(s, "title: 甲") || !strings.Contains(s, "status: published") {
		t.Errorf("front matter 该原样保留：%q", s)
	}
	if !strings.Contains(s, "新正文") || strings.Contains(s, "旧正文") {
		t.Errorf("正文该被换掉：%q", s)
	}

	// 路径不许跑出 vault：这既是安全，也是「别把人家的文件改坏」。
	if err := l.WriteBody("../跑出去.md", "x"); err == nil {
		t.Error("跑出 vault 的路径必须被拒绝")
	}
}

func TestLoadMissingLayersIsEmptyNotError(t *testing.T) {
	l := New(t.TempDir())
	docs, err := l.Load()
	if err != nil {
		t.Fatalf("还没有 docs/ 与 raw/ 时该返回空列表而不是错误：%v", err)
	}
	if len(docs) != 0 {
		t.Errorf("该是空的：%+v", docs)
	}
	tables, err := l.Tables()
	if err != nil || len(tables) != 0 {
		t.Errorf("没有 tables/ 时该返回空：%v %v", tables, err)
	}
}
