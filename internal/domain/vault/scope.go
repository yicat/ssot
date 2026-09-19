package vault

import (
	"path"
	"strings"
)

// 派生层的**收录范围与抽取配置**：纯规则，一个数据词都没有。
//
// 这一层只回答两件事：
//   - `DerivedScope.Covers`：这篇文档要不要进派生层（抽取 / 嵌入 / 检索共用这一份判断）；
//   - `ExtractConfig`：抽取时「实体类型词表」「哪些名字形状要忽略」是什么。
//
// ⚠️ **代码里不许有默认数据知识**（词表、排除路径、噪声词一律不许写死）：
// 默认是「全收 + 只有兜底 Other」。范围与词表全部来自**每个项目自己的**
// `<vault>/.ssot/derived-scope.yml`（见 docs/specs/derived.spec.md「收录范围」一节）。
//
// 为什么在 domain：换掉存储与模型，这两条判断照样成立；而且它必须**确定**——
// 同样的声明 + 同样的文档，结果永远一样。

// ScopeRule 是一条收录规则：路径 glob（用 path.Match 的语法）与标签。
type ScopeRule struct {
	// Paths 是路径 glob。约定：`xx/**` 表示「xx 下的全部」（`**` 只在结尾有意义，够用且可预测）。
	Paths []string
	// Tags 命中 front matter 的 tags（精确匹配，去空白）。
	Tags []string
}

// DerivedScope 是一个项目的收录范围。
//
// 语义（写死的是语义，不是内容）：
//   - 什么都不写 → **全收**（默认不静默少收）；
//   - 命中 Exclude → 排除；
//   - 命中 Include → 收回（**Include 优先于 Exclude**，用来开例外）。
type DerivedScope struct {
	Exclude ScopeRule
	Include ScopeRule
}

// Covers 报告一篇文档要不要进派生层；第二个返回值是人话理由（要能显示给人看）。
func (s DerivedScope) Covers(p string, tags []string) (bool, string) {
	norm := NormalizeSlash(p)
	if pat, ok := matchRule(s.Include, norm, tags); ok {
		return true, "include 命中：" + pat
	}
	if pat, ok := matchRule(s.Exclude, norm, tags); ok {
		return false, "exclude 命中：" + pat
	}
	return true, "没有规则命中：默认全收"
}

// 空说明这个项目还没声明范围。
func (s DerivedScope) Empty() bool {
	return len(s.Exclude.Paths)+len(s.Exclude.Tags)+len(s.Include.Paths)+len(s.Include.Tags) == 0
}

// ExtractConfig 是抽取的配置（每个项目一份）。
type ExtractConfig struct {
	// EntityTypes 是实体类型词表。**空 = 只有兜底 Other**（不是「随便写」）。
	EntityTypes []string
	// IgnoreNamePatterns 是按「名字形状」忽略的 glob（例如 `*.*`、`*/*`、`*<*`、`mcp__*`）。
	// 命中就丢——**判定在代码里是通用的 glob 匹配，具体形状由项目声明**。
	IgnoreNamePatterns []string
	// EmptyWords 是空占位词（例如「无」「待定」），精确匹配。
	EmptyWords []string
	// Examples 是提示词里给模型看的反例（由声明提供，不由代码写死）。
	Examples []string
}

// OtherType 是兜底类型：不在词表里的一律归它（LightRAG 的约定）。
const OtherType = "Other"

// NormalizeType 把模型给的类型收敛到词表：命中就原样（按声明里的写法），否则 Other。
func (c ExtractConfig) NormalizeType(t string) string {
	t = strings.TrimSpace(t)
	for _, k := range c.EntityTypes {
		if strings.EqualFold(strings.TrimSpace(k), t) && t != "" {
			return strings.TrimSpace(k)
		}
	}
	return OtherType
}

// ShouldIgnoreName 判断一个实体名要不要按「名字形状」忽略；第二个返回值是人话理由。
//
// `name` 为空也算忽略（没什么可记的）。词表里的是 glob（`path.Match` 语法），不是数据清单。
func (c ExtractConfig) ShouldIgnoreName(name string) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" {
		return true, "名字是空的"
	}
	for _, w := range c.EmptyWords {
		if strings.EqualFold(strings.TrimSpace(w), n) {
			return true, "声明里的空占位词：" + strings.TrimSpace(w)
		}
	}
	for _, pat := range c.IgnoreNamePatterns {
		if pat == "" {
			continue
		}
		if ok, err := path.Match(pat, n); err == nil && ok {
			return true, "名字形状命中声明里的忽略规则：" + pat
		}
	}
	return false, ""
}

// matchRule 判断规则有没有命中，返回命中的那一条（给人看的理由）。
func matchRule(r ScopeRule, p string, tags []string) (string, bool) {
	for _, pat := range r.Paths {
		if pat == "" {
			continue
		}
		if MatchPathGlob(pat, p) {
			return "路径 " + pat, true
		}
	}
	for _, t := range r.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		for _, have := range tags {
			if strings.EqualFold(strings.TrimSpace(have), t) {
				return "标签 " + t, true
			}
		}
	}
	return "", false
}

// MatchPathGlob 匹配一个路径 glob（收录范围与批量删除共用这一份实现）。
//
// 约定：`dir/**` 表示「dir 下的全部」；其余交给 `path.Match`（它的 `*` 不跨 `/`，
// 正合我们想要的「一层」语义）。⚠️ 不支持把 `**` 放在中间——需要就说清楚再加，
// 别让语义变成猜的。
func MatchPathGlob(pattern, p string) bool {
	pat := NormalizeSlash(pattern)
	if strings.HasSuffix(pat, "/**") {
		prefix := strings.TrimSuffix(pat, "/**")
		return p == prefix || strings.HasPrefix(p, prefix+"/")
	}
	if pat == "**" {
		return true
	}
	ok, err := path.Match(pat, p)
	return err == nil && ok
}

// NormalizeSlash 把反斜杠换成 `/`（Windows 路径也走同一套判断）。
func NormalizeSlash(s string) string { return strings.ReplaceAll(s, "\\", "/") }
