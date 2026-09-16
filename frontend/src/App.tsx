/**
 * 应用入口。
 *
 * 两层数据在这里汇合：
 *  - **会话**（哪些项目、当前是哪个）——顶栏用它，切项目也在顶栏；
 *  - **文档库**（当前项目的 vault）——交给 pages/VaultPage。
 *
 * 编排放这里，交互逻辑放 components/custom/ 下的组件里（见 frontend/AGENTS.md）。
 */
import { useEffect, useState } from "react";

import { Current, Open, Projects } from "../bindings/github.com/ngnl5/ssot/internal/api/projectservice";
import type { ProjectRef, SessionState } from "../bindings/github.com/ngnl5/ssot/internal/api/models";
import { AppTitleBar } from "./components/custom/AppTitleBar";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./components/ui/card";
import { Toaster } from "./components/ui/sonner";
import { VaultPage } from "./pages/VaultPage";

export default function App() {
  const [projects, setProjects] = useState<ProjectRef[]>([]);
  const [session, setSession] = useState<SessionState | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        setProjects((await Projects()) ?? []);
        setSession(await Current());
        setError(null);
      } catch (e) {
        // 「一个项目都没有」不是「出错了」——两者必须能区分，
        // 否则人会把「还没建项目」当成工具坏了。
        setError(String(e));
      }
    })();
  }, []);

  // 切换失败时会话不变（后端保证全有或全无），界面不会塌成空白。
  const openProject = (dir: string) => {
    void (async () => {
      try {
        setSession(await Open(dir));
        setError(null);
      } catch (e) {
        setError(String(e));
      }
    })();
  };

  const noProjects = error !== null && error.includes("没有可用项目");

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <AppTitleBar session={session} projects={projects} onOpenProject={openProject} />

      {error && !noProjects && (
        <div className="border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-800">{error}</div>
      )}

      {noProjects ? (
        <main className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-8">
          <Card className="max-w-2xl">
            <CardHeader>
              <CardTitle>还没有项目</CardTitle>
              <CardDescription>
                项目根目录下还没有任何项目。建一个含 project.yml 的目录，界面里就能选到它。
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <pre className="rounded bg-secondary/60 p-3 text-xs">
{`# projects/<名字>/project.yml
project: 项目名
description: 一句话说明

# 一个项目就是一个 vault：
#   raw/     抓来的原文（原样留存）
#   docs/    整理好的文档（人和 agent 都能改）
#   tables/  数据表（csv / json / yaml）`}
              </pre>
              <p>
                约定见 <code>docs/specs/vault.spec.md</code>；命令行用法见 <code>ssot help</code>。
              </p>
            </CardContent>
          </Card>
        </main>
      ) : (
        // key 跟着项目走：换项目就重挂载，文档库自然重新加载。
        <VaultPage key={session?.dir ?? "none"} />
      )}

      <Toaster position="bottom-right" />
    </div>
  );
}
