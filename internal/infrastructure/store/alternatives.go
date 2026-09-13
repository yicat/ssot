// 备选方案的持久化（见 docs/specs/alternatives.spec.md）。
//
// 方案必须持久化并**复现原始内容与原始依据**：它记录的是「当时基于什么
// 做了什么取舍」，后续数据变化不该让它悄悄变样——那会让人无法复盘。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/alternatives"
	"github.com/ngnl5/ssot/internal/domain/assertion"
)

// PutPlan 写入一个方案。
//
// 只接受**新 ID**：重新生成的方案与历史方案结论相同但依据不同时，
// 视为不同版本、两者都保留。允许覆盖会让「复盘」失去原始依据。
func (s *Store) PutPlan(p alternatives.Plan) error {
	if err := p.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM plan WHERE id = ?`, p.ID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("方案 %s 已存在——重新生成请写新版本，历史方案保留", p.ID)
	}
	if err := putPlanTx(tx, p); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdatePlanStatus 只改状态与选定记录，**不改方案内容**。
func (s *Store) UpdatePlanStatus(id, status, chosenBy, reason string) error {
	cur, ok, err := s.Plan(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("方案 %s 不存在", id)
	}
	cur.Status = alternatives.Status(status)
	cur.ChosenBy = chosenBy
	cur.ChooseReason = reason
	if err := cur.Validate(); err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE plan SET status = ?, chosen_by = ?, choose_reason = ? WHERE id = ?`,
		status, chosenBy, reason, id)
	return err
}

func putPlanTx(tx *sql.Tx, p alternatives.Plan) error {
	must := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			return "[]"
		}
		return string(b)
	}
	fromConflict := 0
	if p.FromConflict {
		fromConflict = 1
	}
	_, err := tx.Exec(`
		INSERT INTO plan
		(id, scenario, title, objective, constraints, assumptions, preference, actions,
		 metrics, opportunity, depends, unverified_ratio, max_confidence, sources,
		 from_conflict, status, at, chosen_by, choose_reason)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Scenario, p.Title, p.Objective,
		must(p.Constraints), must(p.Assumptions), p.Preference, must(p.Actions),
		must(p.Metrics), p.Opportunity, must(p.Depends),
		p.UnverifiedRatio, string(p.MaxConfidence), must(p.Sources),
		fromConflict, string(p.Status), p.At.UTC().Format(time.RFC3339),
		p.ChosenBy, p.ChooseReason)
	if err != nil {
		return fmt.Errorf("保存方案失败：%w", err)
	}
	return nil
}

// Plans 返回某场景的方案，按时间倒序。场景名为空表示全部。
func (s *Store) Plans(scenario string) ([]alternatives.Plan, error) {
	q := planCols
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
	var out []alternatives.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Plan 按 ID 取一个方案。
func (s *Store) Plan(id string) (alternatives.Plan, bool, error) {
	row := s.db.QueryRow(planCols+` WHERE id = ?`, id)
	p, err := scanPlan(row)
	if err == sql.ErrNoRows {
		return alternatives.Plan{}, false, nil
	}
	if err != nil {
		return alternatives.Plan{}, false, err
	}
	return p, true, nil
}

// Assertion 返回某断言的核验状态与分级。第三个返回值为 false 表示不存在。
//
// 方案的可信度用它算：方案不得高于其依赖断言中的最低分级。
func (s *Store) Assertion(id string) (status, confidence string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT status, confidence FROM assertion WHERE id = ?`, id).
		Scan(&status, &confidence)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return status, confidence, true, nil
}

const planCols = `SELECT id, scenario, title, objective, constraints, assumptions, preference,
	actions, metrics, opportunity, depends, unverified_ratio, max_confidence, sources,
	from_conflict, status, at, chosen_by, choose_reason FROM plan`

func scanPlan(row scanner) (alternatives.Plan, error) {
	var (
		p                             alternatives.Plan
		cons, assum, acts, mets, deps string
		srcs                          string
		conf                          string
		ratio                         float64
		status, at                    string
		chosenBy, reason              string
		fromConflict                  int
	)
	err := row.Scan(&p.ID, &p.Scenario, &p.Title, &p.Objective, &cons, &assum, &p.Preference,
		&acts, &mets, &p.Opportunity, &deps, &ratio, &conf, &srcs,
		&fromConflict, &status, &at, &chosenBy, &reason)
	if err != nil {
		return p, err
	}
	p.UnverifiedRatio = ratio
	p.MaxConfidence = assertion.Confidence(conf)
	p.FromConflict = fromConflict != 0
	p.Status = alternatives.Status(status)
	p.ChosenBy, p.ChooseReason = chosenBy, reason
	for _, f := range []struct {
		raw string
		dst any
	}{
		{cons, &p.Constraints}, {assum, &p.Assumptions}, {acts, &p.Actions},
		{mets, &p.Metrics}, {deps, &p.Depends}, {srcs, &p.Sources},
	} {
		if err := json.Unmarshal([]byte(f.raw), f.dst); err != nil {
			return p, fmt.Errorf("方案 %s 的字段无法解析：%w", p.ID, err)
		}
	}
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		p.At = t
	}
	return p, nil
}
