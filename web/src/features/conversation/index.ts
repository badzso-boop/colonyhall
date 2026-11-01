import { lazy } from "react";

export { conversationKeys, useChannels, useMessages, useSendMessage, useThread, type Message } from "./api";
export { ConversationList, useConversationOrder, useMentionTotal } from "./components/ConversationList";
export { ProfileTrigger } from "./components/ProfilePopover";
export { ThreadPane } from "./components/ThreadPane";
export { registerConversationEvents } from "./events";
export { buildSidebar, classifyConversation, conversationTitle, type Channel, type ConversationKind } from "./model";
export { ThreadsEntry } from "./components/ThreadsEntry";
export const ConversationPage = lazy(() => import("./pages/ConversationPage"));
export const ThreadsPage = lazy(() => import("./pages/ThreadsPage"));
