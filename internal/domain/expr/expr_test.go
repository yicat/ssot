package expr

import (
	"strings"
	"testing"

	"github.com/ngnl5/ssot/internal/domain/unit"
)

func env() MapEnv {
	return MapEnv{
		"atk":      {Type: "number", Unit: "point"},
		"spd":      {Type: "number", Unit: "point"},
		"crit":     {Type: "number", Unit: "fraction"},
		"crit_pct": {Type: "number", Unit: "percent"},
		"name":     {Type: "text"},
		"alive":    {Type: "bool"},
		"ratio":    {Type: "number", Unit: "percent"},
	}
}

func compile(t *testing.T, src string) *Expr {
	t.Helper()
	e, err := Compile(src, env(), unit.Default())
	if err != nil {
		t.Fatalf("编译 %q 失败：%v", src, err)
	}
	return e
}

func evalNum(t *testing.T, src string, in MapResolver) float64 {
	t.Helper()
	e := compile(t, src)
	v, err := e.Eval(in, unit.Default())
	if err != nil {
		t.Fatalf("求值 %q 失败：%v", src, err)
	}
	if v.Kind != KNumber {
		t.Fatalf("求值 %q 期望数值，实际 %v", src, v)
	}
	return v.Num
}

// ── 静态检查：必须在撰写期报错 ──────────────────────────────────────────────

func TestUnknownFieldFailsAtCompileTime(t *testing.T) {
	_, err := Compile("atk * nosuchfield", env(), unit.Default())
	if err == nil {
		t.Fatal("引用不存在的字段必须在编译期报错")
	}
	if !strings.Contains(err.Error(), "nosuchfield") {
		t.Errorf("错误应指出字段名，实际：%v", err)
	}
}

func TestTypeMismatchFailsAtCompileTime(t *testing.T) {
	if _, err := Compile(`name + 1`, env(), unit.Default()); err == nil {
		t.Error("文本与数值相加应报错")
	}
	if _, err := Compile(`not atk`, env(), unit.Default()); err == nil {
		t.Error("对数值取 not 应报错")
	}
	if _, err := Compile(`if atk then 1 else 2`, env(), unit.Default()); err == nil {
		t.Error("if 条件非布尔应报错")
	}
}

// 量纲检查的核心理由：percent + point 必须被拦下。
func TestIncompatibleUnitsFailAtCompileTime(t *testing.T) {
	_, err := Compile("atk + ratio", env(), unit.Default())
	if err == nil {
		t.Fatal("percent 与 point 相加必须在编译期报错")
	}
	if !strings.Contains(err.Error(), "量纲") {
		t.Errorf("错误应说明量纲问题，实际：%v", err)
	}
}

func TestErrorCarriesPosition(t *testing.T) {
	_, err := Compile("atk + ", env(), unit.Default())
	if err == nil {
		t.Fatal("不完整的表达式应报错")
	}
	var e *Error
	if !asError(err, &e) {
		t.Fatalf("错误应带位置信息，实际 %T", err)
	}
	if e.Caret() == "" {
		t.Error("应能给出指示符定位")
	}
}

func asError(err error, out **Error) bool {
	if e, ok := err.(*Error); ok {
		*out = e
		return true
	}
	return false
}

func TestUnknownFunctionFails(t *testing.T) {
	if _, err := Compile("frobnicate(atk)", env(), unit.Default()); err == nil {
		t.Error("未知函数应报错")
	}
}

// ── 求值 ────────────────────────────────────────────────────────────────────

func TestArithmetic(t *testing.T) {
	in := MapResolver{"atk": Number(100, "point"), "spd": Number(20, "point")}
	if got := evalNum(t, "atk + spd", in); got != 120 {
		t.Errorf("atk + spd = %v，期望 120", got)
	}
	if got := evalNum(t, "atk - spd * 2", in); got != 60 {
		t.Errorf("atk - spd*2 = %v，期望 60", got)
	}
	if got := evalNum(t, "(atk + spd) / 4", in); got != 30 {
		t.Errorf("(atk+spd)/4 = %v，期望 30", got)
	}
	if got := evalNum(t, "2 ^ 10", in); got != 1024 {
		t.Errorf("2^10 = %v，期望 1024", got)
	}
}

// 这是量纲感知求值的核心理由之一：
// 100 point * 50% 必须是 50 point，而不是 5000。
func TestPercentMultiplicationConvertsToBase(t *testing.T) {
	in := MapResolver{"atk": Number(100, "point")}
	e := compile(t, "atk * 50%")
	v, err := e.Eval(in, unit.Default())
	if err != nil {
		t.Fatalf("求值失败：%v", err)
	}
	if v.Num != 50 {
		t.Errorf("100 point * 50%% = %v，期望 50", v.Num)
	}
	if v.Unit != "point" {
		t.Errorf("结果单位应为 point，实际 %q", v.Unit)
	}
}

func TestLiteralPercentParsing(t *testing.T) {
	in := MapResolver{}
	if got := evalNum(t, "50% + 50%", in); got != 100 {
		t.Errorf("50%% + 50%% = %v，期望 100", got)
	}
}

// 跨单位比较必须归一 —— 否则会把「一致」误判成「冲突」。
func TestCrossUnitComparisonNormalizes(t *testing.T) {
	in := MapResolver{
		"crit":     Number(0.5, "fraction"),
		"crit_pct": Number(50, "percent"),
	}
	e := compile(t, "crit == crit_pct")
	v, err := e.Eval(in, unit.Default())
	if err != nil {
		t.Fatalf("求值失败：%v", err)
	}
	if v.Kind != KBool || !v.Bool {
		t.Errorf("0.5 fraction 应等于 50 percent，实际 %v", v)
	}
}

func TestUnitLiteralSyntax(t *testing.T) {
	in := MapResolver{"spd": Number(113, "point")}
	if got := evalNum(t, "spd + 10 point", in); got != 123 {
		t.Errorf("spd + 10 point = %v，期望 123", got)
	}
}

func TestConditional(t *testing.T) {
	in := MapResolver{"crit": Number(0.5, "fraction"), "alive": Bl(true)}
	if got := evalNum(t, "if alive then 1 + 1 else 0", in); got != 2 {
		t.Errorf("条件为真分支错误：%v", got)
	}
	if got := evalNum(t, "if crit > 0.3 then 10 else 20", in); got != 10 {
		t.Errorf("比较条件分支错误：%v", got)
	}
}

// 未知值参与运算，结果必须仍是未知 —— 不得当作确定值。
func TestUnknownPropagates(t *testing.T) {
	in := MapResolver{"atk": UnknownVal(), "spd": Number(20, "point")}
	e := compile(t, "atk + spd")
	v, err := e.Eval(in, unit.Default())
	if err != nil {
		t.Fatalf("求值失败：%v", err)
	}
	if v.Kind != KUnknown {
		t.Errorf("未知值参与运算后应仍为未知，实际 %v", v)
	}
}

func TestUnknownConditionDoesNotPickBranch(t *testing.T) {
	in := MapResolver{"alive": UnknownVal()}
	e := compile(t, "if alive then 1 else 2")
	v, err := e.Eval(in, unit.Default())
	if err != nil {
		t.Fatalf("求值失败：%v", err)
	}
	if v.Kind != KUnknown {
		t.Errorf("条件未知时不得替调用方选分支，实际 %v", v)
	}
}

func TestDivideByZeroIsError(t *testing.T) {
	in := MapResolver{"atk": Number(1, "point")}
	e := compile(t, "atk / 0")
	if _, err := e.Eval(in, unit.Default()); err == nil {
		t.Error("除以零必须报错")
	}
}

func TestFunctions(t *testing.T) {
	in := MapResolver{"atk": Number(100, "point"), "spd": Number(20, "point")}
	if got := evalNum(t, "min(atk, spd)", in); got != 20 {
		t.Errorf("min = %v，期望 20", got)
	}
	if got := evalNum(t, "max(atk, spd)", in); got != 100 {
		t.Errorf("max = %v，期望 100", got)
	}
	if got := evalNum(t, "abs(0 - atk)", in); got != 100 {
		t.Errorf("abs = %v，期望 100", got)
	}
	if got := evalNum(t, "floor(7 / 2)", in); got != 3 {
		t.Errorf("floor = %v，期望 3", got)
	}
}

func TestShortCircuit(t *testing.T) {
	in := MapResolver{"alive": Bl(false), "atk": Number(1, "point")}
	e := compile(t, "alive and atk > 0")
	v, err := e.Eval(in, unit.Default())
	if err != nil || v.Kind != KBool || v.Bool {
		t.Errorf("false and X 应为 false，实际 %v (err=%v)", v, err)
	}
}

func TestStaticTypeAndUnitAreReported(t *testing.T) {
	e := compile(t, "atk * 50%")
	if e.Type().Type != "number" || e.Type().Unit != "point" {
		t.Errorf("静态类型应为 number/point，实际 %+v", e.Type())
	}
}

func TestMissingInputIsError(t *testing.T) {
	e := compile(t, "atk + spd")
	if _, err := e.Eval(MapResolver{"atk": Number(1, "point")}, unit.Default()); err == nil {
		t.Error("输入缺字段应报错")
	}
}
