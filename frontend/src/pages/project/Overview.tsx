/**
 * 项目概览：定义、规模、质量缺口。
 *
 * 这里的每个数字都必须可追溯到它的来源（哪个项目、哪次加载）——
 * 界面上出现一个不知道从哪来的数字，比没有数字更糟。
 */
import { useSession } from "../../components/custom/Session/useSession";
import { Badge } from "../../components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../components/ui/card";
import { ScrollArea } from "../../components/ui/scroll-area";
import { Separator } from "../../components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../components/ui/table";

export default function ProjectOverview() {
  const s = useSession();
  const ov = s.overview;

  if (!ov) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        {s.loading ? "加载中…" : "没有可显示的项目概览。"}
      </div>
    );
  }

  const pending = ov.byStatus?.["pending"] ?? 0;
  const verified = ov.byStatus?.["verified"] ?? 0;
  // 未核验比例是输出侧的硬规则：任何方案都必须给出它。概览只是提前摆出来。
  const unverifiedRatio = ov.assertions > 0 ? pending / ov.assertions : 0;

  return (
    <ScrollArea className="min-h-0 flex-1">
      <div className="grid gap-4 p-4">
        <div>
          <h1 className="text-lg font-semibold">{ov.name}</h1>
          <p className="text-xs text-muted-foreground">
            {ov.description || "（project.yml 未写描述）"} · {ov.path}
          </p>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Stat title="断言" value={ov.assertions} hint="库里现有的事实条数" />
          <Stat
            title="待核验"
            value={pending}
            hint={`占 ${(unverifiedRatio * 100).toFixed(0)}%——未核验的内容允许使用，但必须可见`}
            tone="amber"
          />
          <Stat title="已核验" value={verified} hint={`核验记录 ${ov.verifications} 条`} tone="emerald" />
          <Stat
            title="冲突"
            value={ov.conflicts}
            hint="同一件事的不同说法，系统不替你裁决"
            tone={ov.conflicts > 0 ? "orange" : undefined}
          />
        </div>

        <div className="grid gap-3 lg:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">按实体</CardTitle>
              <CardDescription>以 schema 声明的实体为准——「定义了却没有数据」正是缺口</CardDescription>
            </CardHeader>
            <CardContent className="px-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>实体</TableHead>
                    <TableHead className="text-right">主体</TableHead>
                    <TableHead className="text-right">断言</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {ov.entities?.map((e) => (
                    <TableRow key={e.entity}>
                      <TableCell>{e.entity}</TableCell>
                      <TableCell className="text-right tabular-nums">{e.subjects}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {e.assertions === 0 ? (
                          <span className="text-amber-700">0（无数据）</span>
                        ) : (
                          e.assertions
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                  {(ov.entities ?? []).length === 0 && (
                    <TableRow>
                      <TableCell colSpan={3} className="text-center text-muted-foreground">
                        schema 中还没有声明任何实体
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">按分级</CardTitle>
              <CardDescription>分级的意义：隐式信息与直引事实必须可分辨</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2">
              {Object.entries(ov.byConfidence ?? {}).length === 0 && (
                <p className="text-xs text-muted-foreground">还没有任何断言。</p>
              )}
              {Object.entries(ov.byConfidence ?? {})
                .sort()
                .map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2 text-sm">
                    <Badge variant="outline" className="border-slate-300 text-slate-700">
                      {k}
                    </Badge>
                    <span className="tabular-nums">{v ?? 0}</span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {ratio(Number(v ?? 0), ov.assertions)}
                    </span>
                  </div>
                ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">定义与规模</CardTitle>
              <CardDescription>项目级的东西不随场景变化</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-2 text-sm">
              <Line k="实体类型" v={String(ov.entityTypes)} />
              <Line k="场景" v={String(ov.scenarioCount)} />
              <Line k="公式" v={String(ov.formulaCount)} />
              <Line k="待判定" v={String(ov.decisions)} />
              <Separator />
              <p className="text-xs leading-relaxed text-muted-foreground">
                场景是「缺什么」的载体：它声明 requires，系统据此把数据缺口
                从「事后发现」变成「事前可见」。选一个场景看它的缺口。
              </p>
            </CardContent>
          </Card>
        </div>

        {ov.scenarioCount === 0 && (
          <Card className="border-dashed">
            <CardContent className="pt-4 text-sm text-muted-foreground">
              该项目还没有场景。场景不拥有数据，它只声明自己需要什么——
              没有场景就没有「缺什么」的可见性。
            </CardContent>
          </Card>
        )}
      </div>
    </ScrollArea>
  );
}

function Stat({
  title,
  value,
  hint,
  tone,
}: {
  title: string;
  value: number;
  hint: string;
  tone?: "amber" | "emerald" | "orange";
}) {
  const cls =
    tone === "amber"
      ? "text-amber-700"
      : tone === "emerald"
        ? "text-emerald-700"
        : tone === "orange"
          ? "text-orange-700"
          : "text-foreground";
  return (
    <Card>
      <CardContent className="pt-4">
        <div className="text-xs text-muted-foreground">{title}</div>
        <div className={"mt-1 text-2xl font-semibold tabular-nums " + cls}>{value}</div>
        <div className="mt-1 text-[11px] leading-snug text-muted-foreground">{hint}</div>
      </CardContent>
    </Card>
  );
}

function Line({ k, v }: { k: string; v: string }) {
  return (
    <div className="flex text-sm">
      <span className="text-muted-foreground">{k}</span>
      <span className="ml-auto tabular-nums">{v}</span>
    </div>
  );
}

function ratio(v: number, total: number): string {
  if (total <= 0) return "—";
  return `${((v / total) * 100).toFixed(1)}%`;
}
