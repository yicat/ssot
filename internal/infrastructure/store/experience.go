// 经验的持久化（见 docs/specs/experience.spec.md）。
//
// 两件事由存储层保证：
//
//	会话记录只追加 —— 不提供修改与删除方法。可删改的依据不是依据。
//	经验只追加     —— 修订写新条目并把旧条目标为「已被取代」，旧条目内容不动。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/experience"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// ── 会话记录（依据）────────────────────────────────────────────────────────

// UpsertSession 保存一段**新**会话记录。
//
// 只接受新建：已存在的会话不会被覆盖，因为它是依据。
func (s *Store) UpsertSession(sess experience.Session) error {
	if err := sess.Validate(); err != nil {
		return err
	}
	turns, err := json.Marshal(sess.Turns)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO experience_session (id, scenario, title, turns, at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		sess.ID, sess.Scenario, sess.Title, string(turns), sess.At.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("保存会话记录失败：%w", err)
	}
	return nil
}

// AppendSessionTurn 追加一段发言并返回追加后的会话。
//
// 只读一次、追加一次、写回一次，全程在一个事务里：
// 并发追加时后到的不会覆盖先到的。
func (s *Store) AppendSessionTurn(id string, role verification.Actor, text string,
	at time.Time) (experience.Session, error) {

	tx, err := s.db.Begin()
	if err != nil {
		return experience.Session{}, err
	}
	defer tx.Rollback()

	sess, ok, err := sessionTx(tx, id)
	if err != nil {
		return experience.Session{}, err
	}
	if !ok {
		return experience.Session{}, fmt.Errorf("会话记录 %s 不存在", id)
	}
	next, err := sess.Append(role, text, at)
	if err != nil {
		return experience.Session{}, err
	}
	turns, err := json.Marshal(next.Turns)
	if err != nil {
		return experience.Session{}, err
	}
	if _, err := tx.Exec(`UPDATE experience_session SET turns = ? WHERE id = ?`,
		string(turns), id); err != nil {
		return experience.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return experience.Session{}, err
	}
	return next, nil
}

// Sessions 返回某场景的会话记录，按时间倒序。场景名为空表示全部。
func (s *Store) Sessions(scenario string) ([]experience.Session, error) {
	q := `SELECT id, scenario, title, turns, at FROM experience_session`
	var args []any
	if scenario != "" {
		q += ` WHERE scenario = ?`
		args = append(args, scenario)
	}
	q += ` ORDER BY at DESC, id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []experience.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Session 按 ID 取会话记录。
func (s *Store) Session(id string) (experience.Session, bool, error) {
	return sessionTx(s.db, id)
}

type queryerRow interface {
	QueryRow(query string, args ...any) *sql.Row
}

func sessionTx(q queryerRow, id string) (experience.Session, bool, error) {
	var (
		sess     experience.Session
		turns    string
		atString string
	)
	err := q.QueryRow(`SELECT id, scenario, title, turns, at FROM experience_session WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Scenario, &sess.Title, &turns, &atString)
	if err == sql.ErrNoRows {
		return experience.Session{}, false, nil
	}
	if err != nil {
		return experience.Session{}, false, err
	}
	if err := json.Unmarshal([]byte(turns), &sess.Turns); err != nil {
		return experience.Session{}, false, fmt.Errorf("会话 %s 的发言无法解析：%w", id, err)
	}
	if t, err := time.Parse(time.RFC3339, atString); err == nil {
		sess.At = t
	}
	return sess, true, nil
}

func scanSession(rows *sql.Rows) (experience.Session, error) {
	var (
		sess     experience.Session
		turns    string
		atString string
	)
	if err := rows.Scan(&sess.ID, &sess.Scenario, &sess.Title, &turns, &atString); err != nil {
		return sess, err
	}
	if err := json.Unmarshal([]byte(turns), &sess.Turns); err != nil {
		return sess, err
	}
	if t, err := time.Parse(time.RFC3339, atString); err == nil {
		sess.At = t
	}
	return sess, nil
}

// SetSupersedes 记录「本条目取代了哪一条」。
//
// 只允许设置一次：取代关系一旦成立就不该再改，
// 改了会让「旧判断为什么不在生效」这件事失去唯一解释。
func (s *Store) SetSupersedes(id, supersedes string) error {
	if supersedes == "" {
		return fmt.Errorf("取代关系不能为空")
	}
	res, err := s.db.Exec(
		`UPDATE experience_entry SET supersedes = ? WHERE id = ? AND supersedes = ''`,
		supersedes, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var exists int
		if err := s.db.QueryRow(`SELECT count(*) FROM experience_entry WHERE id = ?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("经验 %s 不存在", id)
		}
		return fmt.Errorf("经验 %s 的取代关系已经设置过，不得改写", id)
	}
	return nil
}

// AssertionStatus 返回某断言的核验状态。第二个返回值为 false 表示不存在。
//
// 经验失效检查用它：依赖的断言被驳回之后，靠人去想起来
// 「那条经验该失效了」是不现实的。
func (s *Store) AssertionStatus(id string) (string, bool, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM assertion WHERE id = ?`, id).Scan(&status)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return status, true, nil
}

// ── 经验条目 ────────────────────────────────────────────────────────────────
// PutEntry 写入一条经验。
//
// 它只接受**新 ID**：修订走 Supersede 写新条目，而不是覆盖旧条目。
// 允许覆盖会让「旧判断保留可查」这条规则在存储层就失去保障。
func (s *Store) PutEntry(e experience.Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM experience_entry WHERE id = ?`, e.ID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("经验 %s 已存在——修订请写新条目并标记取代，旧条目保留", e.ID)
	}
	if err := putEntryTx(tx, e); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateEntryStatus 只改状态与理由，**不改内容**。
//
// 批准、驳回、失效、待重算都是状态流转；判断本身是记录，不随之改动。
func (s *Store) UpdateEntryStatus(id, status, reason string,
	approvedBy *verification.Actor) error {

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cur, ok, err := entryTx(tx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("经验 %s 不存在", id)
	}
	cur.Status = experience.Status(status)
	cur.ApproveReason = reason
	cur.ApprovedBy = approvedBy
	if err := cur.Validate(); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE experience_entry SET status = ?, approve_reason = ?, approved_kind = ?, approved_id = ? WHERE id = ?`,
		string(cur.Status), reason, kindOf(approvedBy), idOf(approvedBy), id); err != nil {
		return err
	}
	return tx.Commit()
}

func kindOf(a *verification.Actor) string {
	if a == nil {
		return ""
	}
	return string(a.Kind)
}

func idOf(a *verification.Actor) string {
	if a == nil {
		return ""
	}
	return a.ID
}

func putEntryTx(tx *sql.Tx, e experience.Entry) error {
	chain, err := json.Marshal(e.Chain)
	if err != nil {
		return err
	}
	if e.Chain == nil {
		chain = []byte("[]")
	}
	collab, err := json.Marshal(e.Collaborators)
	if err != nil {
		return err
	}
	if e.Collaborators == nil {
		collab = []byte("[]")
	}
	sampleSize, sampleFrom := 0, ""
	if e.Sample != nil {
		sampleSize, sampleFrom = e.Sample.Size, e.Sample.From
	}
	_, err = tx.Exec(`
		INSERT INTO experience_entry
		(id, scenario, topic, kind, statement, rationale, chain, sample_size, sample_from,
		 conditions, preference, proposed_kind, proposed_id, collaborators,
		 approved_kind, approved_id, approve_reason, session_id, anchor, status, at, supersedes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Scenario, e.Topic, string(e.Kind), e.Statement, e.Rationale,
		string(chain), sampleSize, sampleFrom, e.Conditions, e.Preference,
		string(e.ProposedBy.Kind), e.ProposedBy.ID, string(collab),
		kindOf(e.ApprovedBy), idOf(e.ApprovedBy), e.ApproveReason,
		e.SessionID, e.Anchor, string(e.Status), e.At.UTC().Format(time.RFC3339), e.Supersedes)
	if err != nil {
		return fmt.Errorf("保存经验失败：%w", err)
	}
	return nil
}

// Entries 返回某场景的经验，按时间倒序。场景名为空表示全部。
func (s *Store) Entries(scenario string) ([]experience.Entry, error) {
	q := entryCols + ` FROM experience_entry`
	var args []any
	if scenario != "" {
		q += ` WHERE scenario = ?`
		args = append(args, scenario)
	}
	q += ` ORDER BY at DESC, id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []experience.Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Entry 按 ID 取一条经验。
func (s *Store) Entry(id string) (experience.Entry, bool, error) {
	return entryTx(s.db, id)
}

func entryTx(q queryerRow, id string) (experience.Entry, bool, error) {
	row := q.QueryRow(entryCols+` FROM experience_entry WHERE id = ?`, id)
	e, err := scanEntryRow(row)
	if err == sql.ErrNoRows {
		return experience.Entry{}, false, nil
	}
	if err != nil {
		return experience.Entry{}, false, err
	}
	return e, true, nil
}

const entryCols = `SELECT id, scenario, topic, kind, statement, rationale, chain,
	sample_size, sample_from, conditions, preference, proposed_kind, proposed_id, collaborators,
	approved_kind, approved_id, approve_reason, session_id, anchor, status, at, supersedes`

type scanner interface{ Scan(dest ...any) error }

func scanEntryRow(row scanner) (experience.Entry, error) {
	var (
		e                            experience.Entry
		kind, chain, collab, status  string
		cond, pref                   string
		pk, pid, ak, aid, reason     string
		sessionID, anchor, at, super string
		sampleSize                   int
		sampleFrom                   string
	)
	err := row.Scan(&e.ID, &e.Scenario, &e.Topic, &kind, &e.Statement, &e.Rationale, &chain,
		&sampleSize, &sampleFrom, &cond, &pref, &pk, &pid, &collab,
		&ak, &aid, &reason, &sessionID, &anchor, &status, &at, &super)
	if err != nil {
		return e, err
	}
	e.Kind = experience.Kind(kind)
	e.Conditions, e.Preference = cond, pref
	e.ProposedBy = verification.Actor{Kind: verification.ActorKind(pk), ID: pid}
	e.ApproveReason = reason
	e.SessionID, e.Anchor = sessionID, anchor
	e.Status = experience.Status(status)
	e.Supersedes = super
	if sampleSize > 0 || sampleFrom != "" {
		e.Sample = &experience.Sample{Size: sampleSize, From: sampleFrom}
	}
	if err := json.Unmarshal([]byte(chain), &e.Chain); err != nil {
		return e, fmt.Errorf("经验 %s 的推导链无法解析：%w", e.ID, err)
	}
	if err := json.Unmarshal([]byte(collab), &e.Collaborators); err != nil {
		return e, fmt.Errorf("经验 %s 的参与者无法解析：%w", e.ID, err)
	}
	if ak != "" || aid != "" {
		e.ApprovedBy = &verification.Actor{Kind: verification.ActorKind(ak), ID: aid}
	}
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		e.At = t
	}
	return e, nil
}

func scanEntry(rows *sql.Rows) (experience.Entry, error) { return scanEntryRow(rows) }
