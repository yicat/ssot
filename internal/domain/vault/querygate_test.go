package vault

import (
	"strings"
	"testing"
)

func TestSQLTableRefs(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"简单", `SELECT count(*) FROM docs`, []string{"docs"}},
		{"中文表名要引号", `SELECT round(AVG(基础系数),3) FROM "技能倍率"`, []string{"技能倍率"}},
		{"别名与 JOIN", `SELECT * FROM docs d JOIN "增益减益" g ON d.title=g.name`, []string{"docs", "增益减益"}},
		{"子查询里的表也要看", `SELECT * FROM (SELECT 1 FROM chunk) x`, []string{"chunk"}},
		{"逗号列表", `SELECT * FROM 甲, 乙 WHERE 1=1`, []string{"甲", "乙"}},
		{"字符串里的 FROM 不算", `SELECT * FROM docs WHERE body LIKE '%FROM chunk%'`, []string{"docs"}},
		{"注释里的 FROM 不算", "SELECT * FROM /* FROM chunk */ docs", []string{"docs"}},
		{"CTE 名字不算表", `WITH t AS (SELECT * FROM docs) SELECT * FROM t`, []string{"docs"}},
		{"CTE 带列名与多个", `WITH a(x) AS (SELECT 1), b AS (SELECT * FROM entity) SELECT * FROM a, b`, []string{"entity"}},
		{"表值函数会被记下（等着被拒）", `SELECT * FROM pragma_table_info('chunk')`, []string{"pragma_table_info"}},
		{"schema 前缀取最后一段", `SELECT * FROM main.docs`, []string{"docs"}},
		{"反引号与方括号", "SELECT * FROM `chunk` , [entity]", []string{"chunk", "entity"}},
		{"同一个表出现两次只算一个", `SELECT * FROM docs a JOIN docs b ON a.path=b.path`, []string{"docs"}},
		{"没有 FROM", `SELECT 1`, nil},
		{"反引号里的双引号", "SELECT * FROM `a\"b`", []string{`a"b`}},
	}
	for _, c := range cases {
		got, err := SQLTableRefs(c.sql)
		if err != nil {
			t.Errorf("%s：不该报错，却报 %v", c.name, err)
			continue
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s：拿到 %v，想要 %v", c.name, got, c.want)
		}
	}
}

// 看不懂的一律报错：门是 fail closed 的，宁可拒掉一条合法查询，也不能放行一条读不懂的。
func TestSQLTableRefsFailsClosed(t *testing.T) {
	bad := []string{
		``,
		`   `,
		`SELECT 1 FROM`,
		`SELECT * FROM 'chunk'`,   // 字符串当表名
		`SELECT * FROM (SELECT 1`, // 括号不配对
		`SELECT * FROM "chunk`,    // 引号不配对
		`SELECT * FROM docs WHERE body = 'it''s ok`, // 字符串没结束
		`WITH t AS SELECT 1`,                        // CTE 缺括号
		`SELECT * FROM /* 没结束`,
		`SELECT * FROM 1 + 2`, // FROM 后面不是表名
	}
	for _, sql := range bad {
		if _, err := SQLTableRefs(sql); err == nil {
			t.Errorf("这条该被拒：%q", sql)
		}
	}
}

func TestTableQueryable(t *testing.T) {
	dataTables := []string{"技能倍率", "增益减益"}
	cases := []struct {
		name string
		want bool
	}{
		{"docs", true},
		{"DOCS", true},
		{`"docs"`, true},
		{"技能倍率", true},
		{`"技能倍率"`, true},
		{"增益减益", true},
		{"chunk", false},
		{"embedding", false},
		{"entity", false},
		{"relation", false},
		{"tables_meta", false},
		{"sqlite_master", false},
		{"", false},
	}
	for _, c := range cases {
		if got := TableQueryable(c.name, dataTables); got != c.want {
			t.Errorf("TableQueryable(%q) = %v，想要 %v", c.name, got, c.want)
		}
	}
}

// 派生层的表名清单：与 derived.spec.md §一 那张表同步；写死在这里是为了让实现与测试
// 指着同一个清单（判定本身用白名单，不靠它）。
func TestReservedTablesCoversUnderwater(t *testing.T) {
	have := map[string]bool{}
	for _, n := range ReservedTables() {
		have[n] = true
	}
	for _, want := range []string{"chunk", "extract_chunk", "embedding", "entity", "relation"} {
		if !have[want] {
			t.Errorf("派生层的表 %q 不在清单里", want)
		}
	}
	for _, n := range ReservedTables() {
		if TableQueryable(n, nil) {
			t.Errorf("派生层的表 %q 不该放行", n)
		}
	}
}
