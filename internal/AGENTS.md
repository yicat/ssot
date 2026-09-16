# internal/

> 动手写代码之前先看根 `AGENTS.md`：**要改到代码的，先讨论、形成 spec 再动手**；
> 已定下的规范不得擅自更改。

## 分层铁律

依赖方向：`api → application → domain ← infrastructure`，**禁止反向**。

- `domain/`：纯业务规则，**只允许 Go 标准库**，禁止 import 本仓库其他层
- `application/`：用例编排（先 A 后 B、失败回滚这类流程）
- `infrastructure/`：外部适配（存储/设备/日志），实现 `domain` 定义的端口
- `api/`：暴露给前端的 wails3 绑定层，只做参数转换与调用转发，不写业务规则

## 落位判断

问自己：**这段逻辑换掉外部系统后还需要吗？** 需要 → `domain`。

## 动这一层之前

先确认落位（见上）；当前 `domain/` 与 `application/` **都还不存在**——
新方案定下来再建，不要为了「结构完整」先摆空目录。

依赖检查（纯手工，旧的 `layering-guard` skill 已随旧方案删除）：

```powershell
# domain 是否混入了非标准库依赖或反向引用
go list -deps ./internal/domain/... | Select-String -NotMatch '^(internal/|vendor/|errors|fmt|sort|strings|strconv|time|math|regexp|encoding/|unicode|cmp|slices|maps|iter|sync|context|os|path|io|bufio|bytes|hash|log|net|reflect|runtime|testing|gopkg.in/yaml.v3|github.com/ngnl5/ssot)'
```

## 提交前

`go vet ./...` + `wails3 task test` 通过。
