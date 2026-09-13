package expr

import (
	"fmt"

	"github.com/ngnl5/ssot/internal/domain/unit"
)

// ── 抽象语法树 ──────────────────────────────────────────────────────────────

type node interface{ position() int }

type numLit struct {
	v    float64
	unit string
	p    int
}

type strLit struct {
	v string
	p int
}

type boolLit struct {
	v bool
	p int
}

type fieldRef struct {
	path string
	p    int
}

type unaryExpr struct {
	op string
	x  node
	p  int
}

type binaryExpr struct {
	op   string
	l, r node
	p    int
}

type condExpr struct {
	c, a, b node
	p       int
}

type callExpr struct {
	name string
	args []node
	p    int
}

func (n *numLit) position() int    { return n.p }
func (n *strLit) position() int    { return n.p }
func (n *boolLit) position() int   { return n.p }
func (n *fieldRef) position() int  { return n.p }
func (n *unaryExpr) position() int { return n.p }
func (n *binaryExpr) position() int {
	return n.p
}
func (n *condExpr) position() int { return n.p }
func (n *callExpr) position() int { return n.p }

// Expr 是编译后的表达式：已解析、已静态检查。
type Expr struct {
	Src        string
	root       node
	ResultType FieldSpec
}

// Type 返回静态检查得出的结果类型与单位。
func (e *Expr) Type() FieldSpec { return e.ResultType }

// ── 语法分析 ────────────────────────────────────────────────────────────────

type parser struct {
	toks  []token
	pos   int
	src   string
	units *unit.Table
}

func (p *parser) cur() token  { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) errAt(pos int, msg string) error {
	return &Error{Pos: pos, Msg: msg, Source: p.src}
}

func (p *parser) isKeyword(kw string) bool {
	t := p.cur()
	return t.kind == tIdent && t.text == kw
}

func (p *parser) isOp(op string) bool {
	t := p.cur()
	return t.kind == tOp && t.text == op
}

// parseIf 处理 if 条件 then 值 else 值
func (p *parser) parseIf() (node, error) {
	if p.isKeyword("if") {
		start := p.next().pos
		cond, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.isKeyword("then") {
			return nil, p.errAt(p.cur().pos, "if 之后需要 then")
		}
		p.next()
		a, err := p.parseIf()
		if err != nil {
			return nil, err
		}
		if !p.isKeyword("else") {
			return nil, p.errAt(p.cur().pos, "then 分支之后需要 else")
		}
		p.next()
		b, err := p.parseIf()
		if err != nil {
			return nil, err
		}
		return &condExpr{c: cond, a: a, b: b, p: start}, nil
	}
	return p.parseOr()
}

func (p *parser) parseOr() (node, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") {
		pos := p.next().pos
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = &binaryExpr{op: "or", l: l, r: r, p: pos}
	}
	return l, nil
}

func (p *parser) parseAnd() (node, error) {
	l, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		pos := p.next().pos
		r, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l = &binaryExpr{op: "and", l: l, r: r, p: pos}
	}
	return l, nil
}

func (p *parser) parseNot() (node, error) {
	if p.isKeyword("not") {
		pos := p.next().pos
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: "not", x: x, p: pos}, nil
	}
	return p.parseCmp()
}

func (p *parser) parseCmp() (node, error) {
	l, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind != tOp {
			return l, nil
		}
		switch t.text {
		case "==", "!=", "<", "<=", ">", ">=":
			p.next()
			r, err := p.parseAdd()
			if err != nil {
				return nil, err
			}
			l = &binaryExpr{op: t.text, l: l, r: r, p: t.pos}
		default:
			return l, nil
		}
	}
}

func (p *parser) parseAdd() (node, error) {
	l, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind == tOp && (t.text == "+" || t.text == "-") {
			p.next()
			r, err := p.parseMul()
			if err != nil {
				return nil, err
			}
			l = &binaryExpr{op: t.text, l: l, r: r, p: t.pos}
			continue
		}
		return l, nil
	}
}

func (p *parser) parseMul() (node, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind == tOp && (t.text == "*" || t.text == "/" || t.text == "%") {
			p.next()
			r, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			l = &binaryExpr{op: t.text, l: l, r: r, p: t.pos}
			continue
		}
		return l, nil
	}
}

func (p *parser) parseUnary() (node, error) {
	if p.isOp("-") {
		pos := p.next().pos
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: "-", x: x, p: pos}, nil
	}
	return p.parsePower()
}

func (p *parser) parsePower() (node, error) {
	l, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.isOp("^") {
		pos := p.next().pos
		r, err := p.parseUnary() // 右结合
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: "^", l: l, r: r, p: pos}, nil
	}
	return l, nil
}

func (p *parser) parsePrimary() (node, error) {
	t := p.cur()
	switch t.kind {
	case tNumber:
		p.next()
		u := t.unit
		// 数值后紧跟已知单位名时，作为单位字面量，例如 `10 point`
		if u == "" && p.cur().kind == tIdent && p.units != nil && p.units.Known(p.cur().text) {
			u = p.next().text
		}
		return &numLit{v: t.num, unit: u, p: t.pos}, nil

	case tString:
		p.next()
		return &strLit{v: t.text, p: t.pos}, nil

	case tLParen:
		p.next()
		e, err := p.parseIf()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != tRParen {
			return nil, p.errAt(p.cur().pos, "缺少右括号")
		}
		p.next()
		return e, nil

	case tIdent:
		if t.text == "true" || t.text == "false" {
			p.next()
			return &boolLit{v: t.text == "true", p: t.pos}, nil
		}
		p.next()
		// 函数调用
		if p.cur().kind == tLParen {
			p.next()
			var args []node
			if p.cur().kind != tRParen {
				for {
					a, err := p.parseIf()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.cur().kind == tComma {
						p.next()
						continue
					}
					break
				}
			}
			if p.cur().kind != tRParen {
				return nil, p.errAt(p.cur().pos, "函数调用缺少右括号")
			}
			p.next()
			return &callExpr{name: t.text, args: args, p: t.pos}, nil
		}
		// 字段引用 a.b.c
		path := t.text
		for p.cur().kind == tDot {
			p.next()
			if p.cur().kind != tIdent {
				return nil, p.errAt(p.cur().pos, "点号之后需要字段名")
			}
			path += "." + p.next().text
		}
		return &fieldRef{path: path, p: t.pos}, nil
	}

	return nil, p.errAt(t.pos, "此处需要数值、文本、字段名或括号，却遇到 "+t.String())
}

// ── 静态检查 ────────────────────────────────────────────────────────────────

// Compile 解析并静态检查表达式。
//
// 静态检查在**撰写期**完成：字段是否存在、类型是否匹配、单位是否相容。
// 这三类错误绝不应当等到运行期才暴露。
func Compile(src string, env Env, units *unit.Table) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: src, units: units}
	root, err := p.parseIf()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tEOF {
		return nil, p.errAt(p.cur().pos, "表达式末尾有多余内容："+p.cur().String())
	}
	typ, err := staticCheck(root, env, units, src)
	if err != nil {
		return nil, err
	}
	return &Expr{Src: src, root: root, ResultType: typ}, nil
}

func staticCheck(n node, env Env, units *unit.Table, src string) (FieldSpec, error) {
	bad := func(pos int, msg string) (FieldSpec, error) {
		return FieldSpec{}, &Error{Pos: pos, Msg: msg, Source: src}
	}

	switch v := n.(type) {
	case *numLit:
		return FieldSpec{Type: "number", Unit: v.unit}, nil
	case *strLit:
		return FieldSpec{Type: "text"}, nil
	case *boolLit:
		return FieldSpec{Type: "bool"}, nil

	case *fieldRef:
		if env == nil {
			return bad(v.p, "未提供字段环境，无法解析 "+v.path)
		}
		spec, ok := env.Field(v.path)
		if !ok {
			return bad(v.p, fmt.Sprintf("引用了不存在的字段 %q", v.path))
		}
		return spec, nil

	case *unaryExpr:
		x, err := staticCheck(v.x, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		switch v.op {
		case "-":
			if x.Type != "number" {
				return bad(v.p, "取负只能作用于数值，实际为 "+x.Type)
			}
			return x, nil
		case "not":
			if x.Type != "bool" {
				return bad(v.p, "not 只能作用于布尔，实际为 "+x.Type)
			}
			return FieldSpec{Type: "bool"}, nil
		}
		return bad(v.p, "未知的一元运算符 "+v.op)

	case *binaryExpr:
		l, err := staticCheck(v.l, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		r, err := staticCheck(v.r, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		return checkBinary(v.op, l, r, v.p, units, bad)

	case *condExpr:
		c, err := staticCheck(v.c, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		if c.Type != "bool" {
			return bad(v.p, "if 的条件必须是布尔，实际为 "+c.Type)
		}
		a, err := staticCheck(v.a, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		b, err := staticCheck(v.b, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		if a.Type != b.Type {
			return bad(v.p, fmt.Sprintf("if 两个分支类型不一致：%s 与 %s", a.Type, b.Type))
		}
		if a.Type == "number" && units != nil && a.Unit != "" && b.Unit != "" && !units.Compatible(a.Unit, b.Unit) {
			return bad(v.p, fmt.Sprintf("if 两个分支单位不同：%s 与 %s", a.Unit, b.Unit))
		}
		return a, nil

	case *callExpr:
		return checkCall(v, env, units, src, bad)
	}
	return bad(n.position(), "无法识别的表达式节点")
}

func checkBinary(op string, l, r FieldSpec, pos int, units *unit.Table, bad func(int, string) (FieldSpec, error)) (FieldSpec, error) {
	isNum := func(s FieldSpec) bool { return s.Type == "number" }

	switch op {
	case "+", "-":
		if !isNum(l) || !isNum(r) {
			return bad(pos, fmt.Sprintf("%s 只能作用于数值，实际为 %s 与 %s", op, l.Type, r.Type))
		}
		if units == nil {
			return FieldSpec{Type: "number", Unit: l.Unit}, nil
		}
		u, err := units.AddUnits(l.Unit, r.Unit)
		if err != nil {
			return bad(pos, err.Error())
		}
		return FieldSpec{Type: "number", Unit: u}, nil

	case "*", "/":
		if !isNum(l) || !isNum(r) {
			return bad(pos, fmt.Sprintf("%s 只能作用于数值，实际为 %s 与 %s", op, l.Type, r.Type))
		}
		if units == nil {
			return FieldSpec{Type: "number", Unit: l.Unit}, nil
		}
		var u string
		var err error
		if op == "*" {
			u, err = units.MulUnits(l.Unit, r.Unit)
		} else {
			u, err = units.DivUnits(l.Unit, r.Unit)
		}
		if err != nil {
			return bad(pos, err.Error())
		}
		return FieldSpec{Type: "number", Unit: u}, nil

	case "%":
		if !isNum(l) || !isNum(r) {
			return bad(pos, "% 只能作用于数值")
		}
		return FieldSpec{Type: "number", Unit: l.Unit}, nil

	case "^":
		if !isNum(l) || !isNum(r) {
			return bad(pos, "^ 只能作用于数值")
		}
		return FieldSpec{Type: "number", Unit: l.Unit}, nil

	case "==", "!=":
		if l.Type != r.Type {
			return bad(pos, fmt.Sprintf("比较两侧类型不同：%s 与 %s", l.Type, r.Type))
		}
		return FieldSpec{Type: "bool"}, nil

	case "<", "<=", ">", ">=":
		if !isNum(l) || !isNum(r) {
			return bad(pos, "大小比较只能作用于数值")
		}
		if units != nil && l.Unit != "" && r.Unit != "" && !units.Compatible(l.Unit, r.Unit) {
			return bad(pos, fmt.Sprintf("比较两侧单位不同：%s 与 %s", l.Unit, r.Unit))
		}
		return FieldSpec{Type: "bool"}, nil

	case "and", "or":
		if l.Type != "bool" || r.Type != "bool" {
			return bad(pos, fmt.Sprintf("%s 只能作用于布尔，实际为 %s 与 %s", op, l.Type, r.Type))
		}
		return FieldSpec{Type: "bool"}, nil
	}
	return bad(pos, "未知的二元运算符 "+op)
}

func checkCall(v *callExpr, env Env, units *unit.Table, src string, bad func(int, string) (FieldSpec, error)) (FieldSpec, error) {
	args := make([]FieldSpec, 0, len(v.args))
	for _, a := range v.args {
		s, err := staticCheck(a, env, units, src)
		if err != nil {
			return FieldSpec{}, err
		}
		args = append(args, s)
	}
	allNum := func() bool {
		for _, a := range args {
			if a.Type != "number" {
				return false
			}
		}
		return true
	}

	switch v.name {
	case "min", "max":
		if len(args) != 2 || !allNum() {
			return bad(v.p, v.name+" 需要两个数值参数")
		}
		return FieldSpec{Type: "number", Unit: args[0].Unit}, nil
	case "abs", "floor", "ceil", "round":
		if len(args) != 1 || !allNum() {
			return bad(v.p, v.name+" 需要一个数值参数")
		}
		return args[0], nil
	}
	return bad(v.p, fmt.Sprintf("未知函数 %q（可用：min, max, abs, floor, ceil, round）", v.name))
}
