---
name: layering-guard
description: 当在 internal/ 下新增包、添加 import、或判断一段代码该落在哪一层时使用。检查依赖方向 api → application → domain ← infrastructure 是否被破坏，以及 domain 是否混入非标准库依赖或反向引用其他层。
---

# 分层守卫

## 依赖方向（禁止反向）

```
api → application → domain ← infrastructure
```

- `internal/domain/`：纯业务规则，**只允许 Go 标准库**，禁止 import 本仓库任何其他层
- `internal/application/`：用例编排，可依赖 `domain`
- `internal/infrastructure/`：外部适配（存储/设备/日志），实现 domain 定义的端口
- `internal/api/`：暴露给前端的绑定层，只做参数转换与调用转发，不写业务规则

## 新增代码落位

1. 与外部世界无关的业务规则 → `domain`
2. 「先 A 后 B、失败回滚」的流程编排 → `application`
3. 某个外部系统的具体实现 → `infrastructure`
4. 给前端的入口方法 → `api`

拿不准时问：**这段逻辑换掉外部系统后还需要吗？** 需要 → `domain`。

## 检查命令

```powershell
# domain 依赖了哪些本仓库内部包——只应看到 domain 自身
go list -deps ./internal/domain/... | Select-String "ssot/internal"

# domain 是否混入非标准库依赖
go list -f '{{join .Imports "\n"}}' ./internal/domain/... | Select-String "\." | Select-String -NotMatch "^\s*$"
```

## 常见违规

| 违规 | 修法 |
|---|---|
| `domain` 直接 import `infrastructure`（调数据库/设备 API） | 在 `domain` 定义接口，由 `infrastructure` 实现 |
| `domain` import 第三方库 | 引入端口接口，把实现推出去 |
| `api` 层写业务判断（`if` 业务条件） | 下沉到 `domain` 或 `application` |
| `application` 依赖 `infrastructure` 的具体类型 | 改为依赖 `domain` 端口接口 |
| 新组件放 `components/ui/` | 应为 `components/custom/<Name>/` |
| 页面里写交互逻辑 | 移到 `useXxx.ts`，页面只做编排 |

## 交付前

- [ ] `go vet ./...` 通过
- [ ] `wails3 task test` 通过
- [ ] 新增的跨层依赖都有端口接口背书
