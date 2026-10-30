import { lazy } from "react";

export { conversationKeys, useChannels, useMessages, useSendMessage, useThread, type Message } from "./api";
export { ConversationList, useMentionTotal } from "./components/ConversationList";
export { registerConversationEvents } from "./events";
export { buildSidebar, classifyConversation, conversationTitle, type Channel, type ConversationKind } from "./model";
export const ConversationPage = lazy(() => import("./pages/ConversationPage"));
