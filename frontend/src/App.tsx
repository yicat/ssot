/**
 * 应用入口。
 *
 * ⚠️ 当前状态：**骨架**。上一套方案（六部件 + 断言库 + 核验流程）已整体作废，
 * 页面与业务组件已清空，新的设计待定。
 *
 * 现在这里只证明一件事：Wails 的 bindings 是通的——顶栏能把项目列出来、能切换。
 * 新方案的页面按设计重写，落位约定不变（见 frontend/AGENTS.md）：
 *   components/ui/      shadcn 生成，勿手改
 *   components/custom/  自研业务组件：index.tsx + useXxx.ts + store.ts
 *   pages/              纯编排，不写交互逻辑
 */
import { useEffect, useState } from "react";

import { Current, Open, Projects } from "../bindings/github.com/ngnl5/ssot/internal/api/projectservice";
import type { ProjectRef, SessionState } from "../bindings/github.com/ngnl5/ssot/internal/api/models";
import { Button } from "./components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./components/ui/card";
import { Toaster } from "./components/ui/sonner";

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

  const noProjects = error !== null && error.includes("没有可用项目");

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <header className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-border bg-card px-5 py-3">
        <div className="text-sm font-semibold">SSOT 工作台</div>
        <div className="text-xs text-muted-foreground">
          当前项目：
          {session ? `${session.name}（${session.dir}）` : "（未打开）"}
        </div>
        <div className="flex flex-wrap gap-2">
          {projects.map((p) => (
            <Button
              key={p.dir}
              size="sm"
              variant={p.current || session?.dir === p.dir ? "default" : "secondary"}
              onClick={() =>
                void (async () => {
                  try {
                    setSession(await Open(p.dir));
                    setError(null);
                  } catch (e) {
                    setError(String(e));
                  }
                })()
              }
            >
              {p.name}
            </Button>
          ))}
        </div>
      </header>

      {error && !noProjects && (
        <div className="border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-800">{error}</div>
      )}

      <main className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-8">
        <Card className="max-w-2xl">
          <CardHeader>
            <CardTitle>{noProjects ? "还没有项目" : "骨架就绪，等新方案"}</CardTitle>
            <CardDescription>
              {noProjects
                ? "项目根目录下还没有任何项目。建一个含 project.yml 的目录就能在这里选到它。"
                : "上一套方案（六部件 + 断言库 + 核验流程）已整体作废：业务代码、规格集与示例数据全部清除，只保留技术栈与骨架。"}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm text-muted-foreground">
            {noProjects && (
              <pre className="rounded bg-secondary/60 p-3 text-xs">
{`# projects/<名字>/project.yml
project: 项目名
description: 一句话说明`}
              </pre>
            )}
            <p>
              技术栈：Go + Wails v3（bindings）+ Vite / React / shadcn，构建编排走 Taskfile
              （<code>wails3 task dev | build | check | test | run:server</code>）。
            </p>
            <p>
              还没有任何业务页面。设计定下来之后，页面落在 <code>pages/</code>、
              交互落在 <code>components/custom/&lt;Name&gt;/</code>。
            </p>
            <p>作废的那套实现留在分支 legacy/mvp-v1 上备查。</p>
          </CardContent>
        </Card>
      </main>

      <Toaster position="bottom-right" />
    </div>
  );
}
