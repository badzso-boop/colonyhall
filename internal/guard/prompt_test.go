package guard

import (
	"strings"
	"testing"
)

func TestWorkPromptGrantedFoldersAndLanguage(t *testing.T) {
	it := PromptItem{
		Item: Item{
			Author: "norbi",
			Body:   "Hol dolgozol most?",
		},
	}

	// 1. Two read-only grants
	grantsRO := []Grant{
		{Path: "/home/user/trader", Mode: "read"},
		{Path: "/data/trader-data", Mode: "read"},
	}
	pRO := WorkPrompt("trader-logs", it, "report your folders", grantsRO...)
	expectedRO := "You can read /home/user/trader (read-only) and /data/trader-data (read-only); nothing else is visible."
	if !strings.Contains(pRO, expectedRO) {
		t.Fatalf("expected WorkPrompt to contain %q, got:\n%s", expectedRO, pRO)
	}

	// 2. Language rule in WorkPrompt
	expectedLang := "Answer in the language of the message you are answering; do not mix languages."
	if !strings.Contains(pRO, expectedLang) {
		t.Fatalf("expected WorkPrompt to contain %q, got:\n%s", expectedLang, pRO)
	}

	// 3. Mixed read/write grants
	grantsMixed := []Grant{
		{Path: "/home/user/trader", Mode: "write", Write: true},
		{Path: "/data/trader-data", Mode: "read"},
	}
	pMixed := WorkPrompt("trader-logs", it, "check files", grantsMixed...)
	expectedMixed := "You can access /home/user/trader (read-write) and /data/trader-data (read-only); nothing else is visible."
	if !strings.Contains(pMixed, expectedMixed) {
		t.Fatalf("expected WorkPrompt to contain %q, got:\n%s", expectedMixed, pMixed)
	}

	// 4. Single write grant
	grantsW := []Grant{
		{Path: "/home/user/trader", Mode: "write", Write: true},
	}
	pW := WorkPrompt("trader-logs", it, "write files", grantsW...)
	expectedW := "You can write /home/user/trader (read-write); nothing else is visible."
	if !strings.Contains(pW, expectedW) {
		t.Fatalf("expected WorkPrompt to contain %q, got:\n%s", expectedW, pW)
	}
}

func TestDirectPromptLengthAndLanguage(t *testing.T) {
	items := []Item{
		{
			ID:      "item-1",
			Author:  "norbi",
			Channel: "general",
			Body:    "Kérlek írj egy részletes összefoglalót a rendszerről.",
		},
	}

	prompt := DirectPrompt(items)

	// 1. Length up to 8000 characters
	expectedLength := "Keep the answer short (≤600 characters) unless someone asks for more; when asked for a longer summary or report, you may write up to 8000 characters."
	if !strings.Contains(prompt, expectedLength) {
		t.Fatalf("expected DirectPrompt to contain %q, got:\n%s", expectedLength, prompt)
	}

	// 2. Same language rule
	expectedLang := "Answer in the language of the message you are answering; do not mix languages."
	if !strings.Contains(prompt, expectedLang) {
		t.Fatalf("expected DirectPrompt to contain %q, got:\n%s", expectedLang, prompt)
	}
}
