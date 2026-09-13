// 核验优先级的计算规则。
//
// 见 docs/specs/verification.spec.md「人力的分配」一节：
// 待核验条目多于人力时，按**影响面**（被引用次数）、断言类型、是否争议、变更频率排序，
// 并**必须包含随机抽检**——否则人会只核验「显眼」的部分，系统性错误永远发现不了。
//
// 本文件只放纯规则：输入若干事实，输出一个分数与理由。
// 数据的获取（引用次数、争议状态）由调用方负责，因此这里可以完整测试。
package verification

import (
	"fmt"
	"math/rand"
	"sort"
)

// Tier 是优先级来源的类别。它让队列能解释「为什么这条排在前面」。
type Tier string

const (
	// TierDisputed 有争议：最高优先级——冲突不解决，下游无法判断该信谁。
	TierDisputed Tier = "争议"
	// TierImpact 影响面：被引用的次数多，错了波及面大。
	TierImpact Tier = "影响面"
	// TierUncertain 不确定性：分级的推断成分越高越需要核验。
	TierUncertain Tier = "分级"
	// TierSample 随机抽检：与分数无关，强制进入队列。
	TierSample Tier = "抽检"
	// TierRoutine 常规。
	TierRoutine Tier = "常规"
)

// PriorityInput 是计算优先级所需的全部事实。
type PriorityInput struct {
	// Disputed 该断言是否处于争议状态。
	Disputed bool
	// References 有多少条派生断言以它为输入。
	References int
	// Confidence 断言分级。
	Confidence string
	// Sampled 该条是否被选中作为强制抽检项。
	Sampled bool
}

// Priority 是一条断言的核验优先级。
type Priority struct {
	Score   int
	Tier    Tier
	Reason  string
	Sampled bool // 是否为强制抽检项——截断时必须保住
}

// 分数权重。刻意用大间隔，保证「争议」永远压过「影响面」，
// 「影响面」永远压过「分级」——类别之间的顺序不该被数量翻转。
const (
	weightDisputed  = 100000
	weightPerRef    = 1000
	weightSample    = 100
	weightUncertain = 10
)

// uncertainWeight 把分级映射为不确定性权重：推断成分越高越需要核验。
//
// 注意方向：**L4 推断最需要核验**，L1 直引虽然同样未核验，
// 但它可与原文逐字比对，风险最低。
func uncertainWeight(confidence string) int {
	switch confidence {
	case "L4":
		return 4
	case "L3":
		return 3
	case "L2":
		return 2
	case "L1":
		return 1
	}
	return 0
}

// Rank 计算一条断言的核验优先级。
//
// Reason 只保留**主导理由**（决定 Tier 的那一条）——界面上它要和断言并列显示，
// 把四条理由全塞进去就没人看了。抽检另由 Sampled 标记。
func Rank(in PriorityInput) Priority {
	score := 0
	tier := TierRoutine
	reason := "常规核验"

	if in.Disputed {
		score += weightDisputed
		tier = TierDisputed
		reason = "存在争议——冲突不解决则下游无法判断该信谁"
	}
	if in.References > 0 {
		score += in.References * weightPerRef
		if tier == TierRoutine {
			tier = TierImpact
			reason = fmt.Sprintf("被 %d 条派生断言引用", in.References)
		}
	}
	if w := uncertainWeight(in.Confidence); w > 0 {
		score += w * weightUncertain
		if tier == TierRoutine {
			tier = TierUncertain
			reason = fmt.Sprintf("分级 %s（推断成分越高越需要核验）", in.Confidence)
		}
	}
	if in.Sampled {
		score += weightSample
		if tier == TierRoutine {
			tier = TierSample
			reason = "随机抽检项"
		}
	}
	return Priority{Score: score, Tier: tier, Reason: reason, Sampled: in.Sampled}
}

// Ranked 是一条带优先级的断言 ID。
type Ranked struct {
	ID       string
	Priority Priority
}

func byScore(items []Ranked) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority.Score != items[j].Priority.Score {
			return items[i].Priority.Score > items[j].Priority.Score
		}
		return items[i].ID < items[j].ID
	})
}

// Order 按优先级排序并截断，**保证抽检项不被挤掉**。
//
// 如果只按分数截断，抽检项几乎必然被高分项挤出队列——
// 那样抽检就等于没做，系统性问题仍然发现不了。
func Order(items []Ranked, limit int) []Ranked {
	byScore(items)
	if limit <= 0 || len(items) <= limit {
		return items
	}

	kept := make([]Ranked, 0, limit)
	for _, it := range items {
		if it.Priority.Sampled && len(kept) < limit {
			kept = append(kept, it)
		}
	}
	for _, it := range items {
		if len(kept) >= limit {
			break
		}
		if it.Priority.Sampled {
			continue
		}
		kept = append(kept, it)
	}
	byScore(kept)
	return kept
}

// Sample 从候选 ID 中随机抽取 ratio 比例（至少 1 个，候选非空时）。
//
// 用调用方传入的随机源，使抽检可复现——「随机」不等于「不可复现」，
// 出问题时必须能重放同一批。
func Sample(ids []string, ratio float64, rnd *rand.Rand) map[string]bool {
	out := map[string]bool{}
	if len(ids) == 0 || ratio <= 0 {
		return out
	}
	n := int(float64(len(ids)) * ratio)
	if n < 1 {
		n = 1
	}
	if n > len(ids) {
		n = len(ids)
	}
	perm := rnd.Perm(len(ids))
	for i := 0; i < n; i++ {
		out[ids[perm[i]]] = true
	}
	return out
}
