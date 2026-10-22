package guard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
	"golang.org/x/net/idna"
)

const (
	MaxURLLength     = 2048
	MaxBodyBytes     = 5 * 1024 * 1024 // 5 MB
	MaxFetchesPerRun = 30
	MaxBytesPerRun   = 10 * 1024 * 1024 // 10 MB
	MaxRedirects     = 5
	ConnectTimeout   = 10 * time.Second
	RequestTimeout   = 30 * time.Second
	UserAgent        = "ColonyhallGuard-web_read/1 (+read-only)"
)

// Page is the result of fetching a web page.
type Page struct {
	FinalURL    string
	Status      int
	ContentType string
	Title       string
	Text        string
	Truncated   bool
}

// FetchOptions configures a fetch operation.
type FetchOptions struct {
	Allow       []string
	Deny        []string
	Resolver    IPResolver
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	Tracker     *RunWebReadTracker
}

// RunWebReadTracker tracks per-run fetch counts and downloaded bytes.
type RunWebReadTracker struct {
	mu          sync.Mutex
	fetches     int
	bytes       int64
	visitedURLs []string
}

// Check returns an error if the per-run limit has been reached.
func (t *RunWebReadTracker) Check() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fetches >= MaxFetchesPerRun || t.bytes >= MaxBytesPerRun {
		return errors.New("web_read limit reached for this run")
	}
	return nil
}

// Record adds a completed fetch and byte count to the tracker.
func (t *RunWebReadTracker) Record(bytesCount int64) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fetches++
	t.bytes += bytesCount
	if t.fetches > MaxFetchesPerRun || t.bytes > MaxBytesPerRun {
		return errors.New("web_read limit reached for this run")
	}
	return nil
}

// Counts returns the current fetch count and bytes.
func (t *RunWebReadTracker) Counts() (int, int64) {
	if t == nil {
		return 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fetches, t.bytes
}

// AddVisitedURL records a visited URL.
func (t *RunWebReadTracker) AddVisitedURL(u string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.visitedURLs = append(t.visitedURLs, u)
}

// VisitedURLs returns all visited URLs in order of fetch.
func (t *RunWebReadTracker) VisitedURLs() []string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.visitedURLs...)
}

// matchHostPattern matches host against a pattern (exact or leading *.).
func matchHostPattern(pattern, host string) bool {
	pattern = strings.ToLower(pattern)
	host = strings.ToLower(host)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // e.g. ".go.dev"
		return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
	}
	return pattern == host
}

type pinnedIPContextKey struct{}

type safeTransport struct {
	base        http.RoundTripper
	opts        FetchOptions
	resolver    IPResolver
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (st *safeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 1. Validate URL
	if len(req.URL.String()) > MaxURLLength {
		return nil, fmt.Errorf("url length exceeds %d characters", MaxURLLength)
	}
	scheme := strings.ToLower(req.URL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q (only http and https are allowed)", scheme)
	}
	if req.URL.User != nil {
		return nil, errors.New("urls with userinfo are not allowed")
	}
	rawHost := req.URL.Hostname()
	if rawHost == "" {
		return nil, errors.New("host is required")
	}
	if ip := net.ParseIP(rawHost); ip != nil {
		return nil, fmt.Errorf("ip literals are not allowed: %s", rawHost)
	}

	asciiHost, err := idna.Lookup.ToASCII(rawHost)
	if err != nil {
		return nil, fmt.Errorf("invalid host %q: %w", rawHost, err)
	}

	// 2. Check deny and allow lists
	for _, d := range st.opts.Deny {
		if matchHostPattern(d, asciiHost) {
			return nil, fmt.Errorf("host %q is denied by configuration", asciiHost)
		}
	}
	if len(st.opts.Allow) > 0 {
		allowed := false
		for _, a := range st.opts.Allow {
			if matchHostPattern(a, asciiHost) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("host %q is not in the allowlist", asciiHost)
		}
	}

	// 3. DNS resolution & anti-SSRF IP check
	resolver := st.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx := req.Context()
	addrs, err := resolver.LookupIPAddr(ctx, asciiHost)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("lookup host %s: %w", asciiHost, err)
	}
	for _, addr := range addrs {
		if isBlockedIP(addr.IP) {
			return nil, fmt.Errorf("host %q resolved to blocked address %s", asciiHost, addr.IP)
		}
	}

	// Pin the exact resolved IP to prevent DNS rebinding
	pinnedIP := addrs[0].IP.String()
	ctxWithIP := context.WithValue(ctx, pinnedIPContextKey{}, pinnedIP)
	reqWithIP := req.WithContext(ctxWithIP)

	return st.base.RoundTrip(reqWithIP)
}

// Fetch fetches a web page safely according to Colonyhall guard rules.
func Fetch(ctx context.Context, rawURL string, opts FetchOptions) (Page, error) {
	if len(rawURL) > MaxURLLength {
		return Page{}, fmt.Errorf("url length exceeds %d characters", MaxURLLength)
	}

	if err := opts.Tracker.Check(); err != nil {
		return Page{}, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return Page{}, fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return Page{}, fmt.Errorf("unsupported scheme %q (only http and https are allowed)", scheme)
	}
	if parsedURL.User != nil {
		return Page{}, errors.New("urls with userinfo are not allowed")
	}
	if parsedURL.Hostname() == "" {
		return Page{}, errors.New("host is required")
	}
	if ip := net.ParseIP(parsedURL.Hostname()); ip != nil {
		return Page{}, fmt.Errorf("ip literals are not allowed: %s", parsedURL.Hostname())
	}

	// Setup custom dialer that uses the pinned IP from context
	dialFn := func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			port = "80"
		}
		target := addr
		if pinned, ok := ctx.Value(pinnedIPContextKey{}).(string); ok && pinned != "" {
			target = net.JoinHostPort(pinned, port)
		}
		if opts.DialContext != nil {
			return opts.DialContext(ctx, network, target)
		}
		dialer := &net.Dialer{Timeout: ConnectTimeout}
		return dialer.DialContext(ctx, network, target)
	}

	baseTransport := &http.Transport{
		DialContext:           dialFn,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		Proxy:                 nil, // Explicitly no environment proxy
	}

	st := &safeTransport{
		base:        baseTransport,
		opts:        opts,
		resolver:    opts.Resolver,
		dialContext: opts.DialContext,
	}

	client := &http.Client{
		Transport: st,
		Timeout:   RequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return errors.New("stopped after 5 redirects")
			}
			return nil
		},
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, RequestTimeout)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html, text/plain, text/markdown, application/json, application/xml;q=0.9, */*;q=0.1")

	resp, err := client.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	finalURL := resp.Request.URL.String()

	// Read body with limit
	limitReader := io.LimitReader(resp.Body, int64(MaxBodyBytes+1))
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return Page{}, fmt.Errorf("reading response body: %w", err)
	}

	truncated := false
	if len(bodyBytes) > MaxBodyBytes {
		truncated = true
		bodyBytes = bodyBytes[:MaxBodyBytes]
	}

	if err := opts.Tracker.Record(int64(len(bodyBytes))); err != nil {
		return Page{}, err
	}

	contentTypeHeader := resp.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(contentTypeHeader)
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	charsetParam := strings.ToLower(params["charset"])

	page := Page{
		FinalURL:    finalURL,
		Status:      resp.StatusCode,
		ContentType: mediaType,
		Truncated:   truncated,
	}

	switch {
	case mediaType == "text/html":
		title, text, err := parseHTML(resp.Request.URL, bodyBytes, charsetParam)
		if err != nil {
			return Page{}, fmt.Errorf("parsing html: %w", err)
		}
		page.Title = title
		page.Text = text

	case isTextMediaType(mediaType):
		text := decodeText(bodyBytes, charsetParam)
		page.Text = text

	default:
		sizeStr := formatByteSize(int64(len(bodyBytes)))
		page.Text = fmt.Sprintf("not a text page: %s, %s", mediaType, sizeStr)
	}

	return page, nil
}

func isTextMediaType(mediaType string) bool {
	switch mediaType {
	case "text/plain", "text/markdown", "application/json", "application/xml", "text/xml", "text/csv":
		return true
	}
	return false
}

func formatByteSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func decodeText(bodyBytes []byte, charsetLabel string) string {
	if charsetLabel != "" && charsetLabel != "utf-8" && charsetLabel != "us-ascii" {
		r, err := charset.NewReaderLabel(charsetLabel, bytes.NewReader(bodyBytes))
		if err == nil {
			decoded, err := io.ReadAll(r)
			if err == nil {
				return strings.ToValidUTF8(string(decoded), "")
			}
		}
	}
	return strings.ToValidUTF8(string(bodyBytes), "")
}

func parseHTML(baseURL *url.URL, bodyBytes []byte, charsetLabel string) (string, string, error) {
	var r io.Reader = bytes.NewReader(bodyBytes)
	if charsetLabel != "" && charsetLabel != "utf-8" && charsetLabel != "us-ascii" {
		cr, err := charset.NewReaderLabel(charsetLabel, bytes.NewReader(bodyBytes))
		if err == nil {
			r = cr
		}
	}

	doc, err := html.Parse(r)
	if err != nil {
		return "", "", err
	}

	// 1. Extract <title>
	title := ""
	var findTitle func(*html.Node)
	findTitle = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Title {
			var sb strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					sb.WriteString(c.Data)
				}
			}
			title = strings.TrimSpace(sb.String())
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if title != "" {
				return
			}
			findTitle(c)
		}
	}
	findTitle(doc)

	// 2. Render content to text
	var buf strings.Builder
	renderHTMLNode(baseURL, doc, &buf)

	cleaned := cleanWhitespace(buf.String())
	return title, cleaned, nil
}

func renderHTMLNode(baseURL *url.URL, n *html.Node, buf *strings.Builder) {
	if n.Type == html.TextNode {
		text := n.Data
		buf.WriteString(text)
		return
	}

	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderHTMLNode(baseURL, c, buf)
		}
		return
	}

	// Drop unwanted elements
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Noscript, atom.Svg, atom.Iframe, atom.Nav, atom.Footer, atom.Form, atom.Head:
		return
	}

	switch n.DataAtom {
	case atom.H1:
		buf.WriteString("\n\n# ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")
	case atom.H2:
		buf.WriteString("\n\n## ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")
	case atom.H3:
		buf.WriteString("\n\n### ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")
	case atom.H4:
		buf.WriteString("\n\n#### ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")
	case atom.H5:
		buf.WriteString("\n\n##### ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")
	case atom.H6:
		buf.WriteString("\n\n###### ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")

	case atom.P:
		buf.WriteString("\n\n")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")

	case atom.Br:
		buf.WriteString("\n")

	case atom.Hr:
		buf.WriteString("\n\n---\n\n")

	case atom.Ul, atom.Ol:
		buf.WriteString("\n")
		renderList(baseURL, n, buf, n.DataAtom == atom.Ol)
		buf.WriteString("\n")

	case atom.Pre:
		buf.WriteString("\n\n```\n")
		buf.WriteString(extractNodeText(n))
		buf.WriteString("\n```\n\n")

	case atom.Code:
		if isInsidePre(n) {
			renderChildren(baseURL, n, buf)
		} else {
			codeText := extractNodeText(n)
			buf.WriteString("`" + codeText + "`")
		}

	case atom.Table:
		buf.WriteString("\n\n")
		renderTable(baseURL, n, buf)
		buf.WriteString("\n\n")

	case atom.A:
		href := getAttr(n, "href")
		var linkTextBuf strings.Builder
		renderChildren(baseURL, n, &linkTextBuf)
		linkText := strings.TrimSpace(linkTextBuf.String())

		if href != "" && baseURL != nil {
			if u, err := url.Parse(href); err == nil {
				href = baseURL.ResolveReference(u).String()
			}
		}

		if href == "" {
			buf.WriteString(linkText)
		} else if linkText == "" || linkText == href {
			buf.WriteString(href)
		} else {
			_, _ = fmt.Fprintf(buf, "%s (%s)", linkText, href)
		}

	case atom.Blockquote:
		buf.WriteString("\n\n> ")
		renderChildren(baseURL, n, buf)
		buf.WriteString("\n\n")

	default:
		renderChildren(baseURL, n, buf)
	}
}

func renderChildren(baseURL *url.URL, n *html.Node, buf *strings.Builder) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderHTMLNode(baseURL, c, buf)
	}
}

func renderList(baseURL *url.URL, n *html.Node, buf *strings.Builder, ordered bool) {
	idx := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == atom.Li {
			var liBuf strings.Builder
			renderChildren(baseURL, c, &liBuf)
			line := strings.TrimSpace(liBuf.String())
			if line != "" {
				if ordered {
					_, _ = fmt.Fprintf(buf, "%d. %s\n", idx, line)
					idx++
				} else {
					_, _ = fmt.Fprintf(buf, "- %s\n", line)
				}
			}
		}
	}
}

func renderTable(baseURL *url.URL, tableNode *html.Node, buf *strings.Builder) {
	var rows [][]string
	var walkTable func(*html.Node)
	walkTable = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Tr {
			var row []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.DataAtom == atom.Th || c.DataAtom == atom.Td) {
					var cellBuf strings.Builder
					renderChildren(baseURL, c, &cellBuf)
					cellText := strings.TrimSpace(cellBuf.String())
					cellText = strings.ReplaceAll(cellText, "\n", " ")
					cellText = strings.ReplaceAll(cellText, "|", "\\|")
					row = append(row, cellText)
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkTable(c)
		}
	}
	walkTable(tableNode)

	if len(rows) == 0 {
		return
	}

	// Compute max columns
	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}

	for i, r := range rows {
		buf.WriteString("|")
		for col := 0; col < maxCols; col++ {
			val := ""
			if col < len(r) {
				val = r[col]
			}
			buf.WriteString(" " + val + " |")
		}
		buf.WriteString("\n")

		if i == 0 {
			buf.WriteString("|")
			for col := 0; col < maxCols; col++ {
				buf.WriteString(" --- |")
			}
			buf.WriteString("\n")
		}
	}
}

func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func isInsidePre(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.DataAtom == atom.Pre {
			return true
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func cleanWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	blankCount := 0

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			blankCount++
			if blankCount <= 2 {
				result = append(result, "")
			}
		} else {
			blankCount = 0
			result = append(result, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}
