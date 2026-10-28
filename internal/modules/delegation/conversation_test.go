package delegation_test

import (
	"testing"

	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/platform/ids"
)

func TestMessageConversation(t *testing.T) {
	gen := &ids.Sequential{}
	chanID := gen.New()
	msgID := gen.New()
	threadID := gen.New()

	tests := []struct {
		name         string
		isDM         bool
		channelID    ids.ID
		messageID    ids.ID
		threadRootID ids.ID
		want         delegation.ConversationKey
	}{
		{
			name:         "DM top level",
			isDM:         true,
			channelID:    chanID,
			messageID:    msgID,
			threadRootID: ids.Nil,
			want: delegation.ConversationKey{
				ChannelID:    chanID,
				ThreadRootID: ids.Nil,
			},
		},
		{
			name:         "DM thread",
			isDM:         true,
			channelID:    chanID,
			messageID:    msgID,
			threadRootID: threadID,
			want: delegation.ConversationKey{
				ChannelID:    chanID,
				ThreadRootID: ids.Nil,
			},
		},
		{
			name:         "channel thread",
			isDM:         false,
			channelID:    chanID,
			messageID:    msgID,
			threadRootID: threadID,
			want: delegation.ConversationKey{
				ChannelID:    chanID,
				ThreadRootID: threadID,
			},
		},
		{
			name:         "channel top level",
			isDM:         false,
			channelID:    chanID,
			messageID:    msgID,
			threadRootID: ids.Nil,
			want: delegation.ConversationKey{
				ChannelID:    chanID,
				ThreadRootID: msgID,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := delegation.MessageConversation(tc.isDM, tc.channelID, tc.messageID, tc.threadRootID)
			if got != tc.want {
				t.Fatalf("MessageConversation() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
