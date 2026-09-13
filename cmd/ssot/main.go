// Command ssot 是 SSOT 工具的命令行入口。
//
// MVP 阶段用它验证机制；核验工作台是后期的事。
//
//	go run ./cmd/ssot schema  projects/onmyoji
//	go run ./cmd/ssot sync    projects/onmyoji --artifacts .huiji/raw --manifest .huiji/manifest.json
//	go run ./cmd/ssot derive  projects/onmyoji crit_factor
//	go run ./cmd/ssot status  projects/onmyoji
//	go run ./cmd/ssot run     projects/onmyoji damage-calc 262 --bind def_reduction="0.5 fraction"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ngnl5/ssot/internal/application/admit"
	"github.com/ngnl5/ssot/internal/application/derive"
	"github.com/ngnl5/ssot/internal/application/formula"
	"github.com/ngnl5/ssot/internal/application/ingest"
	"github.com/ngnl5/ssot/internal/application/scenario"
	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/schema"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/verification"
	"github.com/ngnl5/ssot/internal/infrastructure/artifact"
	"github.com/ngnl5/ssot/internal/infrastructure/schemafile"
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

type project struct {
	dir    string
	units  *unit.Table
	schema *schema.Set
	store  *store.Store
}

func loadProject(dir string, withStore bool) (*project, error) {
	units, err := schemafile.LoadUnits(filepath.Join(dir, "units.yml"))
	if err != nil {
		return nil, fmt.Errorf("加载单位表：%w", err)
	}
	set, problems, err := schemafile.LoadSet(filepath.Join(dir, "schema"), units)
	if err != nil {
		return nil, fmt.Errorf("加载 schema：%w", err)
	}
	if len(problems) > 0 {
		var sb strings.Builder
		sb.WriteString("schema 校验未通过：\n")
		for _, p := range problems {
			sb.WriteString("  " + p.String() + "\n")
		}
		return nil, fmt.Errorf("%s", sb.String())
	}
	p := &project{dir: dir, units: units, schema: set}
	if withStore {
		st, err := store.Open(filepath.Join(dir, ".data", "store.db"))
		if err != nil {
			return nil, err
		}
		p.store = st
	}
	return p, nil
}

func (p *project) close() {
	if p.store != nil {
		p.store.Close()
	}
}

// splitFlags 把参数拆成「选项」与「位置参数」。
//
// Go 的 flag 包遇到第一个位置参数就停止解析，因此 `run <目录> <场景> <主体> --bind x=y`
// 里的 --bind 会被整个忽略。本函数先做一次预扫描把两者分开。
// 本工具的选项**全部带值**，因此预扫描是安全的。
func splitFlags(args []string) (flags, pos []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
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
	fmt.Printf("单位（%d）：%s\n", len(p.units.Names()), strings.Join(p.units.Names(), " "))
	fmt.Printf("\n实体（%d）：\n", len(p.schema.Names()))
	for _, name := range p.schema.Names() {
		e, _ := p.schema.Lookup(name)
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
	defer p.close()

	arts, err := artifact.New(*artifacts)
	if err != nil {
		return err
	}
	revs, err := loadRevisions(*manifestPath)
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
			if err := syncShikigami(p, arts, revs, *parser); err != nil {
				return err
			}
		case "skill":
			if err := syncSkills(p, arts, revs); err != nil {
				return err
			}
		default:
			return fmt.Errorf("未知实体 %q", ent)
		}
	}
	n, _ := p.store.Count()
	fmt.Printf("\n库中现有断言 %d 条\n", n)
	return nil
}

// revInfo 是一个原件的修订标识与采集时间——溯源的本体。
type revInfo struct {
	Revision   string
	CapturedAt time.Time
}

func loadRevisions(path string) (map[string]revInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取清单失败：%w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("解析清单失败：%w", err)
	}
	fallback, _ := time.Parse(time.RFC3339, m.FetchedAt)
	out := map[string]revInfo{}
	for _, pg := range m.Pages {
		t, _ := time.Parse(time.RFC3339, pg.Anchor)
		if t.IsZero() {
			t = fallback
		}
		out[pg.Title] = revInfo{Revision: fmt.Sprintf("revid:%d", pg.Revid), CapturedAt: t}
	}
	return out, nil
}

func syncShikigami(p *project, arts *artifact.Store, revs map[string]revInfo, parser string) error {
	const title = "Data:Attribute.json"
	ri, ok := revs[title]
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

func syncSkills(p *project, arts *artifact.Store, revs map[string]revInfo) error {
	files, err := arts.List()
	if err != nil {
		return err
	}
	var cands []ingest.Candidate
	var unresolved []ingest.Unresolved
	var stats ingest.SkillStats
	filesRead := 0
	var latest time.Time

	for _, f := range files {
		title := artifact.Title(f)
		if !strings.HasPrefix(title, "Data:Character/") {
			continue
		}
		ri, ok := revs[title]
		if !ok {
			ri = revInfo{Revision: "unknown"}
		}
		body, _, err := arts.Read(f)
		if err != nil {
			return err
		}
		ex, err := ingest.SkillsJSON(title, body, ri.Revision, ri.CapturedAt)
		if err != nil {
			// 单个文件解析失败不应中断整批，但必须报告
			fmt.Printf("  ⚠ 跳过 %s：%v\n", title, err)
			continue
		}
		cands = append(cands, ex.Candidates...)
		unresolved = append(unresolved, ex.Unresolved...)
		stats.Add(ex.Stats)
		filesRead++
		if ri.CapturedAt.After(latest) {
			latest = ri.CapturedAt
		}
	}

	fmt.Printf("\n── 技能 ──\n读取 %d 个角色文件，解析 %d 个候选\n", filesRead, len(cands))
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
		fmt.Printf("\n未解决（文本里有多个候选值，无法确定——**不猜**，需人工判定）：共 %d 项\n", len(unresolved))
		byPred := map[string]int{}
		for _, u := range unresolved {
			byPred[u.Predicate]++
		}
		for _, k := range sortedKeys(byPred) {
			fmt.Printf("  %-12s %d 项\n", k, byPred[k])
		}
		fmt.Printf("  示例（前 6）：\n")
		for i, u := range unresolved {
			if i >= 6 {
				fmt.Printf("    … 另有 %d 项\n", len(unresolved)-6)
				break
			}
			fmt.Printf("    %s\n", u)
		}
	}
	return nil
}

// applyCandidates 走完准入到原子应用。两个实体共用。
func applyCandidates(p *project, entity string, cands []ingest.Candidate, capturedAt time.Time) error {
	existing, err := p.store.KeysFor(entity)
	if err != nil {
		return err
	}
	cs, rep, err := admit.Run(cands, p.schema, existing, admit.Options{
		Entity:       entity,
		Source:       assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		CapturedAt:   capturedAt,
		Units:        p.units,
		UniqueExists: p.store.UniqueExists,
		RefExists:    p.store.RefExists,
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

	res, err := p.store.Apply(cs)
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
	defer p.close()

	formulas, err := formula.LoadDir(filepath.Join(dir, "formulas"), p.units)
	if err != nil {
		return err
	}
	f, ok := formulas[formulaName]
	if !ok {
		return fmt.Errorf("找不到公式 %q（可用：%s）", formulaName, formulaNames(formulas))
	}

	cases, st := f.Verify(p.units)
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

	res, err := derive.Run(p.store, f, p.units, derive.Options{
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

	ar, err := p.store.Apply(res.ChangeSet)
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

	fargs, pos := splitFlags(args)
	if err := fs.Parse(fargs); err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("需要项目目录与子命令（list / approve / reject / log）")
	}
	dir, sub := pos[0], pos[1]

	p, err := loadProject(dir, true)
	if err != nil {
		return err
	}
	defer p.close()

	switch sub {
	case "list":
		return reviewList(p, *entity, *status, *limit)
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
		return fmt.Errorf("未知子命令 %q（可用：list / approve / reject / log）", sub)
	}
}

func reviewList(p *project, entity, status string, limit int) error {
	as, err := p.store.PendingByEntity(entity, status, limit)
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
	fmt.Printf("\n批准：ssot review %s approve <ID> --by <你的名字> --reason \"...\"\n", p.dir)
	fmt.Printf("驳回：ssot review %s reject  <ID> --by <你的名字> --reason \"...\"\n", p.dir)
	return nil
}

// resolveOne 按前缀唯一定位一条断言。
//
// **前缀匹配到多条时报歧义，不猜**——与准入层处理主体歧义的原则一致。
func resolveOne(p *project, prefix string) (assertion.Assertion, error) {
	ms, err := p.store.FindByIDPrefix(prefix)
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
	if err := p.store.Verify(rec); err != nil {
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
	recs, err := p.store.Verifications(a.ID)
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
	defer p.close()

	total, err := p.store.Count()
	if err != nil {
		return err
	}
	fmt.Printf("断言总数：%d\n", total)

	all, err := p.store.All()
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
	defer p.close()

	spec, err := scenario.LoadDir(filepath.Join(dir, "scenarios", scenName))
	if err != nil {
		return err
	}
	formulas, err := formula.LoadDir(filepath.Join(dir, "formulas"), p.units)
	if err != nil {
		return err
	}

	fmt.Printf("场景 %s：%s\n\n", spec.Name, spec.Description)

	rep, err := scenario.Check(spec, p.store, formulas, p.units)
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
	_, fst := f.Verify(p.units)
	if fst == formula.StatusFailed {
		return fmt.Errorf("公式 %s 的算例未通过，拒绝运行", fname)
	}

	in := scenario.RunInput{
		Entity:  "shikigami",
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

	out, err := scenario.Execute(spec, f, p.store, p.units, in, fst)
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
