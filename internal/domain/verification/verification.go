// Package verification 定义核验记录与责任归属。
//
// 见 docs/specs/verification.spec.md。三条核心规则：
//
//  1. **无追责的核验等于没有核验。** 提出者与批准者必须分别标识。
//  2. **agent 可以产出大量候选，但不能自己决定什么算数。** 批准者必须是人。
//  3. **方法四级不可互相等同。** 「编审」是人工判断，不是验证。
//
// 默认一切未核验；未核验内容允许使用，但必须可见——而不是「核验通过才能用」，
// 那在人力上不现实，结果是工具直接不可用。
package verification

import (
	"fmt"
	"time"
)

// Method 是核验方法，强度从高到低。
type Method string

const (
	// Measurement 实测：在游戏内实际观测。最强。
	// 必须同时记录版本、配置与样本数，否则无从判断可信度。
	Measurement Method = "measurement"
	// Recompute 重算：由其他已核验断言推导且结果一致。
	Recompute Method = "recompute"
	// CrossSource 多源：多个**独立**来源一致。
	// 必须先判独立性——三个互相转载的攻略不是一个证据。
	CrossSource Method = "cross-source"
	// Editorial 编审：人工阅读后判断接受。
	// **这是判断，不是验证。** 常常是唯一可行的手段，但不得等同于实测或多源。
	Editorial Method = "editorial"
)

// Valid 报告方法是否合法。
func (m Method) Valid() bool {
	switch m {
	case Measurement, Recompute, CrossSource, Editorial:
		return true
	}
	return false
}

// Rank 返回方法强度，数值越大越强。用于「可信度不得高于其依赖」这类规则。
func (m Method) Rank() int {
	switch m {
	case Measurement:
		return 4
	case Recompute:
		return 3
	case CrossSource:
		return 2
	case Editorial:
		return 1
	}
	return 0
}

// Label 返回中文名。
func (m Method) Label() string {
	switch m {
	case Measurement:
		return "实测"
	case Recompute:
		return "重算"
	case CrossSource:
		return "多源"
	case Editorial:
		return "编审"
	}
	return string(m)
}

// ActorKind 是参与者的种类。
type ActorKind string

const (
	Human ActorKind = "human"
	Agent ActorKind = "agent"
)

// Actor 是一个参与者。人与 agent 都有身份，都可追溯。
type Actor struct {
	Kind ActorKind
	ID   string
}

func (a Actor) Valid() bool {
	if a.ID == "" {
		return false
	}
	return a.Kind == Human || a.Kind == Agent
}

func (a Actor) IsHuman() bool { return a.Kind == Human }

func (a Actor) String() string { return string(a.Kind) + ":" + a.ID }

// Decision 是核验结论。
type Decision string

const (
	Approved Decision = "approved"
	Rejected Decision = "rejected"
	Disputed Decision = "disputed"
)

// Valid 报告结论是否合法。
func (d Decision) Valid() bool {
	switch d {
	case Approved, Rejected, Disputed:
		return true
	}
	return false
}

// Record 是一条核验记录。
//
// 它必须能回答「**谁、何时、凭什么、核验到哪一部分**」——
// 缺任何一项都不是一条合格的核验记录。
type Record struct {
	AssertionID string
	Decision    Decision
	Method      Method
	ProposedBy  Actor
	ApprovedBy  *Actor
	Reason      string
	// Evidence 是依据。实测时它承载版本、配置与样本数——
	// 缺依据的「实测」与猜测没有区别。
	Evidence string
	At       time.Time
}

// Validate 校验核验记录的完整性。
func (r Record) Validate() error {
	if r.AssertionID == "" {
		return fmt.Errorf("核验记录缺少断言 ID")
	}
	if !r.Decision.Valid() {
		return fmt.Errorf("未知的核验结论 %q", r.Decision)
	}
	if !r.Method.Valid() {
		return fmt.Errorf("未知的核验方法 %q（合法：measurement / recompute / cross-source / editorial）", r.Method)
	}
	if !r.ProposedBy.Valid() {
		return fmt.Errorf("核验记录缺少提出者——无追责的核验等于没有核验")
	}
	if r.ApprovedBy == nil {
		return fmt.Errorf("核验记录缺少批准者——无追责的核验等于没有核验")
	}
	if !r.ApprovedBy.Valid() {
		return fmt.Errorf("批准者标识不合法：%q", r.ApprovedBy.ID)
	}
	if !r.ApprovedBy.IsHuman() {
		return fmt.Errorf("批准者必须是**人**；agent 可以提出与取证，但不能自己决定什么算数")
	}
	if r.At.IsZero() {
		return fmt.Errorf("核验记录缺少时间")
	}
	if r.Reason == "" {
		return fmt.Errorf("核验记录缺少理由——只有状态没有理由的不是核验")
	}
	if r.Method == Measurement && r.Evidence == "" {
		return fmt.Errorf("以「实测」核验时必须记录依据（版本、配置、样本数）")
	}
	return nil
}

// StatusFor 返回该结论对应的断言状态。
func (d Decision) StatusFor() string {
	switch d {
	case Approved:
		return "verified"
	case Rejected:
		return "rejected"
	case Disputed:
		return "disputed"
	}
	return "pending"
}

// Summarize 返回一行摘要。
func (r Record) Summarize() string {
	by := "?"
	if r.ApprovedBy != nil {
		by = r.ApprovedBy.String()
	}
	return fmt.Sprintf("%s 由 %s 提出 / %s 批准（%s）→ %s",
		r.Method.Label(), r.ProposedBy, by, r.Decision, r.Reason)
}
