package delegation

import "colonyhall/internal/platform/ids"

// MessageConversation determines the conversation scope for a message.
// - In a DM, a top-level message -> the whole DM (thread_root_id empty).
// - A message in a thread (any channel kind) -> that thread (channel_id, thread_root_id).
// - A top-level message in a channel -> the thread that message starts (its own id).
func MessageConversation(isDM bool, channelID, messageID, threadRootID ids.ID) ConversationKey {
	if threadRootID != ids.Nil {
		return ConversationKey{
			ChannelID:    channelID,
			ThreadRootID: threadRootID,
		}
	}
	if isDM {
		return ConversationKey{
			ChannelID:    channelID,
			ThreadRootID: ids.Nil,
		}
	}
	return ConversationKey{
		ChannelID:    channelID,
		ThreadRootID: messageID,
	}
}
