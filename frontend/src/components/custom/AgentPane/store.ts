/**
 * AgentPane 的状态（交互逻辑在 useAgent.ts，渲染在 index.tsx）。
 *
 * 消息流是**追加**的：ACP 的更新是一条条来的（消息块、思考块、工具调用），
 * 我们不能等到整轮结束才显示——那样聊天就变成了「转圈等结果」。
 */
import { create } from "zustand";

import type { AgentModelOption, AgentSessionRef } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

/**
 * 事件载荷的类型**本地声明**：bindings 生成器只走服务方法的签名，
 * 而权限提示是**事件**（`agent:permission`）推过来的，所以它们不在 models.ts 里。
 * 形状以 Go 侧 internal/api/agent.go 的定义为准，改那边要一起改这里。
 */
export type AgentPermissionOp = {
  optionId: string;
  name: string;
  kind: string;
};

/** 聊天里的一个条目。 */
export type ChatItem =
  | {
      kind: "user";
      /** 本地序号，做 key 用。 */
      id: number;
      text: string;
    }
  | {
      kind: "assistant";
      id: number;
      text: string;
    }
  | {
      kind: "thought";
      id: number;
      text: string;
    }
  | {
      kind: "tool";
      id: number;
      /** 工具调用 id：同一次调用的开始/结束要能对上（否则轨迹会两行）。 */
      callId: string;
      title: string;
      status: string;
    }
  | {
      kind: "notice";
      id: number;
      text: string;
      isError: boolean;
    };

/** 权限弹窗的状态。 */
export type PendingPermission = {
  id: string;
  tool: string;
  options: AgentPermissionOp[];
};

/** 追加时不用给 id（store 自己发号）。 */
// 注意：`Omit<A | B, "id">` 在联合类型上**不分配**（会把共有字段留下、把分支字段丢掉），
// 所以要写成分配式的形式——不这么写就会得到「text 不存在于 Omit<ChatItem,"id">」这种报错。
type DistributiveOmit<T, K extends PropertyKey> = T extends unknown ? Omit<T, K> : never;
export type NewChatItem = DistributiveOmit<ChatItem, "id">;

export type AgentState = {
  /** 后端起来了、会话开了才为 true。 */
  running: boolean;
  /** 正在跑一轮：这时只能停，不能再发。 */
  busy: boolean;
  agent: string;
  version: string;
  sessionId: string;
  vault: string;
  /** 当前模型与可选清单（来自会话的 configOptions，不是全局配置）。 */
  model: string;
  models: AgentModelOption[];
  /** 已经聊了什么。 */
  items: ChatItem[];
  /** 输入框内容。 */
  draft: string;
  /** 等人点的权限提示。 */
  permission: PendingPermission | null;
  /** 历史会话（来自后端，已按 vault 过滤）。 */
  sessions: AgentSessionRef[];
  /** 会话列表是否展开。 */
  sessionsOpen: boolean;
  busyMessage: string | null;
  set: (patch: Partial<AgentState>) => void;
  /** 往聊天里追加一条（自动分配 id）。 */
  push: (item: NewChatItem) => void;
  reset: () => void;
};

let seq = 0;

const initial = {
  running: false,
  busy: false,
  agent: "",
  version: "",
  sessionId: "",
  vault: "",
  model: "",
  models: [] as AgentModelOption[],
  items: [] as ChatItem[],
  draft: "",
  permission: null as PendingPermission | null,
  sessions: [] as AgentSessionRef[],
  sessionsOpen: false,
  busyMessage: null as string | null,
};

export const useAgentStore = create<AgentState>((set) => ({
  ...initial,
  set: (patch) => set(patch),
  push: (item) => set((s) => ({ items: [...s.items, { ...item, id: ++seq } as ChatItem] })),
  reset: () => set({ ...initial, items: [] }),
}));

