# 0002 git 当版本层，写入由能力层代跑提交

状态：已定
关联：`docs/specs/vault.spec.md` §5、`docs/specs/agent.spec.md` §3、`internal/infrastructure/vaultgit`

## 背景

要能回答「这条结论什么时候被谁改的」「改了什么」（diff）、以及「回滚到上一版」。
实现方式有两条路：自建修订存储，或者用 git。

## 决策

**vault 本身就是 git 仓库**，改动记录/diff/回滚全用 git，应用只做薄封装。
并且**写入成功后由能力层立即提交**：

1. 只 `git add` 改动的那个文件（不用 `add -A`）；
2. commit 信息两段，最后一段是 trailer `Edited-By: agent:<名字>` / `human:<名字>`；
3. `.data/` 永不提交（派生索引可重建）。

## 为什么

- 自建修订存储等于重新实现 git 能做好的事（这是旧方案被砍掉的一层）。
- **留痕不能靠人记得做**：`agent.spec.md` 的验证项写着「每次写入的 commit trailer 里有
  `Edited-By`」——如果工具只给「建议的提交信息」而不提交，这一条永远验不了。

## 否掉的选项

- **自建修订/快照存储**：要自己解决 diff、合并、回滚、体积，且与用户已有的 git 习惯并存。
- **只返回建议的提交信息、由人和流程决定何时提交**（代码里曾这么写）：结果是 agent 的改动
  大量无痕；且 trailer 的验证项形同虚设。**已按 spec 改正**。
- **`add -A` 后提交**：会把用户手边未完成的改动、以及 `.data/` 一并卷进去。

## 后果

- vault 必须是**它自己的** git 仓库：`projects/` 在工具仓库里整段忽略（否则外层把 vault 记成
  gitlink，两边版本互相搅）。⚠️ 若 vault 只是某个大仓库的子目录，提交会提到**外层仓库**去——
  `vaultgit.IsRepo()` 因此要求 `--show-toplevel` 等于 vault 根。
- vault 不是 git 仓库时：写入照常成功，但结果里明确写「本次未留痕」+ 怎么补（不静默）。
