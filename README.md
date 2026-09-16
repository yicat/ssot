# ssot

单一事实源（Single Source of Truth）工具。技术栈：Go + Wails v3 + Vite / React / shadcn，构建编排走 Taskfile。

> ⚠️ **当前是骨架，方案重写中。**
> 上一套方案（六部件 + 断言库 + 核验流程）已整体作废：业务代码与规格集（`docs/specs/`）
> 已清空，只保留技术栈、装配骨架与项目发现能力。新的设计定下来之后再往里长。
>
> 作废的那套实现（规格、领域、用例、接口、界面、测试）存在分支 `legacy/mvp-v1` 上备查；
> 对它为什么不成立的判断记录见 `docs/notes/ssot-conformance.md`。

## 快速开始

```powershell
# 前端依赖（首次）
cd frontend; npm install

# 开发运行（热重载）
wails3 task dev

# 服务模式（浏览器打开 http://localhost:8080）
wails3 task run:server

# 测试与检查
wails3 task test          # go test ./... + 前端 vitest
wails3 task check         # go vet + 全部测试 + 前端构建（提交前跑）

# 构建
wails3 task build         # 产物 bin/ssot.exe（GUI）
```

> `task` 无需单独安装——Wails v3 自带 `wails3 task`。

## 骨架里现在有什么

```
main.go                      Wails 应用入口（服务列表只剩「项目能列出来、能切」）
cmd/ssot/main.go             CLI 入口（只剩 help —— 不铺空壳子命令）
internal/compose/            组合根：会话 + 项目装配（会话切换是全有或全无）
internal/api/project.go      项目接口：Projects / Open / Current
internal/infrastructure/
  └─ projectfile/            project.yml 的读取与项目发现
frontend/src/App.tsx         最小外壳：顶栏列项目、可切换，正文说明骨架状态
frontend/src/components/ui/  shadcn 生成的原件（勿手改）
frontend/bindings/           wails3 generate bindings 生成（勿手改）
projects/onmyoji/            项目数据与定义（**保留**：units.yml、formulas、schema 等是实测内容）
tools/wiki/                  灰机 wiki 抓取与体检脚本（Node，保留）
docs/notes/                  调研与实测记录（保留；非规范，可能过期）
```

## 重写时沿用的约定

1. **分层铁律**：`api → application → domain ← infrastructure`；`domain` 只允许 Go 标准库；
   组合根 `compose` 是唯一允许同时依赖各层的包。
2. **前端落位**：`components/ui/` 与 `bindings/` 不手改；业务组件落 `components/custom/<Name>/`
   （`index.tsx` + `useXxx.ts` + `store.ts`）；`pages/` 只做编排。
3. **改动 Go 侧导出后要重新生成 bindings**：
   `wails3 generate bindings -ts -d frontend/bindings`。
4. **提交前跑 `wails3 task check`**。

环境与已知坑（TLS、代理、沙箱、灰机抓取）见 `AGENTS.md`。
