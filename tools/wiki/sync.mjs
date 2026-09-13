// 全量同步灰机 wiki 的 Data 命名空间到本地缓存（带 revid 溯源）。
//
// 前置：一个**可见窗口**的 Chrome 已启动并开着调试端口：
//   chrome.exe --remote-debugging-port=9226 --user-data-dir=<仓库内目录> \
//              --no-first-run --no-default-browser-check
//
// 用法：
//   node tools/wiki/sync.mjs [port]
//
// 输出：.huiji/raw/<页面>  与  .huiji/manifest.json

import fs from "node:fs";
import {
  connect,
  openWikiPage,
  waitForChallenge,
  evaluator,
  apiCaller,
  rawPath,
  RAW,
  MANIFEST,
  NS_DATA,
  DEFAULT_PORT,
} from "./lib.mjs";

const BATCH = 50;

const port = Number(process.argv[2]) || DEFAULT_PORT;
const { ws, send, browser } = await connect(port);
console.log(`已连接: ${browser}`);

const sessionId = await openWikiPage(send);
const { title, seconds } = await waitForChallenge(send, sessionId);
console.log(`Cloudflare 通过（${seconds}s）: ${title}`);

const ev = evaluator(send, sessionId);
const api = apiCaller(ev);

console.log(`\n列出 Data 命名空间（NS ${NS_DATA}）页面…`);
const list = (
  await api({ action: "query", list: "allpages", apnamespace: NS_DATA, aplimit: 500 })
).query.allpages.map((p) => p.title);
console.log(`  共 ${list.length} 个页面`);

fs.mkdirSync(RAW, { recursive: true });
const pages = [];
let chars = 0;
const batches = Math.ceil(list.length / BATCH);

for (let i = 0; i < list.length; i += BATCH) {
  const batch = list.slice(i, i + BATCH);
  const j = await api({
    action: "query",
    prop: "revisions",
    rvprop: "content|ids|timestamp|size",
    rvslots: "main",
    titles: batch.join("|"),
  });
  for (const p of Object.values(j.query.pages)) {
    if (!p.revisions) {
      pages.push({ title: p.title, missing: true });
      console.warn(`  ⚠ 无内容: ${p.title}`);
      continue;
    }
    const rev = p.revisions[0];
    const content = rev.slots.main["*"];
    chars += content.length;
    pages.push({
      title: p.title,
      pageid: p.pageid,
      revid: rev.revid,
      parentid: rev.parentid,
      timestamp: rev.timestamp,
      bytes: rev.size,
      chars: content.length,
    });
    fs.writeFileSync(rawPath(p.title), content, "utf8");
  }
  console.log(`  批 ${Math.floor(i / BATCH) + 1}/${batches}  ${Math.min(i + BATCH, list.length)}/${list.length}`);
}

fs.writeFileSync(
  MANIFEST,
  JSON.stringify(
    {
      source: "yys.huijiwiki.com",
      ns: NS_DATA,
      fetchedAt: new Date().toISOString(),
      count: pages.length,
      pages,
    },
    null,
    2,
  ),
);

const revs = pages.filter((p) => p.revid).map((p) => p.revid);
console.log(`\n完成`);
console.log(`  页面 ${pages.length}  字符 ${chars.toLocaleString()}  revid ${Math.min(...revs)}~${Math.max(...revs)}`);
console.log(`  缓存 ${RAW}`);
console.log(`  清单 ${MANIFEST}`);

await send("Browser.close");
ws.close();
