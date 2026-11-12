package search

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chunking targets, in estimated tokens (docs/11-rag-terv.md, 6.). The
// embedder's window is 512 tokens including the header.
const (
	wholeMessageTokens = 350
	targetTokens       = 300
	overlapTokens      = 50
)

// EstimateTokens is a cheap token estimate for multilingual text: about
// four characters per token, never less than the word count.
func EstimateTokens(s string) int {
	chars := utf8.RuneCountInString(s)
	words := len(strings.Fields(s))
	return max((chars+3)/4, words)
}

// SplitText cuts text into pieces of about targetTokens with overlapTokens
// of overlap, along paragraph boundaries; a fenced code block under
// wholeMessageTokens is never split. Text that fits stays one piece.
func SplitText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if EstimateTokens(text) <= wholeMessageTokens {
		return []string{text}
	}
	var pieces []string
	var cur []string
	curTokens, fresh := 0, false
	flush := func() {
		if !fresh {
			return
		}
		pieces = append(pieces, strings.Join(cur, "\n\n"))
		// Carry the tail of the piece as overlap.
		var tail []string
		tailTokens := 0
		for i := len(cur) - 1; i >= 0; i-- {
			t := EstimateTokens(cur[i])
			if tailTokens+t > overlapTokens {
				break
			}
			tail = append([]string{cur[i]}, tail...)
			tailTokens += t
		}
		cur, curTokens, fresh = tail, tailTokens, false
	}
	for _, p := range paragraphs(text) {
		t := EstimateTokens(p)
		if t > wholeMessageTokens || (t > targetTokens && !isFence(p)) {
			// One long paragraph: split it by sentences or, failing that, words.
			flush()
			cur, curTokens = nil, 0
			pieces = append(pieces, splitLong(p)...)
			continue
		}
		if curTokens+t > targetTokens && fresh {
			flush()
		}
		cur = append(cur, p)
		curTokens += t
		fresh = true
	}
	flush()
	return pieces
}

func isFence(p string) bool { return strings.HasPrefix(strings.TrimSpace(p), "```") }

// paragraphs splits on blank lines and before markdown headings, keeping
// fenced code blocks together.
func paragraphs(text string) []string {
	var out []string
	var cur []string
	inFence := false
	push := func() {
		if s := strings.TrimSpace(strings.Join(cur, "\n")); s != "" {
			out = append(out, s)
		}
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				push()
			}
			cur = append(cur, line)
			if inFence {
				push()
			}
			inFence = !inFence
			continue
		}
		if inFence {
			cur = append(cur, line)
			continue
		}
		if trimmed == "" {
			push()
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			push()
		}
		cur = append(cur, line)
	}
	push()
	return out
}

// splitLong cuts one long paragraph into targetTokens pieces at sentence
// ends (or word boundaries), with overlap.
func splitLong(p string) []string {
	words := strings.Fields(p)
	var out []string
	start := 0
	for start < len(words) {
		end, tokens := start, 0
		for end < len(words) {
			t := EstimateTokens(words[end] + " ")
			if tokens+t > targetTokens && end > start {
				break
			}
			tokens += t
			end++
		}
		// Prefer to end at a sentence in the last third.
		if end < len(words) {
			for i := end - 1; i > start+(end-start)*2/3; i-- {
				if w := words[i]; strings.HasSuffix(w, ".") || strings.HasSuffix(w, "!") || strings.HasSuffix(w, "?") {
					end = i + 1
					break
				}
			}
		}
		out = append(out, strings.Join(words[start:end], " "))
		if end >= len(words) {
			break
		}
		// Overlap: step back about overlapTokens worth of words.
		back, bt := end, 0
		for back > start+1 && bt < overlapTokens {
			back--
			bt += EstimateTokens(words[back] + " ")
		}
		start = back
	}
	return out
}

// FirstLine returns the first non-empty line of s, at most n characters
// (with an ellipsis when cut), for thread titles in headers.
func FirstLine(s string, n int) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#>*- "))
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) <= n {
			return line
		}
		r := []rune(line)
		return strings.TrimRightFunc(string(r[:n-1]), unicode.IsSpace) + "…"
	}
	return ""
}

// ContentHash identifies a chunk's text for idempotent re-indexing.
func ContentHash(header, body string) string {
	h := sha256.Sum256([]byte(header + "\x00" + body))
	return hex.EncodeToString(h[:])
}

// EmbedText is what the embedder sees for a chunk: the e5 passage prefix,
// the header and the body.
func EmbedText(header, body string) string {
	return "passage: " + header + "\n" + body
}

// EmbedQuery is what the embedder sees for a query.
func EmbedQuery(q string) string { return "query: " + q }
