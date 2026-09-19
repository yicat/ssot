// Package dshstore 只读地读 DSH 存下来的会话数据（标题、首句）。
//
// 为什么需要它：ACP 的 `session/list` **只回 sessionId 与 cwd，不回标题**——于是界面上
// 只能显示一串 hex（人看不懂）。而 DSH 自己其实把标题落在盘上了：
//
//	<DSH_HOME>/storages[/session_projcache]/<会话 id>.json
//	  { "record": { "rows": {
//	      "title":      { "val": "…" },                     ← DSH 自动起的标题
//	      "titleInput": { "val": { "first": { "text": "…" } } } ← 第一句用户消息
//	  } } }
//
// 所以标题**不用猜、也不用调模型**：读它就行（老会话也有）。
//
// ⚠️ 边界：这是**读别人的私有存储格式**（DSH 的实现细节），字段没了就该安静地退化成「没有标题」，
// 不许因此让会话列表报错——标题是便利信息，不是事实。将来 DSH 若在 ACP 里回 title，
// 这里可以直接删掉（调用方优先用后端给的）。
package dshstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Titles 读某个 DSH_HOME 下所有会话的标题：sessionId → 标题。
//
// 找不到目录、文件读不动、结构和预期不一样——一律跳过，不返回错误（见包注释的边界）。
func Titles(dshHome string) map[string]string {
	out := map[string]string{}
	if dshHome == "" {
		return out
	}
	for _, dir := range []string{
		filepath.Join(dshHome, "storages", "session_projcache"),
		filepath.Join(dshHome, "storages"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			id = strings.TrimPrefix(id, "session-")
			if _, ok := out[id]; ok && out[id] != "" {
				continue // 已经有了（projcache 那份更全）
			}
			if t := titleOf(filepath.Join(dir, e.Name())); t != "" {
				out[id] = t
			}
		}
	}
	return out
}

// titleOf 从一份会话记录里取标题：优先 DSH 起的 title，其次第一句用户消息。
func titleOf(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var doc struct {
		Record struct {
			Rows struct {
				Title struct {
					Val string `json:"val"`
				} `json:"title"`
				TitleInput struct {
					Val struct {
						First struct {
							Text string `json:"text"`
						} `json:"first"`
					} `json:"val"`
				} `json:"titleInput"`
			} `json:"rows"`
		} `json:"record"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return ""
	}
	if t := clean(doc.Record.Rows.Title.Val); t != "" {
		return t
	}
	return clean(doc.Record.Rows.TitleInput.Val.First.Text)
}

// clean 把标题压成一行并截断（界面上是一行下拉项，太长没意义）。
func clean(s string) string {
	one := strings.Join(strings.Fields(s), " ")
	r := []rune(one)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return one
}
