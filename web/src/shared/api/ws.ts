// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { create } from "zustand";
import { dispatch } from "./events";

export type ConnectionState = "connecting" | "online" | "reconnecting" | "offline";

/** The connection state, for the header's "Újracsatlakozás…" bar. */
export const useConnection = create<{ state: ConnectionState; since: number }>(() => ({ state: "offline", since: 0 }));

function setState(state: ConnectionState) {
  if (useConnection.getState().state !== state) useConnection.setState({ state, since: Date.now() });
}

type Socket = Pick<WebSocket, "send" | "close" | "readyState"> & {
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
};

/**
 * The one WebSocket client (docs/08, 5.): hello with the last seq (the
 * session cookie authenticates the browser), replay, resync, and reconnect
 * with backoff. Frames go to the event dispatcher.
 */
export class WsClient {
  private socket: Socket | null = null;
  private lastSeq: number | null = null;
  private attempt = 0;
  private retry: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;

  constructor(
    private readonly url: string,
    private readonly open: (url: string) => Socket = (u) => new WebSocket(u),
  ) {}

  start(): void {
    this.stopped = false;
    this.connect();
  }

  stop(): void {
    this.stopped = true;
    if (this.retry) clearTimeout(this.retry);
    this.socket?.close();
    this.socket = null;
    setState("offline");
  }

  private connect(): void {
    setState(this.attempt === 0 ? "connecting" : "reconnecting");
    const s = this.open(this.url);
    this.socket = s;
    s.onopen = () => {
      s.send(JSON.stringify(this.lastSeq === null ? { type: "hello" } : { type: "hello", last_seq: this.lastSeq }));
    };
    s.onmessage = (ev) => {
      let frame: { type: string; seq?: number; head?: number } & Record<string, unknown>;
      try {
        frame = JSON.parse(String(ev.data));
      } catch {
        return;
      }
      if (frame.type === "welcome") {
        this.attempt = 0;
        if (this.lastSeq === null && typeof frame.head === "number") this.lastSeq = frame.head;
        setState("online");
      }
      if (frame.type === "event" && typeof frame.seq === "number") this.lastSeq = frame.seq;
      if (frame.type === "resync_required" && typeof frame.head === "number") this.lastSeq = frame.head;
      dispatch(frame);
    };
    s.onclose = () => {
      if (this.socket !== s || this.stopped) return;
      this.socket = null;
      this.attempt++;
      setState("reconnecting");
      // 1 s, 2 s, 4 s … up to 30 s, with some jitter.
      const delay = Math.min(30_000, 1000 * 2 ** (this.attempt - 1)) * (0.8 + Math.random() * 0.4);
      this.retry = setTimeout(() => this.connect(), delay);
    };
  }
}

export function wsURL(): string {
  return `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/ws/ui`;
}
