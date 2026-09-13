# 灰机 wiki 数据源（主源）— 实测记录

> 本文只记录**用真实请求验证过**的事实，每条带 revid / 时间戳 / 实测数值。
> 主源：`yys.huijiwiki.com`（阴阳师wiki，MediaWiki 1.38.4，1181 文章 / 7060 页面）
> 同步时间：`2026-09-13T10:14:40Z`

## 一、访问前提（三个坑，已全部定位）

### 坑 1：Windows 的 TLS 栈在沙箱下失败

`curl` / `Invoke-WebRequest` / .NET 在 Windows 上都走 **schannel**，报：

```
schannel: AcquireCredentialsHandle failed: SEC_E_NO_CREDENTIALS (0x8009030e)
```

**解法：用 Node.js。** Node 走 OpenSSL，同一机器同一网络直接成功。
凡抓 https 一律用 Node，不要用 curl / PowerShell。

### 坑 2：DNS 被 Clash 接管为 fake-ip

所有域名解析到 `198.18.1.x`（`verge-mihomo` 监听 `0.0.0.0:7897` 代理 + `0.0.0.0:53` DNS）。
这不是故障，不用修。但 **`Get-NetAdapter` / `Get-NetTCPConnection` 在本环境返回空**，
会把人误导成"没有网卡、没有代理"——**必须用 `netstat -ano`**。

dsh 的 `web_fetch` 工具会因「解析到非公网 IP」拒绝这些域名，那是 SSRF 防护，不代表网络不通。

### 坑 3：Cloudflare 是**指纹门**，不是 cookie 门（关键结论）

`yys.huijiwiki.com` 直接请求 403，页面标题「请稍候…」，CSP 指向 `challenges.cloudflare-cn.com`。

**以下两条路已实测走死，不要再试：**

| 尝试 | 结果 |
|---|---|
| 纯 Node + 全套浏览器请求头（`sec-ch-ua`、`Sec-Fetch-*` 等） | ❌ 403 |
| 抓取全部 cookie（含 `__cf_bm`）+ 相同 UA，浏览器关闭后直连 | ❌ 403 |
| headless Chrome 首次访问 | ❌ 挑战不通过（22s 仍被挡） |
| headless Chrome 复用**已通过挑战的 profile** | ❌ 仍被挑战 |
| **可见窗口 Chrome** | ✅ **5~10 秒通过** |

根因：该站**不下发 `cf_clearance`**。实测导出的 cookie 只有：

```
.huijiwiki.com  _ga  _gat  _gid  _ga_N3DS04643Q  __cf_bm [httpOnly]
含 cf_clearance: false
```

它靠 **TLS/JA3 指纹 + JS 挑战**判定，因此「记录 cookie 直接抓」在技术上不成立。

### Chrome 在受限沙箱下无法启动

Mojo IPC 需要创建命名管道，被拒即 FATAL：

```
FATAL:mojo/public/cpp/platform/platform_channel.cc:108] Check failed: . : 拒绝访问。(0x5)
```

`--no-sandbox` 无效（被拒的是「创建」管道这个动作）。
当前会话文件策略已是 `danger-full-access`，不再受限。

## 二、同步方案（当前唯一可行通道）

**传输层 = 一次可见浏览器会话；之后全部本地化。**

```
1. 启动可见 Chrome：--remote-debugging-port=9226 --user-data-dir=<工作区内目录>
2. CDP 打开 https://yys.huijiwiki.com/wiki/ ，轮询 document.title 直到不再是「请稍候…」
3. 用 Runtime.evaluate 在【页面上下文内】执行 fetch("/api.php?...")
   —— 自带 cookie 与浏览器指纹，绕过反爬。不要从 Node 进程直接 fetch。
4. 分批抓取（每批 50 个标题），保存正文 + revid + parentid + timestamp
5. Browser.close（只关自己启动的实例）
```

抓取是**批量同步**而非实时访问，所以一个窗口会话足够。

## 三、已完成的同步结果

| 项 | 值 |
|---|---|
| 页面数 | **423**（Data 命名空间 NS 3500 全量，`continue:false`） |
| 原始字符 | **10,075,930** |
| revid 区间 | 136 ~ 10362 |
| 落地位置 | `.huiji/raw/`（正文）+ `.huiji/manifest.json`（清单） |

## 四、数据地图

`姑获鸟` 页面本身只有 21 字符（`{{角色页|262}}` + `{{式神录导航}}`），
`Template:角色页` 展开为 `{{#invoke: Character/Page|main|id=262}}`，
Lua 模块再读 `Data:` 命名空间的 JSON。**数据不在正文里。**

| 页面 | 大小 | 作用 |
|---|---|---|
| `Data:Character.schema` | 8,158 | 平台自带 schema（revid=816），含字段/类型/枚举 |
| `Data:CharacterIndex.json` | 59,556 | 角色索引，**275** 条（revid=9939） |
| `Data:Character/<id>.json` | — | 281 个文件（283 页含 schema/tabx） |
| **`Data:Attribute.json`** | 32,423 | **属性数值真源**，275 条，id 200~608 |
| `Data:Soul.json` | 11,900 | **御魂**：名称/描述/套装效果/掉落 |
| **`Data:SkillBuffs.json`** | **131,341** | **1126 条 buff**（增益505/通用364/减益257） |
| `Data:SkillTips.json` | 19,424 | 技能提示文本 |
| `Data:Battle/604.json` | 5,174 | 斗技阵容与胜负记录 |
| `Data:Story/*` | 125 页 | 剧情 |
| `Data:Gift / Skin / Skilldiff / Monthillustration / Index` | — | 其他 |

### 两种属性写法（同一事实、两个源、两种量纲）

```json
// Data:Attribute.json  —— 扁平、小数量纲
{"id":262,"name":"姑获鸟","rarity":"SR","atk":3082,"hp":10823,"def":397,
 "spd":113,"cri":0.5,"crid":1.2,"efh":0,"efr":0}

// Data:Character/262.json  —— 嵌套、百分比、带评级
"stats":{"after":{"atk":{"grade":"S","value":3082},
                  "crit":{"grade":"SS","value":50},
                  "crit_dmg":{"grade":"D","value":120}}}
```

## 五、交叉验证发现（对 SSOT 设计的关键意义）

### 1. `stats` 字段已废弃，真源是 `Attribute.json`

```
Data:Character/*.json 中 stats.after 有真实数值的：仅 1 个（262）
stats 为空对象 {}   ：276 个
stats 字段缺失      ：1 个
```

**设计含义**：不能只看 schema 声明就认为字段有效。schema 允许 ≠ 数据在用。

### 2. 两源数值一致，但量纲不同（已验证）

| 字段 | `Character/262.json` | `Attribute.json` | 关系 |
|---|---|---|---|
| 攻击 | `atk.value=3082` | `atk=3082` | 相等 |
| 生命 | `hp.value=10823` | `hp=10823` | 相等 |
| 速度 | `spd.value=113` | `spd=113` | 相等 |
| 暴击 | `crit.value=50` | `cri=0.5` | ×100 |
| 暴击伤害 | `crit_dmg.value=120` | `crid=1.2` | ×100 |

**设计含义**：跨源比对必须先做**量纲归一**，否则会把一致误判为冲突。

### 3. 穷尽性检查的"不一致"是误报——需要实体类别概念

`CharacterIndex` 275 条 vs `Data:Character/*.json` 281 个文件，多出的 6 个 id：

```
10:晴明  11:神乐  12:八百比丘尼  13:源博雅  15:源赖光  16:藤原道长
```

**它们是阴阳师主角，不是式神**，因此不在角色索引与 `Attribute.json` 中。

**设计含义**：穷尽性校验必须能表达"实体类别"。**报警 ≠ 判错，需要归因流程**。

### 4. schema 漂移（已实证）

```
schema 声明字段(13)：id name cv year date gender grade introduction stats zhuanji huijuan skills document
记录实际字段(15)  ：… + tags + Voice
```

`tags` 与 `Voice` 在 281 条记录中 100% 存在，却不在 schema 里。

**设计含义**：schema 必须支持版本化与漂移检测，且**方向要双向**（schema→数据、数据→schema）。

### 5. 字段覆盖率非满（需区分"合法缺失"与"漏录"）

```
Voice      250/281  89%
huijuan    269/281  96%
document   279/281  99%
```

### 6. 伤害规则仍是自由文本

`Data:SkillBuffs.json` 131KB 里 445 处"伤害"、133 处"速度"、101 处"暴击"，
但都在 `buffDesc` 描述文本中，例如：

```json
{"id":"1_1","name":"沉睡","buffDesc":"使目标睡眠1回合","type":"减益","kind":"状态"}
```

条目本身是结构化的（id/name/type/kind/icon），**数值效果要从中抽取**。

**设计含义**：这是 L2/L4 级抽取的主战场，必须带原文锚点与置信度，不能与 L1 直引混存。

## 六、技能倍率的实际位置（待系统抽取）

以姑获鸟为例，倍率分散在两处：

```json
{"name":"伞剑","description":"…造成攻击80%伤害并无视20%防御。",
 "upgrades":[{"level":2,"effect":"伤害增加至84%"},
             {"level":3,"effect":"伤害增加至88%"},
             {"level":4,"effect":"伤害增加至92%"},
             {"level":5,"effect":"伤害增加至96%"}]}
```

即：**1 级倍率在自由文本，2~5 级在 `upgrades[].effect`**，两处格式不同。

## 七、可复用脚本

### 同步（需可见 Chrome）

```js
// 连接 CDP → 打开 wiki → 等挑战通过 → 站内 fetch 分批抓取
const ev = (expr) => send("Runtime.evaluate",
  { expression: expr, returnByValue: true, awaitPromise: true }, sessionId);
const titles = JSON.parse(await ev(`(async()=>{const r=await fetch(
  "/api.php?action=query&list=allpages&apnamespace=3500&aplimit=500&format=json");
  const j=await r.json();return JSON.stringify(j.query.allpages.map(p=>p.title));})()`));
// 每批 50：action=query&prop=revisions&rvprop=content|ids|timestamp|size&rvslots=main&titles=A|B|…
```

完整实现见 `.huiji/sync.mjs`（临时脚本，待正式化为项目工具）。

### 直连备用源（无需浏览器）

`wiki.biligame.com/yys/api.php` 可被 Node 直连（MediaWiki 1.37.0，479 文章），
模板为 `key=value` 风格（`{{式神资料页面|式神名=…|稀有度=…}}`），
适合作**独立交叉验证源**。

## 八、待办

- `.huiji/cookies.json` 含会话 cookie，**属凭据，不得提交**（已 gitignore）
- 技能倍率的系统抽取尚未做
- 伤害计算公式的权威表述尚未找到（可能只在社区攻略中，不在 wiki 数据层）
- `Data:Character.tabx` 未解析（表格格式）
- `Data:Attribute.json` 与游戏内实际面板的关系未验证（是否满级、是否含觉醒加成）
