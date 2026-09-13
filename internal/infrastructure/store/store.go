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
`

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
	var res ApplyResult
	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

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
	if err := tx.Commit(); err != nil {
		return res, err
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
	rows, err := s.db.Query(`SELECT id, entity, subject, predicate, qualifiers, value_kind, value_json,
		value_unit, confidence, status, source_name, source_tier, artifact, anchor, revision, captured_at, derived_json
		FROM assertion ORDER BY entity, subject, predicate`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// ByEntity 返回某实体的全部断言。
func (s *Store) ByEntity(entity string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT id, entity, subject, predicate, qualifiers, value_kind, value_json,
		value_unit, confidence, status, source_name, source_tier, artifact, anchor, revision, captured_at, derived_json
		FROM assertion WHERE entity = ? ORDER BY subject, predicate`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows)
}

// BySubject 返回某主体的全部断言。
func (s *Store) BySubject(entity, subject string) ([]assertion.Assertion, error) {
	rows, err := s.db.Query(`SELECT id, entity, subject, predicate, qualifiers, value_kind, value_json,
		value_unit, confidence, status, source_name, source_tier, artifact, anchor, revision, captured_at, derived_json
		FROM assertion WHERE entity = ? AND subject = ? ORDER BY predicate`, entity, subject)
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
	rows, err := s.db.Query(`SELECT status, count(*) FROM assertion GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
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

func scanAll(rows *sql.Rows) ([]assertion.Assertion, error) {
	var out []assertion.Assertion
	for rows.Next() {
		var (
			a          assertion.Assertion
			qual, vk   string
			vj, vu     string
			conf, st   string
			sn, stier  string
			art, anc   string
			rev, cap   string
			derived    sql.NullString
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
