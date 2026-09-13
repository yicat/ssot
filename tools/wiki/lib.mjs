// 灰机 wiki 抓取的共享工具。
//
// 背景（详见 docs/notes/wiki-data-source.md）：
//   - 该站是 Cloudflare **指纹门**，不下发 cf_clearance，cookie 复用无效
//   - 唯一可行通道：可见窗口 Chrome + CDP，在页面上下文内 fetch
//   - 本模块把 CDP 连接、挑战等待、API 调用、本地缓存读写收敛到一处

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

export const CACHE = path.join(ROOT, ".huiji");
export const RAW = path.join(CACHE, "raw");
export const MANIFEST = path.join(CACHE, "manifest.json");

export const WIKI = "https://yys.huijiwiki.com";
export const NS_DATA = 3500;
export const DEFAULT_PORT = 9226;

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ── 本地缓存 ────────────────────────────────────────────────────────────────

/** 页面标题 → 缓存文件名（标题含 : / \ 等字符，需净化） */
export function rawPath(title) {
  return path.join(RAW, title.replace(/[:/\\]/g, "_"));
}

export function loadRaw(title) {
  const f = rawPath(title);
  return fs.existsSync(f) ? fs.readFileSync(f, "utf8") : null;
}

export function loadJson(title) {
  const s = loadRaw(title);
  return s === null ? null : JSON.parse(s);
}

export function loadManifest() {
  return JSON.parse(fs.readFileSync(MANIFEST, "utf8"));
}

/** 清单里 Data:Character/<id>.json 形式的页面，按 id 升序 */
export function characterPages(manifest = loadManifest()) {
  return manifest.pages
    .filter((p) => /^Data:Character\/\d+\.json$/.test(p.title))
    .map((p) => ({ ...p, id: Number(p.title.match(/(\d+)\.json$/)[1]) }))
    .sort((a, b) => a.id - b.id);
}

// ── CDP ─────────────────────────────────────────────────────────────────────

export async function connect(port = DEFAULT_PORT) {
  const v = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();
  const ws = new WebSocket(v.webSocketDebuggerUrl);
  let id = 0;
  const pending = new Map();
  ws.addEventListener("message", (e) => {
    const m = JSON.parse(e.data);
    if (m.id && pending.has(m.id)) {
      pending.get(m.id)(m);
      pending.delete(m.id);
    }
  });
  await new Promise((res, rej) => {
    ws.addEventListener("open", res);
    ws.addEventListener("error", rej);
  });
  const send = (method, params = {}, sessionId) =>
    new Promise((res) => {
      const i = ++id;
      pending.set(i, res);
      ws.send(JSON.stringify({ id: i, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  return { ws, send, browser: v.Browser };
}

export async function openWikiPage(send, url = `${WIKI}/wiki/`) {
  const {
    result: { targetId },
  } = await send("Target.createTarget", { url });
  const {
    result: { sessionId },
  } = await send("Target.attachToTarget", { targetId, flatten: true });
  await send("Page.enable", {}, sessionId);
  return sessionId;
}

export function evaluator(send, sessionId) {
  return async (expression) => {
    const r = await send(
      "Runtime.evaluate",
      { expression, returnByValue: true, awaitPromise: true },
      sessionId,
    );
    if (r.result?.exceptionDetails) {
      throw new Error(JSON.stringify(r.result.exceptionDetails).slice(0, 400));
    }
    return r.result?.result?.value;
  };
}

/** 轮询直到 Cloudflare 挑战通过。headless 会一直失败，必须可见窗口。 */
export async function waitForChallenge(send, sessionId, { tries = 15, intervalMs = 5000 } = {}) {
  const ev = evaluator(send, sessionId);
  for (let i = 1; i <= tries; i++) {
    await sleep(intervalMs);
    const title = (await ev("document.title")) || "";
    if (title && !/请稍候|Just a moment/i.test(title)) {
      return { title, seconds: (i * intervalMs) / 1000 };
    }
  }
  throw new Error("Cloudflare 挑战未通过——请确认 Chrome 是**可见窗口**（headless 必失败）");
}

/** 页面上下文内的 MediaWiki API 调用（自带 cookie 与浏览器指纹） */
export function apiCaller(ev) {
  return async (params) => {
    const qs = new URLSearchParams({ format: "json", ...params }).toString();
    const raw = await ev(
      `(async()=>{const r=await fetch(${JSON.stringify("/api.php?" + qs)});return await r.text();})()`,
    );
    const j = JSON.parse(raw);
    if (j.error) throw new Error(`API 错误: ${JSON.stringify(j.error)}`);
    return j;
  };
}
