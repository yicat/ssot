/**
 * 尚未实现的页面。
 *
 * 刻意不去「假装有个页面」：一个空白页会让人以为是加载失败。
 * 这里说清楚「还没做」以及它打算做什么。
 */
export default function Todo({ route }: { route: string }) {
  return (
    <div className="p-6">
      <div className="max-w-lg rounded-lg border border-dashed p-6">
        <h2 className="text-sm font-medium">这个页面还没实现</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          <code className="rounded bg-muted px-1 py-0.5 text-xs">{route}</code>
          {" "}还在开发计划里。左侧导航里带 P2 / P3 标记的项都属于这一类——
          标出来而不是藏起来，免得让人以为它不存在。
        </p>
      </div>
    </div>
  );
}
