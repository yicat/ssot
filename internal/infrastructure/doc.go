// Package infrastructure 持有外部适配：文件系统、存储、进程、网络等具体实现。
//
// 约束：负责实现 domain 定义的端口接口；可以依赖 domain，
// 不被 domain 依赖。
package infrastructure
