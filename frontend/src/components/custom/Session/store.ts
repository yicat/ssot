/**
 * 会话状态：当前项目 + 当前场景 + 当前页面。
 *
 * 只放状态与纯 setter——取数在 useSession.ts。这不是形式主义：
 * 「项目加载失败时界面该显示什么」是个必须一次想清楚的问题，
 * 混在渲染里就只会变成「白屏」。
 */
import { create } from "zustand";

import type {
  ProjectOverview,
  ProjectRef,
  SessionState as SessionSnapshot,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";

/**
 * 页面路由。
 *
 * 用字符串而不是对象：它要能直接与左侧导航的 key 比较，
 * 也要能一眼看懂当前在哪——`"scenario.run"` 比 `{kind,page}` 更省事。
 */
export type Route =
  | "project.overview"
  | "project.data"
  | "project.formulas"
  | "scenario.overview"
  | "scenario.run"
  | "scenario.experience"
  | "scenario.alternatives"
  | "review.queue"
  | "review.conflicts"
  | "review.decision";

type SessionStore = {
  projects: ProjectRef[];
  session: SessionSnapshot | null;
  overview: ProjectOverview | null;
  route: Route;
  /** 当前使用者。它是会话级事实：在核验里填了一次，经验与待判定里不该再填一次。 */
  by: string;
  loading: boolean;
  /** 加载失败的原因。**必须显示**：留白与「加载失败」在界面上无法区分。 */
  error: string | null;

  setProjects: (v: ProjectRef[]) => void;
  setSession: (v: SessionSnapshot | null) => void;
  setOverview: (v: ProjectOverview | null) => void;
  setRoute: (v: Route) => void;
  setBy: (v: string) => void;
  setLoading: (v: boolean) => void;
  setError: (v: string | null) => void;
  reset: () => void;
};

export const useSessionStore = create<SessionStore>((set) => ({
  projects: [],
  session: null,
  overview: null,
  route: "project.overview",
  by: "",
  loading: false,
  error: null,

  setProjects: (projects) => set({ projects }),
  setSession: (session) => set({ session }),
  setOverview: (overview) => set({ overview }),
  setRoute: (route) => set({ route }),
  setBy: (by) => set({ by }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () =>
    set({ projects: [], session: null, overview: null, route: "project.overview", by: "", error: null }),
}));

/** 当前场景名；null 表示未选中或该项目没有场景。 */
export function currentScenario(s: SessionStore): string | null {
  return s.session?.scenario ?? null;
}
