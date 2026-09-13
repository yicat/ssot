// 待判定事项的持久化（见 docs/specs/decision.spec.md）。
//
// 三件事必须由存储层保证，不能在应用层「先查再插」：
//
//	幂等    —— 重复接入不得改变已裁决事项的状态
//	原子    —— 裁决产出的断言与核验记录要么都在，要么都不在
//	只追加  —— 改判把旧结论推进历史，不覆盖
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/decision"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// DecisionUpsert 报告一次接入发现的结果。
type DecisionUpsert = decision.Upsert

// UpsertDecisions 保存接入层发现的歧义事项。
//
// 已存在的事项按领域层的 Refresh 规则合并：修订变化则重新打开，
// 原选项消失则标记需复核，修订未变则**原样不动**。
//
// 注意：本轮未再出现的事项**不删除**。原件仍在，人仍可裁决；
// 静默删掉待办等于把问题变没。
func (s *Store) UpsertDecisions(items []decision.Item) (decision.Upsert, error) {
	var res decision.Upsert
	if len(items) == 0 {
		return res, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, in := range items {
		if err := in.Validate(); err != nil {
			return res, fmt.Errorf("拒绝保存待判定事项：%w", err)
		}
		res.Seen++

		old, ok, err := decisionTx(tx, in.ID)
		if err != nil {
			return res, err
		}
		if !ok {
			if err := putDecisionTx(tx, in); err != nil {
				return res, err
			}
			res.Created++
			continue
		}

		merged := old.Refresh(in.Candidates, in.Revision, in.Reason, in.Context)
		if merged.Status != old.Status {
			res.Reopened++
		}
		if sameDecision(old, merged) {
			res.Unchanged++
			continue
		}
		if err := putDecisionTx(tx, merged); err != nil {
			return res, err
		}
		res.Updated++
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

func sameDecision(a, b decision.Item) bool {
	aj, _ := json.Marshal(a.Candidates)
	bj, _ := json.Marshal(b.Candidates)
	return a.Status == b.Status && a.Revision == b.Revision &&
		a.Reason == b.Reason && string(aj) == string(bj)
}

// Decisions 返回待判定事项。
//
// status 为空表示「还需要人看的」：待判定 + 已暂缓 + 需复核。
// limit <= 0 表示不限制。
func (s *Store) Decisions(status string, limit int) ([]decision.Item, error) {
	q := `SELECT id, entity, subject, predicate, reason, context, artifact, revision, captured_at,
	             source_name, candidates, status, resolution, deferral, history
	      FROM decision`
	var args []any
	switch status {
	case "all":
		// 不加筛选：统计与导出需要看全量
	case "", string(decision.StatusOpen):
		// 「待判定」在界面上被理解为「还没处理的」——暂缓与需复核同样还没处理完，
		// 因此空串与 open 是同一组。
		q += ` WHERE status IN (?,?,?)`
		args = append(args, string(decision.StatusOpen), string(decision.StatusDeferred), string(decision.StatusStale))
	default:
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY subject, predicate, id`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDecisions(rows)
}

// Decision 返回单个事项。第二个返回值为 false 表示不存在。
func (s *Store) Decision(id string) (decision.Item, bool, error) {
	return decisionTx(s.db, id)
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func decisionTx(q queryer, id string) (decision.Item, bool, error) {
	var (
		it                                                    decision.Item
		cands, resJSON, defJSON, histJSON                     sql.NullString
		revision, sourceName, status, reason, ctx, capturedAt string
	)
	err := q.QueryRow(`SELECT id, entity, subject, predicate, reason, context, artifact, revision, captured_at,
	             source_name, candidates, status, resolution, deferral, history
	                   FROM decision WHERE id = ?`, id).Scan(
		&it.ID, &it.Entity, &it.Subject, &it.Predicate, &reason, &ctx, &it.Artifact, &revision, &capturedAt,
		&sourceName, &cands, &status, &resJSON, &defJSON, &histJSON)
	if err == sql.ErrNoRows {
		return decision.Item{}, false, nil
	}
	if err != nil {
		return decision.Item{}, false, err
	}
	it.Reason, it.Context, it.Revision, it.Source = reason, ctx, revision, sourceName
	if t, err := time.Parse(time.RFC3339, capturedAt); err == nil {
		it.CapturedAt = t
	}
	it.Status = decision.Status(status)
	if err := unmarshalInto(cands.String, &it.Candidates); err != nil {
		return decision.Item{}, false, fmt.Errorf("事项 %s 的候选无法解析：%w", id, err)
	}
	if resJSON.Valid && resJSON.String != "" {
		var r decision.Resolution
		if err := unmarshalInto(resJSON.String, &r); err != nil {
			return decision.Item{}, false, fmt.Errorf("事项 %s 的裁决无法解析：%w", id, err)
		}
		it.Resolution = &r
	}
	if defJSON.Valid && defJSON.String != "" {
		var d decision.Deferral
		if err := unmarshalInto(defJSON.String, &d); err != nil {
			return decision.Item{}, false, fmt.Errorf("事项 %s 的暂缓记录无法解析：%w", id, err)
		}
		it.Deferral = &d
	}
	if histJSON.Valid && histJSON.String != "" {
		if err := unmarshalInto(histJSON.String, &it.History); err != nil {
			return decision.Item{}, false, fmt.Errorf("事项 %s 的历史无法解析：%w", id, err)
		}
	}
	return it, true, nil
}

func putDecisionTx(tx *sql.Tx, it decision.Item) error {
	cands, err := json.Marshal(it.Candidates)
	if err != nil {
		return err
	}
	res, err := marshalNullable(it.Resolution)
	if err != nil {
		return err
	}
	def, err := marshalNullable(it.Deferral)
	if err != nil {
		return err
	}
	hist, err := marshalNullable(it.History)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO decision
		(id, entity, subject, predicate, reason, context, artifact, revision, captured_at,
		 source_name, candidates, status, resolution, deferral, history)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			reason=excluded.reason, context=excluded.context, revision=excluded.revision,
			captured_at=excluded.captured_at, source_name=excluded.source_name,
			candidates=excluded.candidates, status=excluded.status,
			resolution=excluded.resolution, deferral=excluded.deferral, history=excluded.history`,
		it.ID, it.Entity, it.Subject, it.Predicate, it.Reason, it.Context, it.Artifact, it.Revision,
		it.CapturedAt.UTC().Format(time.RFC3339), it.Source,
		string(cands), string(it.Status), res, def, hist)
	if err != nil {
		return fmt.Errorf("保存待判定事项失败：%w", err)
	}
	return nil
}

func marshalNullable(v any) (any, error) {
	switch x := v.(type) {
	case *decision.Resolution:
		if x == nil {
			return nil, nil
		}
	case *decision.Deferral:
		if x == nil {
			return nil, nil
		}
	case []decision.Resolution:
		if len(x) == 0 {
			return nil, nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func unmarshalInto(s string, v any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

func scanDecisions(rows *sql.Rows) ([]decision.Item, error) {
	var out []decision.Item
	for rows.Next() {
		var (
			it                                                    decision.Item
			cands, resJSON, defJSON, histJSON                     sql.NullString
			revision, sourceName, status, reason, ctx, capturedAt string
		)
		if err := rows.Scan(&it.ID, &it.Entity, &it.Subject, &it.Predicate, &reason, &ctx,
			&it.Artifact, &revision, &capturedAt, &sourceName, &cands, &status, &resJSON, &defJSON, &histJSON); err != nil {
			return nil, err
		}
		it.Reason, it.Context, it.Revision, it.Source = reason, ctx, revision, sourceName
		if t, err := time.Parse(time.RFC3339, capturedAt); err == nil {
			it.CapturedAt = t
		}
		it.Status = decision.Status(status)
		if err := unmarshalInto(cands.String, &it.Candidates); err != nil {
			return nil, err
		}
		if resJSON.Valid && resJSON.String != "" {
			var r decision.Resolution
			if err := unmarshalInto(resJSON.String, &r); err != nil {
				return nil, err
			}
			it.Resolution = &r
		}
		if defJSON.Valid && defJSON.String != "" {
			var d decision.Deferral
			if err := unmarshalInto(defJSON.String, &d); err != nil {
				return nil, err
			}
			it.Deferral = &d
		}
		if histJSON.Valid && histJSON.String != "" {
			if err := unmarshalInto(histJSON.String, &it.History); err != nil {
				return nil, err
			}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ResolveDecision 原子地应用一次裁决：写入断言（若有）、写入核验记录、更新事项状态。
//
// **三件事必须在同一个事务里。** 否则会出现「断言已入库但没有对应裁决记录」，
// 而核验的全部意义就在于那条记录。
func (s *Store) ResolveDecision(it decision.Item, cs assertion.ChangeSet, rec *verification.Record) error {
	if err := it.Validate(); err != nil {
		return err
	}
	if rec != nil {
		if err := rec.Validate(); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := applyTx(tx, cs); err != nil {
		return err
	}
	if rec != nil {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM assertion WHERE id = ?`, rec.AssertionID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("断言 %s 不存在——裁决与断言必须同时成立", rec.AssertionID)
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
	}
	if err := putDecisionTx(tx, it); err != nil {
		return err
	}
	return tx.Commit()
}

// DeferDecision 更新事项为已暂缓。不产生断言。
func (s *Store) DeferDecision(it decision.Item) error {
	if err := it.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := putDecisionTx(tx, it); err != nil {
		return err
	}
	return tx.Commit()
}

// DecisionCounts 按状态统计事项数。
func (s *Store) DecisionCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, count(*) FROM decision GROUP BY status`)
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

// SubjectImpact 统计每个主体上已有断言被引用（作为推导输入）的总次数。
//
// 这是待判定队列「影响面」的数据来源：一个倍率错了，
// 牵连的是引用该式神数据的派生断言。
func (s *Store) SubjectImpact() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT id, entity, subject FROM assertion`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owner := map[string]string{}
	for rows.Next() {
		var id, ent, sub string
		if err := rows.Scan(&id, &ent, &sub); err != nil {
			return nil, err
		}
		owner[id] = ent + "|" + sub
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	refs, err := s.ReferenceCounts()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for id, n := range refs {
		if k, ok := owner[id]; ok {
			out[k] += n
		}
	}
	return out, nil
}
