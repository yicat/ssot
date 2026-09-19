// 收录范围（scope）的用例：看声明、提议、检查一致性，以及**让抽取按声明办事**。
//
// 声明在 `<vault>/.ssot/derived-scope.yml`（进 vault 的 git，不进 SQLite）；
// 规则本身在 domain（`vault.DerivedScope` / `vault.ExtractConfig`），读写适配在 scopefile。
// 这一层只做编排与门（`set` 只认人）。
package vaultapp

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ngnl5/ssot/internal/infrastructure/scopefile"

	"github.com/ngnl5/ssot/internal/domain/vault"
)

// ScopeView 是当前声明（生效的 + 有没有待确认的建议）。
type ScopeView struct {
	Effective scopefile.Decl
	Proposal  scopefile.Decl
}

// Scope 读当前声明。
func (s *Service) Scope() (ScopeView, error) {
	eff, err := scopefile.Load(s.loader.Root)
	if err != nil {
		return ScopeView{}, err
	}
	prop, err := scopefile.LoadProposal(s.loader.Root)
	if err != nil {
		return ScopeView{}, err
	}
	return ScopeView{Effective: eff, Proposal: prop}, nil
}

// ProposeScope 写一份建议（agent 也能做；**建议不生效**）。
func (s *Service) ProposeScope(sc vault.DerivedScope, ex vault.ExtractConfig, reason string) (string, error) {
	if err := scopefile.SaveProposal(s.loader.Root, sc, ex, reason); err != nil {
		return "", err
	}
	return scopefile.ProposalPath(s.loader.Root), nil
}

// SetScope 把声明提升为生效。**门在这里：只有人能设**（与发布同一条规矩）。
func (s *Service) SetScope(sc vault.DerivedScope, ex vault.ExtractConfig, actor vault.Actor) (Change, error) {
	if actor.Kind != vault.ActorHuman {
		return Change{}, fmt.Errorf("只有人能改收录范围（agent 只能 scope propose）。收到 actor=%s", actor.Kind)
	}
	if err := scopefile.Save(s.loader.Root, sc, ex); err != nil {
		return Change{}, err
	}
	rel := scopefile.RelDir + "/" + scopefile.FileName
	change := Change{Path: rel, Actor: actor, Action: "scope"}
	s.version(&change)
	return change, nil
}

// SetScopeFromProposal 把建议原样提升为生效（人确认的常用路径）。
func (s *Service) SetScopeFromProposal(actor vault.Actor) (Change, error) {
	prop, err := scopefile.LoadProposal(s.loader.Root)
	if err != nil {
		return Change{}, err
	}
	if !prop.Exists {
		return Change{}, fmt.Errorf("还没有建议文件（先 scope propose）")
	}
	ch, err := s.SetScope(prop.Scope, prop.Extract, actor)
	if err != nil {
		return ch, err
	}
	// 建议被消费掉：留着会让 `scope show` 一直提示「有待确认的建议」。
	if err := os.Remove(scopefile.ProposalPath(s.loader.Root)); err != nil && !os.IsNotExist(err) {
		// 删不掉不算失败（生效已经写进去了），但要说出来。
		ch.VersionNote = strings.TrimSpace(ch.VersionNote + " 建议文件没删掉：" + err.Error())
	}
	return ch, nil
}

// ScopeCheck 是「声明 vs 派生层」的一致性检查。
type ScopeCheck struct {
	// OutOfScope 是**已经进了派生层、但按声明不该收**的文档（越界）。
	OutOfScope []string
	// Missing 是**该收但还没进派生层**的文档。
	Missing []string
	// Total 是文档总数；InScope 是按声明该收的篇数。
	Total   int
	InScope int
}

// ScopeCheck 报两类偏差：越界的、该收没收的。
//
// 「不静默」的落地：改了声明之后，旧数据不会自己消失——这里把它数出来、列出来，由人决定清还是留。
func (s *Service) ScopeCheck() (ScopeCheck, error) {
	view, err := s.Scope()
	if err != nil {
		return ScopeCheck{}, err
	}
	docs, err := s.loader.Load()
	if err != nil {
		return ScopeCheck{}, err
	}
	if err := s.ensureIndex(); err != nil {
		return ScopeCheck{}, err
	}
	indexed, err := s.index.IndexedDocs()
	if err != nil {
		return ScopeCheck{}, err
	}
	res := ScopeCheck{Total: len(docs)}
	for _, d := range docs {
		want, _ := view.Effective.Scope.Covers(d.Path, d.Tags)
		_, have := indexed[d.Path]
		if want {
			res.InScope++
			if !have {
				res.Missing = append(res.Missing, d.Path)
			}
			continue
		}
		if have {
			res.OutOfScope = append(res.OutOfScope, d.Path)
		}
	}
	sort.Strings(res.OutOfScope)
	sort.Strings(res.Missing)
	return res, nil
}

// ScopeSummary 是给人看的一行摘要（CLI 与 MCP 都用它）。
func (v ScopeView) Summary() string {
	if !v.Effective.Exists {
		return "没有声明文件 → 默认全收（类型词表为空，只有兜底 Other）"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s：排除路径 %v、排除标签 %v；include 例外 %v/%v；类型词表 %v",
		v.Effective.Path, v.Effective.Scope.Exclude.Paths, v.Effective.Scope.Exclude.Tags,
		v.Effective.Scope.Include.Paths, v.Effective.Scope.Include.Tags, v.Effective.Extract.EntityTypes)
	if v.Proposal.Exists {
		sb.WriteString("；⚠️ 有一份待确认的建议（scope set 才会生效）")
	}
	return sb.String()
}
