/** 场景运行：填外部输入 → 出方案 + 未核验比例。 */
import ScenarioRun from "../../components/custom/ScenarioRun";

export default function Run({ scenario }: { scenario: string | null }) {
  return <ScenarioRun scenario={scenario} />;
}
