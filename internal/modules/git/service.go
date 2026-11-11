package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/git/provider"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the eventlog consumer name for git link unfurling.
const ConsumerName = "git.unfurl"

var urlRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// ExtractURLs returns all HTTP/HTTPS URLs found in a text.
func ExtractURLs(text string) []string {
	matches := urlRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var out []string
	for _, m := range matches {
		cleaned := strings.TrimRight(m, ",.:;)]")
		if cleaned == "" {
			continue
		}
		if !seen[cleaned] {
			seen[cleaned] = true
			out = append(out, cleaned)
		}
	}
	return out
}

type identityService interface {
	EnsureSystemActor(ctx context.Context, workspaceID ids.ID, handle, displayName string) (identity.Actor, error)
}

type conversationService interface {
	EnsureMember(ctx context.Context, channelID, actorID ids.ID) error
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
}

// Service provides Git integration features: unfurling links, handling webhooks, and managing mappings.
type Service struct {
	Store         Store
	Provider      provider.GitProvider
	Conversation  conversationService
	Identity      identityService
	Clock         clock.Clock
	WebhookSecret string
}

// HandleEvent processes message.created events to asynchronously unfurl Git links.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	if s.Provider == nil {
		return nil
	}
	if e.Type != conversation.EventMessageCreated {
		return nil
	}
	var p conversation.MessageCreated
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("git: unmarshal message created: %w", err)
	}
	s.unfurlBody(ctx, e.WorkspaceID, p.Body)
	return nil
}

func (s *Service) unfurlBody(ctx context.Context, workspaceID ids.ID, body string) {
	urls := ExtractURLs(body)
	for _, u := range urls {
		ref, ok := s.Provider.ParseURL(u)
		if !ok {
			continue
		}
		card, err := s.Store.GetLink(ctx, workspaceID, ref.URL)
		if err == nil && s.Clock.Now().Sub(card.FetchedAt) < 10*time.Minute {
			continue
		}
		fetched, err := s.Provider.FetchResource(ctx, ref)
		if err != nil {
			continue
		}
		_ = s.Store.SaveLink(ctx, LinkCard{
			WorkspaceID: workspaceID,
			URL:         fetched.URL,
			Kind:        LinkKind(fetched.Kind),
			Title:       fetched.Title,
			State:       fetched.State,
			Author:      fetched.Author,
			FetchedAt:   s.Clock.Now(),
		})
	}
}

// LinkCardsForMessages satisfies conversation.LinkCardProvider.
func (s *Service) LinkCardsForMessages(ctx context.Context, messages []conversation.Message) (map[ids.ID][]conversation.LinkCard, error) {
	if s.Provider == nil || len(messages) == 0 {
		return nil, nil
	}
	var allURLs []string
	msgURLs := make(map[ids.ID][]string, len(messages))
	seen := make(map[string]bool)

	for _, m := range messages {
		urls := ExtractURLs(m.Body)
		var matched []string
		for _, u := range urls {
			ref, ok := s.Provider.ParseURL(u)
			if !ok {
				continue
			}
			matched = append(matched, ref.URL)
			if !seen[ref.URL] {
				seen[ref.URL] = true
				allURLs = append(allURLs, ref.URL)
			}
		}
		if len(matched) > 0 {
			msgURLs[m.ID] = matched
		}
	}

	if len(allURLs) == 0 {
		return nil, nil
	}

	// A page of messages is always from one workspace.
	cards, err := s.Store.GetLinks(ctx, messages[0].WorkspaceID, allURLs)
	if err != nil {
		return nil, err
	}

	cardMap := make(map[string]LinkCard, len(cards))
	for _, c := range cards {
		cardMap[c.URL] = c
	}

	result := make(map[ids.ID][]conversation.LinkCard, len(msgURLs))
	for msgID, urls := range msgURLs {
		var list []conversation.LinkCard
		for _, u := range urls {
			if c, ok := cardMap[u]; ok {
				list = append(list, conversation.LinkCard{
					URL:    c.URL,
					Kind:   string(c.Kind),
					Title:  c.Title,
					State:  c.State,
					Author: c.Author,
				})
			}
		}
		if len(list) > 0 {
			result[msgID] = list
		}
	}
	return result, nil
}

// HandleWebhookEvent processes a validated Gitea webhook event into channel messages.
func (s *Service) HandleWebhookEvent(ctx context.Context, ev provider.WebhookEvent) error {
	if s.Provider == nil {
		return ErrDisabled
	}

	isNew, err := s.Store.RecordDelivery(ctx, ev.DeliveryID, s.Clock.Now())
	if err != nil {
		return err
	}
	if !isNew {
		// Idempotent: already processed
		return nil
	}

	mappings, err := s.Store.FindRepoChannels(ctx, ev.Repo)
	if err != nil {
		return err
	}
	if len(mappings) == 0 {
		// Unknown repos are ignored
		return nil
	}

	for _, m := range mappings {
		bot, err := s.Identity.EnsureSystemActor(ctx, m.WorkspaceID, "gitea", "Gitea")
		if err != nil {
			return err
		}
		if err := s.Conversation.EnsureMember(ctx, m.ChannelID, bot.ID); err != nil {
			return err
		}
		_, err = s.Conversation.SendMessage(ctx, bot, conversation.SendInput{
			ChannelID: m.ChannelID,
			Body:      defuseMentions(ev.Summary),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// defuseMentions breaks every @mention in webhook text (PR titles, Gitea
// user names): a Gitea user named like an agent, or "@claude" in a title,
// must not wake an agent.
func defuseMentions(s string) string {
	return strings.ReplaceAll(s, "@", "@\u200b")
}

// ListMappings lists all repo-channel mappings in the caller's workspace.
func (s *Service) ListMappings(ctx context.Context, by identity.Actor) ([]RepoChannel, error) {
	if !by.IsAdmin() {
		return nil, identity.ErrForbidden
	}
	return s.Store.ListRepoChannels(ctx, by.WorkspaceID)
}

// SetMapping creates or updates a repo-channel mapping.
func (s *Service) SetMapping(ctx context.Context, by identity.Actor, repo string, channelID ids.ID) (RepoChannel, error) {
	if !by.IsAdmin() {
		return RepoChannel{}, identity.ErrForbidden
	}
	if err := ValidateRepo(repo); err != nil {
		return RepoChannel{}, err
	}
	ch, err := s.Conversation.ChannelByID(ctx, channelID)
	if err != nil || ch.WorkspaceID != by.WorkspaceID {
		return RepoChannel{}, errors.New("channel not found in workspace")
	}
	m := RepoChannel{
		WorkspaceID: by.WorkspaceID,
		Repo:        repo,
		ChannelID:   channelID,
		CreatedAt:   s.Clock.Now(),
	}
	if err := s.Store.SetRepoChannel(ctx, m); err != nil {
		return RepoChannel{}, err
	}
	return m, nil
}

// DeleteMapping removes a repo-channel mapping.
func (s *Service) DeleteMapping(ctx context.Context, by identity.Actor, repo string) error {
	if !by.IsAdmin() {
		return identity.ErrForbidden
	}
	return s.Store.DeleteRepoChannel(ctx, by.WorkspaceID, repo)
}

// Status returns a single-line summary of a git reference for agents.
func (s *Service) Status(ctx context.Context, workspaceID ids.ID, refStr string) (string, error) {
	if s.Provider == nil {
		return "Gitea integration is not configured on this server.", nil
	}
	ref, ok := s.Provider.ParseRef(refStr)
	if !ok {
		return fmt.Sprintf("Unsupported git reference %q; expected URL or owner/repo#12 or owner/repo@sha", refStr), nil
	}
	card, err := s.Store.GetLink(ctx, workspaceID, ref.URL)
	if err != nil || s.Clock.Now().Sub(card.FetchedAt) > 10*time.Minute {
		fetched, fetchErr := s.Provider.FetchResource(ctx, ref)
		if fetchErr == nil {
			card = LinkCard{
				WorkspaceID: workspaceID,
				URL:         fetched.URL,
				Kind:        LinkKind(fetched.Kind),
				Title:       fetched.Title,
				State:       fetched.State,
				Author:      fetched.Author,
				FetchedAt:   s.Clock.Now(),
			}
			_ = s.Store.SaveLink(ctx, card)
		} else if card.Title == "" {
			// No error details for agents: the reason (endpoint, status)
			// stays on the server.
			return fmt.Sprintf("Could not fetch %s from Gitea.", refStr), nil
		}
	}
	if card.Author != "" {
		return fmt.Sprintf("%s [%s] by @%s", card.Title, card.State, card.Author), nil
	}
	return fmt.Sprintf("%s [%s]", card.Title, card.State), nil
}
