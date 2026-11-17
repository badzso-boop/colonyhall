// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseBlocks(t *testing.T) {
	good := `[{"type":"text","text":"Mehet?"},{"type":"buttons","buttons":[{"action_id":"ok","label":"Rendben","style":"primary"},{"action_id":"ask","label":"Kérdezek"}]},
		{"type":"select","action_id":"who","options":[{"value":"a","label":"Anna"}]},{"type":"table","columns":["key","summary"],"rows":[["PROJ-1","Fix"]]}]`
	bs, err := ParseBlocks(json.RawMessage(good))
	if err != nil || len(bs) != 4 {
		t.Fatalf("%v %v", bs, err)
	}
	if l, ok := FindAction(bs, "ok", ""); !ok || l != "Rendben" {
		t.Fatal("button")
	}
	if l, ok := FindAction(bs, "who", "a"); !ok || l != "Anna" {
		t.Fatal("option")
	}
	if _, ok := FindAction(bs, "who", "zzz"); ok {
		t.Fatal("unknown option accepted")
	}
	if BlocksText(bs) != "Mehet?" {
		t.Fatal("text")
	}
	for _, bad := range []string{
		`[{"type":"script"}]`,
		`[{"type":"buttons","buttons":[{"action_id":"A B","label":"x"}]}]`,
		`[{"type":"buttons","buttons":[{"action_id":"x","label":"1"},{"action_id":"x","label":"2"}]}]`,
		`[{"type":"table","columns":["a"],"rows":[["1","2"]]}]`,
		`[{"type":"text","text":""}]`,
		`[{"type":"buttons","buttons":[{"action_id":"x","label":"@other wake up"}]}]`,
		`[{"type":"select","action_id":"s","options":[{"value":"@x","label":"x"}]}]`,
		`{"type":"text"}`,
		`[` + strings.Repeat(`{"type":"text","text":"x"},`, 10) + `{"type":"text","text":"x"}]`,
	} {
		if _, err := ParseBlocks(json.RawMessage(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if bs, err := ParseBlocks(nil); err != nil || bs != nil {
		t.Fatal("nil blocks")
	}
}
