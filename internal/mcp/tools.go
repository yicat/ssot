package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
)

// tool 是一个 MCP 工具：名字、说明、入参 schema，以及真正干活的那一下。
//
// run 只调用 application 层的方法——**不在这里做权限判断**，
// 门在 capability 层（docs/specs/agent.spec.md §1）。
type tool struct {
	name        string
	title       string
	description string
	readOnly    bool
	schema      map[string]any
	run         func(s *Server, a args) (any, error)
}

// args 是 tools/call 的 arguments，按需取字段。
type args map[string]json.RawMessage

func (a args) str(name string) (string, error) {
	var s string
	raw, ok := a[name]
	if !ok {
		return "", fmt.Errorf("缺参数 %q", name)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("参数 %q 要是字符串：%v", name, err)
	}
	return s, nil
}

// num 取可选的整数参数，缺省用 def。
func (a args) num(name string, def int) (int, error) {
	raw, ok := a[name]
	if !ok {
		return def, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("参数 %q 要是整数：%v", name, err)
	}
	return n, nil
}

// ── 入参 schema 的小工具：手写 JSON Schema 字面量太吵，包一层 ──────────────

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str2(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func int2(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

// tools 是第一批工具表。顺序固定（模型看到的列表就固定，便于对照）。
var tools = []tool{
	{
		name: "vault_list", title: "列文档与数据表", readOnly: true,
		description: "列出 vault 里的文档（路径、层、标题、状态、链接数）与数据表文件名。做任何事之前先看这个，别猜路径。",
		schema:      obj(map[string]any{}),
		run: func(s *Server, _ args) (any, error) {
			items, err := s.svc.List()
			if err != nil {
				return nil, err
			}
			tables, err := s.svc.Tables()
			if err != nil {
				return nil, err
			}
			docs := make([]map[string]any, 0, len(items))
			for _, it := range items {
				docs = append(docs, map[string]any{
					"path": it.Path, "layer": string(it.Layer), "title": it.Title,
					"status": string(it.Status), "links": it.Links,
				})
			}
			return map[string]any{"root": s.svc.Root(), "documents": docs, "tables": tables}, nil
		},
	},
	{
		name: "doc_read", title: "读一篇文档", readOnly: true,
		description: "读一篇文档：正文、front matter 解析出来的字段、以及正文里的双链。path 可以只写文件名（[[茨木童子]] 那种写法也认）。",
		schema:      obj(map[string]any{"path": str2("文档路径（相对 vault 根，如 docs/子目录/标题.md），或只写标题/文件名")}, "path"),
		run: func(s *Server, a args) (any, error) {
			p, err := a.str("path")
			if err != nil {
				return nil, err
			}
			doc, err := s.svc.Read(p)
			if err != nil {
				return nil, err
			}
			return docOut(doc), nil
		},
	},
	{
		name: "doc_write", title: "写文档正文",
		description: "写入文档正文（front matter 原样保留）。文档不存在时落在 docs/<名字>.md。写完全部回落为 draft，并在 vault 的 git 里留痕（无变化则不产生提交）。",
		schema: obj(map[string]any{
			"path": str2("文档路径（相对 vault 根，如 docs/子目录/标题.md）"),
			"body": str2("完整正文（不含 front matter）"),
		}, "path", "body"),
		run: func(s *Server, a args) (any, error) {
			p, err := a.str("path")
			if err != nil {
				return nil, err
			}
			body, err := a.str("body")
			if err != nil {
				return nil, err
			}
			ch, err := s.svc.Write(p, body, s.actor)
			if err != nil {
				return nil, err
			}
			return changeOut(ch), nil
		},
	},
	{
		name: "doc_delete", title: "删除文档（连带清派生层）",
		description: "删除一篇文档，并清掉派生层里属于它的块/向量/实体/关系。" +
			"path 支持 glob（例如 raw/子目录/**）用于批量删除；先给 dry=1 只报影响（删几篇、清几行、会造成几条断链），" +
			"确认后再真删。删完会列出还链着它的文档（断链）。删除会在 vault 的 git 里留痕。",
		schema: obj(map[string]any{
			"path": str2("文档路径或 glob（相对 vault 根）"),
			"dry":  int2("1 = 只报影响不删，0 = 真删（默认 0）"),
		}, "path"),
		run: func(s *Server, a args) (any, error) {
			p, err := a.str("path")
			if err != nil {
				return nil, err
			}
			dry, err := a.num("dry", 0)
			if err != nil {
				return nil, err
			}
			docs, patterns, err := s.svc.ExpandArgs([]string{p})
			if err != nil {
				return nil, err
			}
			if dry != 0 {
				rows, broken, n := 0, 0, 0
				for _, d := range docs {
					r, err := s.svc.WouldRemove(d)
					if err != nil {
						return nil, err
					}
					rows += r.DerivedRows
					broken += len(r.BrokenLinks)
					n++
				}
				return map[string]any{
					"dry": true, "matched": n, "patterns": patterns,
					"derived_rows": rows, "broken_links": broken,
					"note": "什么都没删；确认后再用 dry=0 调一次",
				}, nil
			}
			results, err := s.svc.RemoveMany(docs, s.actor)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(results))
			rows, committed := 0, 0
			for _, r := range results {
				rows += r.DerivedRows
				if r.Change.Committed {
					committed++
				}
				out = append(out, map[string]any{
					"path": r.Path, "derived_rows": r.DerivedRows,
					"broken_links": r.BrokenLinks,
					"committed":    r.Change.Committed, "commit": r.Change.CommitSHA,
					"version_note": r.Change.VersionNote,
				})
			}
			return map[string]any{
				"deleted": len(results), "derived_rows": rows, "committed": committed,
				"items": out,
			}, nil
		},
	}, {
		name: "vault_search", title: "全文检索", readOnly: true,
		description: "在文档标题与正文里检索，返回命中的文档与附近原文。找东西先用它，再 doc_read 看全篇。",
		schema: obj(map[string]any{
			"query": str2("检索词"),
			"limit": int2("最多返回几条，默认 10"),
		}, "query"),
		run: func(s *Server, a args) (any, error) {
			q, err := a.str("query")
			if err != nil {
				return nil, err
			}
			limit, err := a.num("limit", 10)
			if err != nil {
				return nil, err
			}
			hits, err := s.svc.Search(q, limit)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				out = append(out, map[string]any{
					"path": h.Path, "layer": string(h.Layer), "title": h.Title, "status": string(h.Status),
					"snippet": h.Snippet, "titleMatch": h.TitleMatch, "occurrences": h.Occurrences,
				})
			}
			return map[string]any{"query": q, "hits": out}, nil
		},
	},
	{
		name: "link_backlinks", title: "反链与问题链接", readOnly: true,
		description: "谁引用了这篇文档（反链），以及这篇文档链出去的**问题链接**（broken 断链 / ambiguous 同名指不清，分开报）。改稿前先看这个。",
		schema:      obj(map[string]any{"path": str2("文档路径")}, "path"),
		run: func(s *Server, a args) (any, error) {
			p, err := a.str("path")
			if err != nil {
				return nil, err
			}
			res, err := s.svc.Backlinks(p)
			if err != nil {
				return nil, err
			}
			back := make([]map[string]any, 0, len(res.Backlinks))
			for _, b := range res.Backlinks {
				back = append(back, map[string]any{
					"from": b.From, "raw": b.Link.Raw, "target": b.Link.Target,
					"heading": b.Link.Heading, "block": b.Link.Block, "alias": b.Link.Alias,
					"embed": b.Link.Embed,
				})
			}
			issues := make([]map[string]any, 0, len(res.Issues))
			for _, is := range res.Issues {
				issues = append(issues, map[string]any{
					"kind": string(is.Kind), "raw": is.Link.Raw, "target": is.Link.Target, "reason": is.Reason,
				})
			}
			return map[string]any{"target": res.Target, "backlinks": back, "issues": issues}, nil
		},
	},
	{
		name: "link_resolve", title: "解析双链", readOnly: true,
		description: "把一条 [[双链]]（含 #标题 与 #^块锚点）解析到具体文档与段落，返回**文件行号**。块锚点就是主张级溯源，引用原文时用它确认指向。",
		schema:      obj(map[string]any{"ref": str2("双链，方括号可省，如 [[raw/灰机wiki/茨木童子#^伤害系数]]")}, "ref"),
		run: func(s *Server, a args) (any, error) {
			ref, err := a.str("ref")
			if err != nil {
				return nil, err
			}
			res, err := s.svc.Resolve(ref)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"path": res.Path, "heading": res.Heading, "headingLine": res.HeadingLine,
				"block": res.Block, "blockLine": res.BlockLine, "blockText": res.BlockText,
				"candidates": res.Candidates,
			}, nil
		},
	},
	{
		name: "file_read", title: "只读地读 vault 里的文本文件", readOnly: true,
		description: "读 vault 里任意**文本**文件（原文、JSON 导出、project.yml、数据表说明…），可按行分页。只读：写文件不在这里——写文档走 doc_write，写表走专门的数据表工具。",
		schema: obj(map[string]any{
			"path":     str2("vault 内的相对路径，如 raw/_原始导出/Data_Character.json"),
			"fromLine": int2("从第几行开始（默认 1）"),
			"maxLines": int2("最多读几行（默认 400，上限 2000）"),
		}, "path"),
		run: func(s *Server, a args) (any, error) {
			p, err := a.str("path")
			if err != nil {
				return nil, err
			}
			from, err := a.num("fromLine", 1)
			if err != nil {
				return nil, err
			}
			max, err := a.num("maxLines", 400)
			if err != nil {
				return nil, err
			}
			fs, err := s.svc.ReadFile(p, from, max)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"path": fs.Path, "text": fs.Text,
				"fromLine": fs.FromLine, "toLine": fs.ToLine, "totalLines": fs.TotalLines,
				"truncated": fs.Truncated,
			}, nil
		},
	}, {
		name: "table_infos", title: "有哪些数据表", readOnly: true,
		description: "列出数据表：查询用的表名、文件路径、格式、行数与列名。要写 SQL 之前先看它。",
		schema:      obj(map[string]any{}),
		run: func(s *Server, _ args) (any, error) {
			infos, err := s.svc.TableInfos()
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(infos))
			for _, t := range infos {
				out = append(out, map[string]any{
					"name": t.Name, "file": t.File, "format": t.Format, "rows": t.Rows, "columns": t.Columns,
				})
			}
			return map[string]any{"tables": out}, nil
		},
	},
	{
		name: "table_query", title: "查询数据表", readOnly: true,
		description: "对派生索引跑一条**只读** SELECT/WITH：能查 tables/ 里的表，也能查 docs 表（path/title/status/tags/source 等 front matter 字段）。",
		schema: obj(map[string]any{
			"sql":   str2("只读 SQL，必须以 SELECT 或 WITH 开头"),
			"limit": int2("最多返回几行，默认 100"),
		}, "sql"),
		run: func(s *Server, a args) (any, error) {
			sql, err := a.str("sql")
			if err != nil {
				return nil, err
			}
			limit, err := a.num("limit", 100)
			if err != nil {
				return nil, err
			}
			rs, err := s.svc.QueryTables(sql, limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"columns": rs.Columns, "rows": rs.Rows}, nil
		},
	},
}

// toolList 是 tools/list 的结果。
func toolList() []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		// annotations 是标准 MCP 元数据（readOnlyHint 让客户端能据此放宽审批）。
		// ⚠️ 已核实：DSH 的 mcp-client 目前**不透传** annotations（lib/index.js 里没有），
		// 所以别指望它现在就能改变审批行为——写上是为了别的客户端与将来。
		out = append(out, map[string]any{
			"name": t.name, "title": t.title, "description": t.description,
			"inputSchema": t.schema,
			"annotations": map[string]any{"readOnlyHint": t.readOnly, "openWorldHint": false},
		})
	}
	return out
}

// callTool 执行一次 tools/call。
//
// 两种失败要分开（MCP 的约定）：
//   - 协议层错误（工具名不认识）→ JSON-RPC error；
//   - 工具跑起来失败（找不到文档、SQL 写错、门拒绝）→ result 里 isError=true，
//     让模型能读到原因并自己改，而不是看到一个协议级失败。
func (s *Server) callTool(req request, notification bool) (response, bool) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.fail(req, notification, codeInvalidParams, "tools/call 的参数解析失败："+err.Error())
	}
	var t *tool
	for i := range tools {
		if tools[i].name == p.Name {
			t = &tools[i]
			break
		}
	}
	if t == nil {
		return s.fail(req, notification, codeInvalidParams, fmt.Sprintf("没有这个工具：%q（可用工具见 tools/list）", p.Name))
	}
	a := args{}
	if len(p.Arguments) > 0 {
		if err := json.Unmarshal(p.Arguments, &a); err != nil {
			return s.fail(req, notification, codeInvalidParams, "arguments 要是对象："+err.Error())
		}
	}
	val, err := t.run(s, a)
	if err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Result: errResult(err)}, true
	}
	text, merr := json.MarshalIndent(val, "", "  ")
	if merr != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Result: errResult(merr)}, true
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(text)}},
	}}, true
}

func (s *Server) fail(req request, notification bool, code int, msg string) (response, bool) {
	if notification {
		return response{}, false
	}
	return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}}, true
}

func errResult(err error) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": "错误：" + err.Error()}},
		"isError": true,
	}
}

// ── 出参：显式列字段，别把领域结构体直接塞出去 ──────────────────────────
//
// 直接 marshal 领域类型会把 Go 的字段名（Path/Layer/…）漏给模型，改名就成破坏性改动；
// 这里逐个列出来，等于给 MCP 面留一份稳定的形状。

func docOut(d vault.Doc) map[string]any {
	links := make([]map[string]any, 0, len(d.Links))
	for _, l := range d.Links {
		links = append(links, map[string]any{
			"raw": l.Raw, "target": l.Target, "heading": l.Heading, "block": l.Block,
			"alias": l.Alias, "embed": l.Embed,
		})
	}
	return map[string]any{
		"path": d.Path, "layer": string(d.Layer), "title": d.Title, "status": string(d.Status),
		"tags": d.Tags, "source": d.Source, "body": d.Body,
		// 正文第一行在**文件**里的行号：块锚点行号是按它换算的。
		"bodyFirstLine": d.BodyOffset, "links": links,
	}
}

// changeOut 把一次写入的结果给出去。
//
// `committed` / `commitSha` / `versionNote` 要如实带上：agent 得知道
// 「这次改动有没有留痕」，不然它会以为自己的改动已经进了版本历史。
func changeOut(c vaultapp.Change) map[string]any {
	out := map[string]any{
		"path": c.Path, "from": string(c.From), "to": string(c.To), "actor": c.Actor.Trailer(),
		"committed": c.Committed, "commitSha": c.CommitSHA,
	}
	if c.VersionNote != "" {
		out["versionNote"] = c.VersionNote
	}
	return out
}
