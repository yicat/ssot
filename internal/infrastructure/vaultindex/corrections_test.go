package vaultindex

import (
	"strings"
	"testing"
)

// 重建时要把纠正块读成 authority=corrected 的来源行——这是「纠正活过重建」那条约束的落地。
func TestRebuildReadsCorrectionsIntoTheGraph(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/式神/茨木.md", strings.Join([]string{
		"---",
		"title: 茨木童子",
		"status: draft",
		"---",
		"",
		"鬼手是左手。",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"> 类型：式神",
		"> 说明：旧版设定里写成左手。",
		"",
		"后面还有正文。",
	}, "\n"))

	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}

	rows, err := idx.EntitiesFor("docs/式神/茨木.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("该有 1 条来源行（这条纠正），实际 %d：%+v", len(rows), rows)
	}
	r := rows[0]
	if r.Name != "茨木童子" || r.Type != "式神" || r.Description != "鬼手是右手" {
		t.Errorf("读出来的纠正不对：%+v", r)
	}
	if r.Authority != AuthorityCorrected {
		t.Errorf("该标 corrected：%+v", r)
	}
	// 行号指到纠正块自己那一行（第 8 行起），要能直接跳过去。
	if r.FromLine != 8 || r.ToLine != 10 || r.Line != 8 {
		t.Errorf("该带纠正块的文件行号（8-10，line=8）：%+v", r)
	}
	// 状态仍旧现读 docs（派生表不复制会变的东西）。
	if r.Status != "draft" {
		t.Errorf("状态该是 draft：%+v", r)
	}

	st, err := idx.ExtractStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Corrected != 1 || st.Entities != 1 {
		t.Errorf("家底该是「1 行来源、其中 1 行纠错」：%+v", st)
	}

	found, ignored, err := idx.CorrectionStat()
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 || ignored != 0 {
		t.Errorf("该报「生效 1 条、未生效 0 条」，实际 %d/%d", found, ignored)
	}
}

// 抽取块里**不含**纠正块（那是给派生层的指令，不是原文事实），嵌入块里**要有**（检索要搜得到）。
func TestRebuildKeepsCorrectionsOutOfExtractChunks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/式神/茨木.md", strings.Join([]string{
		"---",
		"title: 茨木童子",
		"status: draft",
		"---",
		"",
		"鬼手是左手。",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"> 类型：式神",
		"",
		"后面还有正文。",
	}, "\n"))

	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{KindExtract, KindChunk} {
		chunks, err := idx.AllChunks(kind)
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range chunks {
			text += c.Text + "\n"
		}
		has := strings.Contains(text, "鬼手是右手")
		if kind == KindExtract && has {
			t.Errorf("抽取块不该带纠正块（会被再抽一遍）：%q", text)
		}
		if kind == KindChunk && !has {
			t.Errorf("嵌入块该保留纠正块（检索要搜得到更正后的说法）：%q", text)
		}
		if !strings.Contains(text, "鬼手是左手") {
			t.Errorf("%s 的块丢了正文：%q", kind, text)
		}
	}
}

// 整篇都是纠正块时，抽取块会是空的——空的抽取块不该写进索引。
func TestRebuildSkipsEmptyExtractChunks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/式神/只有纠正.md", strings.Join([]string{
		"---",
		"title: 只有纠正",
		"status: draft",
		"---",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"> 类型：式神",
	}, "\n"))

	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	chunks, err := idx.AllChunks(KindExtract)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chunks {
		if strings.TrimSpace(c.Text) == "" {
			t.Errorf("空的抽取块不该进索引：%+v", c)
		}
	}
	// 纠正本身仍然要生效（它的作用不依赖正文）。
	rows, err := idx.EntitiesFor("docs/式神/只有纠正.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Authority != AuthorityCorrected {
		t.Errorf("只有纠正块也该生效：%+v", rows)
	}
}

// 没写对的不生效，但要**计数 + 报出来**（不静默）。
func TestRebuildCountsIgnoredCorrections(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/式神/写错.md", strings.Join([]string{
		"---",
		"title: 写错",
		"status: draft",
		"---",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"",
		"正文。",
	}, "\n"))

	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	found, ignored, err := idx.CorrectionStat()
	if err != nil {
		t.Fatal(err)
	}
	if found != 0 || ignored != 1 {
		t.Errorf("该报「生效 0、未生效 1」，实际 %d/%d", found, ignored)
	}
	rows, err := idx.EntitiesFor("docs/式神/写错.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("没写对的纠正不该生效：%+v", rows)
	}
}

// 重建是幂等的：跑两遍不会出现两份纠正。
func TestRebuildCorrectionsAreIdempotent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/式神/茨木.md", strings.Join([]string{
		"---",
		"title: 茨木童子",
		"status: draft",
		"---",
		"",
		"> [!correction] 茨木童子：鬼手是右手",
		"> 类型：式神",
	}, "\n"))
	idx := New(root)
	for i := 0; i < 2; i++ {
		if err := idx.Rebuild(); err != nil {
			t.Fatal(err)
		}
	}
	st, err := idx.ExtractStat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entities != 1 || st.Corrected != 1 {
		t.Errorf("重建两遍仍该只有 1 行：%+v", st)
	}
}
