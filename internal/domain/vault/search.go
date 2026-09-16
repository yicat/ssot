package vault

import (
	"sort"
	"strings"
)

// Hit 是一条检索命中。
//
// 只带「哪里命中、命中什么」——不含任何存储细节，所以换掉索引实现（现在是 SQLite）
// 也不用动这一层。
type Hit struct {
	// Path 命中的文档（相对 vault 根）。
	Path string
	// Layer / Status 让界面能把「未核验」标出来（这条立场到处都要成立）。
	Layer  Layer
	Status Status
	Title  string
	// Snippet 是命中词附近的一段原文。
	Snippet string
	// TitleMatch 表示标题里就命中了（排序时优先）。
	TitleMatch bool
	// Occurrences 是正文里出现的次数。
	Occurrences int
}

// CountOccurrences 数 needle 在 text 里出现几次（大小写不敏感，中文无关大小写）。
func CountOccurrences(text, needle string) int {
	if needle == "" {
		return 0
	}
	lower, n := strings.ToLower(text), strings.ToLower(needle)
	count, start := 0, 0
	for {
		i := strings.Index(lower[start:], n)
		if i < 0 {
			return count
		}
		count++
		start += i + len(n)
		if start >= len(lower) {
			return count
		}
	}
}

// Snippet 取 needle 第一次出现处附近的一段，两端按需加省略号。
//
// 按**字节**切会影响中文（一个汉字 3 字节），所以按 rune 切。
// 找不到 needle 时返回开头一段——至少让人看到这篇讲的是什么。
func Snippet(text, needle string, width int) string {
	if width <= 0 {
		width = 40
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return ""
	}
	at := -1
	if needle != "" {
		if i := strings.Index(strings.ToLower(text), strings.ToLower(needle)); i >= 0 {
			at = len([]rune(text[:i]))
		}
	}
	if at < 0 {
		at = 0
	}
	start := at - width/2
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(runes) {
		end = len(runes)
	}
	out := string(runes[start:end])
	// 换行会毁掉单行展示，压成空格。
	out = strings.Join(strings.Fields(out), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// RankHits 排序检索结果：标题命中优先 → 出现次数多优先 → 路径稳定排序。
//
// 排序完全确定（没有随机、没有相关度模型）：同一份 vault 同一次查询，
// 结果顺序永远一样——不然「上次搜到的第一条」就说不清了。
func RankHits(hits []Hit) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.TitleMatch != b.TitleMatch {
			return a.TitleMatch
		}
		if a.Occurrences != b.Occurrences {
			return a.Occurrences > b.Occurrences
		}
		return a.Path < b.Path
	})
}

// TableInfo 是索引里的一张数据表。
type TableInfo struct {
	// Name 是查询时用的表名（由文件名规范化而来，见 infra 层）。
	Name string
	// File 是它在 vault 里的相对路径（tables/xxx.csv）。
	File string
	// Format 是 csv / json / yaml。
	Format string
	Rows   int
	// Columns 是列名，按表里的顺序。
	Columns []string
}

// ResultSet 是一次查询的结果。
//
// 单元格一律转成字符串给外面（数字、布尔、null 都在这里定成文本），
// 免得每种调用方各写一套转换；要算数就交给 SQL。
type ResultSet struct {
	Columns []string
	Rows    [][]string
}
