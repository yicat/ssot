// 确定性过滤：把「明显不该当实体」的名字在**代码里**丢掉。
//
// 为什么不能靠提示词（P3 实测的教训，见 docs/notes/extraction-spike.md）：
// 提示词加了一轮「不要文件名/命令示例」之后，`Other` 占比确实从 28% 掉到 5.6%——
// 但复查发现坏例**没消失，只是换了类型**：`docs/式神/<名字>.md` 被标成「物品」、
// `tables/式神技能.csv` 也是「物品」、`ssot vault` 落 Other。
// 换句话说：**劝模型少犯错，会把错误推到更难发现的地方**。
//
// 所以这里做**确定的、可测的**判定：命中就丢，并给出人话原因（进 Result.Dropped）。
// 规则宁少勿多——只拦「一眼就不是实体」的形状，别把真正的实体名误伤（比如「罗生门」）。
package vextract

import (
	"strings"
	"unicode"
)

// 空占位词：抽出来也只是噪声。
var noiseWords = map[string]bool{
	"无": true, "待定": true, "其他": true, "未知": true, "暂无": true,
	"n/a": true, "na": true, "null": true, "none": true, "-": true, "—": true, "?": true,
}

// 命令/工具名的片段（小写比较）。命中说明是**用法示例里的词**，不是正文里的东西。
var noiseMarkers = []string{"ssot ", "dsh ", "mcp__", " npm ", " git ", "go run", "--patch", "--profile", "http://", "https://"}

// 文件后缀：抽到这些说明模型把文件当成了实体。
var noiseExts = []string{".md", ".csv", ".json", ".yml", ".yaml", ".txt", ".html", ".png", ".jpg", ".js", ".go", ".exe"}

// LooksLikeNoise 判断一个名字是不是「一眼就不该当实体」。第二个返回值是给人看的原因。
func LooksLikeNoise(name string) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" {
		return true, "名字是空的"
	}
	lower := strings.ToLower(n)

	for _, ext := range noiseExts {
		if strings.HasSuffix(lower, ext) {
			return true, "像文件名（" + ext + "），不是正文里的实体"
		}
	}
	if strings.ContainsAny(n, `/\`) {
		return true, "像路径，不是正文里的实体"
	}
	if strings.ContainsAny(n, `<>{}`) || strings.Contains(n, "$") {
		return true, "像占位符/模板（`<…>` 这种），不是真实体"
	}
	for _, m := range noiseMarkers {
		if strings.Contains(lower, m) {
			return true, "像命令或工具名（用法示例里的词）"
		}
	}
	if noiseWords[lower] {
		return true, "空占位词"
	}
	if isDigitsOnly(n) {
		return true, "只有数字（序号/年份/数值），不是实体名"
	}
	if len([]rune(n)) > 60 {
		return true, "太长（>60 字），不像实体名"
	}
	return false, ""
}

// isDigitsOnly 判断是不是「只有数字与小数点/百分号/负号」。
func isDigitsOnly(s string) bool {
	hasDigit := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
		// 只认「纯数字」：带单位/符号的（263%、×2、+30）**不是**序号，是量——别拦。
		case r == '.' || r == ',' || r == '-' || r == ' ':
		default:
			return false
		}
	}
	return hasDigit
}
