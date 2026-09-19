// 删除：`ssot vault rm <路径|glob...> [-actor …] [-dry]`。
//
// 删就是删——没有隔离区、没有审批队列。`-dry` 只是先算一份影响（要删几个文件、派生层要清多少行、
// 会造成哪些断链），不动物。参数支持 glob（`raw/剧情/**`），因为「清一层」才是实际用法。
package main

import (
	"fmt"
	"strings"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

func vaultRemove(svc *vaultapp.Service, args []string, actorStr string, dry bool) error {
	if len(args) == 0 {
		return fmt.Errorf("rm 后面要跟至少一个路径或 glob（可以给多个）")
	}
	docs, patterns, err := svc.ExpandArgs(args)
	if err != nil {
		return err
	}
	if patterns > 0 {
		fmt.Printf("展开：%d 个参数（含 %d 个 glob）→ %d 篇文档\n", len(args), patterns, len(docs))
	}

	if dry {
		totalRows, totalBroken, n := 0, 0, 0
		for _, p := range docs {
			r, err := svc.WouldRemove(p)
			if err != nil {
				return err
			}
			totalRows += r.DerivedRows
			totalBroken += len(r.BrokenLinks)
			n++
		}
		fmt.Printf("将删除 %d 篇文档；派生层要清 %d 行；会造成 %d 条断链\n", n, totalRows, totalBroken)
		fmt.Println("（-dry 只报影响，什么都没删）")
		return nil
	}

	if actorStr == "" {
		return fmt.Errorf("删除必须给 -actor（human:名字 或 agent:名字）——谁删的要留痕")
	}
	actor, err := vault.ParseActor(actorStr)
	if err != nil {
		return err
	}
	results, err := svc.RemoveMany(docs, actor)
	rows, committed := 0, 0
	for _, r := range results {
		rows += r.DerivedRows
		if r.Change.Committed {
			committed++
		}
		if len(r.BrokenLinks) > 0 {
			fmt.Printf("  ⚠️ %s 被删了，这些文档还链着它（现在是断链）：%s\n",
				r.Path, strings.Join(r.BrokenLinks, "、"))
		}
	}
	fmt.Printf("已删除 %d 篇，清掉派生行 %d，其中 %d 篇已在 git 留痕\n", len(results), rows, committed)
	return err
}
