package vaultindex

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// bigDoc 造一篇够长的文档：标题 + N 段，段间空行（切块按段落语义断开）。
func bigDoc(title string, paras int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)
	for i := 0; i < paras; i++ {
		fmt.Fprintf(&sb,
			"第 %d 条：最终伤害等于攻击力乘以技能系数再乘以暴击倍率，这条规则在所有式神身上都成立。\n\n", i)
	}
	return sb.String()
}

// readLines 读文件的行（与 vaultfs 的口径一致：按 \n 切）。
func readLines(t *testing.T, root, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
}

const bigRel = "docs/机制/伤害计算大.md"

// newChunkVault 造一个能切出多个块的 vault。
func newChunkVault(t *testing.T) (string, *Index) {
	t.Helper()
	root := t.TempDir()
	write(t, root, bigRel,
		"---\ntitle: 伤害计算大\nstatus: published\n---\n\n"+bigDoc("伤害计算大", 40))
	write(t, root, "docs/式神/茨木童子.md",
		"---\ntitle: 茨木童子\nstatus: draft\n---\n\n三技能伤害系数 263%。\n")
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return root, idx
}

// TestChunksLineRangesMatchFile 是 P0 的验收点：切块的行号必须对得上文件，
// 且正文一个字都不许丢（块合起来的覆盖 = 全部非空正文行）。
func TestChunksLineRangesMatchFile(t *testing.T) {
	root, idx := newChunkVault(t)
	lines := readLines(t, root, bigRel)

	chunks, err := idx.ChunksOf(bigRel)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("这篇该切出多个块，实际 %d 个", len(chunks))
	}

	prevTo := 0
	for i, c := range chunks {
		if c.Ord != i {
			t.Errorf("第 %d 个块的 ord 该是 %d：%+v", i, i, c)
		}
		if c.FromLine < 1 || c.ToLine > len(lines) || c.ToLine < c.FromLine {
			t.Fatalf("块 %d 的行号区间越界：%d-%d（文件 %d 行）", c.Ord, c.FromLine, c.ToLine, len(lines))
		}
		if c.FromLine <= prevTo {
			t.Errorf("块 %d 与前一块重叠：前一块到 %d，这块从 %d 开始", c.Ord, prevTo, c.FromLine)
		}
		prevTo = c.ToLine
		// 块的首行文本 == 文件里 FromLine 那一行（块从段落起点开始，这一条是溯源的地基）。
		want := strings.TrimSpace(lines[c.FromLine-1])
		got := strings.SplitN(c.Text, "\n", 2)[0]
		if got != want {
			t.Errorf("块 %d 的首行与文件第 %d 行对不上：块里是 %q，文件里是 %q", c.Ord, c.FromLine, got, want)
		}
		if c.Tokens != vault.EstimateTokens(c.Text) {
			t.Errorf("块 %d 的 token 数与估算不一致：%d", c.Ord, c.Tokens)
		}
	}

	// 覆盖：每个非空**正文**行都要落在某个块里（不许丢内容）。
	// front matter 不是正文（vaultfs 会剥掉），不参与覆盖检查。
	bodyStart := 0
	seenFM := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			seenFM++
			if seenFM == 2 {
				bodyStart = i + 1
				break
			}
		}
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || i < bodyStart {
			continue
		}
		ln := i + 1
		covered := false
		for _, c := range chunks {
			if ln >= c.FromLine && ln <= c.ToLine {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("文件第 %d 行没被任何块覆盖：%q", ln, line)
		}
	}
}

// TestExtractChunksCoverEmbedChunks 抽取块要罩住嵌入块：
// 抽取产物（实体/关系）靠抽取块定位，嵌入块靠它找回来。
func TestExtractChunksCoverEmbedChunks(t *testing.T) {
	root, idx := newChunkVault(t)

	embeds, err := idx.ChunksOf(bigRel)
	if err != nil {
		t.Fatal(err)
	}
	extracts, err := idx.ExtractChunksOf(bigRel)
	if err != nil {
		t.Fatal(err)
	}
	if len(extracts) == 0 || len(extracts) >= len(embeds) {
		t.Fatalf("抽取块该比嵌入块少（2000 vs 512）：抽取 %d，嵌入 %d", len(extracts), len(embeds))
	}

	// 抽取块也要能对上文件行。
	lines := readLines(t, root, bigRel)
	for _, c := range extracts {
		if c.FromLine < 1 || c.ToLine > len(lines) {
			t.Fatalf("抽取块 %d 行号越界：%d-%d", c.Ord, c.FromLine, c.ToLine)
		}
	}
	for _, e := range embeds {
		ok := false
		for _, x := range extracts {
			if e.FromLine >= x.FromLine && e.ToLine <= x.ToLine {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("嵌入块（%d-%d）不在任何抽取块里", e.FromLine, e.ToLine)
		}
	}
}

// TestSyncRowsFreshAndVisible：「不静默」——每篇文档都要有 sync 行；
// 标 stale 之后能被人看见（而不是悄悄降级）。
func TestSyncRowsFreshAndVisible(t *testing.T) {
	_, idx := newChunkVault(t)

	st, err := idx.ChunkStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Docs != 2 || st.Chunks == 0 || st.ExtractChunks == 0 || st.Stale != 0 {
		t.Fatalf("刚重建的统计不对：%+v", st)
	}

	si, ok, err := idx.SyncOf(bigRel)
	if err != nil || !ok {
		t.Fatalf("该有 sync 行：ok=%v err=%v", ok, err)
	}
	if len(si.Hash) != 64 || si.Stale || si.Mtime == 0 || si.BuiltAt == 0 {
		t.Fatalf("sync 行内容不对：%+v", si)
	}

	// 索引比文件旧：正文变了，hash 就该变（这是增量同步的判据）。
	if si.Hash == "" {
		t.Fatal("hash 不该是空的")
	}
	before := si.Hash
	write(t, idx.root, bigRel,
		"---\ntitle: 伤害计算大\nstatus: published\n---\n\n"+bigDoc("伤害计算大", 41))
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	after, _, err := idx.SyncOf(bigRel)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash == before {
		t.Error("正文改了，hash 该变")
	}

	// 手工把一篇标成 stale：界面要能看见它和原因。
	db, err := sql.Open("sqlite", idx.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`UPDATE sync SET stale=1, reason='嵌入失败：模型文件缺失' WHERE doc=?`, bigRel); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.StaleDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Doc != bigRel || !strings.Contains(stale[0].Reason, "嵌入失败") {
		t.Fatalf("stale 文档该被列出来：%+v", stale)
	}
	if st2, err := idx.ChunkStat(); err != nil || st2.Stale != 1 {
		t.Fatalf("统计里的 stale 数该是 1：%+v err=%v", st2, err)
	}
}

// TestSyncRowForEmptyDoc：空正文的文档也要有 sync 行、零个块，不许当成错误。
func TestSyncRowForEmptyDoc(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/空.md", "---\ntitle: 空\nstatus: draft\n---\n")
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	chunks, err := idx.ChunksOf("docs/空.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Fatalf("空文档不该有块：%+v", chunks)
	}
	if _, ok, err := idx.SyncOf("docs/空.md"); err != nil || !ok {
		t.Fatalf("空文档也该有 sync 行：ok=%v err=%v", ok, err)
	}
}
