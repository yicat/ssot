/** 核验队列：按优先级核验断言。编排在 components/custom/Review 里。 */
import Review from "../../components/custom/Review";

export default function Queue() {
  return <Review mode="queue" />;
}
