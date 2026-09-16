// Package vault 是 vault 的领域规则：文档、双链、状态，以及「谁能改状态」。
//
// vault 是一个项目的文档库（见 docs/specs/vault.spec.md）：
// 两层内容（raw 原始层 / docs 整理层）+ 数据表，人写的 markdown 是主产物。
//
// 本包**只允许 Go 标准库**。文件系统与解析细节在 infrastructure/vaultfs，
// 用例编排在 application/vaultapp——依赖方向 api → application → domain ← infrastructure。
package vault

import (
	"fmt"
	"strings"
)

// Layer 是文档所在的两层之一。
type Layer string

const (
	// LayerDocs 是整理层：人和 agent 都可编辑，表达「我们认定的说法」。
	LayerDocs Layer = "docs"
	// LayerRaw 是原始层：抓来什么样就什么样，只做原样留存。
	LayerRaw Layer = "raw"
)

// Status 是文档的发布态，照 wiki 的发布态（见 vault.spec.md §2）。
type Status string

const (
	// StatusDraft 是未核验——默认一切未核验，但必须可见。
	StatusDraft Status = "draft"
	// StatusPublished 是已发布。
	StatusPublished Status = "published"
	// StatusArchived 是已归档。
	StatusArchived Status = "archived"
)

// ParseStatus 解析发布态，拒绝不认识的值。
//
// 拒绝而不是退回默认值：把打错的 status 悄悄当成 draft，会让人以为
// 「我明明标了已发布」，而这正是最不该出错的地方。
func ParseStatus(s string) (Status, error) {
	switch Status(strings.TrimSpace(s)) {
	case StatusDraft:
		return StatusDraft, nil
	case StatusPublished:
		return StatusPublished, nil
	case StatusArchived:
		return StatusArchived, nil
	case "":
		return "", fmt.Errorf("status 为空")
	default:
		return "", fmt.Errorf("不认识的 status %q（只认 draft / published / archived）", s)
	}
}

// Doc 是一篇文档：front matter 已解析，正文原样保留。
type Doc struct {
	// Path 是相对 vault 根的路径，永远用 / 分隔（如 docs/式神/茨木童子.md）。
	Path string
	// Layer 由 Path 的首段决定：docs / raw。
	Layer Layer
	// Title 取自 front matter；缺省时用文件名（保证界面上永远有可读的标识）。
	Title string
	// Tags 取自 front matter。
	Tags []string
	// Status 取自 front matter；缺省视为 draft（未核验是默认状态）。
	Status Status
	// Source 是 front matter 里指向原始层的双链（可空）。
	Source string
	// Body 是正文（不含 front matter）。
	Body string
	// BodyOffset 是正文第一行在**文件里**的行号（1 起）。
	// 锚点行号要靠它换算成文件行号，否则人拿这个行号去编辑器里跳会跳错——
	// front matter 占几行，正文内行号就偏几行。
	BodyOffset int
	// Links 是正文里解析出来的双链。
	Links []Link
}

// LayerOf 从路径判断它属于哪一层；既不是 docs 也不是 raw 时返回空。
func LayerOf(rel string) Layer {
	switch strings.SplitN(strings.TrimPrefix(rel, "/"), "/", 2)[0] {
	case string(LayerDocs):
		return LayerDocs
	case string(LayerRaw):
		return LayerRaw
	default:
		return ""
	}
}

// ActorKind 是人还是 agent。
type ActorKind string

const (
	ActorHuman ActorKind = "human"
	ActorAgent ActorKind = "agent"
)

// Actor 是操作者。每次写操作都必须带上它——见 agent.spec.md §1。
type Actor struct {
	Kind ActorKind
	Name string
}

// ParseActor 解析 `human:名字` / `agent:名字`；只写 `human` / `agent` 也接受（名字留空）。
func ParseActor(s string) (Actor, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Actor{}, fmt.Errorf("必须指明操作者（human:名字 或 agent:名字）")
	}
	kind, name, _ := strings.Cut(s, ":")
	switch ActorKind(strings.TrimSpace(kind)) {
	case ActorHuman:
		return Actor{Kind: ActorHuman, Name: strings.TrimSpace(name)}, nil
	case ActorAgent:
		return Actor{Kind: ActorAgent, Name: strings.TrimSpace(name)}, nil
	default:
		return Actor{}, fmt.Errorf("不认识的操作者 %q（只认 human / agent）", s)
	}
}

// Trailer 返回写进 git commit 的 trailer 值（见 agent.spec.md §3）。
func (a Actor) Trailer() string { return string(a.Kind) + ":" + a.Name }

// CanChangeStatus 报告这个操作者能不能改发布态。
//
// **只有人能改。** 这条故意落在能力层而不是 agent 后端里：
// 后端是可替换的，门放在后端，换一个后端就绕过去了（见 agent.spec.md §1）。
func (a Actor) CanChangeStatus() bool { return a.Kind == ActorHuman }

// StatusAfterEdit 返回被该操作者改动之后，文档应有的发布态。
//
// agent 改过一律回落 draft：否则 agent 能绕开人改掉已发布的内容（agent.spec.md §2）。
func (a Actor) StatusAfterEdit(current Status) Status {
	if a.Kind == ActorAgent {
		return StatusDraft
	}
	return current
}
