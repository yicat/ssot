// 删一篇文档留下的**全部派生行**：块、抽取块、向量、同步状态、实体、关系。
//
// 为什么要有它：文件删掉之后，派生层里属于它的行就是**残骸**——留着会让检索命中一篇不存在的文档、
// 让图里挂着没有出处的实体。删文件时必须顺手清干净（要么在这里清，要么等下次 Rebuild，
// 但「等下次」意味着中间这段时间是脏的）。
package vaultindex

import (
	"fmt"
	"strings"
)

// PurgeDoc 清掉某个文档的派生数据，返回清掉的行数。
func (idx *Index) PurgeDoc(doc string) (int, error) {
	doc = strings.ReplaceAll(doc, "\\", "/")
	db, err := idx.openForWrite()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	// 向量是按 `文档#序号` 存的，去掉文档自己那一段前缀（用 length/substr 比 LIKE 安全：
	// 文档名里可能有 `%` `_` 这类 LIKE 元字符）。
	likeKey := doc + "#"
	steps := []struct {
		sql string
		arg any
	}{
		{`DELETE FROM embedding WHERE owner_kind = ? AND substr(owner_id, 1, length(?)) = ?`, nil},
		{`DELETE FROM chunk WHERE doc = ?`, doc},
		{`DELETE FROM extract_chunk WHERE doc = ?`, doc},
		{`DELETE FROM sync WHERE doc = ?`, doc},
		{`DELETE FROM entity WHERE doc = ?`, doc},
		{`DELETE FROM relation WHERE doc = ?`, doc},
		// docs 行也要清：不清的话 search（走 docs 表）还能命中一篇已经删掉的文档（测试抓到的）。
		{`DELETE FROM docs WHERE path = ?`, doc},
	}
	total := 0
	for i, s := range steps {
		var res interface{ RowsAffected() (int64, error) }
		var err error
		if i == 0 {
			res, err = tx.Exec(s.sql, KindChunk, likeKey, likeKey)
		} else {
			res, err = tx.Exec(s.sql, s.arg)
		}
		if err != nil {
			tx.Rollback()
			return total, fmt.Errorf("清理派生行失败（%s）：%w", s.sql, err)
		}
		if n, err := res.RowsAffected(); err == nil {
			total += int(n)
		}
	}
	return total, tx.Commit()
}

// PurgePreview 数一篇文档在派生层里有多少行（dry-run 用，只数不删）。
func (idx *Index) PurgePreview(doc string) (int, error) {
	doc = strings.ReplaceAll(doc, "\\", "/")
	db, err := idx.open()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	likeKey := doc + "#"
	total := 0
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`SELECT COUNT(*) FROM embedding WHERE owner_kind = ? AND substr(owner_id, 1, length(?)) = ?`, []any{KindChunk, likeKey, likeKey}},
		{`SELECT COUNT(*) FROM chunk WHERE doc = ?`, []any{doc}},
		{`SELECT COUNT(*) FROM extract_chunk WHERE doc = ?`, []any{doc}},
		{`SELECT COUNT(*) FROM sync WHERE doc = ?`, []any{doc}},
		{`SELECT COUNT(*) FROM entity WHERE doc = ?`, []any{doc}},
		{`SELECT COUNT(*) FROM relation WHERE doc = ?`, []any{doc}},
		{`SELECT COUNT(*) FROM docs WHERE path = ?`, []any{doc}},
	} {
		var n int
		if err := db.QueryRow(q.sql, q.args...).Scan(&n); err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
