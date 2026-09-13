package value

import "testing"

func TestFourStatesAreDistinguishable(t *testing.T) {
	states := []Value{Of(1), UnknownValue(), NullValue(), AbsentValue()}
	seen := map[Value]bool{}
	for _, v := range states {
		if seen[v] {
			t.Fatalf("状态不可区分：%v 与已有的重复", v)
		}
		seen[v] = true
	}
}

func TestRequiredAcceptsAllButAbsent(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"有值满足", Of(1), true},
		{"未知满足 required", UnknownValue(), true},
		{"空值满足 required", NullValue(), true},
		{"缺失不满足", AbsentValue(), false},
	}
	for _, c := range cases {
		if got := c.v.SatisfiesRequired(); got != c.want {
			t.Errorf("%s：SatisfiesRequired = %v，期望 %v", c.name, got, c.want)
		}
	}
}

func TestOnlyPresentCountsAsFilled(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"有值计入", Of(1), true},
		{"未知不计入", UnknownValue(), false},
		{"空值不计入", NullValue(), false},
		{"缺失不计入", AbsentValue(), false},
	}
	for _, c := range cases {
		if got := c.v.CountsAsFilled(); got != c.want {
			t.Errorf("%s：CountsAsFilled = %v，期望 %v", c.name, got, c.want)
		}
	}
}

func TestNumber(t *testing.T) {
	if n, err := Of(3.5).Number(); err != nil || n != 3.5 {
		t.Errorf("float64 取值失败：%v %v", n, err)
	}
	if n, err := Of(7).Number(); err != nil || n != 7 {
		t.Errorf("int 取值失败：%v %v", n, err)
	}
	if _, err := UnknownValue().Number(); err == nil {
		t.Error("未知值取数值应当报错")
	}
	if _, err := Of("abc").Number(); err == nil {
		t.Error("文本取数值应当报错")
	}
}
