/**
 * 文档渲染：把 vault 里的 markdown 变成界面上的 HTML。
 *
 * 支持范围见 docs/specs/document.spec.md 第三节——底座是 **CommonMark + GFM + LaTeX**
 * （Obsidian 自己的口径），再补上 Obsidian 那几样扩展：
 *
 *   [[链接]] / [[链接#标题]] / [[链接#^块]]   内部链接（带目标状态）
 *   ^块id                                     块锚点（主张级溯源）
 *   ![[表.csv]] / ![[文档]]                    嵌入（表格渲染成真表格）
 *   %%注释%%  ==高亮==  ~~删除线~~              行内标记
 *   > [!note] 等                               Callout
 *   $公式$ / $$公式$$                          KaTeX
 *   - [ ] / - [x]                              任务列表（先只渲染）
 *   表格 / 脚注 / 代码块                         GFM 与插件负责
 *
 * 两条实现纪律：
 *  1. **注释默认隐藏但留在 DOM 里**（靠 CSS 切换），这样开关注释不用重渲染。
 *  2. 渲染是**纯函数**：解析链接、取表格数据都由调用方传进来，这里不碰后端——
 *     同一份源码永远渲染出同样的 HTML，好测也好缓存。
 */
import katex from "katex";
import MarkdownIt from "markdown-it";
import footnote from "markdown-it-footnote";

/** 一条双链的解析结果（渲染时只知道「指向谁、那篇什么状态」）。 */
export type ResolvedLink =
  | { kind: "doc"; path: string; title: string; status: string }
  | { kind: "broken" }
  | { kind: "ambiguous"; candidates: string[] };

/** 嵌入的数据表（已取好的行列）。 */
export type EmbedTableData = { columns: string[]; rows: string[][] };

export type RenderOptions = {
  /** 解析 `[[目标]]`；返回 undefined 视作断链。 */
  resolve: (target: string) => ResolvedLink | undefined;
  /** 取 `![[tables/xxx.csv]]` 的数据；返回 undefined 表示不是表或取不到。 */
  embedTable?: (target: string) => EmbedTableData | undefined;
  /** 取 `![[某文档]]` 的摘要；返回 undefined 表示取不到。 */
  embedDoc?: (target: string) => { title: string; status: string; excerpt: string } | undefined;
};

const STATUS_LABEL: Record<string, string> = { published: "已发布", archived: "已归档", draft: "未核验" };

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function statusChip(status: string): string {
  const cls = status === "published" ? "ok" : status === "archived" ? "muted" : "draft";
  return `<span class="md-chip md-chip-${cls}">${esc(STATUS_LABEL[status] ?? status)}</span>`;
}

/** 拆 `目标#锚点|别名`。与 Go 侧 domain/vault 的解析规则保持一致：先拆别名，再拆锚点。 */
export function splitWikiTarget(inner: string): { target: string; heading: string; block: string; alias: string } {
  let rest = inner.trim();
  let alias = "";
  const pipe = rest.lastIndexOf("|");
  if (pipe >= 0) {
    alias = rest.slice(pipe + 1).trim();
    rest = rest.slice(0, pipe);
  }
  let heading = "";
  let block = "";
  const hash = rest.indexOf("#");
  if (hash >= 0) {
    const anchor = rest.slice(hash + 1).trim();
    rest = rest.slice(0, hash);
    if (anchor.startsWith("^")) block = anchor.slice(1).trim();
    else heading = anchor;
  }
  return { target: rest.trim(), heading, block, alias };
}

/** wiki 链接渲染成什么：解析成功给状态标记，断链/指不清给醒目样式。 */
function wikiLinkHTML(inner: string, opts: RenderOptions): string {
  const { target, heading, block, alias } = splitWikiTarget(inner);
  const label = alias || (block ? `${target || "本文"} ^${block}` : heading ? `${target || "本文"} › ${heading}` : target);
  if (!target && !block && !heading) return `<span class="md-broken">[[${esc(inner)}]]</span>`;

  const res = opts.resolve(target);
  if (!res) return `<a class="md-wikilink md-broken" data-target="${esc(inner)}">${esc(label)}</a>`;
  if (res.kind === "broken") return `<a class="md-wikilink md-broken" data-target="${esc(inner)}">${esc(label)}</a>`;
  if (res.kind === "ambiguous") {
    return `<a class="md-wikilink md-ambiguous" data-target="${esc(inner)}" title="有 ${res.candidates.length} 篇同名">${esc(label)}</a>`;
  }
  const anchor = block ? `#^${esc(block)}` : heading ? `#${esc(heading)}` : "";
  return (
    `<a class="md-wikilink" data-target="${esc(inner)}" href="#" title="${esc(res.path + anchor)}">` +
    `${esc(label)}${anchor ? `<span class="md-anchor">${esc(anchor)}</span>` : ""}</a>${statusChip(res.status)}`
  );
}

/** 嵌入：表 → 真表格；文档 → 摘要卡。 */
function embedHTML(inner: string, opts: RenderOptions): string {
  const { target } = splitWikiTarget(inner);
  const table = opts.embedTable?.(target);
  if (table) {
    const head = table.columns.map((c) => `<th>${esc(c)}</th>`).join("");
    const body = table.rows
      .map((r) => `<tr>${r.map((c) => `<td>${esc(c)}</td>`).join("")}</tr>`)
      .join("");
    return (
      `<figure class="md-embed md-embed-table"><figcaption>${esc(target)}<span class="md-embed-rows">${table.rows.length} 行</span></figcaption>` +
      `<table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></figure>`
    );
  }
  const doc = opts.embedDoc?.(target);
  if (doc) {
    return (
      `<aside class="md-embed md-embed-doc"><header><span class="md-embed-title">${esc(doc.title)}</span>${statusChip(doc.status)}</header>` +
      `<p>${esc(doc.excerpt)}</p>` +
      `<a class="md-wikilink" data-target="${esc(inner)}" href="#">打开 →</a></aside>`
    );
  }
  return `<span class="md-broken">![[${esc(inner)}]]</span>`;
}

export function createMarkdown(opts: RenderOptions) {
  const md = new MarkdownIt({ html: false, linkify: true, breaks: false, typographer: false });
  md.use(footnote);

  // ── 行内：[[链接]] / ![[嵌入]] ───────────────────────────────
  md.inline.ruler.before("link", "wikilink", (state, silent) => {
    const start = state.pos;
    const src = state.src;
    const embed = src.startsWith("![[", start);
    const open = embed ? start + 1 : start;
    if (!src.startsWith("[[", open)) return false;
    const end = src.indexOf("]]", open + 2);
    if (end < 0) return false;
    if (!silent) {
      const inner = src.slice(open + 2, end);
      const token = state.push("html_inline", "", 0);
      if (embed) {
        // `!` 已经在上一个 token 里了吗？这里直接把 `!` 之前的字符交回去，避免重复。
        token.content = embedHTML(inner, opts);
      } else {
        token.content = wikiLinkHTML(inner, opts);
      }
    }
    state.pos = end + 2;
    return true;
  });

  // ── 行内：==高亮== ──────────────────────────────────────────
  md.inline.ruler.before("emphasis", "highlight", (state, silent) => {
    const start = state.pos;
    if (!state.src.startsWith("==", start)) return false;
    const end = state.src.indexOf("==", start + 2);
    if (end < 0 || end === start + 2) return false;
    if (!silent) {
      state.push("mark_open", "mark", 1);
      const t = state.push("text", "", 0);
      t.content = state.src.slice(start + 2, end);
      state.push("mark_close", "mark", -1);
    }
    state.pos = end + 2;
    return true;
  });

  // ── 行内：%%注释%%（留在 DOM 里，靠 CSS 默认隐藏）───────────────
  md.inline.ruler.before("emphasis", "comment", (state, silent) => {
    const start = state.pos;
    if (!state.src.startsWith("%%", start)) return false;
    const end = state.src.indexOf("%%", start + 2);
    if (end < 0) return false;
    if (!silent) {
      const token = state.push("html_inline", "", 0);
      token.content = `<span class="md-comment">${esc(state.src.slice(start + 2, end))}</span>`;
    }
    state.pos = end + 2;
    return true;
  });

  // ── 块级：$$公式$$ ─────────────────────────────────────────
  md.block.ruler.before("fence", "math_block", (state, startLine, endLine, silent) => {
    const start = state.bMarks[startLine] + state.tShift[startLine];
    const max = state.eMarks[startLine];
    const first = state.src.slice(start, max).trim();
    if (!first.startsWith("$$")) return false;
    if (silent) return true;
    let content = first.replace(/^\$\$/, "");
    let line = startLine;
    if (!content.endsWith("$$")) {
      const parts = [content];
      for (line = startLine + 1; line < endLine; line++) {
        const s = state.bMarks[line] + state.tShift[line];
        const e = state.eMarks[line];
        const text = state.src.slice(s, e);
        if (text.trim().endsWith("$$")) {
          parts.push(text.trim().replace(/\$\$$/, ""));
          break;
        }
        parts.push(text);
      }
      content = parts.join("\n");
    } else {
      content = content.replace(/\$\$$/, "");
    }
    const token = state.push("html_block", "", 0);
    token.content = mathHTML(content, true);
    token.block = true;
    state.line = line + 1;
    return true;
  });

  // ── 行内：$公式$ ───────────────────────────────────────────
  md.inline.ruler.before("escape", "math_inline", (state, silent) => {
    const start = state.pos;
    if (state.src[start] !== "$" || state.src.startsWith("$$", start)) return false;
    const end = state.src.indexOf("$", start + 1);
    if (end < 0 || end === start + 1) return false;
    if (state.src[end - 1] === "\\") return false;
    if (!silent) {
      const token = state.push("html_inline", "", 0);
      token.content = mathHTML(state.src.slice(start + 1, end), false);
    }
    state.pos = end + 1;
    return true;
  });

  // ── 核心：Callout（把 `> [!note]` 的引用块换成带样式的块）────────
  md.core.ruler.after("block", "callout", (state) => {
    const tokens = state.tokens;
    for (let i = 0; i < tokens.length; i++) {
      if (tokens[i].type !== "blockquote_open") continue;
      const para = tokens[i + 1];
      const inline = tokens[i + 2];
      if (!para || para.type !== "paragraph_open" || !inline || inline.type !== "inline") continue;
      const m = /^\[!(\w+)\]([+-])?\s*(.*)$/.exec(inline.content.split("\n")[0]);
      if (!m) continue;
      const kind = m[1].toLowerCase();
      const title = m[3] || kind;
      // 去掉 `[!type] 标题` 那一行，剩的当正文
      const rest = inline.content.split("\n").slice(1).join("\n");
      inline.content = rest;
      inline.children = md.parseInline(rest, state.env)[0]?.children ?? [];
      tokens[i].type = "callout_open";
      tokens[i].tag = "div";
      tokens[i].attrSet("class", `md-callout md-callout-${kind}`);
      // 在块首插一行标题
      const titleTok = new state.Token("html_block", "", 0);
      titleTok.content = `<div class="md-callout-title">${esc(title)}</div>`;
      tokens.splice(i + 1, 0, titleTok);
      // 找到配对的 close 并换掉
      let depth = 0;
      for (let j = i + 1; j < tokens.length; j++) {
        if (tokens[j].type === "blockquote_open") depth++;
        if (tokens[j].type === "blockquote_close") {
          if (depth === 0) {
            tokens[j].type = "callout_close";
            tokens[j].tag = "div";
            break;
          }
          depth--;
        }
      }
    }
    return true;
  });

  // ── 核心：任务列表（GFM 没内置；渲染成勾选框，行号带上）─────────
  md.core.ruler.after("inline", "tasklist", (state) => {
    for (const token of state.tokens) {
      if (token.type !== "inline" || !token.children?.length) continue;
      const first = token.children[0];
      if (first.type !== "text") continue;
      const m = /^\[([ xX])\]\s+/.exec(first.content);
      if (!m) continue;
      const checked = m[1].toLowerCase() === "x";
      first.content = first.content.slice(m[0].length);
      const box = new state.Token("html_inline", "", 0);
      const line = (token.map?.[0] ?? 0) + 1;
      box.content =
        `<input type="checkbox" class="md-task" data-line="${line}" data-checked="${checked}"${checked ? " checked" : ""}` +
        ` aria-label="${checked ? "已完成" : "未完成"}">`;
      token.children.unshift(box);
      token.attrJoin("class", "md-task-item");
    }
    return true;
  });

  // ── 块锚点：行尾的 `^id` ───────────────────────────────────
  md.core.ruler.after("inline", "blockref", (state) => {
    for (const token of state.tokens) {
      if (token.type !== "inline" || !token.children?.length) continue;
      const last = token.children[token.children.length - 1];
      if (last.type !== "text") continue;
      const m = /(?:^|\s)\^([\w\u4e00-\u9fff-]+)\s*$/.exec(last.content);
      if (!m) continue;
      last.content = last.content.slice(0, m.index).replace(/\s+$/, "");
      const ref = new state.Token("html_inline", "", 0);
      ref.content = `<a class="md-blockref" id="^${esc(m[1])}" data-block="${esc(m[1])}" title="块引用：${esc(m[1])}">^${esc(m[1])}</a>`;
      token.children.push(ref);
    }
    return true;
  });

  return md;
}

function mathHTML(src: string, display: boolean): string {
  try {
    return `<span class="md-math${display ? " md-math-display" : ""}">${katex.renderToString(src.trim(), {
      displayMode: display,
      throwOnError: false,
      output: "html",
    })}</span>`;
  } catch {
    // 公式写错时原样显示，而不是把整篇文档弄崩——**不确定就报告，不降级**。
    return `<code class="md-math-error" title="公式渲染失败">${esc(display ? `$$${src}$$` : `$${src}$`)}</code>`;
  }
}

/** 渲染入口。 */
export function renderMarkdown(src: string, opts: RenderOptions): string {
  return createMarkdown(opts).render(src);
}


