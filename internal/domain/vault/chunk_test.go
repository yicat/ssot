package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimateTokensMatchesTheJSProbe(t *testing.T) {
	// 与 scripts/check/chunk-sizing.mjs 同一套规则：汉字 1、拉丁 4 字符 1、标点 1。
	// ⚠️ 参照样例：`暴击伤害提高20%` 在 bge 词表下是 **8 个内容 token**——spike 里记的
	// `[101,…,102]` 共 10 个，头尾是 [CLS]/[SEP]，不算内容。我第一版把这个 8 写成 10，
	// 测试立刻把估算函数判成错。记下来免得下次又看错。
	if got := EstimateTokens("暴击伤害提高20%"); got != 8 {
		t.Errorf("参照样例该是 8（不含 [CLS]/[SEP]），实际 %d", got)
	}
	if got := EstimateTokens("茨木童子"); got != 4 {
		t.Errorf("四个汉字该是 4，实际 %d", got)
	}
	if got := EstimateTokens("abcd"); got != 1 {
		t.Errorf("四个拉丁字符该是 1，实际 %d", got)
	}
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("空串该是 0，实际 %d", got)
	}
}

// TestChunkBodyLineNumbersAreFileLines 是这一层最要紧的事：
// 块的行号必须是**文件行号**，拿它去编辑器里跳要跳得准。
func TestChunkBodyLineNumbersAreFileLines(t *testing.T) {
	root := t.TempDir()
	rel := "docs/a.md"
	body := "第一段。\n\n## 小标题\n\n第二段。\n"
	full := "---\ntitle: A\nstatus: draft\n---\n\n" + body
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	// front matter 4 行 + 1 空行 → 正文第一行是第 6 行
	const bodyOffset = 6
	chunks := ChunkBody(body, bodyOffset, 512)
	if len(chunks) == 0 {
		t.Fatal("该切出块")
	}
	fileLines := strings.Split(strings.ReplaceAll(full, "\r\n", "\n"), "\n")
	for _, c := range chunks {
		if c.FromLine < 1 || c.ToLine < c.FromLine || c.ToLine > len(fileLines) {
			t.Fatalf("行号区间不合法：%+v（文件 %d 行）", c, len(fileLines))
		}
		// 用行号回文件里取，必须能取到块文本的第一行
		first := strings.TrimSpace(fileLines[c.FromLine-1])
		head := strings.TrimSpace(strings.Split(c.Text, "\n")[0])
		if first != head {
			t.Errorf("FromLine 指错了：文件第 %d 行是 %q，块首行是 %q", c.FromLine, first, head)
		}
	}
}

func TestChunkBodyBreaksAtHeadings(t *testing.T) {
	body := "开头一段。\n\n## 甲\n\n甲的内容。\n\n## 乙\n\n乙的内容。\n"
	chunks := ChunkBody(body, 1, 2000)
	if len(chunks) != 3 {
		t.Fatalf("标题处该断开，期望 3 块，实际 %d：%+v", len(chunks), chunks)
	}
	if !strings.HasPrefix(chunks[1].Text, "## 甲") || !strings.HasPrefix(chunks[2].Text, "## 乙") {
		t.Errorf("块该从标题开始：%+v", chunks)
	}
}

func TestChunkBodyRespectsTargetAndSplitsLongParagraph(t *testing.T) {
	// 一句话 60 字的段落 ×3，目标 100 token → 必须切开，且不切碎句子
	sent := strings.Repeat("这是一句话内容。", 5) // 40 字 ≈ 40 token
	long := sent + sent + sent            // 120 token，超过目标
	chunks := ChunkBody(long, 1, 100)
	if len(chunks) < 2 {
		t.Fatalf("超长段该被切开：%+v", chunks)
	}
	for i, c := range chunks {
		if c.Tokens > 120 { // 单句 40 token，允许合并到 ~100 出头
			t.Errorf("第 %d 块超出预期：%d token", i, c.Tokens)
		}
		if !strings.HasSuffix(c.Text, "。") {
			t.Errorf("第 %d 块没在句子边界结束：%q", i, c.Text)
		}
	}
}

func TestChunkBodyEmptyAndBlank(t *testing.T) {
	if got := ChunkBody("", 1, 512); len(got) != 0 {
		t.Errorf("空正文该给空切片：%+v", got)
	}
	if got := ChunkBody("\n\n   \n", 1, 512); len(got) != 0 {
		t.Errorf("全空白该给空切片：%+v", got)
	}
	// 没给 bodyOffset（0）时按 1 处理，不该出现 0 或负行号
	got := ChunkBody("正文。\n", 0, 512)
	if len(got) != 1 || got[0].FromLine != 1 {
		t.Errorf("bodyOffset<=0 该按 1 处理：%+v", got)
	}
}

func TestChunkBodyOrdIsStable(t *testing.T) {
	body := "甲。\n\n## 乙\n\n丙。\n"
	first := ChunkBody(body, 1, 512)
	second := ChunkBody(body, 1, 512)
	if len(first) != len(second) {
		t.Fatal("同样输入该切出同样多的块")
	}
	for i := range first {
		if first[i].Ord != i || first[i].FromLine != second[i].FromLine ||
			first[i].ToLine != second[i].ToLine || first[i].Text != second[i].Text {
			t.Errorf("切块必须是确定的（第 %d 块两次不一致）", i)
		}
	}
}
