// 把文档里的**纠正块**读成派生图里 `authority: corrected` 的来源行。
//
// 为什么在重建时做（而不是抽取时）：抽取产物是「重新跑一遍就没了」的东西
// （`TestRebuildDropsGraph` 钉着：重建会把它全丢掉），而纠正**不是抽出来的**，
// 它是文件里的内容——重建时重读一遍，它自然还在。ADR 0009 那条
// 「纠正必须活过重建」就是靠这一点落地的。
//
// 语法定在 docs/specs/document.spec.md「纠正块的写法」，语义在 derived.spec.md §九.2。
package vaultindex

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// writeCorrections 把每篇文档里的纠正块写成一条 corrected 来源行，返回（生效, 未生效）。
//
// 「未生效」的逐条打到 stderr（与「跳过坏掉的数据表」同一手法）：纠正块是手写的，
// 写漏一行就静默不生效的话，人只会以为「改了没反应」。
func (idx *Index) writeCorrections(db *sql.DB, docs []vault.Doc) (int, int, error) {
	found, ignored := 0, 0
	for _, d := range docs {
		cors, bad := vault.ParseCorrections(d.Body, d.BodyOffset)
		for _, b := range bad {
			ignored++
			fmt.Fprintf(os.Stderr, "纠正块没生效（%s 第 %d 行）：%s\n", d.Path, b.Line, b.Reason)
		}
		for _, c := range cors {
			// 主键是「来源」：(name,type,doc,from_line,to_line,line)。
			// line 取块的第一行——这是这条纠正最精确的出处。
			if _, err := db.Exec(
				`INSERT INTO entity(name,type,description,authority,doc,from_line,to_line,line)
				 VALUES(?,?,?,?,?,?,?,?)`,
				c.Name, c.Type, c.Description, AuthorityCorrected, d.Path, c.FromLine, c.ToLine, c.FromLine,
			); err != nil {
				return found, ignored, err
			}
			found++
		}
	}
	return found, ignored, nil
}

// CorrectionStat 读上次重建认出来的纠正块：生效几条、没写对几条。
//
// 老索引（还没跑过带纠正块的重建）没有这两个键——那就当 0，不是错误。
func (idx *Index) CorrectionStat() (found, ignored int, err error) {
	db, err := idx.open()
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	for k, dst := range map[string]*int{"correction_count": &found, "correction_ignored": &ignored} {
		var v string
		if qerr := db.QueryRow(`SELECT v FROM meta WHERE k=?`, k).Scan(&v); qerr != nil {
			continue
		}
		if n, cerr := strconv.Atoi(v); cerr == nil {
			*dst = n
		}
	}
	return found, ignored, nil
}
