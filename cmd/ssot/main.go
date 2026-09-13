// Command ssot 是 SSOT 工具的命令行入口。
//
// MVP 阶段用它验证机制；核验工作台是后期的事。
//
//	go run ./cmd/ssot schema  projects/onmyoji
//	go run ./cmd/ssot sync    projects/onmyoji --artifacts .huiji/raw --manifest .huiji/manifest.json
//	go run ./cmd/ssot derive  projects/onmyoji crit_factor
//	go run ./cmd/ssot status  projects/onmyoji
//	go run ./cmd/ssot decision projects/onmyoji list
//	go run ./cmd/ssot run     projects/onmyoji damage-calc 262 --bind def_reduction="0.5 fraction"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/admit"
	"github.com/ngnl5/ssot/internal/application/derive"
	"github.com/ngnl5/ssot/internal/application/disambig"
	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/application/review"
	"github.com/ngnl5/ssot/internal/application/scenario"
	"github.com/ngnl5/ssot/internal/compose"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/decision"
	"github.com/ngnl5/ssot/internal/domain/verification"
	"github.com/ngnl5/ssot/internal/infrastructure/artifact"
	"github.com/ngnl5/ssot/internal/infrastructure/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "schema":
		err = cmdSchema(args)
	case "sync":
		err = cmdSync(args)
	case "derive":
		err = cmdDerive(args)
	case "status":
		err = cmdStatus(args)
	case "review":
		err = cmdReview(args)
	case "decision":
		err = cmdDecision(args)
	case "run":
		err = cmdRun(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败："+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`ssot —— 单一事实源工具（MVP）

用法：
  ssot schema <项目目录>                       校验 schema 与单位表
  ssot sync   <项目目录> [选项]                接入并准入原件
  ssot derive <项目目录> <公式名>              按公式派生 L3 断言
  ssot status <项目目录>                       查看断言库状态
  ssot review <项目目录> list [选项]           查看待核验队列
  ssot review <项目目录> approve <ID> [选项]   批准（批准者必须是人）
  ssot review <项目目录> reject  <ID> [选项]   驳回
  ssot review <项目目录> log     <ID>          查看某断言的核验历史
  ssot decision <项目目录> list  [选项]        查看待判定事项（原文里的歧义）
  ssot decision <项目目录> show  <ID>          看某个事项的全部候选与原文片段
  ssot decision <项目目录> resolve <ID> <序号> 裁决：选中第 N 个候选（-1 表示都不对）
  ssot decision <项目目录> defer <ID> [选项]   暂缓（不产生断言，仍在队列里）
  ssot run    <项目目录> <场景名> <主体> [选项] 运行场景

sync 选项：
  --artifacts <目录>   原件目录（默认 .huiji/raw）
  --manifest <文件>    同步清单（默认 .huiji/manifest.json）
  --entity <名>        实体类型（默认 shikigami）
  --parser <名>        解析器（默认 attribute-json）

run 选项：
  --bind <路径=字面量>   外部输入（库中本不该有的值），可重复
  --ref  <路径=实体:主体> 取自其他实体（例如技能倍率在 skill 实体上），可重复

项目目录结构：
  project.yml  units.yml  schema/  formulas/  scenarios/  .data/
`)
}

// ── 项目装载 ────────────────────────────────────────────────────────────────

// 项目装配统一走组合根（internal/compose），CLI 与 GUI 共用同一套加载逻辑。
// 各自实现一套的话，两边对「schema 校验失败怎么办」迟早会有分歧。
type project = compose.Project

func loadProject(dir string, withStore bool) (*project, error) {
	return compose.Load(dir, withStore)
}

// splitFlags 把参数拆成「选项」与「位置参数」。
//
// Go 的 flag 包遇到第一个位置参数就停止解析，因此 `run <目录> <场景> <主体> --bind x=y`
// 里的 --bind 会被整个忽略。本函数先做一次预扫描把两者分开。
// 本工具的选项**全部带值**，因此预扫描是安全的。
//
// 例外：**负数不是选项**。`decision resolve <ID> -1` 里的 -1 是「都不对」，
// 若按前缀判断它就成了一个未定义的 flag，人会看到一句莫名其妙的 usage。
func splitFlags(args []string) (flags, pos []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !isFlag(a) {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		if !strings.Contains(a, "=") && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return flags, pos
}

// isFlag 报告该参数是不是一个选项。-0 到 -9 开头的纯数字当作位置参数。
func isFlag(a string) bool {
	if !strings.HasPrefix(a, "-") {
		return false
	}
	return !isNegativeNumber(a)
}

func isNegativeNumber(a string) bool {
	body := strings.TrimPrefix(a, "-")
	if body == "" {
		return false
	}
	for _, r := range body {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// ── schema ─────────────────────────────────────────────────────────────────

func cmdSchema(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("需要项目目录")
	}
	p, err := loadProject(args[0], false)
	if err != nil {
		return err
	}
	fmt.Printf("✓ schema 与单位表校验通过\n\n")
	fmt.Printf("单位（%d）：%s\n", len(p.Units.Names()), strings.Join(p.Units.Names(), " "))
	fmt.Printf("\n实体（%d）：\n", len(p.Schema.Names()))
	for _, name := range p.Schema.Names() {
		e, _ := p.Schema.Lookup(name)
		id := "-"
		if f := e.Identity(); f != nil {
			id = f.Key
		}
		fmt.Printf("  %-14s 字段 %2d  身份字段 %s\n", e.Name, len(e.Fields), id)
		for _, f := range e.Fields {
			extra := ""
			if f.Unit != "" {
				extra = " unit=" + f.Unit
			}
			if len(f.Values) > 0 {
				extra = " values=" + strings.Join(f.Values, "|")
			}
			fmt.Printf("      %-14s %-8s%s%s\n", f.Key, f.Type, extra, reqMark(f.Required))
		}
	}
	return nil
}

func reqMark(req bool) string {
	if req {
		return "  required"
	}
	return ""
}

// ── sync ───────────────────────────────────────────────────────────────────

type manifest struct {
	FetchedAt string `json:"fetchedAt"`
	Pages     []struct {
		Title  string `json:"title"`
		Revid  int    `json:"revid"`
		Anchor string `json:"timestamp"`
	} `json:"pages"`
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	artifacts := fs.String("artifacts", ".huiji/raw", "原件目录")
	manifestPath := fs.String("manifest", ".huiji/manifest.json", "同步清单")
	entity := fs.String("entity", "all", "实体类型（all / shikigami / skill）")
	parser := fs.String("parser", "attribute-json", "解析器")
	fargs, pos := splitFlags(args)
	if err := fs.Parse(fargs); err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("需要项目目录")
	}
	dir := pos[0]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.Close()

	arts, err := artifact.New(*artifacts)
	if err != nil {
		return err
	}
	pages, err := loadRevisions(*manifestPath)
	if err != nil {
		return err
	}

	entities := []string{*entity}
	if *entity == "all" {
		entities = []string{"shikigami", "skill"}
	}

	fmt.Printf("接入：原件目录 %s\n", *artifacts)
	for _, ent := range entities {
		switch ent {
		case "shikigami":
			if err := syncShikigami(p, arts, pages, *parser); err != nil {
				return err
			}
		case "skill":
			if err := syncSkills(p, arts, pages); err != nil {
				return err
			}
		default:
			return fmt.Errorf("未知实体 %q", ent)
		}
	}
	n, _ := p.Store.Count()
	fmt.Printf("\n库中现有断言 %d 条\n", n)
	return nil
}

// revInfo 是一个原件的修订标识与采集时间——溯源的本体。
type revInfo struct {
	Title      string
	Revision   string
	CapturedAt time.Time
}

// loadRevisions 读同步清单。
//
// **以清单为遍历基准**，而不是以原件目录为基准：清单记录了「这次同步到底抓了什么、
// 每份的修订号是多少」。改从目录反推的话，标题形式（带不带扩展名）就会对不上，
// 溯源会退化成 unknown——实测踩过：清单标题是 `Data:Character/605.json`，
// 而目录反推得到 `Data:Character/605`，查不到修订号。
func loadRevisions(path string) ([]revInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取清单失败：%w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("解析清单失败：%w", err)
	}
	fallback, _ := time.Parse(time.RFC3339, m.FetchedAt)
	out := make([]revInfo, 0, len(m.Pages))
	for _, pg := range m.Pages {
		t, _ := time.Parse(time.RFC3339, pg.Anchor)
		if t.IsZero() {
			t = fallback
		}
		out = append(out, revInfo{
			Title:      pg.Title,
			Revision:   fmt.Sprintf("revid:%d", pg.Revid),
			CapturedAt: t,
		})
	}
	return out, nil
}

func findPage(pages []revInfo, title string) (revInfo, bool) {
	for _, p := range pages {
		if p.Title == title {
			return p, true
		}
	}
	return revInfo{}, false
}

func syncShikigami(p *project, arts *artifact.Store, pages []revInfo, parser string) error {
	const title = "Data:Attribute.json"
	ri, ok := findPage(pages, title)
	if !ok {
		return fmt.Errorf("清单中没有 %s", title)
	}
	name, err := arts.Find(title)
	if err != nil {
		return err
	}
	body, _, err := arts.Read(name)
	if err != nil {
		return err
	}

	var cands []ingest.Candidate
	switch parser {
	case "attribute-json":
		cands, err = ingest.AttributeJSON(title, body, ri.Revision, ri.CapturedAt)
	default:
		return fmt.Errorf("未知解析器 %q", parser)
	}
	if err != nil {
		return err
	}
	fmt.Printf("\n── 式神 ──\n修订 %s，解析 %d 个候选\n", ri.Revision, len(cands))
	return applyCandidates(p, "shikigami", cands, ri.CapturedAt)
}

func syncSkills(p *project, arts *artifact.Store, pages []revInfo) error {
	var cands []ingest.Candidate
	var unresolved []ingest.Unresolved
	var stats ingest.SkillStats
	filesRead, missing := 0, 0
	var latest time.Time

	for _, pg := range pages {
		if !strings.HasPrefix(pg.Title, "Data:Character/") {
			continue
		}
		file, err := arts.Find(pg.Title)
		if err != nil {
			// 清单里有、原件目录里没有——不静默跳过，要报出来
			fmt.Printf("  ⚠ 清单中的 %s 在原件目录中缺失\n", pg.Title)
			missing++
			continue
		}
		body, _, err := arts.Read(file)
		if err != nil {
			return err
		}
		ex, err := ingest.SkillsJSON(pg.Title, body, pg.Revision, pg.CapturedAt)
		if err != nil {
			// 单个文件解析失败不应中断整批，但必须报告
			fmt.Printf("  ⚠ 跳过 %s：%v\n", pg.Title, err)
			continue
		}
		cands = append(cands, ex.Candidates...)
		unresolved = append(unresolved, ex.Unresolved...)
		stats.Add(ex.Stats)
		filesRead++
		if pg.CapturedAt.After(latest) {
			latest = pg.CapturedAt
		}
	}

	fmt.Printf("\n── 技能 ──\n读取 %d 个角色文件（清单缺失 %d），解析 %d 个候选\n",
		filesRead, missing, len(cands))
	if err := applyCandidates(p, "skill", cands, latest); err != nil {
		return err
	}

	// 打印抽取分布：没有这组数字，就无法判断低命中率是抽取不足还是数据本来如此
	fmt.Printf("\n抽取分布：技能 %d（被动 %d，无升级数据 %d）\n",
		stats.Skills, stats.Passive, stats.MaxNoUpgrade)
	fmt.Printf("  一级倍率  唯一命中 %4d   多值歧义 %3d   文本无倍率 %4d\n",
		stats.RatioUnique, stats.RatioMultiple, stats.RatioNone)
	fmt.Printf("  满级倍率  唯一命中 %4d   多值歧义 %3d   文本无数值 %4d\n",
		stats.MaxUnique, stats.MaxMultiple, stats.MaxNone)
	note := stats.RatioNone - stats.Passive
	if note > 0 {
		fmt.Printf("  说明：文本无倍率的 %d 个中，%d 个是被动；其余 %d 个是治疗/控制/增益类技能，**本来就不该有伤害倍率**\n",
			stats.RatioNone, stats.Passive, note)
	}

	if len(unresolved) > 0 {
		up, err := disambig.Record(p.Store, unresolved, "huijiwiki")
		if err != nil {
			return err
		}
		fmt.Printf("\n待判定（文本里有多个候选值，无法确定——**不猜**，交给人裁决）：\n  %s\n", up)
		byPred := map[string]int{}
		for _, u := range unresolved {
			byPred[u.Predicate]++
		}
		for _, k := range sortedKeys(byPred) {
			fmt.Printf("  %-12s %d 项\n", k, byPred[k])
		}
		fmt.Printf("  用 `ssot decision <项目目录> list` 看待判定队列。\n")
	}
	return nil
}

// applyCandidates 走完准入到原子应用。两个实体共用。
func applyCandidates(p *project, entity string, cands []ingest.Candidate, capturedAt time.Time) error {
	existing, err := p.Store.KeysFor(entity)
	if err != nil {
		return err
	}
	cs, rep, err := admit.Run(cands, p.Schema, existing, admit.Options{
		Entity:       entity,
		Source:       assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		CapturedAt:   capturedAt,
		Units:        p.Units,
		UniqueExists: p.Store.UniqueExists,
		RefExists:    p.Store.RefExists,
	})
	if err != nil {
		return err
	}

	fmt.Printf("准入：%s\n", rep.Summary())
	if len(rep.Undeclared) > 0 {
		fmt.Printf("  ⚠ 数据中出现的、schema 未声明的谓词（漂移）：%s\n", strings.Join(rep.Undeclared, " "))
	}
	if len(rep.Problems) > 0 {
		fmt.Printf("  拒绝明细（前 10）：\n")
		for i, pr := range rep.Problems {
			if i >= 10 {
				fmt.Printf("    … 另有 %d 条\n", len(rep.Problems)-10)
				break
			}
			fmt.Printf("    %s\n", pr)
		}
	}
	if len(rep.Markers) > 0 {
		fmt.Printf("  标记（接受但不静默，前 10）：\n")
		for i, m := range rep.Markers {
			if i >= 10 {
				fmt.Printf("    … 另有 %d 条\n", len(rep.Markers)-10)
				break
			}
			fmt.Printf("    %s %s.%s %s\n", m.Kind, m.Record, m.Field, m.Detail)
		}
	}

	res, err := p.Store.Apply(cs)
	if err != nil {
		return err
	}
	fmt.Printf("应用（原子）：新增 %d，重复跳过 %d，标记冲突 %d\n", res.Inserted, res.Duplicated, res.Conflicted)
	return nil
}

// ── derive ─────────────────────────────────────────────────────────────────

func cmdDerive(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("需要项目目录与公式名")
	}
	dir, formulaName := args[0], args[1]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.Close()

	formulas, err := formula.LoadDir(filepath.Join(dir, "formulas"), p.Units)
	if err != nil {
		return err
	}
	f, ok := formulas[formulaName]
	if !ok {
		return fmt.Errorf("找不到公式 %q（可用：%s）", formulaName, formulaNames(formulas))
	}

	cases, st := f.Verify(p.Units)
	fmt.Printf("公式 %s v%s：%s\n", f.Name, f.Version, st)
	for _, c := range cases {
		mark := "✗"
		if c.Passed {
			mark = "✓"
		}
		line := fmt.Sprintf("  %s %s  得到 %s，期望 %s", mark, c.Name, c.Got, c.Want)
		if c.Err != nil {
			line += "  错误：" + c.Err.Error()
		}
		fmt.Println(line)
	}
	if st == formula.StatusFailed {
		return fmt.Errorf("公式 %s 的算例未通过，拒绝用于派生", f.Name)
	}
	if st == formula.StatusUnverified {
		fmt.Println("  ⚠ 该公式无算例，派生结果标注为未验证")
	}

	res, err := derive.Run(p.Store, f, p.Units, derive.Options{
		Entity:    "shikigami",
		Predicate: "crit_factor",
		Source:    assertion.Source{Name: "derived", Tier: "internal"},
	})
	if err != nil {
		return err
	}
	fmt.Printf("\n派生：主体 %d，成功 %d，跳过 %d\n", res.Subjects, res.Derived, res.Skipped)
	for i, r := range res.SkipReason {
		if i >= 5 {
			fmt.Printf("  … 另有 %d 条跳过原因\n", len(res.SkipReason)-5)
			break
		}
		fmt.Printf("  跳过 %s\n", r)
	}

	ar, err := p.Store.Apply(res.ChangeSet)
	if err != nil {
		return err
	}
	fmt.Printf("应用：新增 %d，重复跳过 %d\n", ar.Inserted, ar.Duplicated)
	return nil
}

func formulaNames(m map[string]*formula.Formula) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// ── review ─────────────────────────────────────────────────────────────────

type whereFlags []string

func (w *whereFlags) String() string { return strings.Join(*w, ",") }
func (w *whereFlags) Set(v string) error {
	*w = append(*w, v)
	return nil
}

func cmdReview(args []string) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	entity := fs.String("entity", "", "限定实体（空表示全部）")
	status := fs.String("status", "pending", "限定状态")
	limit := fs.Int("limit", 20, "最多列出多少条")
	by := fs.String("by", "", "批准者（必须是**人**）")
	method := fs.String("method", "editorial", "核验方法：measurement / recompute / cross-source / editorial")
	reason := fs.String("reason", "", "理由（必填）")
	evidence := fs.String("evidence", "", "依据。以 measurement 核验时必填：版本、配置、样本数")
	proposedBy := fs.String("proposed-by", "ingest-pipeline", "提出者（agent 标识）")
	sample := fs.Float64("sample", 0, "强制随机抽检比例（0~1），抽检项不会被高分项挤出队列")
	seed := fs.Int64("seed", 1, "抽检随机种子——「随机」不等于「不可复现」")
	var wheres whereFlags
	fs.Var(&wheres, "where", "批量筛选条件，格式 字段=值，可重复")
	yes := fs.Bool("yes", false, "批量操作默认只预览；加此项才真正执行")

	fargs, pos := splitFlags(args)
	if err := fs.Parse(fargs); err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("需要项目目录与子命令（queue / list / conflicts / batch / approve / reject / log）")
	}
	dir, sub := pos[0], pos[1]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.Close()

	switch sub {
	case "queue":
		return reviewQueue(p, *entity, *status, *limit, *sample, *seed)
	case "list":
		return reviewList(p, *entity, *status, *limit)
	case "conflicts":
		return reviewConflicts(p)
	case "batch":
		if len(pos) < 3 {
			return fmt.Errorf("batch 需要 approve 或 reject")
		}
		return reviewBatch(p, pos[2], wheres, *by, *method, *reason, *evidence, *proposedBy, *yes)
	case "approve", "reject":
		if len(pos) < 3 {
			return fmt.Errorf("%s 需要断言 ID（或其前缀）", sub)
		}
		dec := verification.Approved
		if sub == "reject" {
			dec = verification.Rejected
		}
		return reviewDecide(p, pos[2], dec, *by, *method, *reason, *evidence, *proposedBy)
	case "log":
		if len(pos) < 3 {
			return fmt.Errorf("log 需要断言 ID（或其前缀）")
		}
		return reviewLog(p, pos[2])
	default:
		return fmt.Errorf("未知子命令 %q（可用：queue / list / conflicts / batch / approve / reject / log）", sub)
	}
}

// ── decision：待判定（原文里的歧义，必须由人选）─────────────────────────────

func cmdDecision(args []string) error {
	fs := flag.NewFlagSet("decision", flag.ContinueOnError)
	status := fs.String("status", "", "限定状态：open / deferred / decided / stale（空 = 还需要人看的）")
	limit := fs.Int("limit", 20, "最多列出多少条（0 = 不限）")
	by := fs.String("by", "", "裁决人（必须是**人**）")
	method := fs.String("method", "editorial", "核验方法：editorial / measurement")
	reason := fs.String("reason", "", "理由（必填）")
	evidence := fs.String("evidence", "", "依据。以 measurement 裁决时必填：版本、配置、样本数")
	proposedBy := fs.String("proposed-by", "ingest-pipeline", "提出候选者（agent 标识）")

	fargs, pos := splitFlags(args)
	if err := fs.Parse(fargs); err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("需要项目目录与子命令（list / show / resolve / defer）")
	}
	dir, sub := pos[0], pos[1]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.Close()

	opts := admit.Options{
		Entity: "", Source: assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		CapturedAt: time.Now().UTC(), Units: p.Units,
		UniqueExists: p.Store.UniqueExists, RefExists: p.Store.RefExists,
	}
	_ = opts

	switch sub {
	case "list":
		return decisionList(p, *status, *limit)
	case "show":
		if len(pos) < 3 {
			return fmt.Errorf("show 需要事项 ID（或其前缀）")
		}
		return decisionShow(p, pos[2])
	case "resolve":
		if len(pos) < 4 {
			return fmt.Errorf("resolve 需要事项 ID 与候选序号（-1 表示都不对）")
		}
		choice, err := strconv.Atoi(pos[3])
		if err != nil {
			return fmt.Errorf("候选序号必须是整数：%q", pos[3])
		}
		return decisionResolve(p, pos[2], choice, *by, *method, *reason, *evidence, *proposedBy)
	case "defer":
		if len(pos) < 3 {
			return fmt.Errorf("defer 需要事项 ID")
		}
		return decisionDefer(p, pos[2], *by, *reason)
	default:
		return fmt.Errorf("未知子命令 %q（可用：list / show / resolve / defer）", sub)
	}
}

// decisionOpts 构造该事项所在实体的准入上下文。
//
// Source 与 CapturedAt 取自被裁决候选自己的溯源——裁决不是一次新的采集，
// 它是「对已有原件的一次判断」，因此必须沿用原件的来源与采集时间。
func decisionOpts(p *project, it decision.Item) admit.Options {
	src := assertion.Source{Name: it.Source, Tier: "semi-official"}
	if it.Source == "" {
		src.Name = "huijiwiki"
	}
	captured := time.Now().UTC()
	if len(it.Candidates) > 0 {
		captured = time.Now().UTC()
	}
	return admit.Options{
		Entity: it.Entity, Source: src, CapturedAt: captured, Units: p.Units,
		UniqueExists: p.Store.UniqueExists, RefExists: p.Store.RefExists,
	}
}

func decisionList(p *project, status string, limit int) error {
	entries, err := disambig.Queue(p.Store, status, limit)
	if err != nil {
		return err
	}
	st, err := disambig.Summary(p.Store)
	if err != nil {
		return err
	}
	fmt.Printf("待判定：待判定 %d，已暂缓 %d，需复核 %d，已裁决 %d（其中判为缺失 %d）\n",
		st.Open, st.Deferred, st.Stale, st.Decided, st.Missing)
	if len(entries) == 0 {
		fmt.Println("没有需要人看的待判定事项。")
		return nil
	}
	fmt.Printf("\n%-12s %-10s %-10s %-6s %s\n", "ID", "主体", "谓词", "候选", "为什么排在前面")
	for _, e := range entries {
		fmt.Printf("%-12s %-10s %-10s %-6d %s\n",
			e.Item.ID, e.Item.Subject, e.Item.Predicate, len(e.Item.Candidates), e.Reason)
	}
	fmt.Printf("\n用 `ssot decision <项目目录> show <ID>` 看候选，`resolve <ID> <序号>` 裁决。\n")
	return nil
}

func decisionShow(p *project, prefix string) error {
	it, err := findDecision(p, prefix)
	if err != nil {
		return err
	}
	fmt.Printf("事项 %s\n", it.ID)
	fmt.Printf("  %s.%s  %s\n", it.Subject, it.Predicate, it.Status.Label())
	fmt.Printf("  歧义原因：%s\n", it.Reason)
	fmt.Printf("  来源：%s %s\n", it.Artifact, it.Revision)
	fmt.Printf("  原文片段：%s\n", it.Context)
	fmt.Printf("\n候选（%d 个）：\n", len(it.Candidates))
	for i, c := range it.Candidates {
		fmt.Printf("  [%d] %-14s %s\n", i, c.Value.String(), c.Anchor)
		fmt.Printf("      上下文：%s\n", c.Context)
		if c.Note != "" {
			fmt.Printf("      说明：%s\n", c.Note)
		}
	}
	if it.Resolution != nil {
		fmt.Printf("\n裁决：%s（由 %s，%s）\n", chosenText(it), it.Resolution.By.String(),
			it.Resolution.At.Format(time.RFC3339))
		fmt.Printf("  理由：%s\n", it.Resolution.Reason)
		if it.Resolution.AssertionID != "" {
			fmt.Printf("  断言：%s\n", it.Resolution.AssertionID)
		}
	}
	for i, h := range it.History {
		fmt.Printf("\n历史结论 %d：%s（由 %s，%s）—— %s\n", i+1, chosenTextOf(h),
			h.By.String(), h.At.Format(time.RFC3339), h.Reason)
	}
	if it.Deferral != nil {
		fmt.Printf("\n暂缓：由 %s，%s —— %s\n", it.Deferral.By.String(),
			it.Deferral.At.Format(time.RFC3339), it.Deferral.Reason)
	}
	if it.Status.NeedsAttention() {
		fmt.Printf("\n裁决：`ssot decision <项目目录> resolve %s <候选序号>`（-1 表示都不对）\n", it.ID)
	}
	return nil
}

func chosenText(it decision.Item) string {
	if it.Resolution == nil {
		return "—"
	}
	return chosenTextOf(*it.Resolution)
}

func chosenTextOf(r decision.Resolution) string {
	if r.IsNone() {
		return "都不对（该谓词记为缺失）"
	}
	return r.ChosenValue.String()
}

func decisionResolve(p *project, prefix string, choice int, by, method, reason, evidence, proposedBy string) error {
	it, err := findDecision(p, prefix)
	if err != nil {
		return err
	}
	res, err := disambig.Resolve(p.Store, p.Schema, decisionOpts(p, it), disambig.ResolveInput{
		ID:       it.ID,
		Choice:   choice,
		By:       verification.Actor{Kind: verification.Human, ID: by},
		Method:   verification.Method(method),
		Reason:   reason,
		Evidence: evidence,
		Proposed: verification.Actor{Kind: verification.Agent, ID: proposedBy},
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Println(res.Message)
	if res.AssertionID != "" {
		fmt.Printf("断言 %s（%s）\n", res.AssertionID, it.Entity)
	}
	return nil
}

func decisionDefer(p *project, prefix, by, reason string) error {
	it, err := findDecision(p, prefix)
	if err != nil {
		return err
	}
	next, err := disambig.Defer(p.Store, it.ID,
		verification.Actor{Kind: verification.Human, ID: by}, reason, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("已暂缓 %s——它仍在待判定队列里，只是标为「人已看过、先放着」。\n", next.ID)
	return nil
}

// findDecision 支持用 ID 前缀定位，省去每次复制长 ID。
func findDecision(p *project, prefix string) (decision.Item, error) {
	items, err := p.Store.Decisions("all", 0)
	if err != nil {
		return decision.Item{}, err
	}
	var hit []decision.Item
	for _, it := range items {
		if it.ID == prefix {
			return it, nil
		}
		if strings.HasPrefix(it.ID, prefix) {
			hit = append(hit, it)
		}
	}
	switch len(hit) {
	case 1:
		return hit[0], nil
	case 0:
		return decision.Item{}, fmt.Errorf("找不到待判定事项 %q", prefix)
	default:
		return decision.Item{}, fmt.Errorf("%q 匹配到 %d 个事项，请写全 ID", prefix, len(hit))
	}
}

func reviewList(p *project, entity, status string, limit int) error {
	as, err := p.Store.PendingByEntity(entity, status, limit)
	if err != nil {
		return err
	}
	if len(as) == 0 {
		fmt.Printf("没有状态为 %s 的断言", status)
		if entity != "" {
			fmt.Printf("（实体 %s）", entity)
		}
		fmt.Println()
		return nil
	}
	fmt.Printf("核验队列（状态 %s）：%d 条\n\n", status, len(as))
	fmt.Printf("%-18s %-11s %-10s %-14s %-22s %-4s\n", "断言 ID", "实体", "主体", "谓词", "取值", "分级")
	for _, a := range as {
		v := a.Value.String()
		if len(v) > 20 {
			v = v[:20] + "…"
		}
		fmt.Printf("%-18s %-11s %-10s %-14s %-22s %-4s\n", a.ID, a.Entity, a.Subject, a.Predicate, v, a.Confidence)
	}
	fmt.Printf("\n批准：ssot review %s approve <ID> --by <你的名字> --reason \"...\"\n", p.Dir)
	fmt.Printf("驳回：ssot review %s reject  <ID> --by <你的名字> --reason \"...\"\n", p.Dir)
	return nil
}

// reviewQueue 按优先级列出待核验项，并说明「为什么它排在前面」。
//
// 排序规则见 docs/specs/verification.spec.md「人力的分配」：
// 争议 > 影响面 > 分级，且**必须包含随机抽检**——
// 否则人会只核验「显眼」的部分，系统性错误永远发现不了。
func reviewQueue(p *project, entity, status string, limit int, sampleRatio float64, seed int64) error {
	total, err := p.Store.Select(assertion.Filter{Entity: entity, Status: status})
	if err != nil {
		return err
	}
	if len(total) == 0 {
		fmt.Printf("没有状态为 %s 的断言\n", status)
		return nil
	}

	// 排序规则与抽检都在应用层，CLI 与 GUI 共用同一套
	items, err := review.Queue(p.Store,
		assertion.Filter{Entity: entity, Status: status}, limit, sampleRatio, seed)
	if err != nil {
		return err
	}

	sampled := 0
	for _, it := range items {
		if it.Priority.Sampled {
			sampled++
		}
	}
	fmt.Printf("核验队列（状态 %s，共 %d 条，显示 %d 条）\n", status, len(total), len(items))
	if sampleRatio > 0 {
		fmt.Printf("本次强制抽检 %d 条（比例 %.0f%%，种子 %d）\n", sampled, sampleRatio*100, seed)
	}
	fmt.Println()
	fmt.Printf("%-3s %-18s %-10s %-8s %-13s %-18s %-8s %s\n",
		"#", "断言 ID", "实体", "主体", "谓词", "取值", "类别", "主导理由")
	for i, it := range items {
		a := it.Assertion
		v := a.Value.String()
		if runes := []rune(v); len(runes) > 16 {
			v = string(runes[:16]) + "…"
		}
		tier := string(it.Priority.Tier)
		if it.Priority.Sampled && it.Priority.Tier != verification.TierSample {
			tier += "·抽检"
		}
		fmt.Printf("%-3d %-18s %-10s %-8s %-13s %-18s %-8s %s\n",
			i+1, a.ID, a.Entity, a.Subject, a.Predicate, v, tier, it.Priority.Reason)
	}
	fmt.Printf("\n批准：ssot review %s approve <ID> --by <你的名字> --reason \"...\"\n", p.Dir)
	return nil
}

// reviewConflicts 成对呈现冲突，供人工裁决。
func reviewConflicts(p *project) error {
	groups, err := review.Conflicts(p.Store)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		fmt.Println("未发现冲突——同一身份上没有任何两种取值。")
		fmt.Println("（注意：这只说明「没有两个来源给出不同值」，不说明数据已经正确。）")
		return nil
	}
	fmt.Printf("发现 %d 组冲突。**系统不替你裁决**——下面是每一组的全部说法：\n", len(groups))
	for i, g := range groups {
		fmt.Printf("\n── 冲突 %d/%d：%s.%s ──\n", i+1, len(groups), g.Subject, g.Predicate)
		for _, a := range g.Claims {
			fmt.Printf("  %-22s 来源 %-12s 分级 %-3s 状态 %s\n",
				a.Value.String(), a.Source.Name, a.Confidence, a.Status)
			fmt.Printf("      溯源 %s %s @ %s\n", a.Provenance.Artifact, a.Provenance.Anchor, a.Provenance.Revision)
			fmt.Printf("      ID   %s\n", a.ID)
		}
		fmt.Printf("  裁决：ssot review %s approve <ID> --by <你的名字> --reason \"采信理由\"\n", p.Dir)
	}
	return nil
}

// reviewBatch 批量核验。**默认只预览**——批量操作最容易造成大面积错误。
func reviewBatch(p *project, action string, wheres whereFlags, by, method, reason, evidence, proposed string, yes bool) error {
	var dec verification.Decision
	var label string
	switch action {
	case "approve":
		dec, label = verification.Approved, "批准"
	case "reject":
		dec, label = verification.Rejected, "驳回"
	default:
		return fmt.Errorf("batch 的动作只能是 approve 或 reject，收到 %q", action)
	}

	f, err := parseWheres(wheres)
	if err != nil {
		return err
	}

	// 预览与执行都走应用层，CLI 与 GUI 共用同一套
	as, err := review.Preview(p.Store, f)
	if err != nil {
		return err
	}
	if len(as) == 0 {
		fmt.Println("没有匹配的待核验断言。")
		return nil
	}
	if f.Status == "" {
		f.Status = string(assertion.StatusPending)
	}

	fmt.Printf("批量%s：%d 条\n", label, len(as))
	fmt.Printf("筛选条件：%s\n", f.Describe())
	fmt.Printf("示例（前 5 条）：\n")
	for i, a := range as {
		if i >= 5 {
			fmt.Printf("  … 另有 %d 条\n", len(as)-5)
			break
		}
		fmt.Printf("  %s.%s = %s\n", a.Subject, a.Predicate, a.Value)
	}

	if !yes {
		fmt.Printf("\n这是**预览**（dry-run）。确认无误后加 --yes 才真正执行。\n")
		fmt.Printf("每条断言会各自留下核验记录，审计轨迹不会合并——这是刻意的：\n")
		fmt.Printf("一条记录代表一个人的一次判断，批量不等于免责。\n")
		return nil
	}

	res, err := review.Batch(p.Store, p.Store, review.BatchInput{
		Filter:   f,
		Decision: dec,
		Method:   verification.Method(method),
		By:       by,
		Reason:   reason,
		Evidence: evidence,
		Proposed: proposed,
	}, time.Now().UTC())
	if err != nil && res.Applied == 0 {
		return err
	}
	fmt.Printf("\n完成：成功 %d，失败 %d\n", res.Applied, res.Failed)
	if res.FirstErr != nil {
		fmt.Printf("首个失败：%v\n", res.FirstErr)
	}
	return nil
}

// parseWheres 把 字段=值 列表转成筛选条件。
func parseWheres(ws []string) (store.Filter, error) {
	var f store.Filter
	for _, w := range ws {
		i := strings.Index(w, "=")
		if i < 0 {
			return f, fmt.Errorf("--where 需要 字段=值 形式，收到 %q", w)
		}
		k := strings.TrimSpace(w[:i])
		v := strings.TrimSpace(w[i+1:])
		switch k {
		case "entity", "实体":
			f.Entity = v
		case "status", "状态":
			f.Status = v
		case "predicate", "谓词":
			f.Predicate = v
		case "artifact", "原件":
			f.Artifact = v
		case "revision", "修订":
			f.Revision = v
		case "confidence", "分级":
			f.Confidence = v
		case "subject", "主体":
			f.Subject = v
		default:
			return f, fmt.Errorf("未知的筛选字段 %q（可用：entity / status / predicate / artifact / revision / confidence / subject）", k)
		}
	}
	return f, nil
}

// resolveOne 按前缀唯一定位一条断言。
//
// **前缀匹配到多条时报歧义，不猜**——与准入层处理主体歧义的原则一致。
func resolveOne(p *project, prefix string) (assertion.Assertion, error) {
	ms, err := p.Store.FindByIDPrefix(prefix)
	if err != nil {
		return assertion.Assertion{}, err
	}
	switch len(ms) {
	case 0:
		return assertion.Assertion{}, fmt.Errorf("没有 ID 以 %q 开头的断言", prefix)
	case 1:
		return ms[0], nil
	default:
		fmt.Printf("前缀 %q 匹配到 %d 条，**不猜**——请给出更长的前缀：\n", prefix, len(ms))
		for _, m := range ms {
			fmt.Printf("  %s  %s.%s = %s\n", m.ID, m.Subject, m.Predicate, m.Value)
		}
		return assertion.Assertion{}, fmt.Errorf("断言 ID 前缀有歧义")
	}
}

func reviewDecide(p *project, prefix string, dec verification.Decision, by, method, reason, evidence, proposed string) error {
	a, err := resolveOne(p, prefix)
	if err != nil {
		return err
	}
	if by == "" {
		return fmt.Errorf("必须用 --by 指明批准者（必须是人）——无追责的核验等于没有核验")
	}
	if reason == "" {
		return fmt.Errorf("必须用 --reason 说明理由——只有状态没有理由的不是核验")
	}
	rec := verification.Record{
		AssertionID: a.ID,
		Decision:    dec,
		Method:      verification.Method(method),
		ProposedBy:  verification.Actor{Kind: verification.Agent, ID: proposed},
		ApprovedBy:  &verification.Actor{Kind: verification.Human, ID: by},
		Reason:      reason,
		Evidence:    evidence,
		At:          time.Now().UTC(),
	}
	if err := p.Store.Verify(rec); err != nil {
		return err
	}
	fmt.Printf("%s.%s = %s\n", a.Subject, a.Predicate, a.Value)
	fmt.Printf("→ 状态 %s（%s，由 %s 批准）\n", dec.StatusFor(), verification.Method(method).Label(), by)
	fmt.Printf("  溯源：%s %s @ %s\n", a.Provenance.Artifact, a.Provenance.Anchor, a.Provenance.Revision)
	return nil
}

func reviewLog(p *project, prefix string) error {
	a, err := resolveOne(p, prefix)
	if err != nil {
		return err
	}
	fmt.Printf("%s.%s = %s（当前状态 %s，分级 %s）\n\n", a.Subject, a.Predicate, a.Value, a.Status, a.Confidence)
	recs, err := p.Store.Verifications(a.ID)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		fmt.Println("尚无核验记录——该断言尚未被任何人核验")
		return nil
	}
	for _, r := range recs {
		fmt.Printf("  %s  %s\n", r.At.Format(time.RFC3339), r.Summarize())
		if r.Evidence != "" {
			fmt.Printf("      依据：%s\n", r.Evidence)
		}
	}
	return nil
}

// ── status ─────────────────────────────────────────────────────────────────

func cmdStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("需要项目目录")
	}
	p, err := loadProject(args[0], true)
	if err != nil {
		return err
	}
	defer p.Close()

	total, err := p.Store.Count()
	if err != nil {
		return err
	}
	fmt.Printf("断言总数：%d\n", total)

	all, err := p.Store.All()
	if err != nil {
		return err
	}
	byEntity := map[string]int{}
	byStatus := map[string]int{}
	byConf := map[string]int{}
	subjects := map[string]map[string]bool{}
	for _, a := range all {
		byEntity[a.Entity]++
		byStatus[string(a.Status)]++
		byConf[string(a.Confidence)]++
		if subjects[a.Entity] == nil {
			subjects[a.Entity] = map[string]bool{}
		}
		subjects[a.Entity][a.Subject] = true
	}

	fmt.Println("\n按实体：")
	for _, e := range sortedKeys(byEntity) {
		fmt.Printf("  %-14s %5d 条  %3d 个主体\n", e, byEntity[e], len(subjects[e]))
	}
	fmt.Println("\n按状态：")
	for _, s := range sortedKeys(byStatus) {
		fmt.Printf("  %-14s %5d\n", s, byStatus[s])
	}
	fmt.Println("\n按分级：")
	for _, c := range sortedKeys(byConf) {
		fmt.Printf("  %-14s %5d\n", c, byConf[c])
	}
	return nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── run ────────────────────────────────────────────────────────────────────

type bindFlags []string

func (b *bindFlags) String() string { return strings.Join(*b, ",") }
func (b *bindFlags) Set(v string) error {
	*b = append(*b, v)
	return nil
}

type refFlags []string

func (r *refFlags) String() string { return strings.Join(*r, ",") }
func (r *refFlags) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var binds bindFlags
	var refs refFlags
	fs.Var(&binds, "bind", "外部输入，格式 路径=字面量，可重复")
	fs.Var(&refs, "ref", "取自其他实体，格式 路径=实体:主体，可重复")
	fargs, pos := splitFlags(args)
	if err := fs.Parse(fargs); err != nil {
		return err
	}
	if len(pos) < 3 {
		return fmt.Errorf("需要项目目录、场景名与主体")
	}
	dir, scenName, subject := pos[0], pos[1], pos[2]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.Close()

	spec, err := scenario.LoadDir(filepath.Join(dir, "scenarios", scenName))
	if err != nil {
		return err
	}
	formulas, err := formula.LoadDir(filepath.Join(dir, "formulas"), p.Units)
	if err != nil {
		return err
	}

	fmt.Printf("场景 %s：%s\n\n", spec.Name, spec.Description)

	rep, err := scenario.Check(spec, p.Store, formulas, p.Units)
	if err != nil {
		return err
	}

	fmt.Println("完整性检查：")
	for _, q := range rep.Requirements {
		mark := "✗"
		if q.Status == scenario.Satisfied {
			mark = "✓"
		}
		line := fmt.Sprintf("  %s %-28s %s", mark, q.Want, q.Status)
		if q.Detail != "" {
			line += "  " + q.Detail
		}
		fmt.Println(line)
	}
	for _, f := range rep.Formulas {
		fmt.Printf("  · 公式 %-16s %s\n", f.Name, f.Status)
		for _, c := range f.Cases {
			m := "✗"
			if c.Passed {
				m = "✓"
			}
			fmt.Printf("      %s %s（得到 %s，期望 %s）\n", m, c.Name, c.Got, c.Want)
		}
	}
	if len(spec.Inputs) > 0 {
		fmt.Println("  外部输入（库中本不该有，由调用方提供）：")
		for _, in := range spec.Inputs {
			fmt.Printf("      · %-16s %s\n", in.Name, in.Description)
		}
	}

	if !rep.Runnable() {
		fmt.Println("\n✗ 场景不可运行，缺失：")
		for _, m := range rep.Missing() {
			fmt.Println("    " + m)
		}
		return fmt.Errorf("场景 %s 的 requires 未满足，拒绝运行（不以不完整数据产出方案）", spec.Name)
	}

	// 求值
	if len(spec.Formulas) == 0 {
		return fmt.Errorf("场景没有声明公式")
	}
	fname := spec.Formulas[0]
	f, ok := formulas[fname]
	if !ok {
		return fmt.Errorf("场景依赖的公式 %q 不存在", fname)
	}
	_, fst := f.Verify(p.Units)
	if fst == formula.StatusFailed {
		return fmt.Errorf("公式 %s 的算例未通过，拒绝运行", fname)
	}

	in := scenario.RunInput{
		Entity:  spec.Entity,
		Subject: subject,
		Extra:   map[string]scenario.Ref{},
		Values:  map[string]string{},
	}
	for _, b := range binds {
		i := strings.Index(b, "=")
		if i < 0 {
			return fmt.Errorf("--bind 需要 路径=字面量 形式，收到 %q", b)
		}
		in.Values[strings.TrimSpace(b[:i])] = strings.TrimSpace(b[i+1:])
	}
	for _, rf := range refs {
		i := strings.Index(rf, "=")
		if i < 0 {
			return fmt.Errorf("--ref 需要 路径=实体:主体 形式，收到 %q", rf)
		}
		path := strings.TrimSpace(rf[:i])
		v := strings.TrimSpace(rf[i+1:])
		j := strings.Index(v, ":")
		if j < 0 {
			return fmt.Errorf("--ref 的主体部分需要 实体:主体 形式，收到 %q", v)
		}
		in.Extra[path] = scenario.Ref{Entity: v[:j], Subject: v[j+1:]}
	}

	out, err := scenario.Execute(spec, f, p.Store, p.Units, in, fst)
	if err != nil {
		return err
	}

	fmt.Printf("\n产出（场景 %s，主体 %s）：\n", out.Scenario, out.Subject)
	fmt.Printf("  输入：%s\n", formula.FormatBindings(out.Bindings))
	fmt.Printf("  结果：%s\n", out.Result)
	for _, n := range out.Notes {
		fmt.Printf("  注：%s\n", n)
	}
	return nil
}
