// 文档的持久化（见 docs/specs/document.spec.md）。
//
// 两件事由存储层保证：
//
//	只追加   正文不可改。换修订写新记录，旧记录标为「已被取代」但内容不动。
//	变更传导 文档当前修订变化时，引用它的断言**回到待核验**——
//	         这条规则 core.spec 早就要求，此前做不到是因为文档只是溯源里的一个字符串。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/document"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// PutDocument 写入一份**新**修订。
//
// 只接受新 RevID：更正只能登记新修订，不能改正文。
func (s *Store) PutDocument(d document.Doc) error {
	if err := d.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM document WHERE rev_id = ?`, d.RevID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("文档修订 %s 已存在——更正请登记新修订", d.RevID)
	}
	if err := putDocumentTx(tx, d); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkDocumentSuperseded 把一份旧修订标为「已被新修订取代」。
func (s *Store) MarkDocumentSuperseded(revID, by string) error {
	res, err := s.db.Exec(
		`UPDATE document SET status = ?, superseded_by = ? WHERE rev_id = ? AND status <> ?`,
		string(document.StatusSuperseded), by, revID, string(document.StatusSuperseded))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		if err := s.db.QueryRow(`SELECT count(*) FROM document WHERE rev_id = ?`, revID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("文档修订 %s 不存在", revID)
		}
	}
	return nil
}

// UpdateDocumentStatus 只改核验状态，**不改内容**。
func (s *Store) UpdateDocumentStatus(revID, status, reason string,
	by *verification.Actor, method verification.Method) error {

	cur, ok, err := s.DocumentByRev(revID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("文档修订 %s 不存在", revID)
	}
	cur.Status = document.Status(status)
	cur.Reason = reason
	cur.VerifiedBy = by
	cur.Method = method
	if err := cur.Validate(); err != nil {
		return err
	}
	kind, id := "", ""
	if by != nil {
		kind, id = string(by.Kind), by.ID
	}
	_, err = s.db.Exec(
		`UPDATE document SET status = ?, reason = ?, verified_kind = ?, verified_id = ?, method = ? WHERE rev_id = ?`,
		status, reason, kind, id, string(method), revID)
	return err
}

func putDocumentTx(tx *sql.Tx, d document.Doc) error {
	vers, err := json.Marshal(d.Applies.Versions)
	if err != nil {
		return err
	}
	scens, err := json.Marshal(d.Applies.Scenarios)
	if err != nil {
		return err
	}
	if d.Applies.Versions == nil {
		vers = []byte("[]")
	}
	if d.Applies.Scenarios == nil {
		scens = []byte("[]")
	}
	kind, id := "", ""
	if d.VerifiedBy != nil {
		kind, id = string(d.VerifiedBy.Kind), d.VerifiedBy.ID
	}
	_, err = tx.Exec(`
		INSERT INTO document
		(rev_id, doc_id, source, title, kind, revision, hash, captured_at, body, artifact_path,
		 applies_versions, applies_scenarios, status, verified_kind, verified_id, method, reason,
		 registered_at, superseded_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.RevID, d.ID, d.Source, d.Title, string(d.Kind), d.Revision, d.Hash,
		d.CapturedAt.UTC().Format(time.RFC3339), d.Body, d.ArtifactPath,
		string(vers), string(scens), string(d.Status), kind, id, string(d.Method), d.Reason,
		d.RegisteredAt.UTC().Format(time.RFC3339), d.SupersededBy)
	if err != nil {
		return fmt.Errorf("保存文档失败：%w", err)
	}
	return nil
}

// Documents 返回每个文档的**当前修订**。场景名为空表示全部。
//
// 场景名非空时，含「未声明适用范围」（= 全项目适用）的文档，
// 但不含声明了别的场景的——否则「适用场景」就没有约束力。
func (s *Store) Documents(scenario string) ([]document.Doc, error) {
	rows, err := s.db.Query(docCols+` WHERE status <> ? ORDER BY title, registered_at DESC`,
		string(document.StatusSuperseded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDoc := map[string]document.Doc{}
	var order []string
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		if _, seen := byDoc[d.ID]; !seen {
			order = append(order, d.ID)
		}
		// 同一次查询按 registered_at DESC 排过，因此首次遇到的就是当前修订
		if _, seen := byDoc[d.ID]; !seen {
			byDoc[d.ID] = d
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]document.Doc, 0, len(order))
	for _, id := range order {
		d := byDoc[id]
		if scenario != "" && !d.Applies.AppliesToScenario(scenario) {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// DocumentHistory 返回某个文档的全部修订，登记时间倒序。
func (s *Store) DocumentHistory(docID string) ([]document.Doc, error) {
	rows, err := s.db.Query(docCols+` WHERE doc_id = ? ORDER BY registered_at DESC, rev_id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []document.Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DocumentByRev 按修订标识取一份文档。
func (s *Store) DocumentByRev(revID string) (document.Doc, bool, error) {
	row := s.db.QueryRow(docCols+` WHERE rev_id = ?`, revID)
	d, err := scanDoc(row)
	if err == sql.ErrNoRows {
		return document.Doc{}, false, nil
	}
	if err != nil {
		return document.Doc{}, false, err
	}
	return d, true, nil
}

// FindDocument 按 (来源, 标题) 取当前修订。
func (s *Store) FindDocument(source, title string) (document.Doc, bool, error) {
	row := s.db.QueryRow(
		docCols+` WHERE source = ? AND title = ? AND status <> ? ORDER BY registered_at DESC LIMIT 1`,
		source, title, string(document.StatusSuperseded))
	d, err := scanDoc(row)
	if err == sql.ErrNoRows {
		return document.Doc{}, false, nil
	}
	if err != nil {
		return document.Doc{}, false, err
	}
	return d, true, nil
}

const docCols = `SELECT rev_id, doc_id, source, title, kind, revision, hash, captured_at, body,
	artifact_path, applies_versions, applies_scenarios, status, verified_kind, verified_id,
	method, reason, registered_at, superseded_by FROM document`

func scanDoc(row scanner) (document.Doc, error) {
	var (
		d                               document.Doc
		captured, registered            string
		kind, st, vk, vid, meth, reason string
		vers, scens                     string
	)
	err := row.Scan(&d.RevID, &d.ID, &d.Source, &d.Title, &kind, &d.Revision, &d.Hash,
		&captured, &d.Body, &d.ArtifactPath, &vers, &scens, &st, &vk, &vid, &meth, &reason,
		&registered, &d.SupersededBy)
	if err != nil {
		return d, err
	}
	d.Kind = document.Kind(kind)
	d.Status = document.Status(st)
	d.Method = verification.Method(meth)
	d.Reason = reason
	if vk != "" || vid != "" {
		d.VerifiedBy = &verification.Actor{Kind: verification.ActorKind(vk), ID: vid}
	}
	if t, err := time.Parse(time.RFC3339, captured); err == nil {
		d.CapturedAt = t
	}
	if t, err := time.Parse(time.RFC3339, registered); err == nil {
		d.RegisteredAt = t
	}
	if err := json.Unmarshal([]byte(vers), &d.Applies.Versions); err != nil {
		return d, fmt.Errorf("文档 %s 的适用版本无法解析：%w", d.RevID, err)
	}
	if err := json.Unmarshal([]byte(scens), &d.Applies.Scenarios); err != nil {
		return d, fmt.Errorf("文档 %s 的适用场景无法解析：%w", d.RevID, err)
	}
	return d, nil
}

// ── 变更传导 ────────────────────────────────────────────────────────────────

// RequeueByArtifact 把引用某原件的断言**回到待核验**。
//
// force 为 true 表示「修订标识没变但内容变了」：那时无法分辨哪些断言是
// 从旧内容抽出来的，只能整个原件上的断言全部回退——宁可多核验一次。
//
// **已驳回的保持已驳回**：驳回是人的结论，不因来源变化而复活。
func (s *Store) RequeueByArtifact(title, currentRevision string, force bool) (int, error) {
	q := `UPDATE assertion SET status = ?
	      WHERE artifact = ? AND status <> ? AND revision <> ?`
	args := []any{string(assertion.StatusPending), title, string(assertion.StatusRejected), currentRevision}
	if force {
		q = `UPDATE assertion SET status = ?
		     WHERE artifact = ? AND status <> ?`
		args = []any{string(assertion.StatusPending), title, string(assertion.StatusRejected)}
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// AssertionsByArtifact 返回引用某原件的断言。
func (s *Store) AssertionsByArtifact(title string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(
		assertionCols+` FROM assertion WHERE artifact = ? ORDER BY entity, subject, predicate`, title)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// UnregisteredArtifacts 返回还没有对应文档的原件，按引用数降序。
func (s *Store) UnregisteredArtifacts() ([]document.UnregisteredRef, error) {
	rows, err := s.db.Query(`
		SELECT a.artifact, count(*) AS n FROM assertion a
		WHERE a.artifact <> ''
		  AND NOT EXISTS (SELECT 1 FROM document d WHERE d.title = a.artifact)
		GROUP BY a.artifact ORDER BY n DESC, a.artifact`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []document.UnregisteredRef{}
	for rows.Next() {
		var r document.UnregisteredRef
		if err := rows.Scan(&r.Artifact, &r.Assertions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ArtifactRevisions 返回某原件上断言用到的全部修订标识。
//
// 变更传导要知道「旧断言是从哪一版抽出来的」，才能判断它是否过期。
func (s *Store) ArtifactRevisions(title string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT revision FROM assertion WHERE artifact = ? ORDER BY revision`, title)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ArtifactUsageCounts 统计每个原件被多少条断言引用。
//
// 批量取，避免界面为每份文档各查一次——423 份文档就是 423 次查询。
func (s *Store) ArtifactUsageCounts() (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT artifact, count(*) FROM assertion WHERE artifact <> '' GROUP BY artifact`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}
