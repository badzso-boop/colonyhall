import type { components } from "./schema";

type S = components["schemas"];

/** Event type → its payload (the mapping in the WsEvent description). */
export type EventPayloads = {
  "message.created": S["EventMessageCreated"];
  "message.retracted": S["EventMessageRetracted"];
  "reaction.added": S["EventReaction"];
  "reaction.removed": S["EventReaction"];
  "channel.created": S["EventChannelCreated"];
  "channel.member_added": S["EventMemberAdded"];
  "channel.member_removed": S["EventMemberRemoved"];
  "actor.created": S["EventActorCreated"];
  "actor.state_changed": S["EventActorStateChanged"];
  "actor.profile_updated": S["EventActorProfileUpdated"];
  "workspace.emergency_activated": S["EventEmergency"];
  "workspace.emergency_cleared": S["EventEmergency"];
  "delivery.item_created": S["EventDeliveryItemCreated"];
};

export type KnownEvent = keyof EventPayloads;
/** The envelope of a durable event, with its typed payload. */
export type AppEvent<T extends KnownEvent> = Omit<S["WsEvent"], "data" | "event_type"> & { event_type: T; data: EventPayloads[T] };

export type ActivityFrame = S["WsActivity"];
export type ProgressFrame = S["WsProgress"];

type Handler = (e: never) => void;

const eventHandlers = new Map<string, Set<Handler>>();
const activityHandlers = new Set<(f: ActivityFrame) => void>();
const progressHandlers = new Set<(f: ProgressFrame) => void>();
const resyncHandlers = new Set<() => void>();

function add<T>(set: Set<T>, h: T): () => void {
  set.add(h);
  return () => void set.delete(h);
}

/**
 * Subscribe to a durable event type. Features do this in their events.ts
 * (docs/08, 5.) and update their query cache. Returns the unsubscribe.
 */
export function onEvent<T extends KnownEvent>(type: T, handler: (e: AppEvent<T>) => void): () => void {
  let set = eventHandlers.get(type);
  if (!set) eventHandlers.set(type, (set = new Set()));
  return add(set, handler as Handler);
}

/**
 * Subscribe to an event type whose payload the contract does not describe
 * (yet): the handler only learns that it happened. Use it to refetch.
 */
export function onUntypedEvent(type: string, handler: () => void): () => void {
  let set = eventHandlers.get(type);
  if (!set) eventHandlers.set(type, (set = new Set()));
  return add(set, handler as Handler);
}

/** An agent started or stopped working somewhere (ephemeral). */
export function onActivity(handler: (f: ActivityFrame) => void): () => void {
  return add(activityHandlers, handler);
}

/** Live steps of an agent's running work (ephemeral). */
export function onProgress(handler: (f: ProgressFrame) => void): () => void {
  return add(progressHandlers, handler);
}

/** The server could not replay the gap: every feature refetches its data. */
export function onResync(handler: () => void): () => void {
  return add(resyncHandlers, handler);
}

/** Called by the WebSocket client for each frame. Unknown types are ignored. */
export function dispatch(frame: { type: string } & Record<string, unknown>): void {
  switch (frame.type) {
    case "event": {
      const set = eventHandlers.get(frame.event_type as string);
      set?.forEach((h) => (h as (e: unknown) => void)(frame));
      return;
    }
    case "agent.activity":
      activityHandlers.forEach((h) => h(frame as unknown as ActivityFrame));
      return;
    case "agent.progress":
      progressHandlers.forEach((h) => h(frame as unknown as ProgressFrame));
      return;
    case "resync_required":
      resyncHandlers.forEach((h) => h());
      return;
  }
}
