package formula

import (
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/unit"
)

const critFactor = `
formula: crit_factor
version: "1"
params:
  cri:  {type: number, unit: fraction}
  crid: {type: number, unit: fraction}
bindings:
  cri: cri
  crid: crid
result: 1 + cri * crid
cases:
  - name: 姑获鸟
    given: {cri: "0.5 fraction", crid: "1.2 fraction"}
    expect: "1.6"
  - name: 百分数口径等价
    given: {cri: "50 percent", crid: "120 percent"}
    expect: "1.6"
`

// 有算例且全部通过 -> 已验证。
func TestVerifiedFormula(t *testing.T) {
	f, err := Parse(critFactor, unit.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	cases, st := f.Verify(unit.Default())
	if st != StatusVerified {
		t.Fatalf("应为 verified，实际 %s（算例 %+v）", st, cases)
	}
	for _, c := range cases {
		if !c.Passed {
			t.Errorf("算例 %s 未通过：得到 %s 期望 %s（%v）", c.Name, c.Got, c.Want, c.Err)
		}
	}
}

// 没有算例 -> 允许使用，但标注未验证。
func TestUnverifiedFormula(t *testing.T) {
	src := strings.Replace(critFactor, "cases:", "cases_x:", 1)
	src = src[:strings.Index(src, "cases_x:")]
	f, err := Parse(src, unit.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, st := f.Verify(unit.Default()); st != StatusUnverified {
		t.Errorf("无算例应为 unverified，实际 %s", st)
	}
}

// 算例未通过 -> 必须拒绝使用。
func TestFailedCaseMakesFormulaUnusable(t *testing.T) {
	src := strings.Replace(critFactor, `expect: "1.6"`, `expect: "9.9"`, 1)
	f, err := Parse(src, unit.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	_, st := f.Verify(unit.Default())
	if st != StatusFailed {
		t.Errorf("算例未通过应为 failed，实际 %s", st)
	}
}

// 静态检查必须在撰写期发现字段问题。
func TestCompileRejectsUnknownBinding(t *testing.T) {
	src := `
formula: bad
params:
  cri: {type: number, unit: fraction}
bindings:
  cri: cri
result: cri + nosuch
`
	if _, err := Parse(src, unit.Default(), "test"); err == nil {
		t.Error("引用未声明的绑定必须在编译期报错")
	}
}

// 量纲不相容也必须在撰写期发现。
func TestCompileRejectsIncompatibleUnits(t *testing.T) {
	src := `
formula: bad2
params:
  a: {type: number, unit: point}
  b: {type: number, unit: percent}
bindings:
  a: a
  b: b
result: a + b
`
	_, err := Parse(src, unit.Default(), "test")
	if err == nil {
		t.Fatal("point + percent 必须在编译期报错")
	}
	if !strings.Contains(err.Error(), "量纲") {
		t.Errorf("错误应说明量纲问题，实际：%v", err)
	}
}

func TestBindingWithoutTypeDeclarationIsRejected(t *testing.T) {
	src := `
formula: bad3
params:
  a: {type: number, unit: point}
bindings:
  a: a
  undeclared: x
result: a
`
	if _, err := Parse(src, unit.Default(), "test"); err == nil {
		t.Error("绑定缺少类型声明必须被拒绝——否则无法做撰写期检查")
	}
}

func TestParseLiteral(t *testing.T) {
	cases := map[string]float64{
		"3082 point":   3082,
		"80 percent":   80,
		"1.6":          1.6,
		"0.5 fraction": 0.5,
	}
	for lit, want := range cases {
		v, err := ParseLiteral(lit)
		if err != nil {
			t.Errorf("解析 %q 失败：%v", lit, err)
			continue
		}
		if v.Num != want {
			t.Errorf("解析 %q 得到 %v，期望 %v", lit, v.Num, want)
		}
	}
	if _, err := ParseLiteral(""); err == nil {
		t.Error("空字面量应报错")
	}
	if _, err := ParseLiteral("abc"); err == nil {
		t.Error("非数值应报错")
	}
}
