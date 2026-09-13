/**
 * 应用入口。
 *
 * 它是**纯编排**：一个布局组件 + Toaster。所有交互逻辑都在
 * components/custom/<Name>/ 里，页面只负责组装（见 frontend/AGENTS.md）。
 */
import { Toaster } from "./components/ui/sonner";
import Workspace from "./pages/Workspace";

export default function App() {
  return (
    <>
      <Workspace />
      <Toaster position="bottom-right" />
    </>
  );
}
