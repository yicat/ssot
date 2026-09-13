/** 经验：场景下累积的判断与产生它们的会话记录。 */
import Experience from "../../components/custom/Experience";

export default function ExperiencePage({ scenario }: { scenario: string | null }) {
  return <Experience scenario={scenario} />;
}
