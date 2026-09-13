// Package domain 持有纯业务规则。
//
// 约束：只允许 Go 标准库，禁止 import 本仓库其他层（含 infrastructure）。
// 判断落位：这段逻辑换掉外部系统之后还需要吗？需要 → 放这里。
package domain
