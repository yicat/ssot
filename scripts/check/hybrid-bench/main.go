// hybrid-bench —— 量「混合检索（向量 + 标题/标签/正文字面项）比纯向量好在哪」。
//
// 做什么：**复现 docs/notes/embedding-spike.md §六 的方法**，但走**生产代码路径**
// （`vaultapp.Searcher`，也就是界面/MCP 会用的那条）：
//   - 取按路径排序的前 N 篇（默认 200，与 spike 对齐），每篇取 首/中/尾 三句真句子当查询；
//   - 检索池 = 这 N 篇的全部块；ground truth = 「同文档、且包含该句」的块
//     （没有这种块就退而认该文档的第一块）；
//   - 另加一组**标题式查询**（`<标题> 是什么？`）——这一组考的是实体名，纯向量最弱（R@1 21.6%）；
//   - 对多套权重各跑一遍（β_title × β_body），打印 R@1 / R@5 / MRR 与 head/mid/tail 分解。
//
// 为什么要有它：P2 的验收线是「≥ 纯向量基线 61.6%/79.8%，且实体名式查询 R@1 明显上升」——
// 没有这个脚本，那句话就只是口号。它也**顺手验证基线可复现**（跑纯向量那行应该接近 61.6%/79.8%）。
//
// ⚠️ 什么时候不该用：
//   - 查询是文档原句，属于「自我检索」，不等于真人提问；只能比**相对**差别。
//   - 它不测抽取质量（那要真调 LLM）。
//   - 池子限定在前 N 篇：这是为了跟 spike 的数字可比，不代表全库表现。
//
// 用法：
//
//	SSOT_EMBED_DIR=<模型目录> go run ./scripts/check/hybrid-bench -root <vault> [-docs 200] [-limit 500]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultfs"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultindex"
	"github.com/ngnl5/ssot/internal/infrastructure/vembed"
)

// query 是一条评测查询。
type query struct {
	doc   string // 期望命中的文档
	text  string // 查询串
	kind  string // head / mid / tail / title
	title string // 标题式查询的答案句（标题组的 ground truth 用答案句定位）
}

// poolChunk 是检索池里的一块。
type poolChunk struct {
	doc  string
	ord  int
	text string
}

// variant 是一套字面项权重。
type variant struct {
	name string
	w    vault.HybridWeights
}

func main() {
	root := flag.String("root", "projects/demo", "vault 根目录")
	nDocs := flag.Int("docs", 200, "取前多少篇当池子（与 spike 对齐是 200）")
	limit := flag.Int("limit", 500, "每次检索取多少条（要够大，过滤到池内才等价位次）")
	out := flag.String("json", "", "把结果写成 JSON（可选）")
	debug := flag.Int("debug", 0, "打印前 N 条没排进前 5 的查询的诊断信息")
	flag.Parse()

	start := time.Now()
	svc := vaultapp.New(*root)
	svc.SetEmbedModelDir(vembed.DefaultModelDir(""))

	docs, err := vaultfs.New(*root).Load()
	must(err)
	if len(docs) < *nDocs {
		fmt.Printf("⚠️ vault 里只有 %d 篇，少于要的 %d 篇\n", len(docs), *nDocs)
	}
	poolDocs := map[string]bool{}
	for i, d := range docs {
		if i >= *nDocs {
			break
		}
		poolDocs[d.Path] = true
	}

	queries, titleQueries := buildQueries(*root, docs, *nDocs)
	fmt.Printf("池子 %d 篇 ／ 正文句查询 %d 条 ／ 标题式查询 %d 条\n", len(poolDocs), len(queries), len(titleQueries))
	if len(queries) == 0 {
		fmt.Println("没生成任何查询：vault 里的句子太短或太少")
		os.Exit(1)
	}

	// 索引准备（结构版本不对就重建）。
	idx := vaultindex.New(*root)
	if !idx.Ready() {
		fmt.Println("索引不存在或结构版本对不上：先重建（可能要几分钟）")
		must(idx.Rebuild())
	}
	all, err := idx.AllChunks(vaultindex.KindChunk)
	must(err)
	// 池内块（按 doc, ord，与索引一致）。
	var pool []poolChunk
	firstOfDoc := map[string]int{}
	for _, c := range all {
		if !poolDocs[c.Doc] {
			continue
		}
		if _, ok := firstOfDoc[c.Doc]; !ok {
			firstOfDoc[c.Doc] = len(pool)
		}
		pool = append(pool, poolChunk{doc: c.Doc, ord: c.Ord, text: c.Text})
	}
	fmt.Printf("池内块 %d 个（全库 %d 个）\n\n", len(pool), len(all))

	// ground truth：与 spike 的 recall.mjs **同一条规则**——优先「同文档且包含该句」的块，
	// 没有就退化为「同文档里排最前的块」。两边规则不一致，数字就没法比（踩过）。
	fmt.Printf("ground truth 口径：同文档优先含句、退化为同文档最靠前（与 recall.mjs 一致）\n\n")

	variants := []variant{
		{"纯向量（β_title=0）", vault.HybridWeights{}},
		{"β_title=0.05", vault.HybridWeights{TitleTags: 0.05}},
		{"β_title=0.1", vault.HybridWeights{TitleTags: 0.1}},
		{"β_title=0.2（默认候选）", vault.HybridWeights{TitleTags: 0.2}},
		{"β_title=0.3", vault.HybridWeights{TitleTags: 0.3}},
		{"β_title=0.2 + β_body=0.1", vault.HybridWeights{TitleTags: 0.2, Body: 0.1}},
		{"β_title=0.2 + β_body=0.3", vault.HybridWeights{TitleTags: 0.2, Body: 0.3}},
	}

	// 查询向量编码一次，所有变体复用（省时间，也让变体之间只差权重）。
	first, err := svc.NewSearcher(vault.HybridWeights{})
	must(err)
	defer first.Close()
	fmt.Printf("检索器：%d 块在内存 %.1f MB，加载 %.0f ms\n", first.Chunks(),
		float64(first.MemoryBytes())/(1<<20), float64(first.LoadDuration().Milliseconds()))

	qv := make([][]float32, len(queries))
	embedStart := time.Now()
	for i, q := range queries {
		qv[i], err = first.Embed(q.text)
		must(err)
	}
	tq := make([][]float32, len(titleQueries))
	for i, q := range titleQueries {
		tq[i], err = first.Embed(q.text)
		must(err)
	}
	fmt.Printf("编码 %d 条查询用了 %.1f 秒\n\n", len(queries)+len(titleQueries), time.Since(embedStart).Seconds())

	type result struct {
		variant     string
		r1, r5, mrr float64
		byKind      map[string][2]float64
		titleR1     float64
		titleR5     float64
		msPerQuery  float64
	}
	var results []result

	for _, v := range variants {
		searcher, err := idx.NewSearcher(vaultindex.KindChunk, v.w)
		must(err)

		// 正文句查询。
		ranks := make([]int, len(queries))
		exactCount := 0
		spent := time.Duration(0)
		inDoc := 0 // 前 5 条里有该文档的块的查询数（粗看「召回对没对到该篇」）
		for i, q := range queries {
			t0 := time.Now()
			hits, err := searcher.Search(q.text, qv[i], *limit)
			must(err)
			spent += time.Since(t0)
			r, isExact := rankOf(hits, poolDocs, q.doc, q.text)
			ranks[i] = r
			if isExact {
				exactCount++
			}
			for k, h := range hits {
				if k >= 5 {
					break
				}
				if h.Doc == q.doc {
					inDoc++
					break
				}
			}
			if *debug > 0 && ranks[i] > 4 {
				*debug--
				fmt.Printf("  [debug] 查询 %q（%s，%s）\n", trim(q.text, 26), q.kind, q.doc)
				for k, h := range hits {
					if k >= 3 {
						break
					}
					same := ""
					if h.Doc == q.doc {
						same = "（同文档）"
					}
					fmt.Printf("          top%d %.4f %s #%d %s\n", k+1, h.Score, h.Doc, h.Ord, same)
				}
				if ranks[i] < 0 {
					fmt.Printf("          gt 不在返回的 %d 条里\n", len(hits))
				} else {
					fmt.Printf("          gt 位次 %d\n", ranks[i])
				}
			}
		}
		_ = inDoc
		r1 := ratio(ranks, 1)
		r5 := ratio(ranks, 5)
		mrr := 0.0
		for _, r := range ranks {
			if r >= 0 {
				mrr += 1 / float64(r+1)
			}
		}
		mrr /= float64(len(ranks))

		byKind := map[string][2]float64{}
		for _, k := range []string{"head", "mid", "tail"} {
			var sub []int
			for i, q := range queries {
				if q.kind == k {
					sub = append(sub, ranks[i])
				}
			}
			byKind[k] = [2]float64{ratio(sub, 1), ratio(sub, 5)}
		}

		// 标题式查询。
		titleRanks := make([]int, len(titleQueries))
		for i, q := range titleQueries {
			hits, err := searcher.Search(q.text, tq[i], *limit)
			must(err)
			// 标题组同一条规则：答案句优先（`<标题> 是什么？` 的答案就是那句 head 句）。
			r, _ := rankOf(hits, poolDocs, q.doc, q.title)
			titleRanks[i] = r
		}

		results = append(results, result{
			variant: v.name, r1: r1, r5: r5, mrr: mrr, byKind: byKind,
			titleR1: ratio(titleRanks, 1), titleR5: ratio(titleRanks, 5),
			msPerQuery: float64(spent.Microseconds()) / 1000 / float64(len(queries)),
		})
	}

	fmt.Println("正文句查询（n=" + fmt.Sprint(len(queries)) + "）")
	fmt.Printf("%-26s %7s %7s %7s  %-19s %s\n", "变体", "R@1", "R@5", "MRR", "head R@1/R@5", "mid R@1/R@5  tail R@1/R@5  每次 ms")
	for _, r := range results {
		fmt.Printf("%-26s %6.1f%% %6.1f%% %7.3f  %5.1f%%/%5.1f%%  %5.1f%%/%5.1f%%  %5.1f%%/%5.1f%%  %6.1f\n",
			r.variant, r.r1*100, r.r5*100, r.mrr,
			r.byKind["head"][0]*100, r.byKind["head"][1]*100,
			r.byKind["mid"][0]*100, r.byKind["mid"][1]*100,
			r.byKind["tail"][0]*100, r.byKind["tail"][1]*100, r.msPerQuery)
	}
	fmt.Println("\n标题式查询（`<标题> 是什么？`，n=" + fmt.Sprint(len(titleQueries)) + "）")
	fmt.Printf("%-26s %7s %7s\n", "变体", "R@1", "R@5")
	for _, r := range results {
		fmt.Printf("%-26s %6.1f%% %6.1f%%\n", r.variant, r.titleR1*100, r.titleR5*100)
	}
	fmt.Printf("\n总耗时 %.1f 秒\n", time.Since(start).Seconds())

	if *out != "" {
		b, err := json.MarshalIndent(results, "", "  ")
		must(err)
		must(os.WriteFile(*out, b, 0o644))
		fmt.Println("结果已写入 " + *out)
	}
}

// rankOf 按 spike 的规则给一条查询打位次：
// 优先「同文档且包含该句」的块，没有就退化为「同文档里排最前的块」；两者都没有返回 -1。
//
// 生产的 Searcher 池子是全库，评测要的是前 N 篇（与 spike 对齐）：只要取够多（limit 远大于池内前几名），
// 「过滤掉池外块」得到的顺序与「只在池内排序」一致，所以可以这样换算。
func rankOf(hits []vault.VectorHit, poolDocs map[string]bool, doc, sentence string) (int, bool) {
	firstSame, firstExact := -1, -1
	n := 0
	for _, h := range hits {
		if !poolDocs[h.Doc] {
			continue
		}
		if h.Doc == doc {
			if firstSame < 0 {
				firstSame = n
			}
			if firstExact < 0 && strings.Contains(h.Text, sentence) {
				firstExact = n
			}
		}
		n++
	}
	if firstExact >= 0 {
		return firstExact, true
	}
	return firstSame, false
}

func ratio(ranks []int, k int) float64 {
	if len(ranks) == 0 {
		return 0
	}
	hit := 0
	for _, r := range ranks {
		if r >= 0 && r < k {
			hit++
		}
	}
	return float64(hit) / float64(len(ranks))
}

// buildQueries 复现 spike 的查询集：每篇 首/中/尾 三句真句子 + 标题式查询。
//
// ⚠️ 句子提取规则必须与 `recall.mjs` 一致，否则两边的数字不可比：
// 丢掉标题行与 `|>-*` 开头的行、丢掉长度 ≤12 的行，**行与行直接拼接**（不加分隔符），
// 再按 `。！？；` 切句，保留 12～60 字的句子。
func buildQueries(root string, docs []vault.Doc, nDocs int) ([]query, []query) {
	var body, titles []query
	for i, d := range docs {
		if i >= nDocs {
			break
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(d.Path)))
		must(err)
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		var kept []string
		for _, line := range strings.Split(text, "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, ">") ||
				strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") || len([]rune(t)) <= 12 {
				continue
			}
			kept = append(kept, line)
		}
		joined := strings.Join(kept, "")
		var sents []string
		for _, s := range splitAfter(joined, "。！？；") {
			s = strings.TrimSpace(s)
			if n := len([]rune(s)); n >= 12 && n <= 60 {
				sents = append(sents, s)
			}
		}
		if len(sents) == 0 {
			continue
		}
		picks := []struct {
			idx  int
			kind string
		}{{0, "head"}}
		if len(sents) >= 3 {
			picks = append(picks, struct {
				idx  int
				kind string
			}{len(sents) / 2, "mid"})
		}
		if len(sents) >= 2 {
			picks = append(picks, struct {
				idx  int
				kind string
			}{len(sents) - 1, "tail"})
		}
		seen := map[string]bool{}
		var head string
		for _, p := range picks {
			if seen[sents[p.idx]] {
				continue
			}
			seen[sents[p.idx]] = true
			body = append(body, query{doc: d.Path, text: sents[p.idx], kind: p.kind})
			if p.kind == "head" {
				head = sents[p.idx]
			}
		}
		if head != "" {
			t := d.Title
			if t == "" {
				t = strings.TrimSuffix(filepath.Base(d.Path), filepath.Ext(d.Path))
			}
			titles = append(titles, query{doc: d.Path, text: t + " 是什么？", kind: "title", title: head})
		}
	}
	return body, titles
}

// splitAfter 在给定标点之后切分（保留标点），与 JS 的 lookbehind 切分一致。
func splitAfter(s, ends string) []string {
	var out []string
	var buf strings.Builder
	for _, r := range s {
		buf.WriteRune(r)
		if strings.ContainsRune(ends, r) {
			out = append(out, buf.String())
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

// trim 按字符数截断（终端里看查询长什么样）。
func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
		os.Exit(1)
	}
}
