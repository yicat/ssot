/**
 * 文档库页面。
 *
 * 页面 = **纯编排**：只把组件摆好、参数传下去，不写交互逻辑
 * （交互在 components/custom/VaultBrowser/useVaultBrowser.ts）。
 */
import { VaultBrowser } from "../components/custom/VaultBrowser";

export function VaultPage() {
  return <VaultBrowser />;
}
