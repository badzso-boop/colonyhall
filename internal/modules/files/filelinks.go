package files

import (
	"net/url"
	"strings"
	"unicode"
)

// localPrefixes are directory prefixes for local file paths that look like files.
var localPrefixes = []string{
	"/home/",
	"/mnt/",
	"/root/",
	"/srv/",
	"/tmp/",
	"/var/",
	"/opt/",
	"~/",
}

// FileLinks finds file:// URLs and absolute local paths that look like files
// in the given text, stopping at whitespace, quotes, backticks, ), ], >, and trailing .,;:!?
// file:// URL escapes are decoded. The original order is preserved and duplicates are dropped.
func FileLinks(text string) []string {
	if text == "" {
		return nil
	}

	var rawFound []string
	n := len(text)
	i := 0

	for i < n {
		// Case 1: file:// URL
		if strings.HasPrefix(text[i:], "file://") {
			if i == 0 || !isAlphaNum(rune(text[i-1])) {
				j := i + len("file://")
				for j < n {
					r := rune(text[j])
					if isStopChar(r) {
						break
					}
					j++
				}
				candidate := text[i:j]
				candidate = strings.TrimRight(candidate, ".,;:!?")
				candidate = trimLineSuffix(candidate)
				if path := decodeFileURL(candidate); path != "" {
					rawFound = append(rawFound, path)
				}
				i = j
				continue
			}
		}

		// Case 2: absolute local paths
		matchedPrefix := ""
		for _, prefix := range localPrefixes {
			if strings.HasPrefix(text[i:], prefix) {
				matchedPrefix = prefix
				break
			}
		}

		if matchedPrefix != "" {
			if i == 0 || isLocalPathBoundary(rune(text[i-1])) {
				j := i + len(matchedPrefix)
				for j < n {
					r := rune(text[j])
					if isStopChar(r) {
						break
					}
					j++
				}
				candidate := text[i:j]
				candidate = strings.TrimRight(candidate, ".,;:!?")
				candidate = trimLineSuffix(candidate)
				if len(candidate) > len(matchedPrefix) {
					rawFound = append(rawFound, candidate)
				}
				i = j
				continue
			}
		}

		i++
	}

	seen := make(map[string]bool, len(rawFound))
	var result []string
	for _, l := range rawFound {
		if !seen[l] {
			seen[l] = true
			result = append(result, l)
		}
	}
	return result
}

// NormalizePath normalizes a path or file:// URL, decoding escapes and stripping trailing punctuation.
func NormalizePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "file://") {
		return decodeFileURL(raw)
	}
	raw = strings.TrimRight(raw, ".,;:!?")
	return raw
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isStopChar(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '"', '\'', '`', ')', ']', '>':
		return true
	}
	return false
}

func isLocalPathBoundary(prev rune) bool {
	if isAlphaNum(prev) {
		return false
	}
	switch prev {
	case '/', '\\', '.', ':', '?':
		return false
	}
	return true
}

func trimLineSuffix(s string) string {
	colonIdx := strings.LastIndexByte(s, ':')
	if colonIdx <= 0 {
		return s
	}
	suffix := s[colonIdx+1:]
	if isLineNumberOrRange(suffix) {
		return s[:colonIdx]
	}
	return s
}

func isLineNumberOrRange(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, "-")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func decodeFileURL(raw string) string {
	rawPath := strings.TrimPrefix(raw, "file://")
	if strings.HasPrefix(rawPath, "localhost/") {
		rawPath = strings.TrimPrefix(rawPath, "localhost")
	}
	decoded, err := url.PathUnescape(rawPath)
	if err == nil {
		rawPath = decoded
	}
	rawPath = strings.TrimRight(rawPath, ".,;:!?")
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || rawPath == "/" {
		return ""
	}
	return rawPath
}
