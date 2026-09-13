/**
 * 会话的交互逻辑：发现项目、切换项目、选场景。
 *
 * 三条规格要求落在这里（docs/specs/workspace.spec.md）：
 *  1. 「一个都没有」与「加载失败」必须能区分——前者是提示，后者是错误
 *  2. 切换项目失败时**保持原项目不变**（后端保证），界面照实提示
 *  3. 切换项目后当前场景跟着重置（后端保证），界面不缓存旧场景
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import { Current, Open, Overview, Projects } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/projectservice";
import { Select } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/scenarioservice";
import type {
  ProjectOverview,
  ProjectRef,
  SessionState as SessionSnapshot,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { useSessionStore, type Route } from "./store";

export type SessionApi = {
  projects: ProjectRef[];
  session: SessionSnapshot | null;
  overview: ProjectOverview | null;
  route: Route;
  loading: boolean;
  error: string | null;
  scenario: string | null;
  reload: () => Promise<void>;
  openProject: (dir: string) => Promise<void>;
  selectScenario: (name: string) => Promise<void>;
  go: (r: Route) => void;
};

export function useSession(): SessionApi {
  const s = useSessionStore();
  const session = s.session;
  const scenario = session?.scenario ?? null;

  const reload = useCallback(async () => {
    s.setLoading(true);
    s.setError(null);
    try {
      const list = (await Projects()) ?? [];
      s.setProjects(list);
      if (list.length === 0) {
        // 「一个都没有」不是错误，但它必须说出来：留白会让人以为是加载中。
        s.setSession(null);
        s.setOverview(null);
        s.setError(
          "没有可用项目——一个项目就是一个含 project.yml 的目录。先建一个，或检查项目根目录。",
        );
        return;
      }
      const snap = await Current();
      s.setSession(snap);
      s.setOverview(await Overview());
    } catch (e) {
      s.setError(String(e));
    } finally {
      s.setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const openProject = useCallback(
    async (dir: string) => {
      if (!dir) return;
      s.setLoading(true);
      s.setError(null);
      try {
        const snap = await Open(dir);
        s.setSession(snap);
        s.setOverview(await Overview());
        toast.success(`已打开项目 ${snap.name}`);
      } catch (e) {
        // 后端保证失败时项目不变，因此这里只需要如实报错，
        // 不必回滚界面状态——那反而可能把一个好的状态覆盖成坏的。
        s.setError(String(e));
        toast.error(String(e));
      } finally {
        s.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const selectScenario = useCallback(
    async (name: string) => {
      if (!name) return;
      s.setLoading(true);
      try {
        s.setSession(await Select(name));
      } catch (e) {
        s.setError(String(e));
        toast.error(String(e));
      } finally {
        s.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  return {
    projects: s.projects,
    session,
    overview: s.overview,
    route: s.route,
    loading: s.loading,
    error: s.error,
    scenario,
    reload,
    openProject,
    selectScenario,
    go: s.setRoute,
  };
}
