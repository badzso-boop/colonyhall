package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnsupportedURL = errors.New("url does not belong to this gitea instance")
	ErrFetchFailed    = errors.New("failed to fetch resource from gitea")
)

// GiteaProvider implements GitProvider for Gitea installations.
//
// baseURL is where the server calls the API (e.g. the Tailscale address);
// webURL is how people link to it (e.g. https://gitea.example.com). Links
// with either host are recognised, and cards always point to webURL, so
// the API is never called over the public address.
type GiteaProvider struct {
	baseURL *url.URL
	webURL  *url.URL
	token   string
	client  *http.Client
}

// NewGitea creates a new Gitea provider. publicURL is optional: the address
// people use in links when it differs from the API address.
func NewGitea(rawBaseURL, token string, publicURL ...string) (*GiteaProvider, error) {
	u, err := parseBase(rawBaseURL)
	if err != nil {
		return nil, err
	}
	g := &GiteaProvider{
		baseURL: u,
		webURL:  u,
		token:   token,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
	if len(publicURL) > 0 && strings.TrimSpace(publicURL[0]) != "" {
		w, err := parseBase(publicURL[0])
		if err != nil {
			return nil, fmt.Errorf("public URL: %w", err)
		}
		g.webURL = w
	}
	return g, nil
}

func parseBase(raw string) (*url.URL, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil, errors.New("gitea base URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid gitea base URL: %w", err)
	}
	return u, nil
}

// pathFor returns the repository path of u (base prefix stripped) when u
// belongs to base.
func pathFor(u, base *url.URL) (string, bool) {
	if !strings.EqualFold(u.Host, base.Host) {
		return "", false
	}
	if u.Scheme != "" && base.Scheme != "" && !strings.EqualFold(u.Scheme, base.Scheme) {
		return "", false
	}
	path := strings.Trim(u.Path, "/")
	if prefix := strings.Trim(base.Path, "/"); prefix != "" {
		if path != prefix && !strings.HasPrefix(path, prefix+"/") {
			return "", false
		}
		path = strings.Trim(strings.TrimPrefix(path, prefix), "/")
	}
	return path, true
}

func (g *GiteaProvider) Name() string {
	return "gitea"
}

func (g *GiteaProvider) BaseURL() string {
	return g.baseURL.String()
}

// ParseURL checks if rawURL belongs to this Gitea instance and extracts resource information.
func (g *GiteaProvider) ParseURL(rawURL string) (ResourceRef, bool) {
	ref, ok := g.parseURL(rawURL)
	if !ok || !validRef(ref) {
		return ResourceRef{}, false
	}
	return ref, true
}

func (g *GiteaProvider) parseURL(rawURL string) (ResourceRef, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ResourceRef{}, false
	}
	path, ok := pathFor(u, g.webURL)
	if !ok {
		if path, ok = pathFor(u, g.baseURL); !ok {
			return ResourceRef{}, false
		}
	}

	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return ResourceRef{}, false
	}

	owner := segments[0]
	repo := segments[1]

	// Disallow reserved words as owner
	switch strings.ToLower(owner) {
	case "assets", "api", "explore", "user", "swagger", "v2", "admin", "debug":
		return ResourceRef{}, false
	}

	cleanURL := g.webURL.String() + "/" + owner + "/" + repo

	if len(segments) == 2 {
		return ResourceRef{
			Kind:  ResourceRepo,
			Owner: owner,
			Repo:  repo,
			URL:   cleanURL,
		}, true
	}

	sub := strings.ToLower(segments[2])
	switch sub {
	case "pulls":
		if len(segments) >= 4 && segments[3] != "" {
			return ResourceRef{
				Kind:  ResourcePull,
				Owner: owner,
				Repo:  repo,
				ID:    segments[3],
				URL:   cleanURL + "/pulls/" + segments[3],
			}, true
		}
	case "issues":
		if len(segments) >= 4 && segments[3] != "" {
			return ResourceRef{
				Kind:  ResourceIssue,
				Owner: owner,
				Repo:  repo,
				ID:    segments[3],
				URL:   cleanURL + "/issues/" + segments[3],
			}, true
		}
	case "commit", "commits":
		if len(segments) >= 4 && segments[3] != "" {
			return ResourceRef{
				Kind:  ResourceCommit,
				Owner: owner,
				Repo:  repo,
				ID:    segments[3],
				URL:   cleanURL + "/commit/" + segments[3],
			}, true
		}
	}

	return ResourceRef{}, false
}

// ParseRef parses full URLs or shorthand formats:
// - "owner/repo#12" -> Issue/PR
// - "owner/repo@sha" -> Commit
// - "owner/repo" -> Repo
func (g *GiteaProvider) ParseRef(refStr string) (ResourceRef, bool) {
	ref, ok := g.parseRef(refStr)
	if !ok || !validRef(ref) {
		return ResourceRef{}, false
	}
	return ref, true
}

func (g *GiteaProvider) parseRef(refStr string) (ResourceRef, bool) {
	refStr = strings.TrimSpace(refStr)
	if strings.HasPrefix(refStr, "http://") || strings.HasPrefix(refStr, "https://") {
		return g.parseURL(refStr)
	}

	parts := strings.Split(refStr, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ResourceRef{}, false
	}

	owner := parts[0]
	rest := parts[1]

	if idx := strings.Index(rest, "#"); idx != -1 {
		repo := rest[:idx]
		num := rest[idx+1:]
		if repo == "" || num == "" {
			return ResourceRef{}, false
		}
		if _, err := strconv.Atoi(num); err != nil {
			return ResourceRef{}, false
		}
		urlStr := g.webURL.String() + "/" + owner + "/" + repo + "/issues/" + num
		return ResourceRef{
			Kind:  ResourceIssue,
			Owner: owner,
			Repo:  repo,
			ID:    num,
			URL:   urlStr,
		}, true
	}

	if idx := strings.Index(rest, "@"); idx != -1 {
		repo := rest[:idx]
		sha := rest[idx+1:]
		if repo == "" || sha == "" {
			return ResourceRef{}, false
		}
		urlStr := g.webURL.String() + "/" + owner + "/" + repo + "/commit/" + sha
		return ResourceRef{
			Kind:  ResourceCommit,
			Owner: owner,
			Repo:  repo,
			ID:    sha,
			URL:   urlStr,
		}, true
	}

	urlStr := g.webURL.String() + "/" + owner + "/" + rest
	return ResourceRef{
		Kind:  ResourceRepo,
		Owner: owner,
		Repo:  rest,
		URL:   urlStr,
	}, true
}

var (
	namePart  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	numberRef = regexp.MustCompile(`^[0-9]{1,10}$`)
	commitRef = regexp.MustCompile(`^[0-9a-fA-F]{3,64}$`)
)

// validRef reports whether ref only holds plain owner, repo and id parts:
// they are put into the API path that is called with the server's token,
// so "..", "?" or "%" must never get there.
func validRef(ref ResourceRef) bool {
	if !namePart.MatchString(ref.Owner) || !namePart.MatchString(ref.Repo) ||
		strings.Contains(ref.Owner, "..") || strings.Contains(ref.Repo, "..") {
		return false
	}
	switch ref.Kind {
	case ResourcePull, ResourceIssue:
		return numberRef.MatchString(ref.ID)
	case ResourceCommit:
		return commitRef.MatchString(ref.ID)
	case ResourceRepo:
		return ref.ID == ""
	}
	return false
}

func (g *GiteaProvider) FetchResource(ctx context.Context, ref ResourceRef) (ResourceCard, error) {
	if !validRef(ref) {
		return ResourceCard{}, ErrUnsupportedURL
	}
	var endpoint string
	switch ref.Kind {
	case ResourcePull:
		endpoint = fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%s", g.baseURL.String(), ref.Owner, ref.Repo, ref.ID)
	case ResourceIssue:
		endpoint = fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%s", g.baseURL.String(), ref.Owner, ref.Repo, ref.ID)
	case ResourceCommit:
		endpoint = fmt.Sprintf("%s/api/v1/repos/%s/%s/commits/%s", g.baseURL.String(), ref.Owner, ref.Repo, ref.ID)
	case ResourceRepo:
		endpoint = fmt.Sprintf("%s/api/v1/repos/%s/%s", g.baseURL.String(), ref.Owner, ref.Repo)
	default:
		return ResourceCard{}, fmt.Errorf("%w: unsupported kind %s", ErrFetchFailed, ref.Kind)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ResourceCard{}, err
	}
	req.Header.Set("Accept", "application/json")
	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return ResourceCard{}, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}

	if resp.StatusCode == http.StatusNotFound && ref.Kind == ResourceIssue {
		_ = resp.Body.Close()
		pullEndpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%s", g.baseURL.String(), ref.Owner, ref.Repo, ref.ID)
		pullReq, pErr := http.NewRequestWithContext(ctx, http.MethodGet, pullEndpoint, nil)
		if pErr == nil {
			pullReq.Header.Set("Accept", "application/json")
			if g.token != "" {
				pullReq.Header.Set("Authorization", "token "+g.token)
			}
			if pullResp, pullErr := g.client.Do(pullReq); pullErr == nil {
				if pullResp.StatusCode >= 200 && pullResp.StatusCode < 300 {
					ref.Kind = ResourcePull
					ref.URL = fmt.Sprintf("%s/%s/%s/pulls/%s", g.webURL.String(), ref.Owner, ref.Repo, ref.ID)
					resp = pullResp
					endpoint = pullEndpoint
				} else {
					_ = pullResp.Body.Close()
				}
			}
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ResourceCard{}, fmt.Errorf("%w: status %d from %s", ErrFetchFailed, resp.StatusCode, endpoint)
	}

	// Limit reader to 1MB
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ResourceCard{}, err
	}

	var data map[string]any
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return ResourceCard{}, fmt.Errorf("%w: json decode: %w", ErrFetchFailed, err)
	}

	card := ResourceCard{
		URL:  ref.URL,
		Kind: ref.Kind,
	}

	switch ref.Kind {
	case ResourcePull:
		title, _ := data["title"].(string)
		state, _ := data["state"].(string)
		if merged, ok := data["merged"].(bool); ok && merged {
			state = "merged"
		}
		card.Title = fmt.Sprintf("#%s %s", ref.ID, title)
		card.State = state
		if u, ok := data["user"].(map[string]any); ok {
			author, _ := u["login"].(string)
			if author == "" {
				author, _ = u["username"].(string)
			}
			card.Author = author
		}

	case ResourceIssue:
		title, _ := data["title"].(string)
		state, _ := data["state"].(string)
		if pr, ok := data["pull_request"]; ok && pr != nil {
			card.Kind = ResourcePull
		}
		card.Title = fmt.Sprintf("#%s %s", ref.ID, title)
		card.State = state
		if u, ok := data["user"].(map[string]any); ok {
			author, _ := u["login"].(string)
			if author == "" {
				author, _ = u["username"].(string)
			}
			card.Author = author
		}

	case ResourceCommit:
		state := "commit"
		var title string
		var author string
		if commitObj, ok := data["commit"].(map[string]any); ok {
			msg, _ := commitObj["message"].(string)
			lines := strings.Split(msg, "\n")
			if len(lines) > 0 {
				title = strings.TrimSpace(lines[0])
			}
			if aObj, ok := commitObj["author"].(map[string]any); ok {
				author, _ = aObj["name"].(string)
			}
		}
		if aObj, ok := data["author"].(map[string]any); ok {
			if login, ok := aObj["login"].(string); ok && login != "" {
				author = login
			}
		}
		if title == "" {
			title = fmt.Sprintf("Commit %s", ref.ID)
		}
		card.Title = title
		card.State = state
		card.Author = author

	case ResourceRepo:
		fullName, _ := data["full_name"].(string)
		desc, _ := data["description"].(string)
		isPrivate, _ := data["private"].(bool)
		state := "public"
		if isPrivate {
			state = "private"
		}
		if desc != "" {
			card.Title = fmt.Sprintf("%s: %s", fullName, desc)
		} else {
			card.Title = fullName
		}
		card.State = state
		if ownerObj, ok := data["owner"].(map[string]any); ok {
			card.Author, _ = ownerObj["login"].(string)
		}
	}

	return card, nil
}

func (g *GiteaProvider) ParseWebhook(r *http.Request, secret string) (WebhookEvent, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return WebhookEvent{}, err
	}

	// Without a secret anyone could post events into the mapped channels:
	// an unconfigured secret refuses every webhook instead of trusting all.
	if secret == "" {
		return WebhookEvent{}, ErrUnauthorizedWebhook
	}
	{
		sig := r.Header.Get("X-Gitea-Signature")
		if sig == "" {
			sig = r.Header.Get("X-Hub-Signature-256")
			sig = strings.TrimPrefix(sig, "sha256=")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
			return WebhookEvent{}, ErrUnauthorizedWebhook
		}
	}

	// The replay key is the hash of the signed body, not the delivery
	// header (round 40): the header is not signed, so a captured webhook
	// replayed with new X-Gitea-Delivery values passed as new events and
	// posted (and woke agents) again. Gitea's own redelivery of an event
	// sends the same body and is dropped the same way.
	h := sha256.Sum256(body)
	deliveryID := "sha256:" + hex.EncodeToString(h[:])

	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		eventType = r.Header.Get("X-GitHub-Event")
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookEvent{}, fmt.Errorf("invalid json payload: %w", err)
	}

	var repo string
	if repoObj, ok := payload["repository"].(map[string]any); ok {
		repo, _ = repoObj["full_name"].(string)
	}

	if repo == "" {
		return WebhookEvent{}, errors.New("repository.full_name missing in webhook")
	}

	var action string
	var summary string

	switch eventType {
	case "pull_request":
		act, _ := payload["action"].(string)
		prObj, _ := payload["pull_request"].(map[string]any)
		title, _ := prObj["title"].(string)
		num := prObj["number"]
		merged, _ := prObj["merged"].(bool)
		if merged || act == "closed" && merged {
			act = "merged"
		}
		action = act

		var user string
		if u, ok := prObj["user"].(map[string]any); ok {
			user, _ = u["login"].(string)
			if user == "" {
				user, _ = u["username"].(string)
			}
		}
		if user == "" {
			if s, ok := payload["sender"].(map[string]any); ok {
				user, _ = s["login"].(string)
				if user == "" {
					user, _ = s["username"].(string)
				}
			}
		}

		emoji := "🔀"
		switch action {
		case "merged":
			emoji = "🟣"
		case "closed":
			emoji = "🔴"
		}

		summary = fmt.Sprintf("%s PR #%v %s: %s by @%s", emoji, num, action, title, user)

	case "push":
		action = "push"
		ref, _ := payload["ref"].(string)
		branch := strings.TrimPrefix(ref, "refs/heads/")
		var commitCount int
		if commits, ok := payload["commits"].([]any); ok {
			commitCount = len(commits)
		}
		var pusher string
		if p, ok := payload["pusher"].(map[string]any); ok {
			pusher, _ = p["name"].(string)
			if pusher == "" {
				pusher, _ = p["login"].(string)
			}
			if pusher == "" {
				pusher, _ = p["username"].(string)
			}
		}
		if pusher == "" {
			if s, ok := payload["sender"].(map[string]any); ok {
				pusher, _ = s["login"].(string)
				if pusher == "" {
					pusher, _ = s["username"].(string)
				}
			}
		}
		summary = fmt.Sprintf("📦 Push to %s by @%s (%d commit(s))", branch, pusher, commitCount)

	case "status":
		action = "status"
		ctxName, _ := payload["context"].(string)
		state, _ := payload["state"].(string)
		sha, _ := payload["sha"].(string)
		if len(sha) > 7 {
			sha = sha[:7]
		}
		emoji := "⏳"
		switch state {
		case "success":
			emoji = "✅"
		case "failure", "error":
			emoji = "❌"
		}
		summary = fmt.Sprintf("%s CI %s: %s for %s", emoji, ctxName, state, sha)

	default:
		action = eventType
		summary = fmt.Sprintf("Git event: %s on %s", eventType, repo)
	}

	return WebhookEvent{
		DeliveryID: deliveryID,
		Repo:       repo,
		Action:     action,
		Summary:    summary,
	}, nil
}
