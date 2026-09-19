package vault

import (
	"fmt"
	"strings"
)

// 这个文件只放**纯规则**：查询口能查哪些表、一条 SQL 提到了哪些表。
// 不碰数据库、不碰文件——真正执行在 `infrastructure/vaultindex`。
//
// 为什么要这层门：`table_query` 原来是「对索引库随便一条只读 SQL」，于是派生层的
// `chunk`（**含块正文**）/`embedding`/`entity`/`relation` 全都 reachable。
// 实测里模型就是这么干的——检索撞到上限，它转头 `SELECT ... FROM chunk` 去凑清单，
// 向量与图检索成了可选。这与「派生层沉到水下」（ADR 0007、`derived.spec.md` §一）冲突。
//
// 规则是**白名单**：只放数据表（`tables_meta` 里登记的，名字是动态的）+ `docs`
// （文档的 front matter 字段）。别的表名一律拒。

// QueryableDocsTable 是除数据表之外**唯一**放行的表：文档的 front matter。
// （正文也在 `docs.body` 里，但那是给人/给 agent 明明白白读的文档内容，不是派生层。）
const QueryableDocsTable = "docs"

// ReservedTables 是派生层的表名（**不对外**）。留着它是为了让实现与测试能指着同一个清单，
// 判定本身**不看这张黑名单**——白名单之外的一律拒（这样将来新加的内部表自动被挡住，
// 不需要记得回来补名单）。
func ReservedTables() []string {
	return []string{
		"chunk", "extract_chunk", "embedding", "entity", "relation",
		"sync", "links", "tables_meta", "meta",
	}
}

// TableQueryable 判断一个表名能不能过查询口。dataTables 是数据表的表名（调用方从
// `tables_meta` 读出来）；比较**不分大小写**，两头都去掉引号。
func TableQueryable(name string, dataTables []string) bool {
	want := normalizeTableName(name)
	if want == "" {
		return false
	}
	if want == QueryableDocsTable {
		return true
	}
	for _, t := range dataTables {
		if normalizeTableName(t) == want {
			return true
		}
	}
	return false
}

func normalizeTableName(name string) string {
	n := strings.TrimSpace(name)
	n = strings.Trim(n, "\"`[]")
	return strings.ToLower(n)
}

// SQLTableRefs 从一条查询里挑出**被引用的表名**（`FROM` / `JOIN` 后面那个标识符），
// 全部小写、去重。WITH 里定义的临时名字（CTE）不算表引用。
//
// 为什么自己扫：sqlite 驱动没有暴露 authorizer，而这道门必须 **fail closed**——
// 读不出来就报错、让调用方拒掉，而不是「看不懂就当没问题」。
// 所以只认我们允许的语法子集，认不出来的一律返回错误：
//   - 引号/括号不配对；
//   - `FROM` / `JOIN` 后面不是标识符、也不是子查询；
//   - `FROM 'x'`（字符串当表名）这种写法。
//
// 认得的形态：`FROM a`、`FROM a, b`、`JOIN a ON …`、`FROM "中文表名"`、`FROM main.a`
// （取最后一段）、`FROM (SELECT …) x`（子查询里的 FROM 同样会被扫到）、`WITH t AS (…)`。
func SQLTableRefs(stmt string) ([]string, error) {
	toks, err := lexSQL(stmt)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("查询是空的")
	}

	ctes := map[string]bool{}
	if strings.EqualFold(toks[0].val, "WITH") {
		if _, err := skipCTEHeader(toks, 1, ctes); err != nil {
			return nil, err
		}
	}

	// ⚠️ 从 **0** 开始扫，不是从 CTE 头之后：CTE 体里的 FROM 也是真正的表引用
	// （`WITH t AS (SELECT * FROM docs)` 里的 `docs` 必须被看见——跳过整个体就会漏）。
	// CTE 自己的名字在 ctes 里，扫到也不当成表。
	var out []string
	i := 0
	for ; i < len(toks); i++ {
		if toks[i].kind != tokIdent {
			continue
		}
		kw := strings.ToUpper(toks[i].val)
		if kw != "FROM" && kw != "JOIN" {
			continue
		}
		j := i + 1
		for {
			if j >= len(toks) {
				return nil, fmt.Errorf("%s 后面什么都没有", kw)
			}
			switch {
			case toks[j].val == "(":
				// 子查询 / 括号包起来的表表达式：里面的 FROM 会被这一趟扫描扫到。
				j, err = skipParens(toks, j)
				if err != nil {
					return nil, err
				}
			case isTableNameTok(toks[j]):
				name := strings.ToLower(toks[j].val)
				// schema.table（如 main.docs）：认最后一段。
				for j+2 < len(toks) && toks[j+1].val == "." && toks[j+2].kind == tokIdent {
					name = strings.ToLower(toks[j+2].val)
					j += 2
				}
				if !ctes[name] {
					out = append(out, name)
				}
				j++
				// 表值函数（`pragma_table_info('chunk')` 这种）：名字已经记下，
				// 它不在白名单里，自然会被拒——这里只把括号跳过去。
				if j < len(toks) && toks[j].val == "(" {
					j, err = skipParens(toks, j)
					if err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("%s 后面读不出表名（遇到了 %q）", kw, toks[j].val)
			}
			// `FROM a, b` 这种逗号列表继续读；别的（WHERE/ON/GROUP…）就停。
			if j < len(toks) && toks[j].val == "," {
				j++
				continue
			}
			break
		}
	}
	return dedupLower(out), nil
}

// skipCTEHeader 跳过 `WITH [RECURSIVE] name [(cols)] AS (…) [, name2 AS (…)]`，
// 把临时名字收进 ctes，返回主查询开始的下标。
func skipCTEHeader(toks []sqlTok, i int, ctes map[string]bool) (int, error) {
	if i < len(toks) && strings.EqualFold(toks[i].val, "RECURSIVE") {
		i++
	}
	for i < len(toks) {
		if toks[i].kind != tokIdent {
			return 0, fmt.Errorf("WITH 后面该跟一个名字")
		}
		name := strings.ToLower(toks[i].val)
		i++
		if i < len(toks) && toks[i].val == "(" { // 列名表
			next, err := skipParens(toks, i)
			if err != nil {
				return 0, err
			}
			i = next
		}
		if i < len(toks) && strings.EqualFold(toks[i].val, "AS") {
			i++
			// AS [NOT] MATERIALIZED
			if i < len(toks) && strings.EqualFold(toks[i].val, "NOT") {
				i++
			}
			if i < len(toks) && strings.EqualFold(toks[i].val, "MATERIALIZED") {
				i++
			}
		}
		if i >= len(toks) || toks[i].val != "(" {
			return 0, fmt.Errorf("WITH 子句缺一个括号包起来的查询")
		}
		next, err := skipParens(toks, i)
		if err != nil {
			return 0, err
		}
		i = next
		ctes[name] = true
		if i < len(toks) && toks[i].val == "," {
			i++
			continue
		}
		break
	}
	return i, nil
}

// skipParens 从 `(` 跳到配对的 `)` 之后；不配对就报错（fail closed）。
func skipParens(toks []sqlTok, i int) (int, error) {
	depth := 0
	for ; i < len(toks); i++ {
		if toks[i].kind != tokPunct {
			continue
		}
		switch toks[i].val {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("括号不配对")
}

// isTableNameTok：这个记号能不能当表名读。
// 带引号的都算（`"1"` 可以是一张表的名字）；**裸写的纯数字不算**——
// `FROM 1 + 2` 该被拒，而不是去查一张叫 `1` 的表。
func isTableNameTok(t sqlTok) bool {
	if t.kind != tokIdent {
		return false
	}
	if t.quoted {
		return true
	}
	return !isNumericLiteral(t.val)
}

// isNumericLiteral：纯数字（含小数、可带正负号）。用来把 `LIMIT 10`、`1 + 2` 这类
// 数字与表名分开。
func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	dots := 0
	digits := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] >= '0' && s[i] <= '9':
			digits++
		case s[i] == '.':
			dots++
		default:
			return false
		}
	}
	return digits > 0 && dots <= 1
}

func dedupLower(in []string) []string {	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// —— 一点点词法。只够认识 SELECT 语句里的名字、字符串与括号，
// 不追求做完整 SQL 词法（门要的是「看不懂就拒」，不是「什么都能认」）。

type tokKind int

const (
	tokIdent tokKind = iota // 标识符（含关键字、数字、中文表名）
	tokString               // 字符串字面量：只用来跳过，表名不该长这样
	tokPunct                // 单个符号：括号、逗号、点、分号…
)

type sqlTok struct {
	kind tokKind
	val  string
	// quoted 表示它是引号/方括号包起来的名字（`"chunk"`、`` `chunk` ``、`[chunk]`）。
	// 区分它是为了认「裸写的纯数字不是表名」，同时不影响 `"1"` 这种真名。
	quoted bool
}

func lexSQL(s string) ([]sqlTok, error) {
	var out []sqlTok
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-': // 行注释
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*': // 块注释
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("块注释没结束")
			}
			i += 2 + end + 2
		case c == '\'':
			j := i + 1
			for {
				if j >= len(s) {
					return nil, fmt.Errorf("单引号没配上")
				}
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' { // '' 转义
						j += 2
						continue
					}
					break
				}
				j++
			}
			out = append(out, sqlTok{kind: tokString, val: s[i : j+1]})
			i = j + 1
		case c == '"' || c == '`':
			quote := c
			j := i + 1
			for {
				if j >= len(s) {
					return nil, fmt.Errorf("引号没配上")
				}
				if s[j] == quote {
					if j+1 < len(s) && s[j+1] == quote { // 连写两个表示一个
						j += 2
						continue
					}
					break
				}
				j++
			}
			out = append(out, sqlTok{kind: tokIdent, val: strings.ReplaceAll(s[i+1:j], string(quote)+string(quote), string(quote)), quoted: true})
			i = j + 1
		case c == '[':
			j := strings.IndexByte(s[i+1:], ']')
			if j < 0 {
				return nil, fmt.Errorf("方括号没配上")
			}
			out = append(out, sqlTok{kind: tokIdent, val: s[i+1 : i+1+j], quoted: true})
			i += j + 2
		case isIdentByte(c):
			j := i
			for j < len(s) && isIdentByte(s[j]) {
				j++
			}
			out = append(out, sqlTok{kind: tokIdent, val: s[i:j]})
			i = j
		default:
			out = append(out, sqlTok{kind: tokPunct, val: string(c)})
			i++
		}
	}
	return out, nil
}

// isIdentByte：字母、数字、下划线、$，以及所有非 ASCII 字节（中文表名/列名要能当一个整体）。
func isIdentByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '_' || c == '$':
		return true
	case c >= 0x80:
		return true
	}
	return false
}
