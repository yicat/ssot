// 抽取产物的落库：entity / relation 两张表。
//
// 口径来自 docs/plans/derived-layer.md §3 与 docs/specs/derived.spec.md §九.2：
//   - **归并只合条目、不丢来源**：同一个实体在不同文档/不同块里出现，就各占一行
//     （每条都带自己的 doc + 行号）——主张级溯源在派生层也要成立；
//   - `authority` 分 `derived`（抽取来的）与 `corrected`（人纠错的），**纠错优先**；
//   - 幂等：同一来源重复写不会变多（主键就是「来源」），所以跑两遍抽取不会把图越写越肿。
//
// ⚠️ 这里**不复制文档的 status**：status 会变（人一发布就变了），派生表里复制一份就会过期。
// 读的时候 join `docs` 拿当时的状态——「未核验可见」这条立场因此永远准。
package vaultindex

import (
	"sort"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// AuthorityDerived / AuthorityCorrected 是派生条目的来源性质。
const (
	AuthorityDerived   = "derived"
	AuthorityCorrected = "corrected"
)

// EntityRow 是实体的一条来源（同一个实体可以有多个来源行）。
type EntityRow struct {
	Name        string
	Type        string
	Description string
	Authority   string // derived / corrected
	Doc         string
	FromLine    int
	ToLine      int
	Line        int // 模型给的行号（0 = 只有块区间）
	// Status 只在**读**的时候填（从 docs join 出来），不落库。
	Status string
}

// RelationRow 是关系的一条来源。
type RelationRow struct {
	Src         string
	Dst         string
	Keywords    string
	Description string
	Authority   string
	Doc         string
	FromLine    int
	ToLine      int
	Line        int
	Status      string
}

// PutEntities 写实体来源行（幂等）。
func (idx *Index) PutEntities(rows []EntityRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	db, err := idx.openForWrite()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO entity(name,type,description,authority,doc,from_line,to_line,line)
	  VALUES(?,?,?,?,?,?,?,?)
	  ON CONFLICT(name,type,doc,from_line,to_line,line) DO UPDATE SET
	    description=excluded.description,
	    authority=CASE WHEN entity.authority='corrected' THEN entity.authority ELSE excluded.authority END`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, r := range rows {
		if r.Name == "" || r.Doc == "" {
			continue
		}
		auth := r.Authority
		if auth == "" {
			auth = AuthorityDerived
		}
		if _, err := stmt.Exec(r.Name, r.Type, r.Description, auth, r.Doc, r.FromLine, r.ToLine, r.Line); err != nil {
			tx.Rollback()
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// PutRelations 写关系来源行（幂等）。
func (idx *Index) PutRelations(rows []RelationRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	db, err := idx.openForWrite()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO relation(src,dst,keywords,description,authority,doc,from_line,to_line,line)
	  VALUES(?,?,?,?,?,?,?,?,?)
	  ON CONFLICT(src,dst,keywords,doc,from_line,to_line,line) DO UPDATE SET
	    description=excluded.description,
	    authority=CASE WHEN relation.authority='corrected' THEN relation.authority ELSE excluded.authority END`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, r := range rows {
		if r.Src == "" || r.Dst == "" || r.Doc == "" {
			continue
		}
		auth := r.Authority
		if auth == "" {
			auth = AuthorityDerived
		}
		if _, err := stmt.Exec(r.Src, r.Dst, r.Keywords, r.Description, auth, r.Doc, r.FromLine, r.ToLine, r.Line); err != nil {
			tx.Rollback()
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// ExtractStat 读派生图的家底。
func (idx *Index) ExtractStat() (vault.ExtractStat, error) {
	db, err := idx.open()
	if err != nil {
		return vault.ExtractStat{}, err
	}
	defer db.Close()
	var st vault.ExtractStat
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(*) FROM entity`, &st.Entities},
		{`SELECT COUNT(*) FROM (SELECT DISTINCT name,type FROM entity)`, &st.EntityNames},
		{`SELECT COUNT(*) FROM relation`, &st.Relations},
		{`SELECT COUNT(*) FROM (SELECT DISTINCT src,dst,keywords FROM relation)`, &st.RelationKeys},
		{`SELECT COUNT(*) FROM (SELECT DISTINCT doc FROM entity UNION SELECT DISTINCT doc FROM relation)`, &st.Docs},
		{`SELECT COUNT(*) FROM entity WHERE authority='corrected'`, &st.Corrected},
	} {
		if err := db.QueryRow(q.sql).Scan(q.dst); err != nil {
			return vault.ExtractStat{}, err
		}
	}
	return st, nil
}

// EntitiesFor 读某篇文档抽到的实体（带当时的状态）。
func (idx *Index) EntitiesFor(doc string) ([]EntityRow, error) {
	return idx.reallyEntities(`SELECT e.name,e.type,e.description,e.authority,e.doc,e.from_line,e.to_line,e.line,
	    COALESCE(d.status,'')
	  FROM entity e LEFT JOIN docs d ON d.path=e.doc WHERE e.doc=? ORDER BY e.name,e.type,e.line`, doc)
}

// EntitiesMerged 按 (name,type) 归并后读实体：**只合条目，来源另列**。
//
// 返回的顺序是确定的（名称、类型），所以同样数据同样输出。
func (idx *Index) EntitiesMerged(limit int) ([]EntityRow, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT name,type,
	    MIN(description) AS description,
	    CASE WHEN SUM(authority='corrected')>0 THEN 'corrected' ELSE 'derived' END AS authority,
	    COUNT(*) AS sources,
	    GROUP_CONCAT(DISTINCT doc || ':' || COALESCE(line, from_line)) AS srcs
	  FROM entity GROUP BY name,type ORDER BY name,type LIMIT ?`
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EntityRow{}
	for rows.Next() {
		var r EntityRow
		var sources int
		var srcs string
		if err := rows.Scan(&r.Name, &r.Type, &r.Description, &r.Authority, &sources, &srcs); err != nil {
			return nil, err
		}
		r.Doc = srcs // 归并读法里 Doc 放来源清单（`doc:line` 用逗号连）
		out = append(out, r)
	}
	return out, rows.Err()
}

func (idx *Index) reallyEntities(q, doc string) ([]EntityRow, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(q, doc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EntityRow{}
	for rows.Next() {
		var r EntityRow
		if err := rows.Scan(&r.Name, &r.Type, &r.Description, &r.Authority, &r.Doc,
			&r.FromLine, &r.ToLine, &r.Line, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RelationsFor 读某篇文档抽到的关系（带当时的状态）。
func (idx *Index) RelationsFor(doc string) ([]RelationRow, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT r.src,r.dst,r.keywords,r.description,r.authority,r.doc,r.from_line,r.to_line,r.line,
	    COALESCE(d.status,'')
	  FROM relation r LEFT JOIN docs d ON d.path=r.doc WHERE r.doc=? ORDER BY r.src,r.dst`, doc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RelationRow{}
	for rows.Next() {
		var r RelationRow
		if err := rows.Scan(&r.Src, &r.Dst, &r.Keywords, &r.Description, &r.Authority, &r.Doc,
			&r.FromLine, &r.ToLine, &r.Line, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExtractedDocs 列出已经有抽取产物的文档（给「哪些还没抽」用）。
func (idx *Index) ExtractedDocs() ([]string, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT DISTINCT doc FROM entity UNION SELECT DISTINCT doc FROM relation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out, rows.Err()
}

// EntityNames 返回图里已有实体名的集合（关系端点校验用）。
func (idx *Index) EntityNames() (map[string]bool, error) {
	db, err := idx.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT DISTINCT name FROM entity`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}
