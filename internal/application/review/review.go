// Package review 是核验工作的用例编排。
//
// 它把三件事从界面里抽出来，使 CLI 与 GUI 走同一套逻辑：
//
//	优先级排序 —— 人力有限时先核验哪些
//	冲突呈现   —— 同一身份的不同说法成组摆出来
//	批量核验   —— 按筛选条件处理，默认先预览
//
// 规则本身在 domain/verification 里（纯函数、可测）；
// 本包只负责取数与编排。数据来源由调用方以端口注入。
package review

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Source 是核验所需的读取端口。
type Source interface {
	Select(assertion.Filter) ([]assertion.Assertion, error)
	ReferenceCounts() (map[string]int, error)
	Conflicts() ([][]assertion.Assertion, error)
}

// Writer 是核验的写入端口。
type Writer interface {
	Verify(verification.Record) error
}

// QueueItem 是一条带优先级的队列项。
type QueueItem struct {
	Assertion assertion.Assertion
	Priority  verification.Priority
}

// Queue 按优先级给出待核验队列。
//
// sampleRatio 为强制抽检比例；seed 使其可复现——
// 「随机」不等于「不可复现」，出问题必须能重放同一批。
func Queue(src Source, f assertion.Filter, limit int, sampleRatio float64, seed int64) ([]QueueItem, error) {
	as, err := src.Select(f)
	if err != nil {
		return nil, err
	}
	if len(as) == 0 {
		return nil, nil
	}
	refs, err := src.ReferenceCounts()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(as))
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	sampling := verification.Sample(ids, sampleRatio, rand.New(rand.NewSource(seed)))

	byID := make(map[string]assertion.Assertion, len(as))
	ranked := make([]verification.Ranked, 0, len(as))
	for _, a := range as {
		byID[a.ID] = a
		ranked = append(ranked, verification.Ranked{
			ID: a.ID,
			Priority: verification.Rank(verification.PriorityInput{
				Disputed:   a.Status == assertion.StatusDisputed,
				References: refs[a.ID],
				Confidence: string(a.Confidence),
				Sampled:    sampling[a.ID],
			}),
		})
	}

	ordered := verification.Order(ranked, limit)
	out := make([]QueueItem, 0, len(ordered))
	for _, r := range ordered {
		out = append(out, QueueItem{Assertion: byID[r.ID], Priority: r.Priority})
	}
	return out, nil
}

// ConflictGroup 是一组互相冲突的断言：同一身份而有不同取值。
type ConflictGroup struct {
	Subject   string
	Predicate string
	Claims    []assertion.Assertion
}

// Conflicts 返回冲突分组。
//
// **系统不裁决**——它只负责把同一件事的所有说法摆在一起。
func Conflicts(src Source) ([]ConflictGroup, error) {
	raw, err := src.Conflicts()
	if err != nil {
		return nil, err
	}
	out := make([]ConflictGroup, 0, len(raw))
	for _, g := range raw {
		if len(g) == 0 {
			continue
		}
		out = append(out, ConflictGroup{
			Subject:   g[0].Subject,
			Predicate: g[0].Predicate,
			Claims:    g,
		})
	}
	return out, nil
}

// BatchInput 是一次批量核验的输入。
type BatchInput struct {
	Filter   assertion.Filter
	Decision verification.Decision
	Method   verification.Method
	By       string // 批准者，必须是人
	Reason   string
	Evidence string
	Proposed string // 提出者（agent）
}

// BatchResult 是批量核验的结果。
type BatchResult struct {
	Matched  int
	Applied  int
	Failed   int
	FirstErr error
}

// Preview 返回将要被处理的断言，供 dry-run 展示。
func Preview(src Source, f assertion.Filter) ([]assertion.Assertion, error) {
	if f.Status == "" {
		// 只处理待核验的，避免重复核验已处理项
		f.Status = string(assertion.StatusPending)
	}
	return src.Select(f)
}

// Batch 执行批量核验。
//
// **每条断言各自留下核验记录**，审计轨迹不合并——
// 一条记录代表一个人的一次判断，批量不等于免责。
func Batch(src Source, w Writer, in BatchInput, now time.Time) (BatchResult, error) {
	f := in.Filter
	if f.Status == "" {
		f.Status = string(assertion.StatusPending)
	}

	as, err := src.Select(f)
	if err != nil {
		return BatchResult{}, err
	}
	res := BatchResult{Matched: len(as)}

	if in.By == "" {
		return res, fmt.Errorf("必须指明批准者（必须是人）——无追责的核验等于没有核验")
	}
	if in.Reason == "" {
		return res, fmt.Errorf("必须说明理由——批量核验同样需要理由")
	}
	if !in.Decision.Valid() {
		return res, fmt.Errorf("未知的核验结论 %q", in.Decision)
	}

	for _, a := range as {
		err := w.Verify(verification.Record{
			AssertionID: a.ID,
			Decision:    in.Decision,
			Method:      in.Method,
			ProposedBy:  verification.Actor{Kind: verification.Agent, ID: in.Proposed},
			ApprovedBy:  &verification.Actor{Kind: verification.Human, ID: in.By},
			Reason:      in.Reason,
			Evidence:    in.Evidence,
			At:          now,
		})
		if err != nil {
			res.Failed++
			if res.FirstErr == nil {
				res.FirstErr = fmt.Errorf("%s: %w", a.ID, err)
			}
			continue
		}
		res.Applied++
	}
	return res, nil
}
