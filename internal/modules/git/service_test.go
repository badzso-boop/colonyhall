// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package git

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/git/provider"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

type mockStore struct {
	links        map[string]LinkCard
	repoChannels map[string]RepoChannel
	deliveries   map[string]time.Time
	giteas       map[ids.ID]WorkspaceGitea
}

func newMockStore() *mockStore {
	return &mockStore{
		links:        make(map[string]LinkCard),
		repoChannels: make(map[string]RepoChannel),
		deliveries:   make(map[string]time.Time),
		giteas:       make(map[ids.ID]WorkspaceGitea),
	}
}

func (m *mockStore) GetLink(_ context.Context, ws ids.ID, url string) (LinkCard, error) {
	card, ok := m.links[ws.String()+url]
	if !ok {
		return LinkCard{}, ErrLinkNotFound
	}
	return card, nil
}

func (m *mockStore) GetLinks(_ context.Context, ws ids.ID, urls []string) ([]LinkCard, error) {
	var out []LinkCard
	for _, u := range urls {
		if c, ok := m.links[ws.String()+u]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *mockStore) SaveLink(_ context.Context, card LinkCard) error {
	m.links[card.WorkspaceID.String()+card.URL] = card
	return nil
}

func (m *mockStore) ListRepoChannels(_ context.Context, workspaceID ids.ID) ([]RepoChannel, error) {
	var out []RepoChannel
	for _, rc := range m.repoChannels {
		if rc.WorkspaceID == workspaceID {
			out = append(out, rc)
		}
	}
	return out, nil
}

func (m *mockStore) GetRepoChannel(_ context.Context, workspaceID ids.ID, repo string) (RepoChannel, error) {
	rc, ok := m.repoChannels[workspaceID.String()+":"+repo]
	if !ok {
		return RepoChannel{}, ErrMappingNotFound
	}
	return rc, nil
}

func (m *mockStore) FindRepoChannels(_ context.Context, repo string) ([]RepoChannel, error) {
	var out []RepoChannel
	for _, rc := range m.repoChannels {
		if rc.Repo == repo {
			out = append(out, rc)
		}
	}
	return out, nil
}

func (m *mockStore) SetRepoChannel(_ context.Context, mapping RepoChannel) error {
	m.repoChannels[mapping.WorkspaceID.String()+":"+mapping.Repo] = mapping
	return nil
}

func (m *mockStore) DeleteRepoChannel(_ context.Context, workspaceID ids.ID, repo string) error {
	delete(m.repoChannels, workspaceID.String()+":"+repo)
	return nil
}

func (m *mockStore) GetWorkspaceGitea(_ context.Context, ws ids.ID) (WorkspaceGitea, error) {
	c, ok := m.giteas[ws]
	if !ok {
		return WorkspaceGitea{}, ErrNoOwnGitea
	}
	return c, nil
}

func (m *mockStore) SetWorkspaceGitea(_ context.Context, c WorkspaceGitea) error {
	if m.giteas == nil {
		m.giteas = map[ids.ID]WorkspaceGitea{}
	}
	m.giteas[c.WorkspaceID] = c
	return nil
}

func (m *mockStore) RecordDelivery(_ context.Context, deliveryID string, now time.Time) (bool, error) {
	if _, ok := m.deliveries[deliveryID]; ok {
		return false, nil
	}
	m.deliveries[deliveryID] = now
	return true, nil
}

type fakeProvider struct {
	fetchFunc   func(ctx context.Context, ref provider.ResourceRef) (provider.ResourceCard, error)
	webhookFunc func(r *http.Request, secret string) (provider.WebhookEvent, error)
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) ParseURL(rawURL string) (provider.ResourceRef, bool) {
	if rawURL == "https://gitea.local/owner/repo/pulls/1" {
		return provider.ResourceRef{
			Kind:  provider.ResourcePull,
			Owner: "owner",
			Repo:  "repo",
			ID:    "1",
			URL:   rawURL,
		}, true
	}
	return provider.ResourceRef{}, false
}

func (f *fakeProvider) ParseRef(refStr string) (provider.ResourceRef, bool) {
	if refStr == "owner/repo#1" || refStr == "https://gitea.local/owner/repo/pulls/1" {
		return provider.ResourceRef{
			Kind:  provider.ResourcePull,
			Owner: "owner",
			Repo:  "repo",
			ID:    "1",
			URL:   "https://gitea.local/owner/repo/pulls/1",
		}, true
	}
	return provider.ResourceRef{}, false
}

func (f *fakeProvider) FetchResource(ctx context.Context, ref provider.ResourceRef) (provider.ResourceCard, error) {
	if f.fetchFunc != nil {
		return f.fetchFunc(ctx, ref)
	}
	return provider.ResourceCard{
		URL:    ref.URL,
		Kind:   ref.Kind,
		Title:  "#1 Test PR",
		State:  "open",
		Author: "alice",
	}, nil
}

func (f *fakeProvider) ParseWebhook(r *http.Request, secret string) (provider.WebhookEvent, error) {
	if f.webhookFunc != nil {
		return f.webhookFunc(r, secret)
	}
	return provider.WebhookEvent{}, errors.New("not implemented")
}

type fakeIdentity struct {
	actors map[string]identity.Actor
}

func (f *fakeIdentity) EnsureSystemActor(_ context.Context, workspaceID ids.ID, handle, displayName string) (identity.Actor, error) {
	if a, ok := f.actors[handle]; ok {
		return a, nil
	}
	a := identity.Actor{
		ID:          ids.V7{}.New(),
		WorkspaceID: workspaceID,
		Kind:        identity.KindAgent,
		Handle:      handle,
		DisplayName: displayName,
		Role:        identity.RoleMember,
		State:       identity.StateActive,
	}
	f.actors[handle] = a
	return a, nil
}

type fakeConversation struct {
	messages []conversation.SendInput
	members  map[string]bool
	channels map[ids.ID]conversation.Channel
}

func (f *fakeConversation) EnsureMember(_ context.Context, channelID, actorID ids.ID) error {
	f.members[channelID.String()+":"+actorID.String()] = true
	return nil
}

func (f *fakeConversation) SendMessage(_ context.Context, _ identity.Actor, in conversation.SendInput) (conversation.SendResult, error) {
	f.messages = append(f.messages, in)
	return conversation.SendResult{}, nil
}

func (f *fakeConversation) ChannelByID(_ context.Context, id ids.ID) (conversation.Channel, error) {
	ch, ok := f.channels[id]
	if !ok {
		return conversation.Channel{}, errors.New("channel not found")
	}
	return ch, nil
}

func TestExtractURLs(t *testing.T) {
	text := "Check this https://example.com/test and https://example.com/test, or http://foo.bar/baz?q=1!"
	urls := ExtractURLs(text)
	if len(urls) != 2 {
		t.Fatalf("len(urls) = %d, want 2 (%v)", len(urls), urls)
	}
	if urls[0] != "https://example.com/test" || urls[1] != "http://foo.bar/baz?q=1!" {
		t.Fatalf("unexpected urls: %v", urls)
	}
}

func TestService_HandleEvent_Unfurl(t *testing.T) {
	st := newMockStore()
	clk := clock.NewManual(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	fetchCount := 0

	prov := &fakeProvider{
		fetchFunc: func(_ context.Context, ref provider.ResourceRef) (provider.ResourceCard, error) {
			fetchCount++
			return provider.ResourceCard{
				URL:    ref.URL,
				Kind:   ref.Kind,
				Title:  "#1 Test PR",
				State:  "open",
				Author: "alice",
			}, nil
		},
	}

	svc := &Service{
		Store:    st,
		Provider: prov,
		Clock:    clk,
	}

	ctx := context.Background()
	msg := conversation.MessageCreated{
		ChannelID: ids.V7{}.New().String(),
		Body:      "Look at https://gitea.local/owner/repo/pulls/1 please!",
	}
	payload, _ := json.Marshal(msg)
	ev := eventlog.Event{
		Type:    conversation.EventMessageCreated,
		Payload: payload,
	}

	if err := svc.HandleEvent(ctx, ev); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if fetchCount != 1 {
		t.Fatalf("fetchCount = %d, want 1", fetchCount)
	}

	card, err := st.GetLink(ctx, ids.Nil, "https://gitea.local/owner/repo/pulls/1")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if card.Title != "#1 Test PR" || card.Author != "alice" {
		t.Fatalf("unexpected card: %+v", card)
	}

	// Second event with same link (within 10 minutes) should use cache
	if err := svc.HandleEvent(ctx, ev); err != nil {
		t.Fatalf("HandleEvent 2: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("fetchCount = %d, want 1 (cache hit)", fetchCount)
	}
}

func TestService_LinkCardsForMessages(t *testing.T) {
	st := newMockStore()
	clk := clock.NewManual(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))

	_ = st.SaveLink(context.Background(), LinkCard{
		URL:       "https://gitea.local/owner/repo/pulls/1",
		Kind:      KindPull,
		Title:     "#1 Test PR",
		State:     "open",
		Author:    "alice",
		FetchedAt: clk.Now(),
	})

	svc := &Service{
		Store:    st,
		Provider: &fakeProvider{},
		Clock:    clk,
	}

	msgID := ids.V7{}.New()
	msgs := []conversation.Message{
		{ID: msgID, Body: "See https://gitea.local/owner/repo/pulls/1"},
		{ID: ids.V7{}.New(), Body: "No links here"},
	}

	cardsMap, err := svc.LinkCardsForMessages(context.Background(), msgs)
	if err != nil {
		t.Fatalf("LinkCardsForMessages: %v", err)
	}

	cards := cardsMap[msgID]
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Title != "#1 Test PR" {
		t.Errorf("unexpected card title: %s", cards[0].Title)
	}
}

func TestService_WebhookAndIdempotency(t *testing.T) {
	st := newMockStore()
	clk := clock.NewManual(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	wsID := ids.V7{}.New()
	chID := ids.V7{}.New()

	_ = st.SetRepoChannel(context.Background(), RepoChannel{
		WorkspaceID: wsID,
		Repo:        "owner/repo",
		ChannelID:   chID,
		CreatedAt:   clk.Now(),
	})

	fakeConv := &fakeConversation{
		members:  make(map[string]bool),
		channels: map[ids.ID]conversation.Channel{chID: {ID: chID, WorkspaceID: wsID}},
	}
	fakeId := &fakeIdentity{actors: make(map[string]identity.Actor)}

	prov := &fakeProvider{
		webhookFunc: func(_ *http.Request, _ string) (provider.WebhookEvent, error) {
			return provider.WebhookEvent{
				DeliveryID: "del-1",
				Repo:       "owner/repo",
				Action:     "opened",
				Summary:    "🔀 PR #1 opened by @alice",
			}, nil
		},
	}

	svc := &Service{
		Store:        st,
		Provider:     prov,
		Conversation: fakeConv,
		Identity:     fakeId,
		Clock:        clk,
	}

	ctx := context.Background()
	ev := provider.WebhookEvent{
		DeliveryID: "del-1",
		Repo:       "owner/repo",
		Action:     "opened",
		Summary:    "🔀 PR #1 opened by @alice",
	}

	// First delivery: processes message
	if err := svc.HandleWebhookEvent(ctx, ev); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}
	if len(fakeConv.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(fakeConv.messages))
	}
	if fakeConv.messages[0].Body != "🔀 PR #1 opened by @\u200balice" {
		t.Errorf("unexpected body: %s", fakeConv.messages[0].Body)
	}

	// Replay delivery: idempotent, no duplicate message
	if err := svc.HandleWebhookEvent(ctx, ev); err != nil {
		t.Fatalf("HandleWebhookEvent replay: %v", err)
	}
	if len(fakeConv.messages) != 1 {
		t.Fatalf("expected still 1 message after replay, got %d", len(fakeConv.messages))
	}
}
