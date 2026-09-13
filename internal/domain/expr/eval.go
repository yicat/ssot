package expr

import (
	"fmt"
	"math"

	"github.com/ngnl5/ssot/internal/domain/unit"
)

// Resolver 在运行期提供字段取值。
type Resolver interface {
	Resolve(path string) (Val, bool)
}

// MapResolver 是 Resolver 的简单实现。
type MapResolver map[string]Val

func (m MapResolver) Resolve(path string) (Val, bool) {
	v, ok := m[path]
	return v, ok
}

// Eval 求值。units 为 nil 时跳过运行期单位换算（静态检查仍然生效）。
func (e *Expr) Eval(in Resolver, units *unit.Table) (Val, error) {
	return eval(e.root, in, units)
}

func eval(n node, in Resolver, units *unit.Table) (Val, error) {
	switch v := n.(type) {
	case *numLit:
		return Number(v.v, v.unit), nil
	case *strLit:
		return Str(v.v), nil
	case *boolLit:
		return Bl(v.v), nil

	case *fieldRef:
		if in == nil {
			return UnknownVal(), fmt.Errorf("第 %d 列：没有可用的输入，无法取字段 %s", v.p+1, v.path)
		}
		val, ok := in.Resolve(v.path)
		if !ok {
			return UnknownVal(), fmt.Errorf("第 %d 列：输入中缺少字段 %s", v.p+1, v.path)
		}
		return val, nil

	case *unaryExpr:
		x, err := eval(v.x, in, units)
		if err != nil {
			return UnknownVal(), err
		}
		if x.Kind == KUnknown {
			return UnknownVal(), nil
		}
		switch v.op {
		case "-":
			return Number(-x.Num, x.Unit), nil
		case "not":
			return Bl(!x.Bool), nil
		}
		return UnknownVal(), fmt.Errorf("第 %d 列：未知一元运算符 %s", v.p+1, v.op)

	case *condExpr:
		c, err := eval(v.c, in, units)
		if err != nil {
			return UnknownVal(), err
		}
		if c.Kind == KUnknown {
			// 条件未知时不能替调用方选分支
			return UnknownVal(), nil
		}
		if c.Kind != KBool {
			return UnknownVal(), fmt.Errorf("第 %d 列：if 的条件不是布尔", v.p+1)
		}
		if c.Bool {
			return eval(v.a, in, units)
		}
		return eval(v.b, in, units)

	case *binaryExpr:
		return evalBinary(v, in, units)

	case *callExpr:
		return evalCall(v, in, units)
	}
	return UnknownVal(), fmt.Errorf("无法求值的表达式节点")
}

func evalBinary(v *binaryExpr, in Resolver, units *unit.Table) (Val, error) {
	// and / or 需要短路，单独处理
	if v.op == "and" || v.op == "or" {
		l, err := eval(v.l, in, units)
		if err != nil {
			return UnknownVal(), err
		}
		if l.Kind == KBool {
			if v.op == "and" && !l.Bool {
				return Bl(false), nil
			}
			if v.op == "or" && l.Bool {
				return Bl(true), nil
			}
		}
		r, err := eval(v.r, in, units)
		if err != nil {
			return UnknownVal(), err
		}
		if l.Kind == KUnknown || r.Kind == KUnknown {
			return UnknownVal(), nil
		}
		if v.op == "and" {
			return Bl(l.Bool && r.Bool), nil
		}
		return Bl(l.Bool || r.Bool), nil
	}

	l, err := eval(v.l, in, units)
	if err != nil {
		return UnknownVal(), err
	}
	r, err := eval(v.r, in, units)
	if err != nil {
		return UnknownVal(), err
	}
	// 未知值参与运算，结果仍是未知——不得当作确定值
	if l.Kind == KUnknown || r.Kind == KUnknown {
		return UnknownVal(), nil
	}

	switch v.op {
	case "+", "-":
		lv, rv, err := align(r, l, units)
		if err != nil {
			return UnknownVal(), posErr(v.p, err)
		}
		if v.op == "+" {
			return Number(l.Num+rv, l.Unit), nil
		}
		_ = lv
		return Number(l.Num-rv, l.Unit), nil

	case "*":
		a, b := scaleRatios(l, r, units)
		return finish(a*b, mustUnit(units.MulUnits(l.Unit, r.Unit)), v.p)

	case "/":
		den := r.Num
		if units != nil {
			if d, ok := ratioBase(r, units); ok {
				den = d
			}
		}
		if den == 0 {
			return UnknownVal(), posErr(v.p, fmt.Errorf("除以零"))
		}
		num := l.Num
		if units != nil {
			if d, ok := ratioBase(l, units); ok {
				num = d
			}
		}
		// 同单位相除时，右侧先归一到左侧单位
		if l.Unit != "" && l.Unit == r.Unit {
			num, den = l.Num, r.Num
		}
		return finish(num/den, mustUnit(units.DivUnits(l.Unit, r.Unit)), v.p)

	case "%":
		if r.Num == 0 {
			return UnknownVal(), posErr(v.p, fmt.Errorf("取模的除数为零"))
		}
		return Number(math.Mod(l.Num, r.Num), l.Unit), nil

	case "^":
		return finish(math.Pow(l.Num, r.Num), l.Unit, v.p)

	case "==":
		return Bl(looseEqual(l, r, units)), nil
	case "!=":
		return Bl(!looseEqual(l, r, units)), nil

	case "<", "<=", ">", ">=":
		lv, rv, err := align(r, l, units)
		if err != nil {
			return UnknownVal(), posErr(v.p, err)
		}
		lv = l.Num
		switch v.op {
		case "<":
			return Bl(lv < rv), nil
		case "<=":
			return Bl(lv <= rv), nil
		case ">":
			return Bl(lv > rv), nil
		default:
			return Bl(lv >= rv), nil
		}
	}
	return UnknownVal(), posErr(v.p, fmt.Errorf("未知二元运算符 %s", v.op))
}

func evalCall(v *callExpr, in Resolver, units *unit.Table) (Val, error) {
	args := make([]Val, 0, len(v.args))
	for _, a := range v.args {
		x, err := eval(a, in, units)
		if err != nil {
			return UnknownVal(), err
		}
		args = append(args, x)
	}
	for _, a := range args {
		if a.Kind == KUnknown {
			return UnknownVal(), nil
		}
	}
	switch v.name {
	case "min":
		if args[0].Num <= args[1].Num {
			return args[0], nil
		}
		return args[1], nil
	case "max":
		if args[0].Num >= args[1].Num {
			return args[0], nil
		}
		return args[1], nil
	case "abs":
		return Number(math.Abs(args[0].Num), args[0].Unit), nil
	case "floor":
		return Number(math.Floor(args[0].Num), args[0].Unit), nil
	case "ceil":
		return Number(math.Ceil(args[0].Num), args[0].Unit), nil
	case "round":
		return Number(math.Round(args[0].Num), args[0].Unit), nil
	}
	return UnknownVal(), posErr(v.p, fmt.Errorf("未知函数 %s", v.name))
}

// align 把 r 归一到 l 的单位，返回 (l 的值, 归一后的 r 值)。
func align(r, l Val, units *unit.Table) (float64, float64, error) {
	rv := r.Num
	if units == nil || l.Unit == "" || r.Unit == "" || l.Unit == r.Unit {
		return l.Num, rv, nil
	}
	c, err := units.Convert(r.Num, r.Unit, l.Unit)
	if err != nil {
		return 0, 0, err
	}
	return l.Num, c, nil
}

// ratioBase 报告该值是否属于比值维度，并返回其基准值（fraction 口径）。
func ratioBase(v Val, units *unit.Table) (float64, bool) {
	if units == nil {
		return 0, false
	}
	u, ok := units.Lookup(v.Unit)
	if !ok || u.Dimension != unit.Ratio {
		return 0, false
	}
	return v.Num * u.Scale, true
}

// scaleRatios 在乘法中把比值操作数换算到基准口径。
//
// 这是 `100 point * 50 percent` 必须得到 50（而非 5000）的原因。
func scaleRatios(l, r Val, units *unit.Table) (float64, float64) {
	lv, rv := l.Num, r.Num
	if units == nil {
		return lv, rv
	}
	if b, ok := ratioBase(l, units); ok {
		lv = b
	}
	if b, ok := ratioBase(r, units); ok {
		rv = b
	}
	return lv, rv
}

func mustUnit(u string, err error) string {
	if err != nil {
		return ""
	}
	return u
}

func finish(n float64, u string, pos int) (Val, error) {
	if math.IsInf(n, 0) || math.IsNaN(n) {
		return UnknownVal(), posErr(pos, fmt.Errorf("数值溢出或未定义（%v）", n))
	}
	return Number(n, u), nil
}

func looseEqual(l, r Val, units *unit.Table) bool {
	if l.Kind != r.Kind {
		return false
	}
	switch l.Kind {
	case KNumber:
		_, rv, err := align(r, l, units)
		if err != nil {
			return false
		}
		return l.Num == rv
	case KText:
		return l.Text == r.Text
	case KBool:
		return l.Bool == r.Bool
	}
	return false
}

func posErr(pos int, err error) error {
	return fmt.Errorf("第 %d 列：%v", pos+1, err)
}
