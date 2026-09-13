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
  ssot run    <项目目录> <场景名> <主体> [选项] 运行场景

sync 选项：
  --artifacts <目录>   原件目录（默认 .huiji/raw）
  --manifest <文件>    同步清单（默认 .huiji/manifest.json）
  --entity <名>        实体类型（默认 shikigami）
  --parser <名>        解析器（默认 attribute-json）

run 选项：
  --bind <路径=字面量> 提供外部输入，可重复

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
	entity := fs.String("entity", "shikigami", "实体类型")
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

	// 从清单取修订标识与采集时间 —— 溯源的本体
	rev, capturedAt, err := lookupRevision(*manifestPath, "Data:Attribute.json")
	if err != nil {
		return err
	}
	fmt.Printf("接入：原件 %s，修订 %s，采集 %s\n", *artifacts, rev, capturedAt.Format(time.RFC3339))

	name, err := arts.Find("Data:Attribute.json")
	if err != nil {
		return err
	}
	body, _, err := arts.Read(name)
	if err != nil {
		return err
	}

	var cands []ingest.Candidate
	switch *parser {
	case "attribute-json":
		cands, err = ingest.AttributeJSON("Data:Attribute.json", body, rev, capturedAt)
	default:
		return fmt.Errorf("未知解析器 %q", *parser)
	}
	if err != nil {
		return err
	}
	fmt.Printf("解析：%d 个候选\n", len(cands))

	existing, err := p.store.KeysFor(*entity)
	if err != nil {
		return err
	}

	cs, rep, err := admit.Run(cands, p.schema, existing, admit.Options{
		Entity:     *entity,
		Source:     assertion.Source{Name: "huijiwiki", Tier: "semi-official"},
		CapturedAt: capturedAt,
		Units:      p.units,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n准入：%s\n", rep.Summary())
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
	fmt.Printf("\n应用（原子）：新增 %d，重复跳过 %d，标记冲突 %d\n", res.Inserted, res.Duplicated, res.Conflicted)
	n, _ := p.store.Count()
	fmt.Printf("库中现有断言 %d 条\n", n)
	return nil
}

func lookupRevision(manifestPath, title string) (string, time.Time, error) {
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("读取清单失败：%w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return "", time.Time{}, fmt.Errorf("解析清单失败：%w", err)
	}
	for _, pg := range m.Pages {
		if pg.Title == title {
			t, _ := time.Parse(time.RFC3339, pg.Anchor)
			return fmt.Sprintf("revid:%d", pg.Revid), t, nil
		}
	}
	// 清单里没有该页面时，退化为清单自身的抓取时间
	t, _ := time.Parse(time.RFC3339, m.FetchedAt)
	return "unknown", t, nil
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

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var binds bindFlags
	fs.Var(&binds, "bind", "外部输入，格式 路径=字面量，可重复")
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

	in := scenario.RunInput{Entity: "shikigami", Subject: subject, Values: map[string]string{}}
	for _, b := range binds {
		i := strings.Index(b, "=")
		if i < 0 {
			return fmt.Errorf("--bind 需要 路径=字面量 形式，收到 %q", b)
		}
		in.Values[strings.TrimSpace(b[:i])] = strings.TrimSpace(b[i+1:])
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
