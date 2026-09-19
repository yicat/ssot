package vaultindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// newVault 造一个带文档与两张表（csv + json）的最小 vault。
func newVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "docs/式神/茨木童子.md",
		"---\ntitle: 茨木童子\nstatus: draft\n---\n\n三技能伤害系数 263%。\n")
	write(t, root, "docs/机制/伤害计算.md",
		"---\ntitle: 伤害计算\nstatus: published\n---\n\n最终伤害 = 攻击 × 系数。\n")
	write(t, root, "tables/技能倍率.csv", "技能,基础系数,备注\n罗生门,2.63,单体\n鬼手,1.2,群体\n")
	write(t, root, "tables/式神属性.json", `[{"名字":"茨木童子","稀有度":"SSR","标签":["输出"]}]`)
	return root
}

func TestRebuildAndSearch(t *testing.T) {
	root := newVault(t)
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if !idx.Exists() {
		t.Fatal("重建后索引该存在")
	}

	hits, err := idx.Search("伤害", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("该命中两篇：%+v", hits)
	}
	// 标题命中的那篇排前面。
	if hits[0].Path != "docs/机制/伤害计算.md" {
		t.Errorf("标题命中该排第一：%+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "伤害") {
		t.Errorf("片段该包含命中词：%q", hits[0].Snippet)
	}
	// 未核验状态要带出来：这条立场到处都要成立。
	for _, h := range hits {
		if h.Path == "docs/式神/茨木童子.md" && h.Status != "draft" {
			t.Errorf("draft 文档的状态该带出来：%+v", h)
		}
	}

	if _, err := idx.Search("  ", 10); err == nil {
		t.Error("空搜索词该报错，而不是返回全部")
	}
}

func TestTableInferenceAndQuery(t *testing.T) {
	root := newVault(t)
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}

	tables, err := idx.Tables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("该有两张表：%+v", tables)
	}
	byFile := map[string]struct {
		name string
		cols []string
		rows int
	}{}
	for _, tb := range tables {
		byFile[tb.File] = struct {
			name string
			cols []string
			rows int
		}{tb.Name, tb.Columns, tb.Rows}
	}
	csv := byFile["tables/技能倍率.csv"]
	if csv.name != "技能倍率" || csv.rows != 2 || len(csv.cols) != 3 {
		t.Errorf("csv 表该按文件名建名、按表头建列：%+v", csv)
	}
	if len(byFile["tables/式神属性.json"].cols) != 3 {
		t.Errorf("json 表的列该取所有键的并集：%+v", byFile)
	}

	// 数字列推断成 REAL 才聚合得出来（TEXT 列上 AVG 不是这个结果）。
	rs, err := idx.Query("SELECT round(AVG(基础系数),3) FROM 技能倍率", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0][0] != "1.915" {
		t.Fatalf("数字列该推断成 REAL：%+v", rs)
	}

	// 文档 front matter 也要能查。
	rs, err = idx.Query("SELECT count(*) FROM docs WHERE status='draft'", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0][0] != "1" {
		t.Fatalf("docs 表该能按 status 查：%+v", rs)
	}

	// 嵌套值不该被猜着展开，而是压成 JSON 文本。
	rs, err = idx.Query(`SELECT 标签 FROM 式神属性`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0][0] != `["输出"]` {
		t.Fatalf("嵌套值该存成 JSON 文本：%+v", rs)
	}
}

// 索引是派生的：**不允许**用 SQL 改它。两层防线都要在。
func TestQueryIsReadOnly(t *testing.T) {
	root := newVault(t)
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}

	if _, err := idx.Query("DELETE FROM docs", 10); err == nil {
		t.Error("非 SELECT/WITH 的语句该被前缀检查拒绝")
	} else if !strings.Contains(err.Error(), "只允许查询") {
		t.Errorf("拒绝的理由要说清：%v", err)
	}

	// 绕过前缀检查（以 WITH 开头）：还有 query_only 兜着——这才是真正的保证。
	if _, err := idx.Query("WITH x AS (SELECT 1) DELETE FROM docs", 10); err == nil {
		t.Error("绕过前缀检查的写语句必须被 query_only 拒绝")
	}
}

// 派生层的表**不对外**：查询口只放数据表与 `docs`（`derived.spec.md` §一、ADR 0007）。
//
// 这条是被真跑逼出来的：模型检索撞到上限后，转头 `SELECT ... FROM chunk` 去凑清单，
// 向量与图检索就成了可选。所以这里把「换着法子拿派生层」的几种写法都钉住。
func TestQueryRefusesUnderwaterTables(t *testing.T) {
	root := newVault(t)
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}

	// docs 仍然可查（front matter 是给人和 agent 明明白白读的）。
	if _, err := idx.Query("SELECT count(*) FROM docs", 10); err != nil {
		t.Errorf("docs 该能查：%v", err)
	}

	bad := []string{
		`SELECT text FROM chunk LIMIT 5`,                     // 直接查
		`SELECT * FROM (SELECT name FROM entity) x`,          // 藏在子查询里
		`WITH t AS (SELECT * FROM relation) SELECT * FROM t`, // 藏在 CTE 里
		`SELECT * FROM pragma_table_info('embedding')`,       // 表值函数
		`SELECT * FROM sqlite_master`,                        // 库自己的目录
		`SELECT * FROM embedding, docs`,                      // 逗号列表里混一个
	}
	for _, sql := range bad {
		_, err := idx.Query(sql, 10)
		if err == nil {
			t.Errorf("这条该被拒：%s", sql)
			continue
		}
		if !strings.Contains(err.Error(), "水下") {
			t.Errorf("拒绝的理由要说清「沉在水下」：%v", err)
		}
	}
}

// CountMatches 数的是**一共命中多少篇**，与 limit 无关（截断时要说得出「还有几篇」）。
func TestCountMatchesIgnoresLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		write(t, root, fmt.Sprintf("docs/甲%d.md", i), "---\ntitle: 甲\n---\n\n增益 正文。\n")
	}
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search("增益", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("limit 该生效：拿到 %d 条", len(hits))
	}
	total, err := idx.CountMatches("增益")
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("总数该是 4（不受 limit 影响），拿到 %d", total)
	}
	if _, err := idx.CountMatches("  "); err == nil {
		t.Error("空搜索词该报错")
	}
}

func TestQueryLimitAndTableNameSanitising(t *testing.T) {
	root := t.TempDir()
	write(t, root, "tables/技能 倍率-v2.csv", "a,b\n1,2\n3,4\n5,6\n")
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	tables, err := idx.Tables()
	if err != nil || len(tables) != 1 {
		t.Fatalf("该识别出一张表：%+v %v", tables, err)
	}
	// 文件名里的空格与减号不能进 SQL 标识符，要规范化成下划线。
	if tables[0].Name != "技能_倍率_v2" {
		t.Errorf("表名该被规范化：%q", tables[0].Name)
	}
	rs, err := idx.Query("SELECT a FROM "+quoteIdent(tables[0].Name), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rows) != 2 {
		t.Errorf("limit 该截断行数：%+v", rs)
	}
}

// 单张表坏掉不该让整份索引建不出来。
func TestBrokenTableDoesNotBreakRebuild(t *testing.T) {
	root := newVault(t)
	write(t, root, "tables/坏的.json", `{"不是":"对象数组"}`)
	idx := New(root)
	if err := idx.Rebuild(); err != nil {
		t.Fatalf("坏表该被跳过，而不是让重建失败：%v", err)
	}
	tables, err := idx.Tables()
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range tables {
		if tb.File == "tables/坏的.json" {
			t.Error("坏表不该出现在索引里")
		}
	}
	docs, err := idx.Search("伤害", 10)
	if err != nil || len(docs) != 2 {
		t.Errorf("文档部分该照常建好：%d %v", len(docs), err)
	}
}

func TestSearchBeforeRebuildExplainsItself(t *testing.T) {
	idx := New(t.TempDir())
	_, err := idx.Search("伤害", 10)
	if err == nil {
		t.Fatal("没建索引时该报错")
	}
	if !strings.Contains(err.Error(), "index") {
		t.Errorf("要说清怎么办（跑 index），而不是只说没有：%v", err)
	}
}
