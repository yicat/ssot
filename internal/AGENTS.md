# internal/

## 分层铁律

依赖方向：`api → application → domain ← infrastructure`，**禁止反向**。

- `domain/`：纯业务规则，**只允许 Go 标准库**，禁止 import 本仓库其他层
- `application/`：用例编排（先 A 后 B、失败回滚这类流程）
- `infrastructure/`：外部适配（存储/设备/日志），实现 `domain` 定义的端口
- `api/`：暴露给前端的 wails3 绑定层，只做参数转换与调用转发，不写业务规则

## 落位判断

问自己：**这段逻辑换掉外部系统后还需要吗？** 需要 → `domain`。

## 动这一层之前

加载 `layering-guard` skill，它带了依赖检查命令与常见违规对照表。

## 提交前

`go vet ./...` + `wails3 task test` 通过。
