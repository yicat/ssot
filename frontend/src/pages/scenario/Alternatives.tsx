/** 备选方案：多解并排，系统不裁决。 */
import Alternatives from "../../components/custom/Alternatives";

export default function AlternativesPage({ scenario }: { scenario: string | null }) {
  return <Alternatives scenario={scenario} />;
}
