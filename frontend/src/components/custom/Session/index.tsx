/**
 * 会话栏与左侧导航。
 *
 * 导航按三层模型分区：**项目**（不随场景变化的定义与数据）、
 * **场景**（随选中场景变化的用途）、**核验**（人唯一需要介入的地方）。
 * 这样分区回答了一个具体问题：换个场景，哪些数字不该动。
 */
import { ChevronRight, FolderOpen, RefreshCw } from "lucide-react";

import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Label } from "../../ui/label";
import { ScrollArea } from "../../ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/select";
import { Separator } from "../../ui/separator";
import type { Route } from "./store";
import { useSession } from "./useSession";

type NavItem = {
  route: Route;
  label: string;
  hint: string;
  /** 尚未实现的项照实标出来，而不是藏起来——藏起来会让人以为它不存在。 */
  todo?: string;
};

const PROJECT_NAV: NavItem[] = [
  { route: "project.overview", label: "概览", hint: "定义、规模、质量缺口" },
  { route: "project.data", label: "数据", hint: "断言库、schema 与覆盖率" },
  { route: "project.formulas", label: "公式", hint: "公式与算例验证状态" },
  { route: "project.sources", label: "来源", hint: "不可拆的事实源：原文、机制说明、依据声明" },
];

const SCENARIO_NAV: NavItem[] = [
  { route: "scenario.overview", label: "概览", hint: "需要什么、缺什么、能不能跑" },
  { route: "scenario.run", label: "运行", hint: "填外部输入，出方案" },
  { route: "scenario.experience", label: "经验", hint: "场景下累积的判断与依据" },
  { route: "scenario.alternatives", label: "备选方案", hint: "同一需求的多解比较" },
];

const REVIEW_NAV: NavItem[] = [
  { route: "review.queue", label: "队列", hint: "按优先级核验断言" },
  { route: "review.conflicts", label: "冲突", hint: "同一件事的不同说法" },
  { route: "review.decision", label: "待判定", hint: "原文里的歧义，必须由人选" },
];

/** 顶部：当前项目与当前场景。 */
export function SessionBar() {
  const s = useSession();
  const scenarios = s.session?.scenarios ?? [];

  return (
    <header className="flex flex-wrap items-end gap-x-6 gap-y-3 border-b border-border bg-card px-5 py-3">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <FolderOpen className="size-4 text-muted-foreground" />
        SSOT 工作台
      </div>

      <div className="flex items-end gap-2">
        <div className="grid gap-1">
          <Label htmlFor="project-picker" className="text-xs text-muted-foreground">
            项目
          </Label>
          <Select value={s.session?.dir ?? ""} onValueChange={(v) => void s.openProject(v)}>
            <SelectTrigger id="project-picker" className="h-8 w-56" aria-label="项目">
              <SelectValue placeholder="选择项目" />
            </SelectTrigger>
            <SelectContent>
              {s.projects.map((p) => (
                <SelectItem key={p.dir} value={p.dir}>
                  {p.name}
                  <span className="ml-2 text-xs text-muted-foreground">{p.dir}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {/* 两个项目目录同名是允许的，因此界面上必须显示完整路径 */}
        <span className="pb-1.5 text-xs text-muted-foreground">{s.session?.projectDir ?? "—"}</span>
      </div>

      <div className="grid gap-1">
        <Label htmlFor="scenario-picker" className="text-xs text-muted-foreground">
          场景
        </Label>
        <Select
          value={s.scenario ?? ""}
          onValueChange={(v) => void s.selectScenario(v)}
          disabled={scenarios.length === 0}
        >
          <SelectTrigger id="scenario-picker" className="h-8 w-56" aria-label="场景">
            <SelectValue placeholder={scenarios.length === 0 ? "该项目还没有场景" : "选择场景"} />
          </SelectTrigger>
          <SelectContent>
            {scenarios.map((sc) => (
              <SelectItem key={sc.name} value={sc.name}>
                {sc.name}
                {!sc.requiresMet && (
                  <span className="ml-2 text-xs text-amber-600">
                    {sc.requiresTotal} 项需求未满足
                  </span>
                )}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="ml-auto flex items-center gap-3">
        {s.overview && (
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span>
              断言 <b className="tabular-nums text-foreground">{s.overview.assertions}</b>
            </span>
            <span>
              待核验{" "}
              <b className="tabular-nums text-amber-600">
                {s.overview.byStatus?.["pending"] ?? 0}
              </b>
            </span>
            <span>
              冲突 <b className="tabular-nums text-orange-600">{s.overview.conflicts}</b>
            </span>
            <Badge variant="outline" className="border-sky-300 text-sky-700">
              待判定 {s.overview.decisions}
            </Badge>
          </div>
        )}
        <Button
          size="sm"
          variant="outline"
          disabled={s.loading}
          onClick={() => void s.reload()}
          aria-label="刷新"
        >
          <RefreshCw className="size-3.5" />
          刷新
        </Button>
      </div>
    </header>
  );
}

/** 左栏：按层分区。 */
export function AppNav() {
  const s = useSession();
  const hasScenario = (s.session?.scenarios?.length ?? 0) > 0;

  return (
    <nav className="flex w-56 shrink-0 flex-col border-r border-border bg-card">
      <ScrollArea className="flex-1">
        <div className="grid gap-1 p-3">
          <Group title="项目" items={PROJECT_NAV} current={s.route} onGo={s.go} />
          <Separator className="my-2" />
          <Group
            title={s.scenario ? `场景：${s.scenario}` : "场景"}
            items={SCENARIO_NAV}
            current={s.route}
            onGo={s.go}
            disabled={!hasScenario}
            disabledHint="该项目还没有场景"
          />
          <Separator className="my-2" />
          <Group title="核验" items={REVIEW_NAV} current={s.route} onGo={s.go} />
        </div>
      </ScrollArea>
    </nav>
  );
}

function Group({
  title,
  items,
  current,
  onGo,
  disabled = false,
  disabledHint = "",
}: {
  title: string;
  items: NavItem[];
  current: Route;
  onGo: (r: Route) => void;
  disabled?: boolean;
  disabledHint?: string;
}) {
  return (
    <div>
      <div className="px-2 py-1 text-xs font-medium text-muted-foreground">{title}</div>
      {disabled && disabledHint && (
        <p className="px-2 pb-1 text-[11px] text-muted-foreground">{disabledHint}</p>
      )}
      {items.map((it) => {
        const active = current === it.route;
        const blocked = disabled || Boolean(it.todo);
        return (
          <button
            key={it.route}
            type="button"
            disabled={blocked}
            onClick={() => onGo(it.route)}
            title={it.todo ? `${it.hint}——尚未实现（${it.todo}）` : it.hint}
            className={
              "flex w-full items-center gap-1 rounded-md px-2 py-1.5 text-left text-sm " +
              (active
                ? "bg-secondary font-medium text-secondary-foreground"
                : "text-foreground hover:bg-secondary/60") +
              (blocked ? " cursor-not-allowed opacity-40" : "")
            }
          >
            <ChevronRight
              className={"size-3.5 shrink-0 " + (active ? "opacity-100" : "opacity-0")}
            />
            {it.label}
            {it.todo && (
              <span className="ml-auto text-[10px] text-muted-foreground">{it.todo}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}
