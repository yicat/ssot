# frontend/

## 组件落位

- `components/ui/`：shadcn 生成，**勿手改**
- `components/custom/<Name>/`：自研业务组件，固定三件套
  - `index.tsx` —— 渲染与组合
  - `useXxx.ts` —— 交互逻辑
  - `store.ts` —— 状态
- `pages/`：**纯编排**（Layout + 组件组合 + 传参），不写交互逻辑
- `bindings/`：`wails3 generate bindings` 生成，**勿手改**

## 命令

```powershell
npm run dev      # 开发
npm run build    # 构建
npm run test     # vitest
```

## 约定

- 交互逻辑一律进 `useXxx.ts`，页面只负责组装
- 新业务组件必须落在 `components/custom/<Name>/`，不要塞进 `ui/`
- 改动涉及后端契约时，先在 `docs/specs/` 更新规格（见根 `AGENTS.md` 的「开发流程」）
