/**
 * 词条渲染：**中文名 + 原始标识符**。
 *
 * 只显示中文名，人就没法把它与数据、CLI、规格里的标识符对上；
 * 只显示标识符，人就看不懂。两个都要，中文在前。
 *
 * 定义里没有中文时只显示标识符——那时**不编名字**：
 * 一个听起来合理的错名字比看不懂更糟。
 *
 * 放在 components/custom/ 而不是 ui/：ui/ 是 shadcn 生成目录，不手改。
 */
import type { ReactNode } from "react";

import { useGlossary, type Named } from "../Session/glossary";

function render(named: Named, className: string): ReactNode {
  if (!named.key) return <span className={className}>—</span>;
  return (
    <span className={className} title={[named.note, named.extra].filter(Boolean).join(" · ") || undefined}>
      {named.label ? (
        <>
          {named.label}
          <span className="ml-1 font-mono text-[10px] text-muted-foreground">{named.key}</span>
        </>
      ) : (
        <span className="font-mono text-[11px]">{named.key}</span>
      )}
    </span>
  );
}

export function Named({ value, className = "" }: { value: Named; className?: string }) {
  return <>{render(value, className)}</>;
}

/**
 * 字段：`shikigami.atk` → 攻击 shikigami.atk
 *
 * short 用于已经按实体分组的表格：那里再重复一遍实体名只是噪音。
 */
export function Field({
  entity,
  predicate,
  short = false,
  className,
}: {
  entity: string;
  predicate: string;
  short?: boolean;
  className?: string;
}) {
  const g = useGlossary();
  const named = g.field(entity, predicate);
  return <>{render(short ? { ...named, key: predicate } : named, className ?? "")}</>;
}

/** 实体：`shikigami` → 式神 shikigami */
export function Entity({ name, className }: { name: string; className?: string }) {
  const g = useGlossary();
  return <>{render(g.entity(name), className ?? "")}</>;
}

/** 单位：`percent` → 百分比 percent */
export function Unit({ name, className }: { name: string; className?: string }) {
  const g = useGlossary();
  return <>{render(g.unit(name), className ?? "")}</>;
}

/** 字段类型：`number` → 数值 number */
export function Type({ name, className }: { name: string; className?: string }) {
  const g = useGlossary();
  return <>{render(g.type(name), className ?? "")}</>;
}

/** 分级：`L2` → 结构化 L2 */
export function Confidence({ value, className }: { value: string; className?: string }) {
  const g = useGlossary();
  return <>{render(g.confidence(value), className ?? "")}</>;
}

/** 断言状态 */
export function AssertionStatus({ value, className }: { value: string; className?: string }) {
  const g = useGlossary();
  return <>{render(g.status(value), className ?? "")}</>;
}

/** 参与者：`human:ngnl5` → 人：ngnl5 */
export function Actor({ id, className }: { id: string; className?: string }) {
  const g = useGlossary();
  return <span className={className}>{g.actor(id)}</span>;
}

/** 主体：`shikigami/262` → 姑获鸟（262） */
export function Subject({
  entity,
  subject,
  className,
}: {
  entity: string;
  subject: string;
  className?: string;
}) {
  const g = useGlossary();
  return (
    <span className={className} title={`${entity}/${subject}`}>
      {g.subject(entity, subject)}
    </span>
  );
}

/** 依赖：`assert:a1` → 断言 a1 */
export function Dep({ value, className }: { value: string; className?: string }) {
  const g = useGlossary();
  return <span className={className}>{g.dep(value)}</span>;
}

/** 修订标识：`revid:8053` → 修订 8053 */
export function Revision({ value, className }: { value: string; className?: string }) {
  const g = useGlossary();
  return <span className={className}>{g.revision(value)}</span>;
}

/** 参与计算的取值：`3082 point` → 3082 点数 point */
export function Quantity({
  value,
  unit,
  className,
}: {
  value: string;
  unit: string;
  className?: string;
}) {
  const g = useGlossary();
  return <span className={className}>{g.quantity(value, unit)}</span>;
}
/**
 * 需求项：`shikigami.atk` → 攻击 shikigami.atk
 *
 * 场景的 requires 就是「实体.谓词」，直接显示等于让人自己去查定义。
 */
export function Requirement({ want, className }: { want: string; className?: string }) {
  const g = useGlossary();
  const i = want.indexOf(".");
  if (i < 0) return <span className={className}>{want}</span>;
  return <>{render(g.field(want.slice(0, i), want.slice(i + 1)), className ?? "")}</>;
}
/**
 * 一条被依赖的断言：`skill/262_01 ratio` → 天翔鹤斩（262_01） 一级伤害倍率
 *
 * subject 是 `实体/主体` 形式的复合标识，因此这里拆开再查词表。
 */
export function ClaimRef({
  claim,
  className,
}: {
  claim: { subject: string; predicate: string };
  className?: string;
}) {
  const g = useGlossary();
  const i = claim.subject.indexOf("/");
  if (i < 0) return <span className={className}>{claim.subject}</span>;
  const entity = claim.subject.slice(0, i);
  const subject = claim.subject.slice(i + 1);
  return (
    <span className={className}>
      {g.subject(entity, subject)}
      <span className="ml-1">
        <Field entity={entity} predicate={claim.predicate} />
      </span>
    </span>
  );
}