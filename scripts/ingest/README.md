# scripts/ingest/ —— 导入：外部数据 → vault

这一组把**外部来的数据**灌进一个 vault。与 `check/` 不同：那些是只读探针，这些**会写文件**。

| 脚本 | 一句话 | 依赖 | 写什么 |
|---|---|---|---|
| `huiji-to-vault.mjs` | 把灰机 wiki「Data:」命名空间的抓取导出转成 vault（raw 原文 + 原始 JSON + tables CSV） | **零依赖**（Node 内置） | `raw/式神`、`raw/剧情`、`raw/来源`、`raw/_原始导出`、`tables/*.csv` |

## 怎么跑

```powershell
# 默认：读 .huiji/ 写 projects/demo（整段重建 raw/式神、raw/剧情、raw/来源、raw/_原始导出 与它生成的那批 CSV）
node scripts/ingest/huiji-to-vault.mjs

# 换 vault / 换数据源
node scripts/ingest/huiji-to-vault.mjs --vault projects/其他 --raw .huiji/raw
```

写完要重建派生索引才能检索、查询：

```powershell
ssot vault -root projects/demo index      # 本机实测 135 秒（篇数看你的数据：那次是 406 篇 + 13 张表）
```

## 边界（写在 `AGENTS.md` 的那种「什么时候不该用」）

- **它不整理内容**：`docs/`（整理层）一个字都不碰——整理层是「我们认定的说法」，
  不能由 wiki 数据自动灌进来（见 `docs/specs/vault.spec.md` 的定位）。
- **它不做增量合并**：重跑是整段重建，手写的东西别放进它管的那几个目录。
- **它不联网**：只读 `.huiji/` 里已有的抓取缓存；没有缓存它无从下手（抓取是另一回事）。
- **它不改状态口径**：生成的原文页一律 `status: draft`（未核验是默认状态，发布只能由人做）。

## ⚠️ 踩过的坑（本机 Node 24 / Windows）

1. **`fs.rmSync` 对中文目录名会静默失败**——既不抛错也不删。实测 `.tmp/式神dir`、`raw/剧情`
   原样留着，`.tmp/ascii-dir` 正常删掉；异步的 `fs.promises.rm` 对同一条中文路径能删掉。
   脚本里因此只走异步版，并且**删完必须 `existsSync` 再确认**（第一版没验，于是上一轮的旧文件
   和新文件混在一起，现象只是「页数变多了」，很难看出来）。
2. **不要用 PowerShell 的 `Get-ChildItem -Filter a,b`** 找多个文件名——`-Filter` 不接受数组，
   报 `无法将 System.Object[] 转换为参数 Filter`；用 `Where-Object { $_.Name -in @(...) }`。
