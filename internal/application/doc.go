// Package application 持有用例编排：把 domain 的规则按「先 A 后 B、失败回滚」
// 的次序组织成一次可执行的用例。
//
// 约束：只依赖 domain；需要外部能力时依赖 domain 定义的端口接口，
// 而不是 infrastructure 的具体类型。
package application
