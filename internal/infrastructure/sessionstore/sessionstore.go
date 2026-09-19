// Package sessionstore 把**会话的元数据**存在 vault 里：`<vault>/.ssot/sessions.json`。
//
// 为什么不让 DSH 当唯一来源（踩过的坑）：
// ACP 的 `session/list` 只回 id 与 cwd，**不回标题**；标题本来是去读 DSH 的私有存储
// （`storages/session_projcache/sessions/<id>.json` 的 `record.rows.title.val`）捡来的——
// 结果一次「删会话」把它们连锅端了：会话日志还在（还能被列出来），标题却没了，列表里一堆「没有标题」。
//
// 所以改成：**我们自己存**（这个包），DSH 那份只当老会话的种子。
//
// 存什么：id → { title, createdAt, lastUsedAt }。标题的来源可以是
//   - 机械：第一句用户消息（截断）；
//   - **agent 生成的**（将来：一轮结束后台生成一句更准的，写进同一个字段）。
//
// 谁写：能力层（agentapp）——「起会话 / 切会话 / 发第一句」时写，界面只负责显示 Go 返回的东西。
//
// 边界：这是**便利信息**，不是事实。文件坏了/读不到就当没有（返回空表），绝不让它把会话列表搞崩。
package sessionstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry 是一个会话的元数据。
type Entry struct {
	Title      string `json:"title,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	// TitleFrom 记录标题是谁起的："" / "first-message" / "agent"。将来换标题时能看出处。
	TitleFrom string `json:"titleFrom,omitempty"`
}

// file 是整个存储的形状（带版本，将来好改）。
type file struct {
	Version  int              `json:"version"`
	Sessions map[string]Entry `json:"sessions"`
}

// Path 是存储的位置。
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".ssot", "sessions.json")
}

// Load 读会话元数据。文件不存在/坏了都返回空表（不报错，见包注释的边界）。
func Load(vaultRoot string) map[string]Entry {
	out := map[string]Entry{}
	b, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		return out
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return out
	}
	for id, e := range f.Sessions {
		out[id] = e
	}
	return out
}

// Save 写回（按键排序，diff 稳定）。
func Save(vaultRoot string, entries map[string]Entry) error {
	f := file{Version: 1, Sessions: entries}
	if f.Sessions == nil {
		f.Sessions = map[string]Entry{}
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(Path(vaultRoot))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(Path(vaultRoot), b, 0o644)
}

// Touch 记一次「用到这个会话」（起 / 切 / 发都会调）。
func Touch(vaultRoot, id string) error {
	if id == "" {
		return nil
	}
	all := Load(vaultRoot)
	e := all[id]
	now := time.Now().Format(time.RFC3339)
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	e.LastUsedAt = now
	all[id] = e
	return Save(vaultRoot, all)
}

// SetTitle 设标题（第一句 / agent 生成都走这里）。
//
// force=false 时**只补空标题**（第一次记下就算，后面那句不该把名字改掉）；
// force=true 时覆盖（agent 生成更好的标题用这条）。
func SetTitle(vaultRoot, id, title, from string, force bool) error {
	if id == "" || strings.TrimSpace(title) == "" {
		return nil
	}
	all := Load(vaultRoot)
	e := all[id]
	if e.Title != "" && !force {
		return nil
	}
	e.Title = clean(title, 40)
	e.TitleFrom = from
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().Format(time.RFC3339)
	}
	all[id] = e
	return Save(vaultRoot, all)
}

// Delete 删掉**一个**会话的条目（会话文件被删了，条目别留着）。
//
// 与 Prune 的分工：Prune 是「按一份名单批量清」，Delete 是「就删这一个」——
// 测试留下的会话、用户手删的会话都走这条，不必先拼一份 keep 名单再绕一圈
// （keep 为空又表示「什么都不删」，凑起来容易写错）。
func Delete(vaultRoot, id string) error {
	if id == "" {
		return nil
	}
	all := Load(vaultRoot)
	if _, ok := all[id]; !ok {
		return nil
	}
	delete(all, id)
	return Save(vaultRoot, all)
}

// Prune 把**不在 keep 里**的会话条目删掉（会话被删了，条目别留着）。
// keep 为空表示不删任何东西（免得手滑把整份清空）。
func Prune(vaultRoot string, keep map[string]bool) error {
	if len(keep) == 0 {
		return nil
	}
	all := Load(vaultRoot)
	changed := false
	for id := range all {
		if !keep[id] {
			delete(all, id)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return Save(vaultRoot, all)
}

// Titles 只要标题（给 agentapp.Sessions 用）。
func Titles(vaultRoot string) map[string]string {
	out := map[string]string{}
	for id, e := range Load(vaultRoot) {
		if e.Title != "" {
			out[id] = e.Title
		}
	}
	return out
}

// SortIDs 按「最近用到」排 id（新的在前）——列表要稳定，所以同时用 id 兜底排序。
func SortIDs(entries map[string]Entry) []string {
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := entries[ids[i]].LastUsedAt, entries[ids[j]].LastUsedAt
		if a != b {
			return a > b
		}
		return ids[i] < ids[j]
	})
	return ids
}

// clean 压成一行并截断。
func clean(s string, max int) string {
	one := strings.Join(strings.Fields(s), " ")
	r := []rune(one)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return one
}
