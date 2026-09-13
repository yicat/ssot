// Package formula 加载并执行场景声明的公式，并验证其算例。
//
// 见 docs/specs/computation.spec.md 第三节：
//
//	有算例且全部通过 -> 公式已验证
//	没有算例         -> 允许使用，但方案必须标注「公式未验证」
//	算例未通过       -> **拒绝**使用该公式
//
// 「不能错」的最后一环是算对。数据再准，公式算错，结果就是错的；
// 而公式若不透明、不可测，错误就无法被发现。
package formula

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ngnl5/ssot/internal/domain/expr"
	"github.com/ngnl5/ssot/internal/domain/unit"
	"github.com/ngnl5/ssot/internal/domain/value"

	"gopkg.in/yaml.v3"
)

// Spec 是公式的声明。
//
// params 给出**静态类型**（用于撰写期检查），bindings 给出**运行期取值来源**。
// 二者分开，是因为静态检查必须在没有数据的时候也能进行。
type Spec struct {
	Name        string               `yaml:"formula"`
	Version     string               `yaml:"version"`
	Description string               `yaml:"description"`
	Params      map[string]ParamSpec `yaml:"params"`
	Bindings    map[string]string    `yaml:"bindings"`
	Result      string               `yaml:"result"`
	Cases       []Case               `yaml:"cases"`
}

// ParamSpec 声明一个绑定的静态类型。
type ParamSpec struct {
	Type string `yaml:"type"`
	Unit string `yaml:"unit"`
	Note string `yaml:"note"`
}

// Case 是一条算例：给定输入，期望输出。
type Case struct {
	Name   string            `yaml:"name"`
	Given  map[string]string `yaml:"given"`
	Expect string            `yaml:"expect"`
}

// Formula 是已编译的公式。
type Formula struct {
	Spec
	src string
	ex  *expr.Expr
}

// Load 从文件加载并编译一个公式。
func Load(path string, units *unit.Table) (*Formula, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(b), units, path)
}

// Parse 从字符串加载并编译一个公式。
func Parse(src string, units *unit.Table, origin string) (*Formula, error) {
	var s Spec
	if err := yaml.Unmarshal([]byte(src), &s); err != nil {
		return nil, fmt.Errorf("%s: %w", origin, err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("%s: 公式缺少 formula 名", origin)
	}
	if strings.TrimSpace(s.Result) == "" {
		return nil, fmt.Errorf("%s: 公式 %s 缺少 result", origin, s.Name)
	}

	env := expr.MapEnv{}
	for name, p := range s.Params {
		if p.Type == "" {
			return nil, fmt.Errorf("%s: 参数 %s 缺少 type", origin, name)
		}
		env[name] = expr.FieldSpec{Type: p.Type, Unit: p.Unit}
	}
	// 绑定名必须都有静态声明，否则无法做撰写期检查
	for name := range s.Bindings {
		if _, ok := env[name]; !ok {
			return nil, fmt.Errorf("%s: 绑定 %s 缺少 params 中的类型声明", origin, name)
		}
	}

	ex, err := expr.Compile(s.Result, env, units)
	if err != nil {
		return nil, fmt.Errorf("%s: 公式 %s 的 result 编译失败：%w", origin, s.Name, err)
	}
	return &Formula{Spec: s, src: src, ex: ex}, nil
}

// LoadDir 加载目录下的全部公式。
func LoadDir(dir string, units *unit.Table) (map[string]*Formula, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.formula.yml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := map[string]*Formula{}
	for _, p := range paths {
		f, err := Load(p, units)
		if err != nil {
			return nil, err
		}
		if _, dup := out[f.Name]; dup {
			return nil, fmt.Errorf("公式名重复：%s", f.Name)
		}
		out[f.Name] = f
	}
	return out, nil
}

// Type 返回结果的静态类型。
func (f *Formula) Type() expr.FieldSpec { return f.ex.Type() }

// Eval 用给定的绑定取值求值。bindings 的键是**绑定名**（不是来源路径）。
func (f *Formula) Eval(bindings map[string]expr.Val, units *unit.Table) (expr.Val, error) {
	return f.ex.Eval(expr.MapResolver(bindings), units)
}

// CaseResult 是一条算例的执行结果。
type CaseResult struct {
	Name   string
	Passed bool
	Got    string
	Want   string
	Err    error
}

// Verify 执行全部算例。
//
// 公式状态：Verified（有算例且全过）/ Unverified（无算例）/ Failed（有算例未过）。
func (f *Formula) Verify(units *unit.Table) ([]CaseResult, Status) {
	if len(f.Cases) == 0 {
		return nil, StatusUnverified
	}
	var out []CaseResult
	allPass := true
	for _, c := range f.Cases {
		r := CaseResult{Name: c.Name, Want: c.Expect}
		in := map[string]expr.Val{}
		for name, lit := range c.Given {
			v, err := ParseLiteral(lit)
			if err != nil {
				r.Err = fmt.Errorf("算例 %s 的输入 %s 无法解析：%w", c.Name, name, err)
				allPass = false
				out = append(out, r)
				continue
			}
			in[name] = v
		}
		got, err := f.Eval(in, units)
		if err != nil {
			r.Err = err
			allPass = false
			out = append(out, r)
			continue
		}
		r.Got = got.String()
		want, err := ParseLiteral(c.Expect)
		if err != nil {
			r.Err = fmt.Errorf("期望值无法解析：%w", err)
			allPass = false
			out = append(out, r)
			continue
		}
		r.Passed = equalVals(got, want, units)
		if !r.Passed {
			allPass = false
		}
		out = append(out, r)
	}
	if allPass {
		return out, StatusVerified
	}
	return out, StatusFailed
}

// Status 是公式的验证状态。
type Status string

const (
	StatusVerified   Status = "verified"   // 有算例且全部通过
	StatusUnverified Status = "unverified" // 没有算例
	StatusFailed     Status = "failed"     // 有算例未通过 —— 拒绝使用
)

func equalVals(a, b expr.Val, units *unit.Table) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind != expr.KNumber {
		return a == b
	}
	bv := b.Num
	if units != nil && a.Unit != "" && b.Unit != "" && a.Unit != b.Unit {
		c, err := units.Convert(b.Num, b.Unit, a.Unit)
		if err != nil {
			return false
		}
		bv = c
	}
	d := a.Num - bv
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

// ParseLiteral 解析字面量，例如 "3082 point"、"80 percent"、"1.6"。
func ParseLiteral(s string) (expr.Val, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return expr.Val{}, fmt.Errorf("空字面量")
	}
	parts := strings.Fields(t)
	if len(parts) > 2 {
		return expr.Val{}, fmt.Errorf("无法解析字面量 %q", s)
	}
	n, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return expr.Val{}, fmt.Errorf("无法解析数值 %q", parts[0])
	}
	u := ""
	if len(parts) == 2 {
		u = parts[1]
	}
	return expr.Number(n, u), nil
}

// FormatBindings 把绑定取值格式化为可读文本。
func FormatBindings(b map[string]expr.Val) string {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(b[k].String())
	}
	return sb.String()
}

// FromValue 把断言取值转成表达式值。
//
// 未知 / 空 / 缺失一律转成表达式的未知值——**未知参与运算仍是未知**，
// 绝不当作 0 或默认值使用。
func FromValue(v value.Value) (expr.Val, error) {
	switch v.Kind {
	case value.Unknown, value.Null, value.Absent:
		return expr.UnknownVal(), nil
	case value.Present:
	default:
		return expr.UnknownVal(), fmt.Errorf("无法识别的取值状态 %q", v.Kind)
	}
	switch d := v.Data.(type) {
	case float64:
		return expr.Number(d, v.Unit), nil
	case string:
		return expr.Str(d), nil
	case bool:
		return expr.Bl(d), nil
	default:
		return expr.UnknownVal(), fmt.Errorf("取值 %T 无法参与计算", v.Data)
	}
}

// ToValue 把表达式值转成断言取值。
func ToValue(v expr.Val) value.Value {
	switch v.Kind {
	case expr.KNumber:
		return value.OfUnit(v.Num, v.Unit)
	case expr.KText:
		return value.Of(v.Text)
	case expr.KBool:
		return value.Of(v.Bool)
	default:
		return value.UnknownValue()
	}
}
