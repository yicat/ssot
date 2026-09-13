// Package store 是断言的持久化适配器。
//
// 见 docs/specs/admission.spec.md：准入**不直接写库**，而是产出变更集，
// 由本包原子应用——要么全部成功，要么全部回滚。
//
// 集合级约束（唯一性）由数据库索引保证，不在应用层「先查再插」：
// 后者在并发下有竞态。实测三个进程各写 300 条，唯一性由索引守住，零重复。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
	"github.com/ngnl5/ssot/internal/domain/verification"

	_ "modernc.org/sqlite"
)

// Store 是断言库。
type Store struct {
	db *sql.DB
}

const schemaDDL = `
CREATE TABLE IF NOT EXISTS assertion (
  id           TEXT PRIMARY KEY,
  entity       TEXT NOT NULL,
  subject      TEXT NOT NULL,
  predicate    TEXT NOT NULL,
  qualifiers   TEXT NOT NULL,
  key          TEXT NOT NULL,
  value_kind   TEXT NOT NULL,
  value_json   TEXT NOT NULL,
  value_unit   TEXT NOT NULL,
  confidence   TEXT NOT NULL,
  status       TEXT NOT NULL,
  source_name  TEXT NOT NULL,
  source_tier  TEXT NOT NULL,
  artifact     TEXT NOT NULL,
  anchor       TEXT NOT NULL,
  revision     TEXT NOT NULL,
  captured_at  TEXT NOT NULL,
  derived_json TEXT
);
CREATE INDEX IF NOT EXISTS ix_assertion_key ON assertion(key);
CREATE UNIQUE INDEX IF NOT EXISTS ux_assertion_key_value ON assertion(key, value_json);
CREATE INDEX IF NOT EXISTS ix_assertion_entity ON assertion(entity);
CREATE INDEX IF NOT EXISTS ix_assertion_status ON assertion(status);

-- 核验记录：必须能回答「谁、何时、凭什么、核验到哪一部分」。
-- 只追加，不修改——它是审计轨迹。
CREATE TABLE IF NOT EXISTS verification (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  assertion_id  TEXT NOT NULL,
  decision      TEXT NOT NULL,
  method        TEXT NOT NULL,
  proposed_kind TEXT NOT NULL,
  proposed_id   TEXT NOT NULL,
  approved_kind TEXT NOT NULL,
  approved_id   TEXT NOT NULL,
  reason        TEXT NOT NULL,
  evidence      TEXT NOT NULL,
  at            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_verification_assertion ON verification(assertion_id);

-- 待判定事项：原文里有多个说得通的取值，必须由人选。
-- 它不是断言——在裁决并通过准入之前，它只是「原文里出现过的几种说法」。
-- 候选与裁决都以 JSON 整体保存：候选列表必须**原样保留**，
-- 「当时还有哪些说法」是可追溯性的一部分，不能只留选中的那个。
CREATE TABLE IF NOT EXISTS decision (
  id            TEXT PRIMARY KEY,
  entity        TEXT NOT NULL,
  subject       TEXT NOT NULL,
  predicate     TEXT NOT NULL,
  reason        TEXT NOT NULL,
  context       TEXT NOT NULL,
  artifact      TEXT NOT NULL,
  revision      TEXT NOT NULL,
  captured_at   TEXT NOT NULL,
  source_name   TEXT NOT NULL,
  candidates    TEXT NOT NULL,
  status        TEXT NOT NULL,
  resolution    TEXT,
  deferral      TEXT,
  history       TEXT
);
CREATE INDEX IF NOT EXISTS ix_decision_status ON decision(status);
CREATE INDEX IF NOT EXISTS ix_decision_subject ON decision(entity, subject, predicate);
`

// assertionCols 是断言表的列清单，供各处 SELECT 复用。
const assertionCols = `id, entity, subject, predicate, qualifiers, value_kind, value_json,
	value_unit, confidence, status, source_name, source_tier, artifact, anchor, revision, captured_at, derived_json`

// Open 打开（或创建）库并建表。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建目录失败：%w", err)
		}
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表失败：%w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭库。
func (s *Store) Close() error { return s.db.Close() }

// ApplyResult 报告应用结果。
type ApplyResult struct {
	Inserted   int
	Duplicated int
	Conflicted int
}

// Apply 原子应用变更集。
//
// 冲突断言**并存**而非覆盖：同一件事的不同说法都要保留，
// 由人裁决——系统不替用户选。
func (s *Store) Apply(cs assertion.ChangeSet) (ApplyResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback()
	res, err := applyTx(tx, cs)
	if err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// applyTx 在给定事务内应用变更集。
//
// 抽出来是因为待判定事项的裁决也要用同一条写入路径——
// 「裁决产出的断言」与「普通接入产出的断言」必须是同一种东西，
// 否则分级、冲突标记、幂等性就会各走一套。
func applyTx(tx *sql.Tx, cs assertion.ChangeSet) (ApplyResult, error) {
	var res ApplyResult
	conflict := map[string]bool{}
	for _, id := range cs.Conflict {
		conflict[id] = true
	}

	for _, a := range cs.Insert {
		if err := a.Validate(); err != nil {
			return res, fmt.Errorf("拒绝写入：%w", err)
		}
		status := a.Status
		if conflict[a.ID] {
			status = assertion.StatusDisputed
			res.Conflicted++
		}
		n, err := insertTx(tx, a, status)
		if err != nil {
			return res, err
		}
		if n == 0 {
			res.Duplicated++
		} else {
			res.Inserted++
		}
	}
	return res, nil
}

// insertTx 返回受影响行数；0 表示该 (key, value) 已存在（幂等）。
func insertTx(tx *sql.Tx, a assertion.Assertion, status assertion.Status) (int, error) {
	vj := assertion.ValueJSON(a.Value)
	var dj any
	if a.Derived != nil {
		b, err := json.Marshal(a.Derived)
		if err != nil {
			return 0, err
		}
		dj = string(b)
	}
	r, err := tx.Exec(`
		INSERT OR IGNORE INTO assertion
		(id, entity, subject, predicate, qualifiers, key, value_kind, value_json, value_unit,
		 confidence, status, source_name, source_tier, artifact, anchor, revision, captured_at, derived_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Entity, a.Subject, a.Predicate, a.Qualifiers.String(), a.Key(),
		string(a.Value.Kind), string(vj), a.Value.Unit,
		string(a.Confidence), string(status), a.Source.Name, a.Source.Tier,
		a.Provenance.Artifact, a.Provenance.Anchor, a.Provenance.Revision,
		a.Provenance.CapturedAt.UTC().Format(time.RFC3339), dj,
	)
	if err != nil {
		return 0, fmt.Errorf("写入失败：%w", err)
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

// All 返回全部断言。
func (s *Store) All() ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT ` + assertionCols + ` FROM assertion ORDER BY entity, subject, predicate`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// ByEntity 返回某实体的全部断言。
func (s *Store) ByEntity(entity string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT `+assertionCols+` FROM assertion WHERE entity = ? ORDER BY subject, predicate`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// BySubject 返回某主体的全部断言。
func (s *Store) BySubject(entity, subject string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT `+assertionCols+` FROM assertion WHERE entity = ? AND subject = ? ORDER BY predicate`, entity, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// KeysFor 返回某实体上已存在的 (key, value) 组合，用于冲突检测。
func (s *Store) KeysFor(entity string) (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT key, value_json FROM assertion WHERE entity = ?`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = append(out[k], v)
	}
	return out, rows.Err()
}

// Count 返回断言总数。
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM assertion`).Scan(&n)
	return n, err
}

// Subjects 返回某实体下全部主体（排序）。
func (s *Store) Subjects(entity string) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT subject FROM assertion WHERE entity = ? ORDER BY subject`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// PredicateCount 统计某实体下某谓词有多少条断言（场景的 requires 检查用）。
func (s *Store) PredicateCount(entity, predicate string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM assertion WHERE entity = ? AND predicate = ?`,
		entity, predicate).Scan(&n)
	return n, err
}

// SubjectCount 统计某实体下有多少个主体。
func (s *Store) SubjectCount(entity string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(DISTINCT subject) FROM assertion WHERE entity = ?`, entity).Scan(&n)
	return n, err
}

// CountByEntity 按实体统计。
func (s *Store) CountByEntity() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT entity, count(*) FROM assertion GROUP BY entity`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var e string
		var n int
		if err := rows.Scan(&e, &n); err != nil {
			return nil, err
		}
		out[e] = n
	}
	return out, rows.Err()
}

// StatusCounts 按状态统计。
func (s *Store) StatusCounts() (map[string]int, error) {
	return s.countBy(`SELECT status, count(*) FROM assertion GROUP BY status`)
}

// ConfidenceCounts 按分级统计。
//
// 分级分布是「这批数据有多可信」的一个粗但可追溯的快照——
// 没有它，界面上只能给出一个总数，而总数看不出 L1 与 L4 的比例。
func (s *Store) ConfidenceCounts() (map[string]int, error) {
	return s.countBy(`SELECT confidence, count(*) FROM assertion GROUP BY confidence`)
}

// countBy 执行一个「键 + 计数」查询。
func (s *Store) countBy(q string) (map[string]int, error) {
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// UniqueExists 实现 validate.Checkers 的集合级唯一性检查。
func (s *Store) UniqueExists(entity, field string, v any) (bool, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	var n int
	err = s.db.QueryRow(
		`SELECT count(*) FROM assertion WHERE entity = ? AND predicate = ? AND value_json = ?`,
		entity, field, string(b)).Scan(&n)
	return n > 0, err
}

// RefExists 实现引用完整性检查：目标实体中是否存在该身份值。
func (s *Store) RefExists(entity string, id any) (bool, error) {
	b, err := json.Marshal(id)
	if err != nil {
		return false, err
	}
	var n int
	err = s.db.QueryRow(
		`SELECT count(*) FROM assertion WHERE entity = ? AND predicate = 'id' AND value_json = ?`,
		entity, string(b)).Scan(&n)
	return n > 0, err
}

// ── 核验 ────────────────────────────────────────────────────────────────────

// FindByIDPrefix 按 ID 前缀查找断言。
//
// 返回全部匹配项，**由调用方判断是否有歧义**——前缀匹配到多条时不得猜，
// 必须报歧义让人指定。
func (s *Store) FindByIDPrefix(prefix string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT `+assertionCols+` FROM assertion WHERE id LIKE ? ORDER BY id`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// PendingByEntity 返回某实体上待核验的断言。
//
// 这是核验队列的数据源。status 为空时默认取 pending。
func (s *Store) PendingByEntity(entity, status string, limit int) ([]assertion.Assertion, error) {
	if status == "" {
		status = string(assertion.StatusPending)
	}
	q := `SELECT ` + assertionCols + ` FROM assertion WHERE status = ?`
	args := []any{status}
	if entity != "" {
		q += ` AND entity = ?`
		args = append(args, entity)
	}
	q += ` ORDER BY entity, subject, predicate`
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// Verify 应用一条核验记录：写入审计轨迹 + 流转断言状态，二者原子。
//
// 只追加，不修改历史记录——核验是审计轨迹，不是可编辑的字段。
func (s *Store) Verify(rec verification.Record) error {
	if err := rec.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM assertion WHERE id = ?`, rec.AssertionID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("断言 %s 不存在", rec.AssertionID)
	}

	if _, err := tx.Exec(`
		INSERT INTO verification
		(assertion_id, decision, method, proposed_kind, proposed_id,
		 approved_kind, approved_id, reason, evidence, at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		rec.AssertionID, string(rec.Decision), string(rec.Method),
		string(rec.ProposedBy.Kind), rec.ProposedBy.ID,
		string(rec.ApprovedBy.Kind), rec.ApprovedBy.ID,
		rec.Reason, rec.Evidence, rec.At.UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("写入核验记录失败：%w", err)
	}

	if _, err := tx.Exec(`UPDATE assertion SET status = ? WHERE id = ?`,
		rec.Decision.StatusFor(), rec.AssertionID); err != nil {
		return err
	}
	return tx.Commit()
}

// Verifications 返回某断言的核验历史（按时间正序）。
func (s *Store) Verifications(assertionID string) ([]verification.Record, error) {
	rows, err := s.db.Query(`
		SELECT assertion_id, decision, method, proposed_kind, proposed_id,
		       approved_kind, approved_id, reason, evidence, at
		FROM verification WHERE assertion_id = ? ORDER BY id`, assertionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []verification.Record
	for rows.Next() {
		var (
			r                                         verification.Record
			dec, meth                                 string
			pk, pid, ak, aid, reason, evidence, atStr string
		)
		if err := rows.Scan(&r.AssertionID, &dec, &meth, &pk, &pid, &ak, &aid, &reason, &evidence, &atStr); err != nil {
			return nil, err
		}
		r.Decision = verification.Decision(dec)
		r.Method = verification.Method(meth)
		r.ProposedBy = verification.Actor{Kind: verification.ActorKind(pk), ID: pid}
		r.ApprovedBy = &verification.Actor{Kind: verification.ActorKind(ak), ID: aid}
		r.Reason = reason
		r.Evidence = evidence
		if t, err := time.Parse(time.RFC3339, atStr); err == nil {
			r.At = t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// VerificationCount 返回核验记录总数。
func (s *Store) VerificationCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM verification`).Scan(&n)
	return n, err
}

// ── 优先级、冲突与批量筛选 ──────────────────────────────────────────────────

// ReferenceCounts 统计每条断言被多少条派生断言引用。
//
// 这是「影响面」的数据来源：一条数据被 20 个派生引用，它错了波及 20 处。
// 只统计推导链——场景运行时的临时引用没有持久化，因此不在其中。
func (s *Store) ReferenceCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT derived_json FROM assertion WHERE derived_json IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var d assertion.Derivation
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			continue
		}
		for _, in := range d.Inputs {
			out[in]++
		}
	}
	return out, rows.Err()
}

// Conflicts 返回互相冲突的断言分组：同一身份（主体+谓词+限定条件）而有不同取值。
//
// 分组而非成对，是因为同一件事可能有三种说法。系统只负责摆出来，
// **不替人裁决**。
func (s *Store) Conflicts() ([][]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT key FROM assertion GROUP BY key HAVING count(*) > 1 ORDER BY key`)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([][]assertion.Assertion, 0, len(keys))
	for _, k := range keys {
		rs, err := s.db.Query(`SELECT `+assertionCols+` FROM assertion WHERE key = ? ORDER BY source_name, id`, k)
		if err != nil {
			return nil, err
		}
		group, err := scanAll(rs)
		rs.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	return out, nil
}

// Filter 是断言查询条件。
//
// 它是别名而非新类型：筛选条件是**关于断言的概念**，归领域层；
// 存储层只负责把它翻译成 SQL。
type Filter = assertion.Filter

// Select 按条件查询断言。
func (s *Store) Select(f assertion.Filter) ([]assertion.Assertion, error) {
	q := `SELECT ` + assertionCols + ` FROM assertion WHERE 1=1`
	var args []any
	for _, c := range []struct {
		col, val string
	}{
		{"entity", f.Entity}, {"status", f.Status}, {"predicate", f.Predicate},
		{"artifact", f.Artifact}, {"revision", f.Revision},
		{"confidence", f.Confidence}, {"subject", f.Subject},
	} {
		if c.val != "" {
			q += ` AND ` + c.col + ` = ?`
			args = append(args, c.val)
		}
	}
	q += ` ORDER BY entity, subject, predicate`
	if f.Limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, f.Limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

func scanAll(rows *sql.Rows) ([]assertion.Assertion, error) {
	var out []assertion.Assertion
	for rows.Next() {
		var (
			a         assertion.Assertion
			qual, vk  string
			vj, vu    string
			conf, st  string
			sn, stier string
			art, anc  string
			rev, cap  string
			derived   sql.NullString
		)
		err := rows.Scan(&a.ID, &a.Entity, &a.Subject, &a.Predicate, &qual, &vk, &vj, &vu,
			&conf, &st, &sn, &stier, &art, &anc, &rev, &cap, &derived)
		if err != nil {
			return nil, err
		}
		a.Qualifiers = parseQualifiers(qual)
		a.Value = decodeValue(vk, vj, vu)
		a.Confidence = assertion.Confidence(conf)
		a.Status = assertion.Status(st)
		a.Source = assertion.Source{Name: sn, Tier: stier}
		a.Provenance = assertion.Provenance{Artifact: art, Anchor: anc, Revision: rev}
		if t, err := time.Parse(time.RFC3339, cap); err == nil {
			a.Provenance.CapturedAt = t
		}
		if derived.Valid && derived.String != "" {
			var d assertion.Derivation
			if err := json.Unmarshal([]byte(derived.String), &d); err == nil {
				a.Derived = &d
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func decodeValue(kind, data, u string) value.Value {
	k := value.Kind(kind)
	if k != value.Present {
		return value.Value{Kind: k, Unit: u}
	}
	var v any
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return value.UnknownValue()
	}
	return value.Value{Kind: value.Present, Data: v, Unit: u}
}

func parseQualifiers(s string) assertion.Qualifiers {
	var q assertion.Qualifiers
	if s == "" {
		return q
	}
	fields := map[string]*string{
		"v": &q.Version, "from": &q.ValidFrom, "until": &q.ValidUntil,
		"scene": &q.Scene, "cond": &q.Condition,
	}
	for _, part := range splitSemi(s) {
		for pfx, dst := range fields {
			needle := pfx + "="
			if len(part) >= len(needle) && part[:len(needle)] == needle {
				*dst = part[len(needle):]
			}
		}
	}
	return q
}

func splitSemi(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ';' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	out = append(out, cur)
	return out
}
