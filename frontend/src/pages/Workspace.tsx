/**
 * 工作台布局：顶栏（项目 / 场景）+ 左栏（按层分区）+ 内容。
 *
 * 页面只做编排：取会话、选页面、组装。交互逻辑在各自的
 * components/custom/<Name>/useXxx.ts 里（见 frontend/AGENTS.md）。
 */
import { AppNav, SessionBar } from "../components/custom/Session";
import { useSession } from "../components/custom/Session/useSession";
import ProjectOverview from "./project/Overview";
import Data from "./project/Data";
import Formulas from "./project/Formulas";
import ScenarioOverview from "./scenario/Overview";
import Run from "./scenario/Run";
import Experience from "./scenario/Experience";
import Queue from "./review/Queue";
import Conflicts from "./review/Conflicts";
import Decision from "./review/Decision";
import Todo from "./Todo";

export default function Workspace() {
  const s = useSession();

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <SessionBar />

      {s.error && (
        <div className="border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-800">
          {s.error}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <AppNav />
        <main className="flex min-h-0 flex-1 flex-col">
          {renderRoute(s.route, s.scenario)}
        </main>
      </div>
    </div>
  );
}

function renderRoute(route: string, scenario: string | null) {
  switch (route) {
    case "project.overview":
      return <ProjectOverview />;
    case "project.data":
      return <Data />;
    case "project.formulas":
      return <Formulas />;
    case "scenario.overview":
      return <ScenarioOverview scenario={scenario} />;
    case "scenario.run":
      return <Run scenario={scenario} />;
    case "scenario.experience":
      return <Experience scenario={scenario} />;
    case "review.queue":
      return <Queue />;
    case "review.conflicts":
      return <Conflicts />;
    case "review.decision":
      return <Decision />;
    default:
      return <Todo route={route} />;
  }
}
