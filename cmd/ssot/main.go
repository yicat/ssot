// Command ssot 是 SSOT 工具的命令行入口。
//
// 用例层 internal/application/vaultapp 是对外能力的**唯一入口**：
// CLI、将来的 MCP server 与界面都走它，所以权限规则（谁能发布）只有一份实现。
//
// 约定见 docs/specs/vault.spec.md 与 docs/specs/agent.spec.md。
// 作废的旧方案（六部件 + 断言库 + 核验流程）在分支 legacy/mvp-v1 上备查。
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ngnl5/ssot/internal/application/vaultapp"
	"github.com/ngnl5/ssot/internal/domain/vault"
	"github.com/ngnl5/ssot/internal/mcp"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
	case "vault":
		if err := runVault(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "错误："+err.Error())
			os.Exit(1)
		}
	case "mcp":
		// MCP 服务端：stdout 是协议流，出错也只能往 stderr 说（见 internal/mcp 包注释）。
		if err := runMCP(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "错误："+err.Error())
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
}

// runMCP 跑 MCP 服务端（stdio）。
//
// 用法：ssot mcp -root <vault> [-actor agent:名字]
//
// 与 vault 子命令共用同一套选项解析（选项放前放后都行），但**没有 -h 之外的输出**：
// stdout 是协议流，任何提示语都会把流弄坏。
func runMCP(args []string) error {
	cmd, _, f, err := splitCommand(args)
	if err != nil {
		return err
	}
	if cmd != "" {
		return fmt.Errorf("mcp 不接受位置参数（收到 %q）；用法：ssot mcp -root <vault>，vault 子命令请走 ssot vault …", cmd)
	}
	if f.root == "" {
		return fmt.Errorf("必须用 -root 指明 vault 根目录")
	}
	if f.actor == "" {
		f.actor = "agent:dsh"
	}
	actor, err := vault.ParseActor(f.actor)
	if err != nil {
		return err
	}
	if actor.Kind != vault.ActorAgent {
		// MCP 这条路上没有 human：发布只能走界面或 CLI（docs/specs/dsh.spec.md §4）。
		return fmt.Errorf("mcp 的 actor 只能是 agent（收到 %q）——发布/归档请走界面或 ssot vault status", f.actor)
	}
	srv := mcp.New(vaultapp.New(f.root), actor)
	return srv.Serve(os.Stdin, os.Stdout)
}

func runVault(args []string) error {
	cmd, rest, f, err := splitCommand(args)
	if err != nil {
		return err
	}
	if f.root == "" {
		vaultUsage()
		return fmt.Errorf("必须用 -root 指明 vault 根目录")
	}
	svc := vaultapp.New(f.root)

	switch cmd {
	case "":
		vaultUsage()
		return fmt.Errorf("vault 后面要跟子命令")
	case "list":
		return vaultList(svc)
	case "read":
		return vaultRead(svc, rest)
	case "backlinks":
		return vaultBacklinks(svc, rest)
	case "resolve":
		return vaultResolve(svc, rest)
	case "status":
		return vaultStatus(svc, rest, f.actor)
	case "write":
		return vaultWrite(svc, rest, f.actor)
	case "index":
		return vaultIndex(svc)
	case "search":
		return vaultSearch(svc, rest, f.limit)
	case "tables":
		return vaultTables(svc)
	case "query":
		return vaultQuery(svc, rest, f.limit)
	default:
		vaultUsage()
		return fmt.Errorf("未知子命令 %q", cmd)
	}
}

// vaultFlags 是 vault 子命令共用的选项。
type vaultFlags struct {
	root  string
	actor string
	limit int
}

// splitCommand 手写解析：选项放在子命令**前后都行**。
//
// 用标准库的 flag 包做这件事会踩坑：它遇到第一个非选项参数就停止，
// 于是 `status -actor human:我 ...` 里的 -actor 会被当成位置参数——
// 而那恰好是用法说明里教人写的顺序。
func splitCommand(args []string) (cmd string, positional []string, f vaultFlags, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		takeValue := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 后面要跟一个值", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case a == "-h" || a == "--help":
			vaultUsage()
			return "", nil, f, nil
		case a == "-root" || a == "--root":
			if f.root, err = takeValue(a); err != nil {
				return "", nil, f, err
			}
		case strings.HasPrefix(a, "-root="):
			f.root = strings.TrimPrefix(a, "-root=")
		case a == "-actor" || a == "--actor":
			if f.actor, err = takeValue(a); err != nil {
				return "", nil, f, err
			}
		case strings.HasPrefix(a, "-actor="):
			f.actor = strings.TrimPrefix(a, "-actor=")
		case a == "-limit" || a == "--limit":
			v, verr := takeValue(a)
			if verr != nil {
				return "", nil, f, verr
			}
			n, cerr := strconv.Atoi(v)
			if cerr != nil || n <= 0 {
				return "", nil, f, fmt.Errorf("-limit 要一个正整数，收到 %q", v)
			}
			f.limit = n
		case strings.HasPrefix(a, "-limit="):
			n, cerr := strconv.Atoi(strings.TrimPrefix(a, "-limit="))
			if cerr != nil || n <= 0 {
				return "", nil, f, fmt.Errorf("-limit 要一个正整数")
			}
			f.limit = n
		case strings.HasPrefix(a, "-"):
			return "", nil, f, fmt.Errorf("不认识的选项 %q（只认 -root / -actor / -limit）", a)
		default:
			if cmd == "" {
				cmd = a
			} else {
				positional = append(positional, a)
			}
		}
	}
	return cmd, positional, f, nil
}

func vaultList(svc *vaultapp.Service) error {
	items, err := svc.List()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Printf("%s 下还没有文档（docs/ 与 raw/ 都是空的）\n", svc.Root())
	}
	for _, it := range items {
		// draft 单独标出来：未核验必须可见，这是这套东西的核心立场。
		mark := "·"
		if it.Status == vault.StatusDraft {
			mark = "!"
		}
		fmt.Printf("%s [%-9s] %-40s %s（%d 链）\n", mark, it.Status, it.Path, it.Title, it.Links)
	}
	tables, err := svc.Tables()
	if err != nil {
		return err
	}
	if len(tables) > 0 {
		fmt.Println("\n数据表：")
		for _, t := range tables {
			fmt.Println("  " + t)
		}
	}
	return nil
}

func vaultRead(svc *vaultapp.Service, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("read 后面要跟文档路径")
	}
	doc, err := svc.Read(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("路径：%s（%s 层）\n标题：%s\n状态：%s", doc.Path, doc.Layer, doc.Title, doc.Status)
	if len(doc.Tags) > 0 {
		fmt.Printf("\n标签：%s", strings.Join(doc.Tags, "、"))
	}
	if doc.Source != "" {
		fmt.Printf("\n来源：%s", doc.Source)
	}
	fmt.Printf("\n\n%s\n", strings.TrimRight(doc.Body, "\n"))
	if len(doc.Links) > 0 {
		fmt.Println("\n双链：")
		for _, l := range doc.Links {
			fmt.Println("  " + describeLink(l))
		}
	}
	return nil
}

func vaultBacklinks(svc *vaultapp.Service, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("backlinks 后面要跟文档路径")
	}
	res, err := svc.Backlinks(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s 的反链（%d 条）：\n", res.Target, len(res.Backlinks))
	for _, b := range res.Backlinks {
		fmt.Printf("  ← %s  %s\n", b.From, describeLink(b.Link))
	}
	if len(res.Backlinks) == 0 {
		fmt.Println("  （没有文档链到它）")
	}
	if len(res.Issues) > 0 {
		// 问题链接一并报出来：只报反链会让人以为文档很干净。
		// 「断链」和「指不清」分开说——后者是命名冲突，不是内容缺失。
		fmt.Printf("\n它自己链出去的问题链接（%d 条）：\n", len(res.Issues))
		for _, is := range res.Issues {
			label := "断链"
			if is.Kind == vault.IssueAmbiguous {
				label = "指不清"
			}
			fmt.Printf("  ✗ [%s] %s\n", label, describeLink(is.Link))
		}
	}
	return nil
}

func vaultResolve(svc *vaultapp.Service, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("resolve 后面要跟一条双链（带不带方括号都行）")
	}
	res, err := svc.Resolve(args[0])
	if err != nil {
		// 歧义时把候选一并打出来——「不确定就报告」要报得能让人做决定。
		if len(res.Candidates) > 0 {
			fmt.Fprintln(os.Stderr, "候选：")
			for _, c := range res.Candidates {
				fmt.Fprintln(os.Stderr, "  "+c)
			}
		}
		return err
	}
	fmt.Printf("目标：%s\n", res.Path)
	if res.Heading != "" {
		if res.HeadingLine > 0 {
			fmt.Printf("标题锚点：%s（第 %d 行，命中）\n", res.Heading, res.HeadingLine)
		} else {
			fmt.Printf("标题锚点：%s（**没找到**——锚点写错了，或者文档改了）\n", res.Heading)
		}
	}
	if res.Block != "" {
		if res.BlockLine > 0 {
			fmt.Printf("块锚点：%s（第 %d 行）\n  %s\n", res.Block, res.BlockLine, res.BlockText)
		} else {
			fmt.Printf("块锚点：%s（**没找到**——主张级溯源断了，别当成已经引到）\n", res.Block)
		}
	}
	return nil
}

func vaultStatus(svc *vaultapp.Service, args []string, actor string) error {
	if len(args) < 2 {
		return fmt.Errorf("status 用法：status -root <vault> -actor human:名字 <文档路径> <draft|published|archived>")
	}
	st, err := vault.ParseStatus(args[1])
	if err != nil {
		return err
	}
	a, err := vault.ParseActor(actor)
	if err != nil {
		return err
	}
	change, err := svc.SetStatus(args[0], st, a)
	if err != nil {
		return err
	}
	fmt.Printf("%s：%s → %s\n%s\n", change.Path, change.From, change.To, versionLine(change))
	return nil
}

func vaultWrite(svc *vaultapp.Service, args []string, actor string) error {
	if len(args) == 0 {
		return fmt.Errorf("write 用法：write -root <vault> -actor human:名字 <文档路径>（正文从标准输入读）")
	}
	a, err := vault.ParseActor(actor)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	change, err := svc.Write(args[0], string(body), a)
	if err != nil {
		return err
	}
	fmt.Printf("%s：%s → %s\n%s\n", change.Path, change.From, change.To, versionLine(change))
	return nil
}

// versionLine 说明这次改动在 git 里留痕的结果。
//
// 留痕是硬要求（agent.spec.md §3），所以「没留痕」必须显式打出来——
// 成功就报 short SHA，失败就报原因，没有第三种含糊说法。
func versionLine(change vaultapp.Change) string {
	if change.Committed {
		sha := change.CommitSHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		return "已在 git 留痕：" + sha + "\n" + strings.TrimSpace(change.CommitMessage())
	}
	return "⚠️ " + change.VersionNote
}

// vaultIndex 重建派生索引。索引是全派生的，随时可以重建，所以别怕跑。
func vaultIndex(svc *vaultapp.Service) error {
	if err := svc.Reindex(); err != nil {
		return err
	}
	items, err := svc.List()
	if err != nil {
		return err
	}
	tables, err := svc.TableInfos()
	if err != nil {
		return err
	}
	fmt.Printf("索引已重建：%s\n  文档 %d 篇，数据表 %d 张\n", svc.IndexPath(), len(items), len(tables))
	for _, t := range tables {
		fmt.Printf("  %-18s ← %s（%s，%d 行；列：%s）\n",
			t.Name, t.File, t.Format, t.Rows, strings.Join(t.Columns, "、"))
	}
	return nil
}

func vaultSearch(svc *vaultapp.Service, args []string, limit int) error {
	if len(args) == 0 {
		return fmt.Errorf("search 后面要跟搜索词")
	}
	q := strings.Join(args, " ")
	hits, err := svc.Search(q, limit)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Printf("没有命中「%s」\n", q)
		return nil
	}
	for _, h := range hits {
		mark := "·"
		if h.Status == vault.StatusDraft {
			mark = "!"
		}
		where := ""
		if h.TitleMatch {
			where = "（标题命中）"
		}
		fmt.Printf("%s [%-9s] %s%s\n    %s\n", mark, h.Status, h.Path, where, h.Snippet)
	}
	return nil
}

func vaultTables(svc *vaultapp.Service) error {
	tables, err := svc.TableInfos()
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		fmt.Println("这个 vault 还没有数据表（tables/ 是空的）")
		return nil
	}
	for _, t := range tables {
		fmt.Printf("%-18s ← %s（%s，%d 行）\n  列：%s\n", t.Name, t.File, t.Format, t.Rows, strings.Join(t.Columns, "、"))
	}
	fmt.Println("\n查询示例：  ssot vault -root <vault> query \"SELECT * FROM 技能倍率\"")
	return nil
}

func vaultQuery(svc *vaultapp.Service, args []string, limit int) error {
	if len(args) == 0 {
		return fmt.Errorf("query 后面要跟一条 SELECT（只读：派生索引不在这里改）")
	}
	rs, err := svc.QueryTables(strings.Join(args, " "), limit)
	if err != nil {
		return err
	}
	printResultSet(rs)
	return nil
}

// printResultSet 按列对齐打印查询结果（中文按 rune 数算宽度）。
func printResultSet(rs vault.ResultSet) {
	if len(rs.Columns) == 0 {
		fmt.Println("（没有列）")
		return
	}
	widths := make([]int, len(rs.Columns))
	for i, c := range rs.Columns {
		widths[i] = runeLen(c)
	}
	rows := make([][]string, len(rs.Rows))
	for r, row := range rs.Rows {
		rows[r] = make([]string, len(row))
		for i, v := range row {
			s := truncateRunes(v, 40)
			rows[r][i] = s
			if w := runeLen(s); w > widths[i] {
				widths[i] = w
			}
		}
	}
	header := make([]string, len(rs.Columns))
	for i, c := range rs.Columns {
		header[i] = padRight(c, widths[i])
	}
	fmt.Println(strings.Join(header, " | "))
	sep := make([]string, len(rs.Columns))
	for i := range sep {
		sep[i] = strings.Repeat("-", widths[i])
	}
	fmt.Println(strings.Join(sep, "-+-"))
	for _, row := range rows {
		cells := make([]string, len(rs.Columns))
		for i := range rs.Columns {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			cells[i] = padRight(v, widths[i])
		}
		fmt.Println(strings.Join(cells, " | "))
	}
	fmt.Printf("（%d 行）\n", len(rs.Rows))
}

func runeLen(s string) int { return len([]rune(s)) }

func padRight(s string, w int) string {
	if d := w - runeLen(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func describeLink(l vault.Link) string {
	s := l.Raw
	if s == "" {
		s = "[[" + l.Target + "]]"
	}
	if l.Embed {
		s += "（嵌入）"
	}
	if l.Block != "" {
		s += " → 块 ^" + l.Block
	}
	if l.Heading != "" {
		s += " → 标题 " + l.Heading
	}
	return s
}

func usage() {
	fmt.Print(`ssot —— 单一事实源工具

以文档为中心的项目库（vault）：docs/ 是整理层、raw/ 是原始层，数据表放 tables/，
版本与 diff 用 git。约定见 docs/specs/vault.spec.md 与 docs/specs/agent.spec.md。

用法：
  ssot help                                  看这段说明
  ssot vault -root <vault> <子命令> [...]
  ssot mcp -root <vault> [-actor agent:名字]  MCP 服务端（stdio，给 agent 后端用）

mcp：把同一套能力讲成 MCP。协议流走 stdout、日志走 stderr，中途不许有别的输出。
形状与理由见 docs/specs/dsh.spec.md；接进 DSH 的启动方式见 scripts/dsh/README.md。
⚠️ actor 只能是 agent——发布/归档在 MCP 上没有对应工具，只能走界面或 ssot vault status。

vault 子命令（读）：
  list                                       列文档与数据表（draft 会标 ! ）
  read <路径>                                读一篇（元信息 + 正文 + 双链）
  backlinks <路径>                           反链，以及它自己链出去的问题链接
  resolve <双链>                             解析到具体文档与锚点（块级锚点=主张级溯源）
  search <词>                                在标题与正文里检索（走派生索引）
  tables                                     列数据表：表名、来源文件、推断出来的列
  query "<SQL>"                              对派生索引跑只读查询（数据表 + 文档 front matter）

vault 子命令（写）：
  status -actor human:名字 <路径> <状态>      改发布态（**只有人能发布**）
  write  -actor <人|agent> <路径>            写正文（从标准输入读；agent 写入回落 draft）
  index                                      重建派生索引（.data/index.db，删了能重建）

示例：
  ssot vault -root projects/demo list
  ssot vault -root projects/demo resolve "[[raw/灰机wiki/茨木童子#^第3段]]"
  ssot vault -root projects/demo search 伤害
  ssot vault -root projects/demo query "SELECT title,status FROM docs WHERE status='draft'"
  ssot vault -root projects/demo status -actor human:我 docs/式神/茨木童子.md published

界面（Wails 桌面 / 服务模式）：
  wails3 task dev            开发运行
  wails3 task run:server     以服务模式跑在 localhost:8080
  wails3 task check          提交前全量检查（vet + 测试 + 前端构建）

备查：作废的旧方案（六部件 + 断言库 + 核验流程）在分支 legacy/mvp-v1 上。
`)
}

func vaultUsage() {
	fmt.Print(`ssot vault —— 操作一个 vault

用法：
  ssot vault -root <vault> [-actor <human:名字|agent:名字>] <子命令> [参数...]

选项（放在子命令**前后都行**）：
  -root <vault>     vault 根目录（含 project.yml 的项目目录）
  -actor <身份>     human:名字 或 agent:名字；**写操作必填**
  -limit <n>        检索/查询的行数上限

子命令：
  list                     列文档与数据表
  read <路径>              读一篇文档
  backlinks <路径>         反链 + 问题链接（断链与「指不清」分开）
  resolve <双链>           解析双链到文档与锚点
  search <词>              在标题与正文里检索
  tables                   列数据表（含推断出来的列）
  query "<SQL>"            只读查询派生索引
  status <路径> <状态>     改发布态（只有人能发布；需要 -actor）
  write <路径>             写正文，从标准输入读（需要 -actor）
  index                    重建派生索引

读操作不需要 -actor；写操作必须给，且分人还是 agent——
agent 改过的文档一律回落 draft，等人复核（docs/specs/agent.spec.md）。
`)
}
