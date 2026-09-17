/**
 * mcp-smoke.mjs —— 把 MCP 服务端当「DSH 会怎么用它」那样跑一遍。
 *
 * 做什么：起**真的** ssot 二进制（`bin\ssot.exe mcp -root <vault>`），用换行分隔的
 *   JSON-RPC 跟它对话，逐条断言：协议协商、工具清单、读、写（含 status 回落 draft）、
 *   反链、块锚点解析、只读 SQL、错误路径，以及「写入有没有在 git 里留痕」。
 *
 * 适用范围：改动 internal/mcp、cmd/ssot 的 mcp 子命令、或能力层的写路径时跑它。
 *   它用的是**临时 vault**（自己 git init），所以不会往 projects/demo 里塞提交。
 *
 * 什么时候不该用：
 *   - 不要拿它做界面断言（那是 scripts/check/ui-test.mjs 的活）。
 *   - 它不启动 DSH 本身：**overlay 能不能被 DSH 组装进去**要靠
 *     `scripts\dsh\dsh-ssot.ps1 -DumpConfig`（那条只是 dump 配置，零副作用）。
 *   - 它假设 mcp-server 已按 dsh.spec.md 的形状实现（stdio、snake_case 工具名、
 *     不暴露 status_set）；形状变了要一起改这里，别把它当成需求的来源。
 *
 * 用法：node scripts/check/mcp-smoke.mjs
 */
import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "..", "..");
// ⚠️ 是 **ssot-cli.exe**：`bin\ssot.exe` 是 Wails 那个 GUI 二进制，两者不能同名（会互相覆盖）。
const bin = join(repo, "bin", process.platform === "win32" ? "ssot-cli.exe" : "ssot-cli");

let passed = 0;
const failures = [];
function check(name, ok, detail = "") {
  if (ok) {
    passed++;
    console.log(`  ✓ ${name}${detail ? `  —— ${detail}` : ""}`);
  } else {
    failures.push(name);
    console.log(`  ✗ ${name}${detail ? `  —— ${detail}` : ""}`);
  }
}

function git(cwd, args) {
  const r = spawnSync("git", ["-C", cwd, "-c", "core.quotepath=false", ...args], { encoding: "utf8" });
  if (r.status !== 0) throw new Error(`git ${args.join(" ")} 失败：${r.stderr || r.stdout}`);
  return r.stdout.trim();
}

/** makeVault 造一个临时 vault（含 git 仓库），返回根目录。 */
function makeVault() {
  const root = mkdtempSync(join(tmpdir(), "ssot-mcp-"));
  const files = {
    "docs/式神/茨木童子.md":
      "---\ntitle: 茨木童子\ntags: [式神, SSR]\nstatus: published\n---\n\n整理后的正文。\n",
    "docs/机制/伤害计算.md":
      "---\ntitle: 伤害计算\nstatus: draft\n---\n\n依据 [[raw/灰机wiki/茨木童子#^第3段]]。\n",
    "raw/灰机wiki/茨木童子.md":
      "---\ntitle: 灰机wiki：茨木童子\nsource_url: https://example.com/x\n---\n\n第一段。\n伤害系数 1.5。 ^第3段\n",
    "tables/技能倍率.csv": "技能,倍率\n罗生门,2.63\n",
  };
  for (const [rel, body] of Object.entries(files)) {
    mkdirSync(join(root, dirname(rel)), { recursive: true });
    writeFileSync(join(root, rel), body);
  }
  git(root, ["init", "-q"]);
  git(root, ["config", "user.name", "冒烟测试"]);
  git(root, ["config", "user.email", "smoke@example.com"]);
  git(root, ["add", "-A"]);
  git(root, ["commit", "-q", "-m", "基线"]);
  return root;
}

/** Client 是一个极简的 MCP 客户端：一行一条消息。 */
class Client {
  constructor(child) {
    this.child = child;
    this.queue = [];
    this.waiters = [];
    this.stdoutLines = [];
    this.buf = "";
    child.stdout.on("data", (chunk) => {
      this.buf += chunk.toString("utf8");
      let i;
      while ((i = this.buf.indexOf("\n")) >= 0) {
        const line = this.buf.slice(0, i);
        this.buf = this.buf.slice(i + 1);
        if (line.trim() === "") continue;
        this.stdoutLines.push(line);
        const waiter = this.waiters.shift();
        if (waiter) waiter(line);
        else this.queue.push(line);
      }
    });
    this.stderr = "";
    child.stderr.on("data", (c) => (this.stderr += c.toString("utf8")));
  }

  send(msg) {
    this.child.stdin.write(JSON.stringify(msg) + "\n");
  }

  read() {
    if (this.queue.length) return Promise.resolve(this.queue.shift());
    return new Promise((res) => this.waiters.push(res));
  }

  async call(id, method, params) {
    const req = { jsonrpc: "2.0", id, method };
    if (params !== undefined) req.params = params;
    this.send(req);
    return JSON.parse(await this.read());
  }

  async callTool(id, name, args) {
    const resp = await this.call(id, "tools/call", { name, arguments: args });
    if (resp.error) throw new Error(`协议错误：${JSON.stringify(resp.error)}`);
    const result = resp.result;
    const text = result.content[0].text;
    let parsed = null;
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = { raw: text };
    }
    return { isError: result.isError === true, data: parsed, text };
  }
}

async function main() {
  console.log("== 编译（源码比二进制新时会重编）==");
  const build = spawnSync("go", ["build", "-o", bin, "./cmd/ssot"], {
    cwd: repo,
    encoding: "utf8",
    env: { ...process.env, GOCACHE: join(repo, ".gocache") },
  });
  if (build.status !== 0) {
    console.error(build.stderr || build.stdout);
    process.exit(1);
  }

  const vault = makeVault();
  const child = spawn(bin, ["mcp", "-root", vault, "-actor", "agent:dsh"], { cwd: repo });
  const cli = new Client(child);

  try {
    console.log("\n== 协议 ==");
    const init = await cli.call(1, "initialize", { protocolVersion: "2025-06-18", capabilities: {} });
    check("initialize 回同一版协议", init.result?.protocolVersion === "2025-06-18", init.result?.protocolVersion);
    check("serverInfo.name = ssot", init.result?.serverInfo?.name === "ssot");
    check("声明了 tools 能力", !!init.result?.capabilities?.tools);

    const list = await cli.call(2, "tools/list");
    const names = list.result.tools.map((t) => t.name);
    check("工具数是 9（含只读的 file_read）", names.length === 9, names.join(", "));
    check(
      "工具名都是 snake_case（DSH 只接受 [A-Za-z0-9_-]）",
      names.every((n) => /^[a-z0-9_]+$/.test(n)),
      names.join(", "),
    );
    check("没有暴露 status_set（agent 永远无权，白占上下文）", !names.includes("status_set"));
    check(
      "每个工具都有 description 与 inputSchema",
      list.result.tools.every((t) => t.description && t.inputSchema?.type === "object"),
    );

    console.log("\n== 读 ==");
    const all = await cli.callTool(3, "vault_list", {});
    check("列文档：3 篇", all.data.documents?.length === 3, String(all.data.documents?.length));
    check("列数据表：技能倍率.csv", (all.data.tables ?? []).some((t) => t.includes("技能倍率")));

    const doc = await cli.callTool(4, "doc_read", { path: "docs/式神/茨木童子.md" });
    check("读到正文与状态", doc.data.status === "published" && doc.data.body.includes("整理后的正文"));
    check("读到 front matter 的 tags", (doc.data.tags ?? []).includes("SSR"), JSON.stringify(doc.data.tags));

    // 只读地读任意文本文件（agent 靠这个读原文/JSON/项目文件；写仍然只有 doc_write）
    // 分页读：这个文件 7 行，正文从第 6 行开始（第 1–5 行是 front matter）。
    // ⚠️ 断言要落在**取到的那一段**上：第一版从第 1 行读 3 行却去找正文里的词，自己写错了断言。
    const raw = await cli.callTool(5, "file_read", { path: "raw/灰机wiki/茨木童子.md", fromLine: 6, maxLines: 2 });
    check(
      "file_read 能按行分页读到正文",
      raw.data.totalLines === 7 && raw.data.fromLine === 6 && raw.data.toLine === 7 && raw.data.text.includes("第一段"),
      JSON.stringify({ from: raw.data.fromLine, to: raw.data.toLine, total: raw.data.totalLines, text: raw.data.text }),
    );
    const outside = await cli.callTool(51, "file_read", { path: "../外面.txt" });
    check("file_read 拒绝越出 vault", outside.isError, outside.text.slice(0, 40));

    const amb = await cli.callTool(52, "doc_read", { path: "茨木童子" });
    check("同名两篇时不猜、如实报错", amb.isError && amb.text.includes("同名"), amb.text.slice(0, 60));

    const res = await cli.callTool(6, "link_resolve", { ref: "[[raw/灰机wiki/茨木童子#^第3段]]" });
    check(
      "块锚点解析到文件行号（主张级溯源）",
      res.data.path === "raw/灰机wiki/茨木童子.md" && res.data.blockLine > 0,
      `blockLine=${res.data.blockLine}`,
    );

    const back = await cli.callTool(7, "link_backlinks", { path: "raw/灰机wiki/茨木童子.md" });
    check("反链 1 条", back.data.backlinks?.length === 1);
    check("问题链接单列（这里没有）", Array.isArray(back.data.issues) && back.data.issues.length === 0);

    console.log("\n== 数据表 ==");
    const infos = await cli.callTool(8, "table_infos", {});
    check("看到表与列", infos.data.tables?.[0]?.columns?.includes("技能"), JSON.stringify(infos.data.tables?.[0]));
    const q = await cli.callTool(9, "table_query", { sql: "SELECT 技能, 倍率 FROM 技能倍率" });
    check("只读查询出数", q.data.rows?.[0]?.[1] === "2.63", JSON.stringify(q.data.rows));
    const bad = await cli.callTool(10, "table_query", { sql: "DELETE FROM 技能倍率" });
    check("写操作被只读门挡住", bad.isError, bad.text.slice(0, 60));

    console.log("\n== 写（门与留痕）==");
    const w = await cli.callTool(11, "doc_write", {
      path: "docs/式神/茨木童子.md",
      body: "改写后的正文，依据 [[raw/灰机wiki/茨木童子#^第3段]]。\n",
    });
    check("agent 写已发布文档 → 回落 draft", w.data.from === "published" && w.data.to === "draft", `${w.data.from} → ${w.data.to}`);
    check("actor 是 agent:dsh", w.data.actor === "agent:dsh", w.data.actor);
    check("已在 git 留痕（committed + sha）", w.data.committed === true && /^[0-9a-f]{40}$/.test(w.data.commitSha ?? ""), w.data.commitSha?.slice(0, 8));
    const body = git(vault, ["log", "-1", "--format=%B"]);
    check("提交信息带 Edited-By trailer", body.includes("Edited-By: agent:dsh"), body.split("\n").join(" / "));
    const changed = git(vault, ["show", "--name-only", "--format="]);
    check("一次提交只带改动的那个文件", changed === "docs/式神/茨木童子.md", changed);
    const onDisk = readFileSync(join(vault, "docs/式神/茨木童子.md"), "utf8");
    check("文件真被改了，且 front matter 原样保留", onDisk.includes("改写后的正文") && onDisk.includes("status: draft"));

    const again = await cli.callTool(12, "doc_write", { path: "docs/式神/茨木童子.md", body: "又一遍。\n" });
    check("内容变化会再产生一次提交", again.data.committed === true);
    const noop = await cli.callTool(13, "doc_write", { path: "docs/式神/茨木童子.md", body: "又一遍。\n" });
    check("内容没变则不产生空提交，并说清原因", noop.data.committed === false && (noop.data.versionNote ?? "").includes("没有变化"), noop.data.versionNote);

    console.log("\n== 协议收尾 ==");
    const unknown = await cli.call(14, "resources/list");
    check("不认识的方法 → -32601", unknown.error?.code === -32601, JSON.stringify(unknown.error));
    const noTool = await cli.call(15, "tools/call", { name: "vault_nope", arguments: {} });
    check("不认识的工具 → -32602", noTool.error?.code === -32602, JSON.stringify(noTool.error));
    check(
      "stdout 里只有协议消息（没有混进任何日志）",
      cli.stdoutLines.every((l) => {
        try {
          return JSON.parse(l).jsonrpc === "2.0";
        } catch {
          return false;
        }
      }),
      `${cli.stdoutLines.length} 行`,
    );
  } finally {
    child.stdin.end();
    child.kill();
    await new Promise((r) => setTimeout(r, 200));
    try {
      rmSync(vault, { recursive: true, force: true });
    } catch {
      /* Windows 上偶尔删不掉仓库里的只读对象，忽略——它在临时目录里 */
    }
  }

  const total = passed + failures.length;
  console.log(`\n${passed}/${total} 通过`);
  if (failures.length) {
    console.log("失败：" + failures.join("、"));
    process.exit(1);
  }
  console.log("\n提示：overlay 能不能被 DSH 组装进去是另一件事，跑：");
  console.log("  pwsh -File scripts\\dsh\\dsh-ssot.ps1 -DumpConfig");
}

await main();



