// 管理视图与回滚：`vault stat`（看体量与派生占用）与 `vault restore`（从 git 历史恢复）。
package main

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

func vaultStat(svc *vaultapp.Service) error {
	groups, err := svc.GroupStats()
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		fmt.Println("vault 里还没有文档")
		return nil
	}
	fmt.Printf("%-24s %6s %10s %8s %8s %8s %8s %8s\n",
		"目录", "文档", "字符", "嵌入块", "抽取块", "实体行", "关系行", "被引用")
	for _, g := range groups {
		fmt.Printf("%-24s %6d %10d %8d %8d %8d %8d %8d\n",
			g.Dir, g.Docs, g.Chars, g.Chunks, g.Extracts, g.Entities, g.Relations, g.Inbound)
	}
	fmt.Println("\n（字符数大、被引用为 0 的层，通常是可清掉的原始材料；判断写进 .ssot/derived-scope.yml）")
	return nil
}

func vaultRestore(svc *vaultapp.Service, args []string, actorStr string) error {
	if len(args) == 0 {
		return fmt.Errorf("restore 后面要跟路径（可多个）")
	}
	actor, err := vault.ParseActor(actorStr)
	if err != nil {
		return err
	}
	for _, p := range args {
		change, err := svc.Restore(p, actor)
		if err != nil {
			return fmt.Errorf("恢复 %s 失败：%w", p, err)
		}
		note := ""
		if change.Committed {
			note = "，已留痕 " + short7(change.CommitSHA)
		} else if change.VersionNote != "" {
			note = "（" + change.VersionNote + "）"
		}
		fmt.Printf("已恢复 %s%s\n", change.Path, note)
	}
	fmt.Println("派生层会在下次 index/extract 时把它补回来")
	return nil
}

// short7 取提交号前 7 位（给人看的）。
func short7(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
