// 通用的**形状**规则（不是数据清单）。
//
// 这一条只管一种形状：**纯数字**（`2026`、`1.2`）——序号、年份、裸数值都不是实体名。
// 带单位的量（`263%`、`+30`）**不算纯数字**：词表里可以合法地有「数值」这类实体。
//
// ⚠️ 除它之外的「哪些名字不该当实体」（文件名、路径、占位符、命令/工具名……）**全部由项目声明**
// （`.ssot/derived-scope.yml` 的 `extract.ignore.name_patterns`）。
// 代码里**不许**再出现任何具体的数据词——那正是我们一起踩过的坑（把一次性的数据判断写成代码规则）。
package vextract

import "unicode"

// isPlainNumber 判断是不是「只有数字与小数点/逗号/负号/空格」。
func isPlainNumber(s string) bool {
	hasDigit := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
		// 只认纯数字：带单位/符号的（263%、+30）不是序号，是量——别拦。
		case r == '.' || r == ',' || r == '-' || r == ' ':
		default:
			return false
		}
	}
	return hasDigit
}
