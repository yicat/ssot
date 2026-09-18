// 抽取的 ACP 接线：把 appconfig 里的后端配置与抽取 overlay 拼成 ACPOptions。
//
// 放在 cmd 这一层（组合根）而不是 vextract 里：vextract 只认「命令 + 参数 + 环境」，
// 不知道配置从哪来（那是组合根的事）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/infrastructure/appconfig"
	"github.com/ngnl5/ssot/internal/infrastructure/vextract"
)

// vaultExtract 跑一批抽取：取块 → 多块合一次调用 → 打印产物与丢弃原因（可落盘）。
//
// ⚠️ 这一版**只验证、不入库**：入库要等 entity/relation 表与归并规则（plan §3），
// 触发方式也要等 OPEN.md #2 拍板。所以它按显式参数跑一批，结果打到屏幕/JSON。
func vaultExtract(svc *vaultapp.Service, f vaultFlags) error {
	batches, err := svc.ExtractChunksForDocs(f.docs, f.batch)
	if err != nil {
		return err
	}
	opt, err := extractionACP(svc.Root())
	if err != nil {
		return err
	}
	comp := vextract.NewACPCompleter(opt)
	defer comp.Close()

	var all vextract.Result
	start := time.Now()
	for i, b := range batches {
		chars := 0
		for _, c := range b {
			chars += len([]rune(c.Text))
		}
		t0 := time.Now()
		res, err := vextract.Extract(context.Background(), comp, b, vextract.Options{Gleaning: f.gleaning})
		if err != nil {
			return fmt.Errorf("第 %d/%d 批失败：%w", i+1, len(batches), err)
		}
		all.Entities = append(all.Entities, res.Entities...)
		all.Relations = append(all.Relations, res.Relations...)
		all.Dropped = append(all.Dropped, res.Dropped...)
		all.Rounds = res.Rounds
		fmt.Printf("批 %d/%d：%d 块（正文 %d 字）→ 实体 %d / 关系 %d / 丢弃 %d，用时 %.1f 秒\n",
			i+1, len(batches), len(b), chars, len(res.Entities), len(res.Relations), len(res.Dropped),
			time.Since(t0).Seconds())
		for _, d := range res.Dropped {
			fmt.Printf("    ⚠️ 丢掉 %s %s：%s\n", d.Kind, d.Name, d.Reason)
		}
	}

	elapsed := time.Since(start).Seconds()
	fmt.Printf("\n合计：%d 批 / %d 块，实体 %d 条 / 关系 %d 条 / 丢弃 %d 条，用时 %.1f 秒（后端 %s，补抽 %d 轮）\n",
		len(batches), sumChunks(batches), len(all.Entities), len(all.Relations), len(all.Dropped),
		elapsed, comp.Agent(), f.gleaning)
	if u := comp.LastUsage(); len(u) > 0 {
		fmt.Printf("最后一次 usage_update（原样）：%s\n", string(u))
	}

	if f.out != "" {
		payload := map[string]any{
			"batches": len(batches), "chunks": sumChunks(batches), "seconds": elapsed,
			"gleaning": f.gleaning, "backend": comp.Agent(),
			"entities": all.Entities, "relations": all.Relations, "dropped": all.Dropped,
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(f.out, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("产物写到 %s\n", f.out)
	}
	return nil
}

func sumChunks(batches [][]vextract.Chunk) int {
	n := 0
	for _, b := range batches {
		n += len(b)
	}
	return n
}

// extractionACP 按配置拼出抽取用的 ACP 选项。
//
// 关推理 + 收工具集靠 overlay（`.dsh/extraction.patch.yml`，见 spec §三.2）：
// 找不到 overlay 时会**大声提醒**再继续——因为「关推理」是省一半钱的硬约束，
// 悄悄按默认推理跑是最不该发生的事。
func extractionACP(vaultRoot string) (vextract.ACPOptions, error) {
	settings := appconfig.Defaults()
	if store, err := appconfig.Open(); err == nil {
		if loaded, _, err := store.Load(); err == nil {
			settings = loaded
		}
	}
	agent := settings.Agent
	if agent.DSHInstall == "" {
		return vextract.ACPOptions{}, fmt.Errorf("配置里没有 DSH 安装目录（agent 后端）")
	}
	abs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return vextract.ACPOptions{}, err
	}

	args := agent.Args()
	patch, ok := findExtractionPatch()
	if ok {
		// 放在 launcher 旗标那一段（`--profile X` 之后都还算 launcher 的旗标）。
		args = append(args, "--patch", patch)
	} else {
		fmt.Fprintln(os.Stderr,
			"⚠️ 没找到抽取 overlay（.dsh/extraction.patch.yml）：这一轮会**开着推理**跑，token 大约翻倍。"+
				"用 SSOT_EXTRACTION_PATCH 指定路径可消除这个警告。")
	}

	return vextract.ACPOptions{
		Command: agent.DSHExe(),
		Args:    args,
		Env:     agent.Env(),
		Dir:     abs,
		Timeout: 15 * time.Minute,
		OnLog:   func(s string) { fmt.Fprintln(os.Stderr, "[agent] "+s) },
		OnUsage: func(raw json.RawMessage) { fmt.Fprintln(os.Stderr, "[usage] "+string(raw)) },
	}, nil
}

// findExtractionPatch 找抽取 overlay：环境变量 → CLI 同目录的上层（仓库根）→ 当前目录。
func findExtractionPatch() (string, bool) {
	if p := os.Getenv("SSOT_EXTRACTION_PATCH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	var roots []string
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(filepath.Dir(exe))) // bin/ 的上层
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	for _, r := range roots {
		p := filepath.Join(r, ".dsh", "extraction.patch.yml")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}
