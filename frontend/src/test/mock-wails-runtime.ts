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

const events = createEventsMock();

export const wailsMock = {
  events,
  module: {
    Events: { On: events.on, Off: events.off, OffAll: vi.fn(), Emit: vi.fn() },
    WML: { Reload: vi.fn() },
  },
};
