// 删除：`ssot vault rm <路径...> [-actor …] [-dry]`。
//
// 删就是删——没有隔离区、没有审批队列。`-dry` 只是先算一份影响（文件在不在、派生层要清多少行、
// 会造成哪些断链），不动物。
package main

import (
	"fmt"
	"strings"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

func vaultRemove(svc *vaultapp.Service, args []string, actorStr string, dry bool) error {
	if len(args) == 0 {
		return fmt.Errorf("rm 后面要跟至少一个路径（可以给多个）")
	}
	if dry {
		for _, p := range args {
			r, err := svc.WouldRemove(p)
			if err != nil {
				return err
			}
			fmt.Printf("将删除 %s：派生层要清 %d 行", r.Path, r.DerivedRows)
			if len(r.BrokenLinks) > 0 {
				fmt.Printf("；会造成 %d 条断链：%s", len(r.BrokenLinks), strings.Join(r.BrokenLinks, "、"))
			}
			fmt.Println()
		}
		fmt.Println("（-dry 只报影响，什么都没删）")
		return nil
	}

	actor, err := vault.ParseActor(actorStr)
	if err != nil {
		return err
	}
	if actorStr == "" {
		return fmt.Errorf("删除必须给 -actor（human:名字 或 agent:名字）——谁删的要留痕")
	}
	results, err := svc.RemoveMany(args, actor)
	for _, r := range results {
		note := ""
		if r.Change.Committed {
			note = "，已留痕 " + short7(r.Change.CommitSHA)
		} else if r.Change.VersionNote != "" {
			note = "（" + r.Change.VersionNote + "）"
		}
		fmt.Printf("已删除 %s：清掉派生行 %d%s\n", r.Path, r.DerivedRows, note)
		if len(r.BrokenLinks) > 0 {
			fmt.Printf("  ⚠️ 这些文档还链着它，现在是断链：%s\n", strings.Join(r.BrokenLinks, "、"))
		}
	}
	return err
}

func short7(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
