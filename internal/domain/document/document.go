// Package document 定义文档：不能变成数据的那部分事实源。
//
// 见 docs/specs/document.spec.md。系统此前只接得住「能被拆成
// (主体, 谓词, 取值) 的东西」，于是两类内容无处可去：
//
//	拆了就失真   「防御减免没有权威来源」——拆成字段就是把判断伪装成事实
//	根本不可拆   机制说明、推导理由、版本叙事
//
// 本包给它们一个与断言并列的一等公民：**文档不参与计算，只作为依据。**
// 它的价值在于任何一条断言被质疑时，能回到原文。
//
// 三条不变量：
//
//	只追加   正文不可改，更正只能登记新修订，旧修订保留
//	摘要自算 内容摘要由内容本身算出，不采信来源声明的修订标识
//	不拆     拆不出三元的原文就留在文档里
//
// 本包是纯规则，不做任何 IO。
package document

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Kind 是文档的类型。
type Kind string

const (
	// KindEvidence 依据：原文本身，断言从它抽取。
	KindEvidence Kind = "evidence"
	// KindRule 规则：机制说明，解释世界怎么运转。
	KindRule Kind = "rule"
	// KindExplanation 说明：为什么这样定义、为什么这么算。
	KindExplanation Kind = "explanation"
	// KindChange 变更：版本叙事、公告。
	KindChange Kind = "change"
	// KindUnmodeled 未建模：引擎表达不了但必须记录的整体。
	//
	// 这是 metamodel 里 `opaque` 的原始用途——「未建模但已记录」。
	KindUnmodeled Kind = "unmodeled"
)

// Valid 报告类型是否合法。
func (k Kind) Valid() bool {
	switch k {
	case KindEvidence, KindRule, KindExplanation, KindChange, KindUnmodeled:
		return true
	}
	return false
}

// Label 返回中文名。
func (k Kind) Label() string {
	switch k {
	case KindEvidence:
		return "依据"
	case KindRule:
		return "规则"
	case KindExplanation:
		return "说明"
	case KindChange:
		return "变更"
	case KindUnmodeled:
		return "未建模"
	}
	return string(k)
}

// Status 是文档的核验状态。
type Status string

const (
	// StatusUnverified 未核验（默认）。**默认未核验**与断言一致。
	StatusUnverified Status = "unverified"
	// StatusVerified 已核验：有人读过并为这份原文背书。
	StatusVerified Status = "verified"
	// StatusRejected 已驳回。条目保留。
	StatusRejected Status = "rejected"
	// StatusSuperseded 已被新修订取代。历史保留，引用它的断言仍指向它。
	StatusSuperseded Status = "superseded"
)

// Valid 报告状态是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusUnverified, StatusVerified, StatusRejected, StatusSuperseded:
		return true
	}
	return false
}

// Label 返回中文名。
func (s Status) Label() string {
	switch s {
	case StatusUnverified:
		return "未核验"
	case StatusVerified:
		return "已核验"
	case StatusRejected:
		return "已驳回"
	case StatusSuperseded:
		return "已被新修订取代"
	}
	return string(s)
}

// AppliesTo 是文档的适用范围。
//
// **空表示全项目适用**，不是「哪儿都不适用」——这个区别必须写清楚，
// 否则界面上分不出「通用文档」与「没声明适用范围」。
type AppliesTo struct {
	Versions  []string
	Scenarios []string
}

// AppliesToScenario 报告该文档在某个场景下是否成立。
func (a AppliesTo) AppliesToScenario(scenario string) bool {
	if len(a.Scenarios) == 0 {
		return true // 未声明 = 全项目适用
	}
	for _, s := range a.Scenarios {
		if s == scenario {
			return true
		}
	}
	return false
}

// Doc 是一份文档的**某个修订**。
//
// 身份跨修订稳定（ID），修订本身是历史（RevID）。
type Doc struct {
	// ID 由 (来源, 标题) 决定，**跨修订稳定**。换修订不改身份。
	ID string
	// RevID 由 (来源, 标题, 修订) 决定，标识这一份具体修订。
	RevID string

	Source string
	Title  string
	Kind   Kind

	// Revision 是源声明的修订标识。
	Revision string
	// Hash 是内容摘要，**由内容算出**。
	//
	// 修订标识可以撒谎、也可以忘记更新；摘要不会。两者都记，两个都查。
	Hash string

	CapturedAt time.Time
	// Body 是原文。镜像文档留空（归档里有），自撰文档必填。
	Body string
	// ArtifactPath 是镜像文档在归档里的路径。
	ArtifactPath string

	Applies AppliesTo

	Status     Status
	VerifiedBy *verification.Actor
	Method     verification.Method
	Reason     string

	// RegisteredAt 是登记时间。它与 CapturedAt **不是一回事**：
	// 前者是「我们什么时候把它记进来的」，后者是「我们什么时候抓到的」。
	RegisteredAt time.Time
	// SupersededBy 指向取代它的那个修订。
	SupersededBy string
}

// Input 是一次登记。
type Input struct {
	Source   string
	Title    string
	Kind     Kind
	Revision string
	// Body 是原文；自撰文档必填。
	Body string
	// ArtifactPath 是镜像文档在归档里的路径。
	ArtifactPath string
	// ContentHash 是内容摘要。Body 非空时可留空（自动算）。
	//
	// 镜像文档的正文不在文档里，摘要只能由编排层读归档后算出来给它——
	// **摘要必须来自内容**，不能来自来源声明。
	ContentHash string
	CapturedAt  time.Time
	Applies     AppliesTo
}

// ID 返回文档的稳定身份。
func ID(source, title string) string {
	h := sha1.Sum([]byte(source + "|" + title))
	return "doc" + hex.EncodeToString(h[:8])
}

// RevID 返回某一份修订的记录标识。
//
// **内容摘要进身份**：修订标识没变而内容变了的情况真实存在（来源忘了更新修订号），
// 那时它是另一条记录，不能与旧记录撞同一个 ID。
func RevID(source, title, revision, hash string) string {
	h := sha1.Sum([]byte(source + "|" + title + "|" + revision + "|" + hash))
	return "rev" + hex.EncodeToString(h[:8])
}

// HashOf 返回内容摘要。
//
// 算法带版本前缀：将来换算法时，旧摘要不该被误判成「内容变了」。
func HashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// New 构造一份未核验的文档。
func New(in Input, now time.Time) (Doc, error) {
	hash := in.ContentHash
	if hash == "" {
		// 有正文就自己算——**摘要要来自内容**，不是来自调用方的声明
		hash = HashOf(in.Body)
	}
	d := Doc{
		ID: ID(in.Source, in.Title), RevID: RevID(in.Source, in.Title, in.Revision, hash),
		Source: in.Source, Title: in.Title, Kind: in.Kind,
		Revision: in.Revision, Hash: hash,
		CapturedAt: in.CapturedAt, Body: in.Body, ArtifactPath: in.ArtifactPath,
		Applies: in.Applies, Status: StatusUnverified, RegisteredAt: now,
	}
	if err := d.Validate(); err != nil {
		return Doc{}, err
	}
	return d, nil
}

// Validate 校验文档的不变量。
func (d Doc) Validate() error {
	if d.Source == "" {
		return fmt.Errorf("文档缺少来源")
	}
	if d.Title == "" {
		return fmt.Errorf("文档缺少标题")
	}
	if !d.Kind.Valid() {
		return fmt.Errorf("文档 %s 的类型非法：%q", d.Title, d.Kind)
	}
	if d.Revision == "" {
		return fmt.Errorf("文档 %s 缺少修订标识", d.Title)
	}
	if d.Hash == "" {
		return fmt.Errorf("文档 %s 缺少内容摘要——修订标识可信，但不能只信它", d.Title)
	}
	if d.CapturedAt.IsZero() {
		return fmt.Errorf("文档 %s 缺少采集时间", d.Title)
	}
	if d.RegisteredAt.IsZero() {
		return fmt.Errorf("文档 %s 缺少登记时间", d.Title)
	}
	if !d.Status.Valid() {
		return fmt.Errorf("文档 %s 的状态非法：%q", d.Title, d.Status)
	}
	// 镜像文档可以不存正文（归档里有），自撰文档不行。
	if strings.TrimSpace(d.Body) == "" && d.ArtifactPath == "" {
		return fmt.Errorf("文档 %s 既没有正文也没有归档路径——空文档不是依据", d.Title)
	}
	if d.Status == StatusVerified {
		if d.VerifiedBy == nil || !d.VerifiedBy.IsHuman() {
			return fmt.Errorf("文档 %s 已核验但核验人不是人", d.Title)
		}
		if d.Reason == "" {
			return fmt.Errorf("文档 %s 已核验但缺少理由", d.Title)
		}
		if d.Method == verification.CrossSource {
			return fmt.Errorf(
				"文档 %s 记为多源——一份文档不可能自己构成多源，独立性判定见 verification.spec.md",
				d.Title)
		}
	}
	return nil
}

// Verify 为这份原文背书。
//
// 核验的是「这份原文可信」，**不是**「原文里的事实已被验证」——
// 两者必须分开，否则文档核验会变成事实核验的后门。
func (d Doc) Verify(by verification.Actor, method verification.Method, reason string) (Doc, error) {
	if !by.Valid() || !by.IsHuman() {
		return Doc{}, fmt.Errorf("文档核验必须由人做——无追责的核验等于没有核验")
	}
	if !method.Valid() {
		return Doc{}, fmt.Errorf("未知的核验方法 %q", method)
	}
	if method != verification.Editorial {
		return Doc{}, fmt.Errorf(
			"文档核验只能是编审：一个人读过并背书不构成「%s」——独立性判定见 verification.spec.md",
			method.Label())
	}
	if reason == "" {
		return Doc{}, fmt.Errorf("核验缺少理由——只有状态没有理由的不是核验")
	}
	if d.Status == StatusSuperseded {
		return Doc{}, fmt.Errorf("文档 %s 已被新修订取代，核验应针对当前修订", d.Title)
	}
	if d.Status == StatusRejected {
		return Doc{}, fmt.Errorf("文档 %s 已被驳回——驳回是审计轨迹，不能靠再核验抹掉", d.Title)
	}
	out := d
	out.Status = StatusVerified
	byCopy := by
	out.VerifiedBy = &byCopy
	out.Method = method
	out.Reason = reason
	if err := out.Validate(); err != nil {
		return Doc{}, err
	}
	return out, nil
}

// Reject 驳回。条目保留，驳回是审计轨迹。
func (d Doc) Reject(by verification.Actor, reason string) (Doc, error) {
	if !by.Valid() || !by.IsHuman() {
		return Doc{}, fmt.Errorf("驳回必须由人记录")
	}
	if reason == "" {
		return Doc{}, fmt.Errorf("驳回缺少理由")
	}
	out := d
	out.Status = StatusRejected
	byCopy := by
	out.VerifiedBy = &byCopy
	out.Method = verification.Editorial
	out.Reason = reason
	return out, nil
}

// SameContent 报告两份修订的内容是否相同。
//
// 判据是**摘要**，不是修订标识：来源可以忘记更新修订号，摘要不会。
// 摘要算法不同时不下「一样」的结论——宁可多核验一次。
func (d Doc) SameContent(o Doc) bool {
	return d.SameAlgo(o) && d.Hash == o.Hash
}

// SameAlgo 报告两份摘要是否用同一算法算的。
func (d Doc) SameAlgo(o Doc) bool { return algoOf(d.Hash) == algoOf(o.Hash) }

func algoOf(h string) string {
	if i := strings.Index(h, ":"); i >= 0 {
		return h[:i]
	}
	return ""
}

// Change 是一次修订变更。
type Change struct {
	// Doc 是变更之后的当前修订。
	Doc Doc
	// Previous 是被取代的那份修订；没变时为 nil。
	Previous *Doc
	// Changed 报告当前修订是否真的变了（修订标识或内容）。
	Changed bool
	// Reverted 报告修订标识回退到了旧值——这通常意味着来源出过问题。
	Reverted bool
	// Requeue 是本次需要回到待核验的断言数，由编排层填。
	Requeue int
}

// Apply 把一份新修订落到已有历史之上，返回新的历史与变更说明。
//
// 规则：
//
//	修订与内容都没变 -> 幂等，历史不动
//	修订或内容变了   -> 旧修订转「已被新取代」，新修订成为当前
//	修订回退到旧值   -> 同上，并标注「回退」
//
// **旧修订不删**：当时引用它的断言仍指向它，不能悄悄改写成指向新修订。
func Apply(history []Doc, next Doc) ([]Doc, Change, error) {
	if err := next.Validate(); err != nil {
		return nil, Change{}, err
	}
	cur := Current(history)
	if cur == nil {
		return []Doc{next}, Change{Doc: next, Changed: true}, nil
	}
	if next.RevID == cur.RevID {
		// 幂等：同一份修订重复登记不产生变更
		return history, Change{Doc: *cur, Changed: false}, nil
	}

	out := make([]Doc, 0, len(history)+1)
	for _, h := range history {
		if h.RevID == cur.RevID {
			h.Status = StatusSuperseded
			h.SupersededBy = next.RevID
		}
		out = append(out, h)
	}
	next.SupersededBy = ""
	out = append(out, next)

	ch := Change{Doc: next, Previous: cur, Changed: true}
	// 修订回退：新修订曾出现过，且比当前旧
	for _, h := range history {
		if h.Revision == next.Revision && h.RegisteredAt.Before(cur.RegisteredAt) {
			ch.Reverted = true
		}
	}
	return out, ch, nil
}

// Current 返回当前生效的修订；没有历史时返回 nil。
func Current(history []Doc) *Doc {
	var best *Doc
	for i := range history {
		d := history[i]
		if d.Status == StatusSuperseded {
			continue
		}
		if best == nil || d.RegisteredAt.After(best.RegisteredAt) {
			best = &history[i]
		}
	}
	return best
}

// Latest 返回登记时间最晚的修订，**不论状态**。
//
// 与 Current 的区别：Current 跳过已被取代的，Latest 不跳。
// 「这份文档最新一版是什么」与「现在生效的是哪一版」是两个问题。
func Latest(history []Doc) *Doc {
	var best *Doc
	for i := range history {
		d := history[i]
		if best == nil || d.RegisteredAt.After(best.RegisteredAt) {
			best = &history[i]
		}
	}
	return best
}

// Order 给出修订顺序：登记时间倒序。
func Order(history []Doc) []Doc {
	out := append([]Doc(nil), history...)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].RegisteredAt.Equal(out[j].RegisteredAt) {
			return out[i].RegisteredAt.After(out[j].RegisteredAt)
		}
		return out[i].RevID < out[j].RevID
	})
	return out
}

// UnregisteredRef 是一个「依据未登记」的原件。
//
// 「溯源的终点是一个字符串」正是本层要消灭的状态，因此剩余多少必须能查出来，
// 而不是靠人感觉。
type UnregisteredRef struct {
	Artifact   string
	Assertions int
}
