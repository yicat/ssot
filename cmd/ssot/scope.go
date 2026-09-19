// 收录范围的四个口子：show / propose / set / check（`set` 只有人能调）。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

func vaultScope(svc *vaultapp.Service, rest []string, f vaultFlags) error {
	action := ""
	if len(rest) > 0 {
		action = rest[0]
	}
	switch action {
	case "", "show":
		view, err := svc.Scope()
		if err != nil {
			return err
		}
		fmt.Println(view.Summary())
		if view.Effective.Exists {
			fmt.Printf("\n--- %s ---\n%s", view.Effective.Path, string(view.Effective.Raw))
		}
		if view.Proposal.Exists {
			fmt.Printf("\n--- 待确认的建议：%s ---\n%s", view.Proposal.Path, string(view.Proposal.Raw))
		}
		return nil

	case "propose":
		// 从标准输入的 JSON 读一份建议（agent 走 MCP 的 scope_propose，人也可以这么写）。
		var in struct {
			Reason  string `json:"reason"`
			Exclude struct {
				Paths []string `json:"paths"`
				Tags  []string `json:"tags"`
			} `json:"exclude"`
			Include struct {
				Paths []string `json:"paths"`
				Tags  []string `json:"tags"`
			} `json:"include"`
			EntityTypes        []string `json:"entity_types"`
			IgnoreNamePatterns []string `json:"ignore_name_patterns"`
			EmptyWords         []string `json:"empty_words"`
			Examples           []string `json:"examples"`
		}
		if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
			return fmt.Errorf("从标准输入读建议失败（要 JSON）：%w", err)
		}
		sc := vault.DerivedScope{
			Exclude: vault.ScopeRule{Paths: in.Exclude.Paths, Tags: in.Exclude.Tags},
			Include: vault.ScopeRule{Paths: in.Include.Paths, Tags: in.Include.Tags},
		}
		ex := vault.ExtractConfig{
			EntityTypes: in.EntityTypes, IgnoreNamePatterns: in.IgnoreNamePatterns,
			EmptyWords: in.EmptyWords, Examples: in.Examples,
		}
		p, err := svc.ProposeScope(sc, ex, in.Reason)
		if err != nil {
			return err
		}
		fmt.Printf("建议已写到 %s（**不生效**；人确认走 `ssot vault scope set -actor human:名字`）\n", p)
		return nil

	case "set":
		actor, err := vault.ParseActor(f.actor)
		if err != nil {
			return err
		}
		ch, err := svc.SetScopeFromProposal(actor)
		if err != nil {
			return err
		}
		fmt.Printf("收录范围已生效：%s", ch.Path)
		if ch.Committed {
			fmt.Printf("（已留痕 %s）", short7(ch.CommitSHA))
		} else if ch.VersionNote != "" {
			fmt.Printf("（%s）", ch.VersionNote)
		}
		fmt.Println()
		return nil

	case "check":
		res, err := svc.ScopeCheck()
		if err != nil {
			return err
		}
		fmt.Printf("文档 %d 篇，按声明该收 %d 篇\n", res.Total, res.InScope)
		fmt.Printf("越界（已在派生层但不该收）：%d 篇", len(res.OutOfScope))
		if len(res.OutOfScope) > 0 {
			fmt.Printf("——清理：ssot vault rm -dry '%s'", res.OutOfScope[0])
		}
		fmt.Println()
		fmt.Printf("该收没收（还没进派生层）：%d 篇", len(res.Missing))
		if len(res.Missing) > 0 {
			fmt.Printf("——例如 %s", res.Missing[0])
		}
		fmt.Println()
		return nil

	default:
		return fmt.Errorf("scope 的子命令只有 show / propose / set / check（收到 %q）", action)
	}
}
