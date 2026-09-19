/**
 * AgentPane 的交互逻辑：起后端、发话、收流式更新、回答权限提示。
 *
 * 所有调用都走 bindings（也就是走 Go 侧的应用层）——界面不直接跟 ACP 后端说话。
 * 流式更新是 Go 那边推过来的事件（`agent:update` / `agent:turn` / `agent:permission`）。
 */
import { Events } from "@wailsio/runtime";
import { useCallback, useEffect, useRef } from "react";

import * as AgentService from "../../../../bindings/github.com/ngnl5/ssot/internal/api/agentservice";
import type { AgentStatus } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useAgentStore } from "./store";

/** 与 Go 侧 api/agent.go 里的事件名一致。 */
const EVENT_UPDATE = "agent:update";
const EVENT_TURN = "agent:turn";
const EVENT_PERMISSION = "agent:permission";

type UpdatePayload = {
  kind: string;
  text: string;
  tool?: { id: string; title: string; status: string; kind: string };
};

type TurnPayload = { stopReason: string; error: string };

type PermissionPayload = {
  id: string;
  tool: string;
  options: { optionId: string; name: string; kind: string }[];
};

/**
 * 会话标题的存储。
 *
 * 后端（DSH 的 `session/list`）只回 sessionId 与 cwd，**不给标题**，所以标题只能我们自己记：
 *   - 初始值：**第一句用户消息**（截 24 字）——它通常就说明了这次会话在干什么；
 *   - 以后：**由 agent 生成**更好的标题，走同一个 `renameSession`（谁写的都一样存）。
 *
 * 存本地（按 vault 分组）：这是**界面层的便利信息**，不是事实——丢了只是名字变回时间戳，
 * 不影响会话内容（内容在后端那边）。
 */
const titleKey = (vault: string) => `ssot:session-titles:${vault || "(未打开项目)"}`;

function loadTitles(vault: string): Record<string, string> {
  try {
    const raw = localStorage.getItem(titleKey(vault));
    return raw ? (JSON.parse(raw) as Record<string, string>) : {};
  } catch {
    return {};
  }
}

function saveTitles(vault: string, titles: Record<string, string>) {
  try {
    localStorage.setItem(titleKey(vault), JSON.stringify(titles));
  } catch {
    // 存不下（隐私模式 / 配额满）就算了：标题是便利信息，不该影响聊天。
  }
}

function storedTitle(vault: string, id: string): string {
  return loadTitles(vault)[id] ?? "";
}

/** 记下初始描述：**只记第一次**——后面那句话不该把名字改掉。 */
function rememberTitle(vault: string, id: string, firstMessage: string) {
  if (!vault || !id) return;
  const titles = loadTitles(vault);
  if (titles[id]) return;
  const one = firstMessage.replace(/\s+/g, " ").trim();
  if (!one) return;
  titles[id] = one.length > 24 ? one.slice(0, 24) + "…" : one;
  saveTitles(vault, titles);
}

/**
 * 第一次发送时可能还没拿到 sessionId（后端状态还没回来）——那就先把这句话存着，
 * 等 sessionId 到位再写标题。**不这样做标题会被静默丢掉**：`rememberTitle` 在 id 为空时直接返回，
 * 人发了话却怎么也没有标题。
 */
let pendingTitle = "";
let pendingVault = "";

const lastKey = (vault: string) => `ssot:last-session:${vault || "(未打开项目)"}`;

/** 记住这个 vault 上次用的会话。 */
function rememberLastSession(vault: string, id: string) {
  try {
    localStorage.setItem(lastKey(vault), id);
  } catch {
    // 存不下就算了：只是下次打开时落不回那个会话，不影响聊天。
  }
}

/**
 * 打开应用时调：切回这个 vault 上次用的会话。
 *
 * 为什么走 Resume 而不是 Start：`Start` 会**顺带建一个新会话**——以前「打开面板自动起后端」
 * 就是这么攒出一堆空会话的。改过之后的 `Resume` 会在后端没起时把它拉起来（`agentapp.ensureBackend`），
 * 只切会话、不建新的。
 *
 * 切不过去（会话被删了、后端起不来）就安静放弃：等用户真要说第一句话时再按懒启动处理。
 */
export async function resumeLastSession(vault: string): Promise<void> {
  if (!vault) return;
  let id = "";
  try {
    id = localStorage.getItem(lastKey(vault)) ?? "";
  } catch {
    return;
  }
  if (!id) {
    // 没有「上次的会话」：**只起后端**（EnsureBackend 不建会话），会话等他真说第一句时才建。
    try {
      const st = await AgentService.EnsureBackend();
      useAgentStore.getState().set({
        running: st.running,
        busy: st.busy,
        agent: st.agent,
        version: st.version,
        sessionId: st.sessionId,
        vault: st.vault,
        model: st.model,
        models: st.models ?? [],
      });
    } catch {
      // 起不来就算了：界面会显示「后端未启动」，用户真要说第一句时 send 里还会再试一次。
    }
    return;
  }
  try {
    const st = await AgentService.Resume(id);
    useAgentStore.getState().set({
      running: st.running,
      busy: st.busy,
      agent: st.agent,
      version: st.version,
      sessionId: st.sessionId,
      vault: st.vault,
      model: st.model,
      models: st.models ?? [],
    });
  } catch {
    // 切不过去（会话可能已经被删了）：**只起后端**（EnsureBackend），不再退化成 Start——
    // Start 会顺带建一个空会话（那正是「一堆空会话」的来源）。
    try {
      const st = await AgentService.EnsureBackend();
      useAgentStore.getState().set({
        running: st.running,
        busy: st.busy,
        agent: st.agent,
        version: st.version,
        sessionId: st.sessionId,
        vault: st.vault,
        model: st.model,
        models: st.models ?? [],
      });
    } catch {
      // 起不来就算了：界面会显示「后端未启动」，顶栏有手动入口。
    }
  }
}

/** 控制台行：模块级函数也能写（resumeLastSession 在组件之外）。 */
function logLine(text: string) {
  try {
    useAgentStore.getState().log(text);
  } catch {
    // 控制台本身不该把流程带崩。
  }
}

export function useAgent() {
  const store = useAgentStore();
  const { set, push } = store;

  /** 工具调用按 callId 归一行：ACP 会先发 tool_call、结束再发 tool_call_update。 */
  const toolRows = useRef<Map<string, number>>(new Map());

  const applyStatus = useCallback(
    (st: AgentStatus) => {
      // 待写的标题：等 sessionId 出现就落地（见 pendingTitle 的说明）。
      if (pendingTitle && st.sessionId) {
        rememberTitle(st.vault || pendingVault, st.sessionId, pendingTitle);
        pendingTitle = "";
      }
      // 记住「这个 vault 上次用的会话」：下次打开应用就落回它（见 resumeLastSession）。
      if (st.sessionId) rememberLastSession(st.vault, st.sessionId);
      set({
        running: st.running,
        busy: st.busy,
        agent: st.agent,
        version: st.version,
        sessionId: st.sessionId,
        vault: st.vault,
        model: st.model,
        models: st.models ?? [],
      });
    },
    [set],
  );

  /** 刷新状态（打开面板、起后端之后调）。 */
  const refresh = useCallback(async () => {
    try {
      applyStatus(await AgentService.Status());
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [applyStatus, push]);

  /** 起后端并开会话。 */
  const start = useCallback(async () => {
    set({ busyMessage: "正在起后端…" });
    try {
      applyStatus(await AgentService.Start());
      set({ busyMessage: null });
      push({ kind: "notice", text: "后端就绪，可以开始说话了。", isError: false });
    } catch (err) {
      set({ busyMessage: null });
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [applyStatus, push, set]);

  /**
   * 打开面板时调：拉一次状态；**没起后端就自动起**（DSH 那样的体感——不用人先点一下）。
   *
   * ⚠️ 判断要用**刚拉回来的**状态，不能读 store：`applyStatus` 是 set 之后才生效，
   * 同一次调用里读到的是旧值（这种时序错很隐蔽，所以这里直接把 st 用在判断上）。
   */
  const boot = useCallback(async () => {
    try {
      applyStatus(await AgentService.Status());
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [applyStatus, push]);

  /**
   * 新开一个会话。
   *
   * 以前是 `stop → start`（重启后端进程，几秒）；现在能力层有了 `NewSession`：
   * **只在后端上开一个新会话**，不重启进程、也不影响别的会话。
   */
  async function newSession() {
    set({ items: [], sessionId: "", permission: null, busyMessage: null });
    try {
      applyStatus(await AgentService.NewSession());
      logLine("新会话 " + useAgentStore.getState().sessionId.slice(0, 8));
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
      logLine("开新会话失败：" + String(err));
    }
  }
  /**
   * 给会话改名 / 设标题。
   *
   * **这是留给 agent 生成标题的口子**：将来由 agent 读第一轮对话、生成一句更准的标题，
   * 通过这条（或能力层的一条工具）写进来——存的格式与「第一句话」那套完全一样，
   * 所以界面不用区分是谁写的。
   */
  const renameSession = useCallback(
    async (id: string, title: string) => {
      if (!store.vault || !id) return;
      const clean = title.replace(/\s+/g, " ").trim();
      if (!clean) return;
      const titles = loadTitles(store.vault);
      titles[id] = clean.length > 40 ? clean.slice(0, 40) + "…" : clean;
      saveTitles(store.vault, titles);
      set({ sessions: store.sessions.map((s) => (s.id === id ? { ...s, title: titles[id] } : s)) });
    },
    [set, store.sessions, store.vault],
  );

  /**
   * 当前会话的标题（本地记的：第一句话，或 agent 生成的）。
   *
   * 为什么要露出来：标题原来只在下拉里能看到，等于藏着——顶栏写着短 id，人根本不知道自己
   * 在哪个会话里。这里给一个取值口子，界面在顶栏显示它。
   * 读 localStorage 是刻意的：发第一句时写进去，紧接着的那次重渲染就能读到（不用再加一层 state）。
   */
  const currentTitle = useCallback(
    () => storedTitle(store.vault, store.sessionId),
    [store.sessionId, store.vault],
  );

  /** 发一句。 */
  const send = useCallback(async () => {
    const text = store.draft.trim();
    if (!text) return;
    // **懒启动**：真要说第一句话时才把后端备好。
    // 两条都**不建会话**：EnsureBackend 只起进程，真没会话时才 NewSession 开一个（这时本来也该开）。
    if (!useAgentStore.getState().running) {
      try {
        applyStatus(await AgentService.EnsureBackend());
        logLine("起后端（EnsureBackend，不建会话）");
      } catch (err) {
        push({ kind: "notice", text: String(err), isError: true });
        logLine("起后端失败：" + String(err));
        return;
      }
    }
    if (!useAgentStore.getState().sessionId) {
      try {
        applyStatus(await AgentService.NewSession());
        logLine("开会话 " + useAgentStore.getState().sessionId.slice(0, 8));
      } catch (err) {
        push({ kind: "notice", text: String(err), isError: true });
        logLine("开会话失败：" + String(err));
        return;
      }
    }
    push({ kind: "user", text });
    // 会话的**初始描述**：第一句话就是这次会话在干什么——先拿它当标题（机械截断，不调模型）。
    // 之后可以由 agent 生成更好的标题，走同一个存储（见 renameSession 与 docs/OPEN.md）。
    if (store.sessionId) {
      rememberTitle(store.vault, store.sessionId, text);
    } else {
      // id 还没到：挂起来，等 applyStatus 拿到 sessionId 再写。
      pendingTitle = text;
      pendingVault = store.vault;
    }
    set({ draft: "", busy: true, busyMessage: null });
    try {
      await AgentService.Send(text);
    } catch (err) {
      // 发不出去（没起后端、上一轮还在跑）：如实说出来，别把输入吞掉。
      set({ busy: false, busyMessage: null });
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [push, set, store.draft]);

  /** 停当前这一轮。 */
  const cancel = useCallback(async () => {
    try {
      await AgentService.Cancel();
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [push]);

  /** 关掉后端。 */
  const stop = useCallback(async () => {
    try {
      await AgentService.Stop();
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
    }
    await refresh();
  }, [push, refresh]);

  /** 换模型（会话级）。 */
  const setModel = useCallback(
    async (value: string) => {
      try {
        applyStatus(await AgentService.SetModel(value));
      } catch (err) {
        push({ kind: "notice", text: String(err), isError: true });
      }
    },
    [applyStatus, push],
  );

  /** 列历史会话。 */
  const loadSessions = useCallback(async () => {
    try {
      const list = await AgentService.Sessions();
      // 后端不给 title（只有 id/cwd），所以把本地记的（第一句话 / agent 生成）合并进来。
      const merged = (list ?? []).map((s) => ({ ...s, title: s.title || storedTitle(store.vault, s.id) }));
      set({ sessions: merged, sessionsOpen: true });
    } catch (err) {
      push({ kind: "notice", text: String(err), isError: true });
    }
  }, [push, set]);

  /** 恢复历史会话。 */
  const resume = useCallback(
    async (id: string) => {
      try {
        applyStatus(await AgentService.Resume(id));
        set({ sessionsOpen: false });
        push({ kind: "notice", text: "已切到那个会话（之前的对话内容在后端那边，界面这一版不重放）。", isError: false });
      } catch (err) {
        push({ kind: "notice", text: String(err), isError: true });
      }
    },
    [applyStatus, push, set],
  );

  /** 回答权限提示：**这一步是人点的**（批准必须是人）。 */
  const answer = useCallback(
    async (optionId: string) => {
      const cur = useAgentStore.getState().permission;
      if (!cur) return;
      set({ permission: null });
      try {
        await AgentService.AnswerPermission(cur.id, optionId);
      } catch (err) {
        push({ kind: "notice", text: String(err), isError: true });
      }
    },
    [push, set],
  );

  // 订阅后端推来的事件。
  useEffect(() => {
    const offUpdate = Events.On(EVENT_UPDATE, (e) => {
      const u = e.data as UpdatePayload;
      if (!u) return;
      if (u.kind === "agent_message_chunk" || u.kind === "agent_message") {
        push({ kind: "assistant", text: u.text });
        return;
      }
      if (u.kind === "agent_thought_chunk") {
        push({ kind: "thought", text: u.text });
        return;
      }
      if ((u.kind === "tool_call" || u.kind === "tool_call_update") && u.tool) {
        const map = toolRows.current;
        const existing = map.get(u.tool.id);
        if (existing === undefined) {
          map.set(u.tool.id, useAgentStore.getState().items.length);
          push({ kind: "tool", callId: u.tool.id, title: u.tool.title || "工具调用", status: u.tool.status || "in_progress" });
        } else {
          // 同一次调用结束了：就地更新那一行的状态。
          useAgentStore.setState((s) => ({
            items: s.items.map((it, i) =>
              i === existing && it.kind === "tool" ? { ...it, status: u.tool!.status || it.status } : it,
            ),
          }));
        }
      }
    });

    const offTurn = Events.On(EVENT_TURN, (e) => {
      const t = e.data as TurnPayload;
      useAgentStore.getState().set({ busy: false, busyMessage: null });
      if (t?.error) {
        useAgentStore.getState().push({ kind: "notice", text: t.error, isError: true });
      }
    });

    const offPermission = Events.On(EVENT_PERMISSION, (e) => {
      const p = e.data as PermissionPayload;
      if (!p) return;
      useAgentStore.getState().set({ permission: { id: p.id, tool: p.tool || "工具调用", options: p.options ?? [] } });
    });

    return () => {
      offUpdate();
      offTurn();
      offPermission();
    };
  }, [push]);

  return {
    ...store,
    boot,
    newSession,
    renameSession,
    currentTitle,
    refresh,
    start,
    send,
    cancel,
    stop,
    setModel,
    loadSessions,
    resume,
    answer,
  };
}
