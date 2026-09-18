package vault

import "testing"

// TestDerivedScopeDefaultsToEverything：默认不静默少收——没声明就全收。
func TestDerivedScopeDefaultsToEverything(t *testing.T) {
	var s DerivedScope
	if !s.Empty() {
		t.Error("没写规则时 Empty() 该是真")
	}
	for _, p := range []string{"docs/式神/x.md", "raw/剧情/y.md", "tables/a.csv"} {
		ok, why := s.Covers(p, nil)
		if !ok {
			t.Errorf("%q 在空声明下该收（理由：%s）", p, why)
		}
	}
}

// TestDerivedScopeExcludeAndInclude：排除生效；include 是例外（优先于 exclude）。
func TestDerivedScopeExcludeAndInclude(t *testing.T) {
	s := DerivedScope{
		Exclude: ScopeRule{
			Paths: []string{"raw/DIR_X/**"},
			Tags:  []string{"TAG_X"},
		},
		Include: ScopeRule{
			Paths: []string{"raw/DIR_X/例外/**"},
		},
	}
	cases := []struct {
		path string
		tags []string
		want bool
	}{
		{"raw/DIR_X/x.md", nil, false},
		{"raw/DIR_X/deep/y.md", nil, false},           // `/**` 管到子目录
		{"raw/DIR_X", nil, false},                     // 目录本身也算命中
		{"docs/DIR_X/x.md", []string{"TAG_X"}, false}, // 标签命中（路径不在排除里）
		{"raw/DIR_X/例外/z.md", nil, true},              // include 是例外
		{"raw/别的/x.md", nil, true},
		{"docs/机制/x.md", []string{"整理层"}, true},
		{"raw\\DIR_X\\win.md", nil, false}, // Windows 分隔符
	}
	for _, c := range cases {
		got, why := s.Covers(c.path, c.tags)
		if got != c.want {
			t.Errorf("Covers(%q, %v) = %v（%s），想要 %v", c.path, c.tags, got, why, c.want)
		}
		if why == "" {
			t.Errorf("Covers(%q) 该给出人话理由", c.path)
		}
	}
}

// TestExtractConfigHasNoBuiltinKnowledge：词表为空时只有兜底 Other——
// 代码里不许预置任何数据词（这条是 spec 的验收条款）。
func TestExtractConfigHasNoBuiltinKnowledge(t *testing.T) {
	var c ExtractConfig
	for _, in := range []string{"TYPE_A", "TYPE_B", "Other", ""} {
		if got := c.NormalizeType(in); got != OtherType {
			t.Errorf("空词表时 NormalizeType(%q) 该是 %s，实际 %q", in, OtherType, got)
		}
	}
	if ok, _ := c.ShouldIgnoreName("NAME_X"); ok {
		t.Error("空配置时不该忽略任何名字（那等于代码里预置了数据知识）")
	}
}

func TestExtractConfigNormalizeType(t *testing.T) {
	c := ExtractConfig{EntityTypes: []string{" TYPE_A ", "TYPE_B"}}
	cases := map[string]string{
		"TYPE_A": "TYPE_A", // 命中词表：按声明里的写法回
		"type_a": "TYPE_A", // 大小写不敏感
		"TYPE_B": "TYPE_B",
		"别的类型":   OtherType,
		"":       OtherType,
	}
	for in, want := range cases {
		if got := c.NormalizeType(in); got != want {
			t.Errorf("NormalizeType(%q) = %q，想要 %q", in, got, want)
		}
	}
}

// TestExtractConfigIgnoreNamePatterns：忽略规则是 glob（形状），不是数据清单。
func TestExtractConfigIgnoreNamePatterns(t *testing.T) {
	c := ExtractConfig{
		IgnoreNamePatterns: []string{"*.*", "*/*", "*<*", "--*", "tool__*"},
		EmptyWords:         []string{" EMPTY_X "},
	}
	ignore := []string{"a.md", "t.csv", "dir/x", "<名字>", "--patch", "tool__read", "EMPTY_X", "  "}
	for _, n := range ignore {
		if ok, why := c.ShouldIgnoreName(n); !ok {
			t.Errorf("%q 该被忽略（%s）", n, why)
		}
	}
	keep := []string{"NAME_X", "NAME_X技能", "TYPE_A"}
	for _, n := range keep {
		if ok, why := c.ShouldIgnoreName(n); ok {
			t.Errorf("%q 不该被忽略：%s", n, why)
		}
	}
}
