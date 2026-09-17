// 向量层：把 embedding 表里的向量读懂、写进去，并做**暴力扫**检索。
//
// 为什么敢暴力扫（不上 ANN）：实测 512 维、`CGO_ENABLED=0` 下 1 万条 2ms、10 万条 20ms
// （见 docs/specs/derived.spec.md §九.3 与 docs/notes/embedding-spike.md §六）。
// 我们的 vault 规模离 10 万还很远，而暴力扫**结果确定、无索引可失效**——
// 这比省那几毫秒值钱（ADR 0010）。
package vaultindex

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// 允许被嵌入/检索的层。用白名单拼表名，别把用户输入拼进 SQL。
const (
	KindChunk   = "chunk"
	KindExtract = "extract_chunk"
)

func chunkTable(kind string) (string, error) {
	switch kind {
	case KindChunk:
		return "chunk", nil
	case KindExtract:
		return "extract_chunk", nil
	default:
		return "", fmt.Errorf("未知的嵌入层 %q（只认 %s / %s）", kind, KindChunk, KindExtract)
	}
}

// ChunkKey 是块在 embedding 表里的 owner_id：`文档#序号`。
func ChunkKey(doc string, ord int) string { return doc + "#" + strconv.Itoa(ord) }

// splitChunkKey 把 owner_id 拆回（文档名里可能有 `#`，所以从右边找）。
func splitChunkKey(key string) (string, int, bool) {
	i := strings.LastIndex(key, "#")
	if i < 0 {
		return "", 0, false
	}
	ord, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:i], ord, true
}

// ChunkRef 是一个待嵌入的块（只带嵌入要用的东西）。
type ChunkRef struct {
	Doc      string
	Ord      int
	Text     string
	FromLine int
	ToLine   int
}

// PendingChunks 返回还没嵌入的块（按文档、序号），最多 limit 条；limit<=0 表示不限。
//
// 「没嵌入」= embedding 表里没有对应行——这就是**不静默**：缺哪些随时查得到。
func (idx *Index) PendingChunks(kind string, limit int) ([]ChunkRef, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return nil, err
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	q := fmt.Sprintf(`SELECT c.doc, c.ord, c.text, c.from_line, c.to_line FROM %s c
	  LEFT JOIN embedding e ON e.owner_kind = ? AND e.owner_id = c.doc || '#' || c.ord
	  WHERE e.owner_id IS NULL ORDER BY c.doc, c.ord`, table)
	args := []any{kind}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChunkRef{}
	for rows.Next() {
		var c ChunkRef
		if err := rows.Scan(&c.Doc, &c.Ord, &c.Text, &c.FromLine, &c.ToLine); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PendingCount 是还没嵌入的块数（进度显示用，别为了它把块都查出来）。
func (idx *Index) PendingCount(kind string) (int, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return 0, err
	}
	db, err := idx.open()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	err = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s c
	  LEFT JOIN embedding e ON e.owner_kind = ? AND e.owner_id = c.doc || '#' || c.ord
	  WHERE e.owner_id IS NULL`, table), kind).Scan(&n)
	return n, err
}

// VectorStat 是某一层的嵌入情况：总块数、已嵌入数、维度。
type VectorStat struct {
	Kind     string
	Chunks   int
	Embedded int
	Dim      int
}

// VectorStat 读某一层的嵌入统计。
func (idx *Index) VectorStat(kind string) (VectorStat, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return VectorStat{}, err
	}
	db, err := idx.open()
	if err != nil {
		return VectorStat{}, err
	}
	defer db.Close()
	st := VectorStat{Kind: kind}
	if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&st.Chunks); err != nil {
		return VectorStat{}, err
	}
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(dim),0) FROM embedding WHERE owner_kind = ?`, kind,
	).Scan(&st.Embedded, &st.Dim); err != nil {
		return VectorStat{}, err
	}
	return st, nil
}

// PutEmbeddings 把一批向量写进 embedding 表（同一个 kind 一批一个事务）。
//
// keys 与 vecs 一一对应；维度不一致直接报错——**宁可失败，也不要写进半截的向量**
// （半截的向量会让检索静默地漏掉东西）。
func (idx *Index) PutEmbeddings(kind string, keys []string, vecs [][]float32) error {
	if len(keys) != len(vecs) {
		return fmt.Errorf("键与向量数量不一致：%d vs %d", len(keys), len(vecs))
	}
	if len(keys) == 0 {
		return nil
	}
	if _, err := chunkTable(kind); err != nil {
		return err
	}
	dim := len(vecs[0])
	if dim == 0 {
		return fmt.Errorf("向量维度是 0")
	}
	for i, v := range vecs {
		if len(v) != dim {
			return fmt.Errorf("第 %d 条向量维度 %d，与同批的 %d 不一致", i, len(v), dim)
		}
	}
	db, err := idx.openForWrite()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO embedding(owner_kind, owner_id, dim, vec) VALUES(?,?,?,?)
		 ON CONFLICT(owner_kind, owner_id) DO UPDATE SET dim=excluded.dim, vec=excluded.vec`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for i, k := range keys {
		if _, err := stmt.Exec(kind, k, dim, encodeVector(vecs[i])); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// VectorHit 是向量检索的一条命中：行号区间 + 状态 + 分数。
func (idx *Index) SearchVector(kind string, q []float32, limit int) ([]vault.VectorHit, error) {
	table, err := chunkTable(kind)
	if err != nil {
		return nil, err
	}
	if len(q) == 0 {
		return nil, fmt.Errorf("查询向量是空的")
	}
	if limit <= 0 {
		limit = 10
	}
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	type scored struct {
		key   string
		score float32
	}
	var hits []scored
	rows, err := db.Query(`SELECT owner_id, dim, vec FROM embedding WHERE owner_kind = ?`, kind)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var dim int
		var blob []byte
		if err := rows.Scan(&key, &dim, &blob); err != nil {
			rows.Close()
			return nil, err
		}
		if dim != len(q) {
			continue // 维度对不上（换过模型）：跳过，免得算出没意义的分数
		}
		vec := decodeVector(blob, dim)
		if vec == nil {
			continue
		}
		var dot float32
		for i := range q {
			dot += q[i] * vec[i]
		}
		hits = append(hits, scored{key: key, score: dot})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 确定性排序：分数高的在前；同分按 owner_id 升序（同查询同顺序，ADR 0010）。
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].key < hits[j].key
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}

	out := make([]vault.VectorHit, 0, len(hits))
	for _, h := range hits {
		doc, ord, ok := splitChunkKey(h.key)
		if !ok {
			continue
		}
		var vh vault.VectorHit
		var status string
		err := db.QueryRow(fmt.Sprintf(
			`SELECT c.doc, c.from_line, c.to_line, c.text, COALESCE(d.status,'') FROM %s c
			   LEFT JOIN docs d ON d.path = c.doc WHERE c.doc = ? AND c.ord = ?`, table),
			doc, ord).Scan(&vh.Doc, &vh.FromLine, &vh.ToLine, &vh.Text, &status)
		if err == sql.ErrNoRows {
			continue // 块没了（文件删了）：跳过
		}
		if err != nil {
			return nil, err
		}
		vh.Ord = ord
		vh.Status = vault.Status(status)
		vh.Score = h.score
		out = append(out, vh)
	}
	return out, nil
}

// ScanVectors 把某一层的向量全表读一遍并做点积，返回扫过的条数。
//
// 给「性能是不是还行」一个可测的东西：调用方自己掐表（`time.Now()`），
// 这里只保证干的是**检索本身**的活（不查块内容、不排序、不结算）。
func (idx *Index) ScanVectors(kind string, q []float32) (int, error) {
	if _, err := chunkTable(kind); err != nil {
		return 0, err
	}
	db, err := idx.open()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT dim, vec FROM embedding WHERE owner_kind = ?`, kind)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	var sink float32
	for rows.Next() {
		var dim int
		var blob []byte
		if err := rows.Scan(&dim, &blob); err != nil {
			return 0, err
		}
		if dim != len(q) {
			continue
		}
		vec := decodeVector(blob, dim)
		if vec == nil {
			continue
		}
		for i := range q {
			sink += q[i] * vec[i]
		}
		n++
	}
	if sink == 1e30 { // 不可能成立：只是让编译器别把上面的循环当死代码
		return n, fmt.Errorf("unreachable")
	}
	return n, rows.Err()
}
// encodeVector 把向量编成 float32 小端裸字节。
func encodeVector(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

// decodeVector 解码；长度不对返回 nil（当作坏数据跳过，而不是崩）。
func decodeVector(b []byte, dim int) []float32 {
	if dim <= 0 || len(b) != dim*4 {
		return nil
	}
	v := make([]float32, dim)
	for i := 0; i < dim; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
