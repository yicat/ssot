package unit

import "testing"

func TestDefaultTableHasExpectedUnits(t *testing.T) {
	tab := Default()
	for _, n := range []string{"fraction", "percent", "point", "day", "week", "month"} {
		if !tab.Known(n) {
			t.Errorf("内置单位表缺少 %q", n)
		}
	}
	if tab.Known("nonexistent") {
		t.Error("未注册的单位不应被认作已知")
	}
	if !tab.Known("") {
		t.Error("空单位名（无量纲）应视为合法")
	}
}

// 这是本包存在的核心理由：percent 与 fraction 同维度，
// 50 与 0.5 归一后必须一致——否则会把「一致」误判成「冲突」。
func TestPercentAndFractionConvert(t *testing.T) {
	tab := Default()
	got, err := tab.Convert(50, "percent", "fraction")
	if err != nil {
		t.Fatalf("换算失败：%v", err)
	}
	if got != 0.5 {
		t.Errorf("50 percent -> fraction = %v，期望 0.5", got)
	}
	back, err := tab.Convert(0.5, "fraction", "percent")
	if err != nil || back != 50 {
		t.Errorf("0.5 fraction -> percent = %v (err=%v)，期望 50", back, err)
	}
}

func TestConvertRejectsCrossDimension(t *testing.T) {
	tab := Default()
	if _, err := tab.Convert(1, "point", "percent"); err == nil {
		t.Error("跨维度换算应当报错")
	}
}

func TestAddUnits(t *testing.T) {
	tab := Default()
	cases := []struct {
		a, b    string
		want    string
		wantErr bool
	}{
		{"point", "point", "point", false},
		{"percent", "percent", "percent", false},
		{"percent", "fraction", "percent", false}, // 同维度可换算
		{"point", "", "point", false},             // 无量纲字面量采用上下文单位
		{"", "point", "point", false},
		{"point", "percent", "", true}, // 量纲不同 —— 必须拦下
		{"day", "point", "", true},
	}
	for _, c := range cases {
		got, err := tab.AddUnits(c.a, c.b)
		if c.wantErr {
			if err == nil {
				t.Errorf("AddUnits(%q,%q) 应当报错，却得到 %q", c.a, c.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("AddUnits(%q,%q) 报错：%v", c.a, c.b, err)
		} else if got != c.want {
			t.Errorf("AddUnits(%q,%q) = %q，期望 %q", c.a, c.b, got, c.want)
		}
	}
}

func TestMulUnits(t *testing.T) {
	tab := Default()
	cases := []struct {
		a, b    string
		want    string
		wantErr bool
	}{
		{"point", "percent", "point", false}, // 攻击 × 倍率 = 攻击
		{"percent", "point", "point", false},
		{"percent", "fraction", "", false}, // 比值 × 比值 = 无量纲
		{"point", "point", "", true},       // 不支持复合单位
		{"point", "", "point", false},
	}
	for _, c := range cases {
		got, err := tab.MulUnits(c.a, c.b)
		if c.wantErr {
			if err == nil {
				t.Errorf("MulUnits(%q,%q) 应当报错，却得到 %q", c.a, c.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("MulUnits(%q,%q) 报错：%v", c.a, c.b, err)
		} else if got != c.want {
			t.Errorf("MulUnits(%q,%q) = %q，期望 %q", c.a, c.b, got, c.want)
		}
	}
}

func TestDivUnits(t *testing.T) {
	tab := Default()
	if got, err := tab.DivUnits("point", "percent"); err != nil || got != "point" {
		t.Errorf("point / percent = %q (err=%v)，期望 point", got, err)
	}
	if got, err := tab.DivUnits("point", "point"); err != nil || got != "" {
		t.Errorf("point / point = %q (err=%v)，期望无量纲", got, err)
	}
}
