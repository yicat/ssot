/**
 * Wails runtime 的测试替身。
 *
 * 生成的 bindings 通过 `$Call.ByID(<数字 ID>, ...)` 调后端，因此这里按 ID 派发——
 * 测试因此可以精确控制每个后端方法的返回值，而不必 mock 整个服务。
 */
import { vi } from "vitest";

export type WailsCallback = (ev: { name: string; data: unknown }) => void;

function createEventsMock() {
  const listeners = new Map<string, WailsCallback[]>();
  const on = vi.fn((name: string, cb: WailsCallback) => {
    const arr = listeners.get(name) ?? [];
    arr.push(cb);
    listeners.set(name, arr);
    return () => {
      const next = (listeners.get(name) ?? []).filter((c) => c !== cb);
      listeners.set(name, next);
    };
  });
  const off = vi.fn();
  const emit = (name: string, data: unknown) => {
    for (const cb of listeners.get(name) ?? []) cb({ name, data });
  };
  const reset = () => {
    listeners.clear();
    on.mockClear();
    off.mockClear();
  };
  return { listeners, on, off, emit, reset };
}

/** 后端方法的调用 ID，与 bindings 中生成的一致。 */
export const CallID = {
  Approve: 3028409479,
  History: 3004648246,
  Pending: 1871646081,
  ProjectDir: 50586622,
  Reject: 888414013,
  Stats: 268982341,
} as const;

type Handler = (args: unknown[]) => unknown;

const handlers = new Map<number, Handler>();

export const callMock = {
  handlers,
  /** 注册某个后端方法的返回值。抛错即模拟后端失败。 */
  on(id: number, h: Handler) {
    handlers.set(id, h);
    return this;
  },
  reset() {
    handlers.clear();
  },
};

const events = createEventsMock();

const identity = (v: unknown) => v;

export const wailsMock = {
  events,
  module: {
    Call: {
      ByID: vi.fn((id: number, ...args: unknown[]) => {
        const h = handlers.get(id);
        if (!h) return Promise.resolve(null);
        try {
          return Promise.resolve(h(args));
        } catch (e) {
          return Promise.reject(e);
        }
      }),
    },
    Create: {
      Any: identity,
      Array: () => identity,
      Map: () => identity,
    },
    CancellablePromise: class {},
    Events: { On: events.on, Off: events.off, OffAll: vi.fn(), Emit: vi.fn() },
    WML: { Reload: vi.fn() },
  },
};

// 具名导出：vitest 通过别名把 `@wailsio/runtime` 指向本文件，
// 因此这里必须提供 bindings 实际引用的那几个名字。
export const Call = wailsMock.module.Call;
export const Create = wailsMock.module.Create;
export const CancellablePromise = wailsMock.module.CancellablePromise;
export const Events = wailsMock.module.Events;
export const WML = wailsMock.module.WML;
