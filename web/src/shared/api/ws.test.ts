// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { onEvent, onResync } from "./events";
import { WsClient, useConnection } from "./ws";

class FakeSocket {
  static all: FakeSocket[] = [];
  readyState = 1;
  sent: unknown[] = [];
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  constructor() {
    FakeSocket.all.push(this);
  }
  send(data: string) {
    this.sent.push(JSON.parse(data));
  }
  close() {}
  frame(f: object) {
    this.onmessage?.({ data: JSON.stringify(f) } as MessageEvent);
  }
}

beforeEach(() => {
  FakeSocket.all = [];
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

test("hello, welcome, typed events, and a reconnect that resumes from the last seq", () => {
  const got: string[] = [];
  const off = onEvent("message.created", (e) => got.push(e.data.body));
  const c = new WsClient("ws://x/ws/ui", () => new FakeSocket() as unknown as WebSocket);
  c.start();
  const s1 = FakeSocket.all[0]!;
  s1.onopen?.(new Event("open"));
  expect(s1.sent).toEqual([{ type: "hello" }]);
  s1.frame({ type: "welcome", protocol: 1, head: 10 });
  expect(useConnection.getState().state).toBe("online");
  s1.frame({ type: "event", seq: 11, event_type: "message.created", actor_id: "a", at: "", data: { body: "szia" } });
  s1.frame({ type: "event", seq: 12, event_type: "unknown.thing", actor_id: "a", at: "", data: {} });
  expect(got).toEqual(["szia"]);

  s1.onclose?.({} as CloseEvent);
  expect(useConnection.getState().state).toBe("reconnecting");
  vi.advanceTimersByTime(1300);
  const s2 = FakeSocket.all[1]!;
  s2.onopen?.(new Event("open"));
  expect(s2.sent).toEqual([{ type: "hello", last_seq: 12 }]);
  c.stop();
  off();
});

test("resync_required reaches the subscribers", () => {
  const resync = vi.fn();
  const off = onResync(resync);
  const c = new WsClient("ws://x/ws/ui", () => new FakeSocket() as unknown as WebSocket);
  c.start();
  FakeSocket.all[0]!.frame({ type: "resync_required", head: 99 });
  expect(resync).toHaveBeenCalledOnce();
  c.stop();
  off();
});
