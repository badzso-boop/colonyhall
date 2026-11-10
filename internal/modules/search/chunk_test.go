package search

import (
	"strings"
	"testing"
	"time"

	"colonyhall/internal/platform/ids"
)

func TestSplitTextShortStaysWhole(t *testing.T) {
	got := SplitText("  Rövid üzenet, egy darab.  ")
	if len(got) != 1 || got[0] != "Rövid üzenet, egy darab." {
		t.Fatalf("%q", got)
	}
	if SplitText("   ") != nil {
		t.Fatal("empty text gives no chunks")
	}
}

func TestSplitTextLongByParagraphWithOverlap(t *testing.T) {
	var paras []string
	for i := range 24 {
		paras = append(paras, strings.Repeat("szó"+string(rune('a'+i))+" ", 30)) // ~38 tokens each
	}
	text := strings.Join(paras, "\n\n")
	got := SplitText(text)
	if len(got) < 3 {
		t.Fatalf("expected several chunks, got %d", len(got))
	}
	for i, c := range got {
		if n := EstimateTokens(c); n > wholeMessageTokens {
			t.Fatalf("chunk %d has %d tokens", i, n)
		}
	}
	// Consecutive chunks overlap by at least one paragraph.
	for i := 1; i < len(got); i++ {
		prevParas := strings.Split(got[i-1], "\n\n")
		if !strings.HasPrefix(got[i], prevParas[len(prevParas)-1]) {
			t.Fatalf("chunk %d does not start with the previous one's last paragraph", i)
		}
	}
	// Every paragraph is somewhere.
	all := strings.Join(got, "\n\n")
	for _, p := range paras {
		if !strings.Contains(all, strings.TrimSpace(p)) {
			t.Fatal("a paragraph was lost")
		}
	}
}

func TestSplitTextKeepsSmallCodeBlock(t *testing.T) {
	code := "```go\nfunc main() {\n\n\tprintln(\"x\")\n}\n```"
	text := strings.Repeat("Bevezető mondat a kódhoz. ", 80) + "\n\n" + code + "\n\n" + strings.Repeat("Utána még szöveg jön. ", 80)
	found := false
	for _, c := range SplitText(text) {
		if strings.Contains(c, "```go") {
			if !strings.Contains(c, "println") || !strings.Contains(c, "}\n```") {
				t.Fatalf("code block split: %q", c)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("code block lost")
	}
}

func TestSplitTextOneHugeParagraph(t *testing.T) {
	text := strings.Repeat("Ez egy nagyon hosszú mondat a semmiről. ", 300)
	got := SplitText(text)
	if len(got) < 4 {
		t.Fatalf("got %d chunks", len(got))
	}
	for _, c := range got {
		if EstimateTokens(c) > wholeMessageTokens {
			t.Fatalf("chunk too long: %d tokens", EstimateTokens(c))
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := FirstLine("\n\n## Telepítő: első kiadás\nmásodik sor", 80); got != "Telepítő: első kiadás" {
		t.Fatalf("%q", got)
	}
	if got := FirstLine(strings.Repeat("a", 100), 10); got != "aaaaaaaaa…" {
		t.Fatalf("%q", got)
	}
}

func TestEmbedPrefixes(t *testing.T) {
	if EmbedQuery("hol fut?") != "query: hol fut?" || !strings.HasPrefix(EmbedText("h", "b"), "passage: h\n") {
		t.Fatal("e5 prefixes")
	}
	if ContentHash("a", "b") == ContentHash("ab", "") {
		t.Fatal("hash must separate header and body")
	}
}

func TestFuseRRF(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	c := func(n byte) Chunk { return Chunk{ID: ids.ID{n}, SourceAt: at, Body: "x"} }
	kw := []Candidate{{Chunk: c(1), Rank: 1, Score: 3, Snippet: "«x»"}, {Chunk: c(2), Rank: 2, Score: 2}}
	sem := []Candidate{{Chunk: c(2), Rank: 1, Score: 0.9}, {Chunk: c(3), Rank: 2, Score: 0.8}}
	got := fuse(kw, sem)
	if len(got) != 3 || got[0].Chunk.ID != (ids.ID{2}) {
		t.Fatalf("the hit found by both legs comes first: %+v", got)
	}
	want := 1.0/float64(RRFK+2) + 1.0/float64(RRFK+1)
	if got[0].Fused != want || got[0].Keyword == nil || got[0].Semantic == nil {
		t.Fatalf("fused score: %v want %v", got[0].Fused, want)
	}
	if got[1].Chunk.ID != (ids.ID{1}) || got[1].Snippet != "«x»" || got[1].Semantic != nil {
		t.Fatalf("keyword-only hit: %+v", got[1])
	}
}
