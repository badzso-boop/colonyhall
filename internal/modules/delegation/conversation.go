// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package delegation

import "colonyhall/internal/platform/ids"

// MessageConversation determines the conversation scope for a message.
// - In a DM, the conversation is always the whole DM, any thread (thread_root_id empty).
// - A message in a channel thread -> that thread (channel_id, thread_root_id).
// - A top-level message in a channel -> the thread that message starts (its own id).
func MessageConversation(isDM bool, channelID, messageID, threadRootID ids.ID) ConversationKey {
	if isDM {
		return ConversationKey{
			ChannelID:    channelID,
			ThreadRootID: ids.Nil,
		}
	}
	if threadRootID != ids.Nil {
		return ConversationKey{
			ChannelID:    channelID,
			ThreadRootID: threadRootID,
		}
	}
	return ConversationKey{
		ChannelID:    channelID,
		ThreadRootID: messageID,
	}
}
