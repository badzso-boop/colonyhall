package httpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Security headers (round 40 quick fixes). The PWA gets a strict Content
// Security Policy: its scripts come only from the server, except the inline
// theme script of index.html, allowed by its hash. The development page
// still runs inline scripts and styles, so it gets only the parts that do not
// break it; it is going to be retired.

var inlineScriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// appCSP builds the PWA's policy from its built index.html.
func appCSP(dist fs.FS) string {
	scripts := []string{"'self'"}
	if page, err := fs.ReadFile(dist, "index.html"); err == nil {
		for _, m := range inlineScriptRe.FindAllSubmatch(page, -1) {
			sum := sha256.Sum256(m[1])
			scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scripts, " "),
		// React sets style attributes (colours, sizes): inline styles stay allowed.
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"worker-src 'self'",
		"manifest-src 'self'",
		"media-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}

// devPageCSP keeps the development page working (inline scripts, styles and
// the Monaco editor) while still refusing frames, plugins and base changes.
const devPageCSP = "object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// apiCSP: JSON and downloads never need to run anything.
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

func securityHeaders(appPolicy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		switch p := r.URL.Path; {
		case p == "/app" || strings.HasPrefix(p, "/app/"):
			h.Set("Content-Security-Policy", appPolicy)
		case strings.HasPrefix(p, "/api/"), p == "/mcp", strings.HasPrefix(p, "/ws/"):
			h.Set("Content-Security-Policy", apiCSP)
		default:
			h.Set("Content-Security-Policy", devPageCSP)
		}
		next.ServeHTTP(w, r)
	})
}

// Timeouts bounds how long one request may take to read and to answer.
// Zero values take the defaults.
type Timeouts struct {
	Read  time.Duration // the whole request, body included (uploads)
	Write time.Duration // the whole answer (downloads, the CSV export)
}

const (
	defaultReadTimeout  = 2 * time.Minute
	defaultWriteTimeout = 10 * time.Minute
)

// longLived are connections that stay open by design: WebSockets and the MCP
// stream. Everything else gets the deadlines.
func longLived(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return true
	}
	return r.URL.Path == "/mcp"
}

// deadlines sets per-request read and write deadlines on ordinary requests.
// They are per request (not http.Server's ReadTimeout and WriteTimeout),
// because those would also cut the WebSockets and the MCP stream.
func deadlines(t Timeouts, next http.Handler) http.Handler {
	if t.Read <= 0 {
		t.Read = defaultReadTimeout
	}
	if t.Write <= 0 {
		t.Write = defaultWriteTimeout
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !longLived(r) {
			rc := http.NewResponseController(w)
			now := time.Now()
			_ = rc.SetReadDeadline(now.Add(t.Read))
			_ = rc.SetWriteDeadline(now.Add(t.Write))
		}
		next.ServeHTTP(w, r)
	})
}
