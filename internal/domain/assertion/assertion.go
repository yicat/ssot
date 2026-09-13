// Package assertion 定义系统的最小数据单元。
//
// 见 docs/specs/core.spec.md。断言不是「实体的属性」，而是**一句话**：
// 某个主体、在特定条件下、由某个来源、在某个时间点、以某个可信度成立。
//
// 这条定义直接回答「同一个指代在不同条件下有不同效果」——
// 那不是指代问题，是效果被错建模成了实体的属性。
// 本包用 Qualifiers 承载条件：**限定条件不同的两条断言并存，互不覆盖。**
package assertion

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/domain/value"
)

// Confidence 是断言的分级。
//
// 分级的意义：**隐式信息与直引事实必须可分辨**。
// 没有分级，猜测会与事实混在一起，无从追溯。
type Confidence string

const (
	L1 Confidence = "L1" // 直引：可与原文逐字比对
	L2 Confidence = "L2" // 结构化：从文本解析得出
	L3 Confidence = "L3" // 推导：由其他断言计算得出，必须有推导链
	L4 Confidence = "L4" // 推断：含补全的假设
)

// Valid 报告分级是否合法。
func (c Confidence) Valid() bool {
	switch c {
	case L1, L2, L3, L4:
		return true
	}
	return false
}

// Status 是核验状态。
type Status string

const (
	StatusPending     Status = "pending"      // 待核验（默认）
	StatusAutoChecked Status = "auto-checked" // 已自动预检——不等于已核验
	StatusVerified    Status = "verified"     // 已核验
	StatusDisputed    Status = "disputed"     // 有争议
	StatusRejected    Status = "rejected"     // 已驳回
	StatusExpired     Status = "expired"      // 已过期
	StatusUnmodeled   Status = "unmodeled"    // 未建模但已记录
)

// Qualifiers 是使该断言成立的条件。
// 这些条件参与断言的**身份判定**：限定条件不同即为不同断言。
type Qualifiers struct {
	Version    string `json:"version,omitempty"`
	ValidFrom  string `json:"validFrom,omitempty"`
	ValidUntil string `json:"validUntil,omitempty"`
	Scene      string `json:"scene,omitempty"`
	Condition  string `json:"condition,omitempty"`
}

// IsZero 报告是否没有任何限定条件。
func (q Qualifiers) IsZero() bool { return q == Qualifiers{} }

// String 返回紧凑表示，用于身份比较与展示。
func (q Qualifiers) String() string {
	if q.IsZero() {
		return ""
	}
	return fmt.Sprintf("v=%s;from=%s;until=%s;scene=%s;cond=%s",
		q.Version, q.ValidFrom, q.ValidUntil, q.Scene, q.Condition)
}

// Source 是谁说的。注意它与「依据」不同：依据是凭什么这么说。
type Source struct {
	Name string `json:"name"`           // 来源名，例如 huijiwiki
	Tier string `json:"tier,omitempty"` // official / semi-official / community / measurement
	Ref  string `json:"ref,omitempty"`  // 来源内部的引用
}

// Provenance 是溯源：原文锚点 + 源内容修订标识 + 采集时间。
type Provenance struct {
	Artifact   string    `json:"artifact"`   // 原件标识
	Anchor     string    `json:"anchor"`     // 原文位置
	Revision   string    `json:"revision"`   // 源内容的修订标识
	CapturedAt time.Time `json:"capturedAt"` // 采集时间
}

// Derivation 是推导链，L3 断言必填。
type Derivation struct {
	Formula    string   `json:"formula"`
	FormulaRev string   `json:"formulaRev,omitempty"`
	Inputs     []string `json:"inputs"` // 依赖的断言 ID
}

// Assertion 是一条断言。
type Assertion struct {
	ID         string        `json:"id"`
	Entity     string        `json:"entity"`    // 所属实体类型
	Subject    string        `json:"subject"`   // 主体，稳定标识
	Predicate  string        `json:"predicate"` // 谓词：属性名或关系名
	Value      value.Value   `json:"value"`
	Qualifiers Qualifiers    `json:"qualifiers"`
	Source     Source        `json:"source"`
	Provenance Provenance    `json:"provenance"`
	Confidence Confidence    `json:"confidence"`
	Status     Status        `json:"status"`
	Derived    *Derivation   `json:"derived,omitempty"`
}

// Key 是断言的身份：主体 + 谓词 + 限定条件。
//
// **限定条件不同即为不同断言**——这是「同一效果在不同条件下不同」
// 能被正确表达的前提。
func (a Assertion) Key() string {
	return a.Entity + "|" + a.Subject + "|" + a.Predicate + "|" + a.Qualifiers.String()
}

// SameKey 报告两条断言是否描述同一件事（限定条件相同）。
func (a Assertion) SameKey(b Assertion) bool { return a.Key() == b.Key() }

// Conflicting 报告两条断言是否冲突：
// 身份相同（主体、谓词、限定条件全同）但取值不同。
func (a Assertion) Conflicting(b Assertion) bool {
	return a.SameKey(b) && a.Value != b.Value
}

// ValueJSON 是取值在存储层的规范化序列化形式。
//
// **准入层的冲突检测与存储层的唯一索引必须使用同一形式**——
// 否则会把「一致」误判成「冲突」（实测踩过：准入层多带了 kind/unit 前缀，
// 导致重复同步把全部断言误报为冲突）。
func ValueJSON(v value.Value) string {
	b, err := json.Marshal(v.Data)
	if err != nil {
		return "<unserializable>"
	}
	return string(b)
}

// Filter 是断言查询条件。空字段表示不限定。
type Filter struct {
	Entity     string
	Status     string
	Predicate  string
	Artifact   string
	Revision   string
	Confidence string
	Subject    string
	Limit      int
}

// Describe 返回人类可读的筛选描述。
//
// 无条件时必须**显式提示**——它一次会选中全部断言，批量操作前
// 不能让人误以为筛得很精确。
func (f Filter) Describe() string {
	var parts []string
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add("实体", f.Entity)
	add("状态", f.Status)
	add("谓词", f.Predicate)
	add("原件", f.Artifact)
	add("修订", f.Revision)
	add("分级", f.Confidence)
	add("主体", f.Subject)
	if len(parts) == 0 {
		return "（无条件——这会选中全部断言）"
	}
	return strings.Join(parts, " 且 ")
}

// ChangeSet 是一批待应用的变更。
//
// 准入层**不直接写库**，而是产出变更集，由存储层原子应用：
// 要么全部成功，要么全部回滚。这样校验才不会被绕过。
type ChangeSet struct {
	// Insert 是新增断言
	Insert []Assertion
	// Conflict 是检出冲突的断言 ID——它们**仍会被写入**，只是标记为有争议。
	// 同一件事的不同说法都要保留，由人裁决。
	Conflict []string
}

// Validate 校验断言的完整性。
//
// 缺来源或缺溯源的断言必须被拒绝——没有出处的数据无法核验、无法追溯、
// 也无法在源头变化时被发现过期。
func (a Assertion) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("断言缺少 ID")
	}
	if a.Entity == "" {
		return fmt.Errorf("断言 %s 缺少实体类型", a.ID)
	}
	if a.Subject == "" {
		return fmt.Errorf("断言 %s 缺少主体", a.ID)
	}
	if a.Predicate == "" {
		return fmt.Errorf("断言 %s 缺少谓词", a.ID)
	}
	if !a.Confidence.Valid() {
		return fmt.Errorf("断言 %s 的分级非法：%q", a.ID, a.Confidence)
	}
	if a.Source.Name == "" {
		return fmt.Errorf("断言 %s 缺少来源", a.ID)
	}
	if a.Provenance.Artifact == "" {
		return fmt.Errorf("断言 %s 缺少溯源（原件标识）", a.ID)
	}
	if a.Confidence == L3 && a.Derived == nil {
		return fmt.Errorf("断言 %s 分级为 L3 但没有推导链", a.ID)
	}
	return nil
}
