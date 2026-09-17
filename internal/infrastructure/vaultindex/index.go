// Package vaultindex 是 vault 的派生索引：把文档与数据表装进 SQLite，供检索与查询。
//
// 三条纪律：
//  1. **它全是派生的**：删掉 `.data/index.db` 能从 `docs/` + `raw/` + `tables/` 重建，
//     所以它不进版本控制，也**不允许被当数据源**（见 vault.spec.md §4）。
//  2. **只读查询**：查询连接置 `PRAGMA query_only = 1`，并且只放行 SELECT / WITH——
//     改数据的入口是文档与表格文件本身，不是 SQL。
//  3. **中文检索用 LIKE**：FTS5 默认分词器对中文是整段当一个词，反而搜不准
//     （旧方案实测过 LIKE vs FTS5，结论是用 LIKE）。这个规模下 LIKE 足够，
//     而且行为可预测——不用为检索装一套分词器。
package vaultindex

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite" // 纯 Go 驱动：本机没有 gcc，用不了 cgo 的 mattn/go-sqlite3

	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/infrastructure/vaultfs"
)

// Index 是某个 vault 的索引。
type Index struct {
	root string
}

// New 构造索引（不建文件，Rebuild 时才落盘）。
func New(root string) *Index { return &Index{root: root} }

// Path 返回索引文件路径：`<vault>/.data/index.db`。
func (idx *Index) Path() string { return filepath.Join(idx.root, ".data", "index.db") }

// Exists 报告索引是否已经建过。
func (idx *Index) Exists() bool {
	_, err := os.Stat(idx.Path())
	return err == nil
}

// SchemaVersion 是索引**结构**的版本：表加了一列一张、键换了，就 +1。
//
// 为什么要它：索引是派生的，所以「结构变了」的正确反应是**自动重建**，
// 而不是让人撞上 `no such table: embedding` 这种看不懂的错。
const SchemaVersion = 2

// Ready 报告索引存在**且结构版本对得上**（对不上就该重建）。
func (idx *Index) Ready() bool {
	if !idx.Exists() {
		return false
	}
	db, err := idx.open()
	if err != nil {
		return false
	}
	defer db.Close()
	var v string
	if err := db.QueryRow(`SELECT v FROM meta WHERE k = 'schema_version'`).Scan(&v); err != nil {
		return false
	}
	return v == strconv.Itoa(SchemaVersion)
}

const schema = `
CREATE TABLE docs(
  path TEXT PRIMARY KEY, layer TEXT, title TEXT, status TEXT,
  tags TEXT, source TEXT, body TEXT, body_offset INTEGER, links INTEGER
);
CREATE TABLE links(
  from_path TEXT, target TEXT, heading TEXT, block TEXT, raw TEXT, offset INTEGER
);
CREATE TABLE tables_meta(name TEXT PRIMARY KEY, file TEXT, format TEXT, rows INTEGER, columns TEXT);
CREATE TABLE meta(k TEXT PRIMARY KEY, v TEXT);
-- 派生层的块（见 docs/plans/derived-layer.md §3）：
--   chunk         嵌入单位（默认 512 token）
--   extract_chunk 抽取单位（默认 2000 token）
--   sync          每篇文档的同步状态：「不静默」的落地
CREATE TABLE chunk(
  id INTEGER PRIMARY KEY, doc TEXT, ord INTEGER, from_line INTEGER, to_line INTEGER,
  text TEXT, status TEXT, hash TEXT
);
CREATE INDEX chunk_doc ON chunk(doc, ord);
CREATE TABLE extract_chunk(
  id INTEGER PRIMARY KEY, doc TEXT, ord INTEGER, from_line INTEGER, to_line INTEGER, text TEXT
);
CREATE INDEX extract_chunk_doc ON extract_chunk(doc, ord);
CREATE TABLE sync(
  doc TEXT PRIMARY KEY, hash TEXT, mtime INTEGER, built_at INTEGER, stale INTEGER, reason TEXT
);
-- 向量：owner_kind = chunk / entity / relation，owner_id 是该层的键（块用「文档#序号」）。
-- 向量是 float32 小端裸字节（512 维 = 2048 字节），归一化后点积即余弦。
CREATE TABLE embedding(
  owner_kind TEXT, owner_id TEXT, dim INTEGER, vec BLOB,
  PRIMARY KEY(owner_kind, owner_id)
) WITHOUT ROWID;
`

// Rebuild 从文件重建整份索引（先删后建，保证不会留下上一次的残渣）。
func (idx *Index) Rebuild() error {
	if err := os.MkdirAll(filepath.Dir(idx.Path()), 0o755); err != nil {
		return err
	}
	// 先删旧文件：不然改过的表会留下旧列。
	if err := os.Remove(idx.Path()); err != nil && !os.IsNotExist(err) {
		return err
	}
	db, err := sql.Open("sqlite", idx.Path())
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	docs, err := vaultfs.New(idx.root).Load()
	if err != nil {
		return err
	}
	for _, d := range docs {
		if _, err := db.Exec(
			`INSERT INTO docs(path,layer,title,status,tags,source,body,body_offset,links) VALUES(?,?,?,?,?,?,?,?,?)`,
			d.Path, string(d.Layer), d.Title, string(d.Status), strings.Join(d.Tags, ","),
			d.Source, d.Body, d.BodyOffset, len(d.Links),
		); err != nil {
			return err
		}
		for _, l := range d.Links {
			if _, err := db.Exec(
				`INSERT INTO links(from_path,target,heading,block,raw,offset) VALUES(?,?,?,?,?,?)`,
				d.Path, l.Target, l.Heading, l.Block, l.Raw, l.Offset,
			); err != nil {
				return err
			}
		}
	}

	chunks, extracts, err := idx.writeChunks(db, docs)
	if err != nil {
		return err
	}

	used := map[string]bool{}
	for _, file := range idx.tableFiles() {
		if err := idx.loadTable(db, file, used); err != nil {
			// 单张表坏掉不该让整份索引建不出来：记下来，其余的照建。
			fmt.Fprintf(os.Stderr, "跳过数据表 %s：%v\n", file, err)
		}
	}

	for k, v := range map[string]string{
		"doc_count":           strconv.Itoa(len(docs)),
		"chunk_count":         strconv.Itoa(chunks),
		"extract_chunk_count": strconv.Itoa(extracts),
		"schema_version":      strconv.Itoa(SchemaVersion),
	} {
		if _, err := db.Exec(`INSERT INTO meta(k,v) VALUES(?,?)`, k, v); err != nil {
			return err
		}
	}
	return nil
}

// ChunkStatusFresh / ChunkStatusStale 是 chunk.status 的两个取值。
//
// 口径（plans/derived-layer.md §3）：**派生条目的新鲜度**，不是文档的 draft/published。
// 重建出来的块一律 fresh；P1 嵌入失败或后续增量里文件变了，对应行标 stale
// 并把原因写进 sync.reason——这就是「不静默」。
const (
	ChunkStatusFresh = "fresh"
	ChunkStatusStale = "stale"
)

// writeChunks 把每篇文档切成两套块写进索引，并写 sync 行。
//
// 返回（嵌入块数, 抽取块数, error)。
// 一篇文档一个事务，写完才提交：切块或写库中途失败时，宁可这篇的块全没有，
// 也不留半篇的块——半篇的块会静默地污染检索结果。
func (idx *Index) writeChunks(db *sql.DB, docs []vault.Doc) (int, int, error) {
	targets := vault.DefaultChunkTargets()
	now := time.Now().Unix()
	total, totalExtract := 0, 0

	for _, d := range docs {
		tx, err := db.Begin()
		if err != nil {
			return total, totalExtract, err
		}
		chunks := vault.ChunkBody(d.Body, d.BodyOffset, targets.Embed)
		extracts := vault.ChunkBody(d.Body, d.BodyOffset, targets.Extract)
		sum := sha256.Sum256([]byte(d.Body))

		n := 0
		for _, c := range chunks {
			h := sha256.Sum256([]byte(c.Text))
			if _, err := tx.Exec(
				`INSERT INTO chunk(doc,ord,from_line,to_line,text,status,hash) VALUES(?,?,?,?,?,?,?)`,
				d.Path, c.Ord, c.FromLine, c.ToLine, c.Text, ChunkStatusFresh, hex.EncodeToString(h[:]),
			); err != nil {
				tx.Rollback()
				return total, totalExtract, err
			}
			n++
		}
		for _, c := range extracts {
			if _, err := tx.Exec(
				`INSERT INTO extract_chunk(doc,ord,from_line,to_line,text) VALUES(?,?,?,?,?)`,
				d.Path, c.Ord, c.FromLine, c.ToLine, c.Text,
			); err != nil {
				tx.Rollback()
				return total, totalExtract, err
			}
		}
		// 刚重建出来就是最新的：stale=0。
		if _, err := tx.Exec(
			`INSERT INTO sync(doc,hash,mtime,built_at,stale,reason) VALUES(?,?,?,?,0,'')`,
			d.Path, hex.EncodeToString(sum[:]), idx.mtime(d.Path), now,
		); err != nil {
			tx.Rollback()
			return total, totalExtract, err
		}
		if err := tx.Commit(); err != nil {
			return total, totalExtract, err
		}
		total += n
		totalExtract += len(extracts)
	}
	return total, totalExtract, nil
}

// mtime 返回文件修改时间（Unix 秒）；取不到就返回 0（不是错误：同步状态里 0 表示未知）。
func (idx *Index) mtime(rel string) int64 {
	fi, err := os.Stat(filepath.Join(idx.root, filepath.FromSlash(rel)))
	if err != nil {
		return 0
	}
	return fi.ModTime().Unix()
}

func (idx *Index) tableFiles() []string {
	names, err := vaultfs.New(idx.root).Tables()
	if err != nil {
		return nil
	}
	return names
}

// loadTable 把 tables/ 下的一个文件变成一张真表。
func (idx *Index) loadTable(db *sql.DB, rel string, used map[string]bool) error {
	full := filepath.Join(idx.root, filepath.FromSlash(rel))
	b, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")
	var cols []string
	var rows [][]cell
	switch format {
	case "csv":
		cols, rows, err = parseCSV(b)
	case "json":
		cols, rows, err = parseJSON(b)
	case "yaml", "yml":
		cols, rows, err = parseYAML(b)
	default:
		return fmt.Errorf("不认识的表格式 %q", format)
	}
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return fmt.Errorf("表里没有列")
	}
	name := uniqueName(tableName(rel), used)

	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = quoteIdent(c) + " " + columnType(rows, i)
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE TABLE %s(%s)", quoteIdent(name), strings.Join(defs, ","))); err != nil {
		return err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	stmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s VALUES(%s)", quoteIdent(name), placeholders))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		args := make([]any, len(cols))
		for i := range cols {
			if i < len(r) {
				args[i] = r[i].value
			}
		}
		if _, err := stmt.Exec(args...); err != nil {
			return err
		}
	}
	_, err = db.Exec(`INSERT INTO tables_meta(name,file,format,rows,columns) VALUES(?,?,?,?,?)`,
		name, rel, format, len(rows), strings.Join(cols, ","))
	return err
}

// cell 是表格里的一个格子：值按用途分「文本」与「数值」两种存法。
type cell struct {
	value any
}

func (c cell) isNumber() bool {
	switch v := c.value.(type) {
	case int64, float64:
		_ = v
		return true
	default:
		return false
	}
}

func (c cell) isEmpty() bool { return c.value == nil || c.value == "" }

func parseCSV(b []byte) ([]string, [][]cell, error) {
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 {
		return nil, nil, nil
	}
	cols := recs[0]
	rows := make([][]cell, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		row := make([]cell, len(cols))
		for i := range cols {
			if i < len(rec) {
				row[i] = cell{value: numberOrText(rec[i])}
			}
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

func parseJSON(b []byte) ([]string, [][]cell, error) {
	var items []map[string]any
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, nil, fmt.Errorf("这个 json 不是「对象数组」，暂不建表：%w", err)
	}
	return rowsFromMaps(items)
}

func parseYAML(b []byte) ([]string, [][]cell, error) {
	var items []map[string]any
	if err := yaml.Unmarshal(b, &items); err != nil {
		return nil, nil, fmt.Errorf("这个 yaml 不是「映射序列」，暂不建表：%w", err)
	}
	return rowsFromMaps(items)
}

func rowsFromMaps(items []map[string]any) ([]string, [][]cell, error) {
	var cols []string
	seen := map[string]bool{}
	for _, it := range items {
		for k := range it {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}
	sort.Strings(cols)
	rows := make([][]cell, 0, len(items))
	for _, it := range items {
		row := make([]cell, len(cols))
		for i, c := range cols {
			row[i] = cell{value: flatten(it[c])}
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

// flatten 把嵌套结构压成 JSON 文本——**不猜**怎么展开它。
func flatten(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return t
	case bool:
		return t
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return t
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		f, _ := t.Float64()
		return f
	case map[string]any, []any:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func numberOrText(s string) any {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	if n, err := strconv.ParseInt(t, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil {
		return f
	}
	return s
}

// columnType 看整列的值定类型：全是整数就给 INTEGER，全是数字给 REAL，否则 TEXT。
//
// 不做类型推断反而更糟：`AVG(倍率)` 在 TEXT 列上不会按你的意思算。
func columnType(rows [][]cell, i int) string {
	allNum, allInt, any := true, true, false
	for _, r := range rows {
		if i >= len(r) || r[i].isEmpty() {
			continue
		}
		any = true
		if !r[i].isNumber() {
			allNum, allInt = false, false
			break
		}
		if _, ok := r[i].value.(float64); ok {
			allInt = false
		}
	}
	switch {
	case !any:
		return "TEXT"
	case allInt:
		return "INTEGER"
	case allNum:
		return "REAL"
	default:
		return "TEXT"
	}
}

// tableName 由文件名定表名：去掉扩展名，非字母数字（含中文之外的符号）换成下划线。
func tableName(rel string) string {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff: // 汉字原样留着，查询时好写
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "table"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "t_" + name
	}
	return name
}

func uniqueName(name string, used map[string]bool) string {
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s_%d", name, i)
	}
	used[candidate] = true
	return candidate
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Search 检索文档：标题或正文里含 query。
func (idx *Index) Search(query string, limit int) ([]vault.Hit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("搜索词是空的")
	}
	if limit <= 0 {
		limit = 50
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	like := "%" + q + "%"
	rows, err := db.Query(
		`SELECT path,layer,status,title,body FROM docs WHERE title LIKE ? OR body LIKE ? LIMIT ?`,
		like, like, limit*4) // 多取一些，排序后再截断
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []vault.Hit
	for rows.Next() {
		var path, layer, status, title, body string
		if err := rows.Scan(&path, &layer, &status, &title, &body); err != nil {
			return nil, err
		}
		hits = append(hits, vault.Hit{
			Path:        path,
			Layer:       vault.Layer(layer),
			Status:      vault.Status(status),
			Title:       title,
			Snippet:     vault.Snippet(body, q, 60),
			TitleMatch:  vault.CountOccurrences(title, q) > 0,
			Occurrences: vault.CountOccurrences(body, q),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	vault.RankHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// Tables 列出索引里的数据表。
func (idx *Index) Tables() ([]vault.TableInfo, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name,file,format,rows,columns FROM tables_meta ORDER BY file`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vault.TableInfo
	for rows.Next() {
		var name, file, format, columns string
		var n int
		if err := rows.Scan(&name, &file, &format, &n, &columns); err != nil {
			return nil, err
		}
		info := vault.TableInfo{Name: name, File: file, Format: format, Rows: n}
		if columns != "" {
			info.Columns = strings.Split(columns, ",")
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// Query 跑一条只读查询。
//
// 只放行 SELECT / WITH，并且连接上置 `PRAGMA query_only`：**派生数据不在这里改**。
// 行数上限在读取时截断，不改写调用方的 SQL（改写 SQL 容易在奇怪语法上翻车）。
func (idx *Index) Query(stmt string, limit int) (vault.ResultSet, error) {
	s := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(stmt), ";"))
	if s == "" {
		return vault.ResultSet{}, fmt.Errorf("查询是空的")
	}
	up := strings.ToUpper(s)
	if !strings.HasPrefix(up, "SELECT") && !strings.HasPrefix(up, "WITH") {
		return vault.ResultSet{}, fmt.Errorf("只允许查询（SELECT / WITH）——vault 的数据改文档与表格文件，不在这里改 SQL")
	}
	if limit <= 0 {
		limit = 200
	}
	db, err := idx.open()
	if err != nil {
		return vault.ResultSet{}, err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA query_only = 1`); err != nil {
		return vault.ResultSet{}, err
	}
	rows, err := db.Query(s)
	if err != nil {
		return vault.ResultSet{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return vault.ResultSet{}, err
	}
	out := vault.ResultSet{Columns: cols, Rows: [][]string{}}
	for rows.Next() && len(out.Rows) < limit {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return vault.ResultSet{}, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = cellToString(v)
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}

func cellToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ChunkStat 读块统计。
func (idx *Index) ChunkStat() (vault.ChunkStat, error) {
	db, err := idx.open()
	if err != nil {
		return vault.ChunkStat{}, err
	}
	defer db.Close()
	var st vault.ChunkStat
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(*) FROM sync`, &st.Docs},
		{`SELECT COUNT(*) FROM chunk`, &st.Chunks},
		{`SELECT COUNT(*) FROM extract_chunk`, &st.ExtractChunks},
		{`SELECT COUNT(*) FROM sync WHERE stale <> 0`, &st.Stale},
	} {
		if err := db.QueryRow(q.sql).Scan(q.dst); err != nil {
			return vault.ChunkStat{}, err
		}
	}
	return st, nil
}

// ChunksOf 返回一篇文档的嵌入块（按 ord 升序）。文档没有块时返回空切片。
func (idx *Index) ChunksOf(doc string) ([]vault.Chunk, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT ord, from_line, to_line, text FROM chunk WHERE doc = ? ORDER BY ord`, doc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []vault.Chunk{}
	for rows.Next() {
		var c vault.Chunk
		if err := rows.Scan(&c.Ord, &c.FromLine, &c.ToLine, &c.Text); err != nil {
			return nil, err
		}
		c.Tokens = vault.EstimateTokens(c.Text)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExtractChunksOf 返回一篇文档的抽取块（按 ord 升序）。抽取产物靠它定位。
func (idx *Index) ExtractChunksOf(doc string) ([]vault.Chunk, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT ord, from_line, to_line, text FROM extract_chunk WHERE doc = ? ORDER BY ord`, doc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []vault.Chunk{}
	for rows.Next() {
		var c vault.Chunk
		if err := rows.Scan(&c.Ord, &c.FromLine, &c.ToLine, &c.Text); err != nil {
			return nil, err
		}
		c.Tokens = vault.EstimateTokens(c.Text)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SyncInfo 是一篇文档的同步状态。
type SyncInfo struct {
	Doc     string
	Hash    string // 正文（去掉 front matter）的 sha256
	Mtime   int64  // 文件修改时间（Unix 秒，0 表示未知）
	BuiltAt int64  // 进索引的时间（Unix 秒）
	Stale   bool
	Reason  string // stale 的原因（人话）：失败、半成品、嵌入缺失都写这儿
}

// SyncOf 读一篇文档的同步状态；没有就返回 false（例如索引比文件旧）。
func (idx *Index) SyncOf(doc string) (SyncInfo, bool, error) {
	db, err := idx.open()
	if err != nil {
		return SyncInfo{}, false, err
	}
	defer db.Close()
	var si SyncInfo
	var stale int
	err = db.QueryRow(
		`SELECT doc, hash, mtime, built_at, stale, COALESCE(reason,'') FROM sync WHERE doc = ?`, doc,
	).Scan(&si.Doc, &si.Hash, &si.Mtime, &si.BuiltAt, &stale, &si.Reason)
	if err == sql.ErrNoRows {
		return SyncInfo{}, false, nil
	}
	if err != nil {
		return SyncInfo{}, false, err
	}
	si.Stale = stale != 0
	return si, true, nil
}

// StaleDocs 返回标了 stale 的文档与原因（按路径）。索引状态要能一眼看见。
func (idx *Index) StaleDocs() ([]SyncInfo, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT doc, hash, mtime, built_at, stale, COALESCE(reason,'') FROM sync WHERE stale <> 0 ORDER BY doc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyncInfo{}
	for rows.Next() {
		var si SyncInfo
		var stale int
		if err := rows.Scan(&si.Doc, &si.Hash, &si.Mtime, &si.BuiltAt, &stale, &si.Reason); err != nil {
			return nil, err
		}
		si.Stale = stale != 0
		out = append(out, si)
	}
	return out, rows.Err()
}

func (idx *Index) open() (*sql.DB, error) {
	if !idx.Exists() {
		return nil, fmt.Errorf("索引还没建：先跑 `ssot vault index`（索引是可重建的派生数据）")
	}
	db, err := sql.Open("sqlite", idx.Path())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// openForWrite 是**唯一**允许改索引的连接：只给派生的向量/状态写入用。
//
// 纪律没变（见包注释第 2 条）：事实只在文件里，索引是派生的——
// 所以写只发生在「把派生结果算出来存下去」这一件事上，而且随时可以被 `Rebuild` 抹掉重来。
func (idx *Index) openForWrite() (*sql.DB, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	return db, nil
}
