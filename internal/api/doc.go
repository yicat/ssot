// Package api 是暴露给前端的 wails3 绑定层。
//
// 约束：只做参数转换与调用转发，不写业务规则。业务判断一律下沉到
// domain 或 application。
package api
