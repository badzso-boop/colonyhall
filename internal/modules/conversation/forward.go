// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ForwardInput forwards a message to another conversation (round 55, part F).
type ForwardInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	// Comment is the forwarder's own text; it may mention people and agents.
	Comment     string
	ClientMsgID string
}

// maxQuote bounds the quoted text, so the forward stays within MaxBodyLen.
const maxQuote = 4000

// DefuseMentions breaks @handles and chip references in text that is quoted
// rather than said (forwards): "@x" becomes "@\u200bx", which neither the
// server's mention parser nor the PWA's autolink matches, so a quote never
// wakes an agent or shows an approval card (the git module's webhook text
// pattern).
func DefuseMentions(s string) string {
	s = strings.ReplaceAll(s, "@", "@\u200b")
	for _, chip := range []string{"[approval:", "[error:", "[message:"} {
		s = strings.ReplaceAll(s, chip, "["+"\u200b"+chip[1:])
	}
	return s
}

// ForwardMessage posts a quote of a message in another conversation the
// caller can write to: the forwarder's comment, then the quoted text with
// its author, place and time, and a link back. Access is checked on both
// sides: the source must be readable (else not found), the target writable.
// Only people forward.
func (s *Service) ForwardMessage(ctx context.Context, by identity.Actor, sourceID ids.ID, in ForwardInput) (SendResult, error) {
	if by.Kind != identity.KindHuman {
		return SendResult{}, ErrForbidden
	}
	src, err := s.Store.MessageByID(ctx, sourceID)
	if err != nil {
		return SendResult{}, err
	}
	srcCh, err := s.channelFor(ctx, by.WorkspaceID, src.ChannelID)
	if err != nil {
		return SendResult{}, err
	}
	if ok, err := s.CanRead(ctx, by, srcCh.ID); err != nil {
		return SendResult{}, err
	} else if !ok {
		return SendResult{}, db.ErrNotFound
	}
	if src.DeletedAt != nil {
		return SendResult{}, ErrForwardRetracted
	}
	author := "?"
	if a, err := s.Directory.ActorByID(ctx, src.AuthorID); err == nil {
		author = a.Handle
	}
	// The header names no word in any language (the body is stored once
	// and read by people of any locale and by agents; the PWA labels it
	// "forwarded" from forwarded_from_id): the author, the place (a DM has
	// none to name) and the time.
	place := ""
	if srcCh.Kind != KindDM {
		place = " · #" + srcCh.Name
	}
	loc := time.UTC
	if l, err := time.LoadLocation(by.Timezone); err == nil && by.Timezone != "" {
		loc = l
	}
	quoted := src.Body
	if utf8.RuneCountInString(quoted) > maxQuote {
		quoted = string([]rune(quoted)[:maxQuote]) + "…"
	}
	var b strings.Builder
	if c := strings.TrimSpace(in.Comment); c != "" {
		b.WriteString(c)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "↪ @\u200b%s%s · %s\n", author, place, src.CreatedAt.In(loc).Format("2006-01-02 15:04"))
	for _, line := range strings.Split(DefuseMentions(quoted), "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	link := fmt.Sprintf("[message:%s/%s]", src.ChannelID, src.ID)
	if src.ThreadRootID != ids.Nil {
		link = fmt.Sprintf("[message:%s/%s/%s]", src.ChannelID, src.ID, src.ThreadRootID)
	}
	b.WriteString(link)
	return s.SendMessage(ctx, by, SendInput{ChannelID: in.ChannelID, ThreadRootID: in.ThreadRootID, ClientMsgID: in.ClientMsgID,
		Body: b.String(), ForwardedFromID: src.ID})
}
