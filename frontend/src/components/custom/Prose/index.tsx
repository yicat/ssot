/**
 * 行内文字：把项目里已经在用的记号渲染出来。
 *
 * 界面上的说明文字有两个来源，**两处都写了 markdown 记号**：
 *
 *  - 前端自己的说明（`components/custom/*`）：作者照着规格写，顺手写了 `**强调**`
 *  - 项目定义里的 `description` / `note`（YAML）：同样的写法
 *
 * 这些记号在 JSX 文本里、在纯文本插值里**都不生效**——直接渲染只会让人
 * 看到一堆星号和反引号。所以约定这两个记号的含义，并由这里统一渲染：
 *
 *  - `**这样**` → 加粗（强调）
 *  - `` `这样` `` → 等宽（标识符、代码、字段名）
 *
 * 只做这两个，**不引入 markdown 解析器**：多支持一种语法就多一种写错的方式，
 * 而这里真正需要的只有「强调」与「这是标识符」。
 *
 * 前端自己的说明文字直接写 `<strong>`，不必经过这里；
 * 需要过这里的只有「内容是字符串」的场合：YAML 来的文本、作为值的提示语。
 */
import type { ReactNode } from "react";

/** 捕获组让 split 保留分隔符，因此奇数下标就是被标记的片段。 */
const TOKEN = /(\*\*[^*]+\*\*|`[^`]+`)/g;

export function Em({ children, className }: { children: ReactNode; className?: string }) {
  if (typeof children !== "string") return <span className={className}>{children}</span>;
  return (
    <span className={className}>
      {children.split(TOKEN).map((part, i) => {
        if (part.length > 4 && part.startsWith("**") && part.endsWith("**")) {
          return <strong key={i}>{part.slice(2, -2)}</strong>;
        }
        if (part.length > 2 && part.startsWith("`") && part.endsWith("`")) {
          return (
            <code key={i} className="rounded bg-muted px-1 font-mono text-[11px]">
              {part.slice(1, -1)}
            </code>
          );
        }
        return part;
      })}
    </span>
  );
}
