// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
)

// Interactive messages (round 56c, docs/25-onfejlesztes-terv.md 13.11):
// an agent or an extension's bot sends blocks next to the text body;
// people click a button or choose an option. The body stays the message's
// text for notifications, search and agents.

// Block limits.
const (
	MaxBlocks      = 10
	MaxBlocksBytes = 16 * 1024
	MaxButtons     = 5
	MaxOptions     = 25
	MaxTableRows   = 20
	MaxTableCols   = 6
)

// ErrBadBlocks: the blocks do not follow the format.
var ErrBadBlocks = errors.New("invalid blocks: text, buttons (max 5), select (max 25 options) or table (max 20 rows, 6 columns); at most 10 blocks, 16 KB")

// ErrUnknownAction: the message has no such button or option.
var ErrUnknownAction = errors.New("the message has no such action")

// Action is a recorded click or choice.
type Action struct {
	ID          ids.ID
	WorkspaceID ids.ID
	MessageID   ids.ID
	ActorID     ids.ID
	ActionID    string
	Value       string
	CreatedAt   time.Time
}

// Block is one block of an interactive message.
type Block struct {
	Type        string     `json:"type"` // text, buttons, select, table
	Text        string     `json:"text,omitempty"`
	Buttons     []Button   `json:"buttons,omitempty"`
	ActionID    string     `json:"action_id,omitempty"` // select
	Placeholder string     `json:"placeholder,omitempty"`
	Options     []Option   `json:"options,omitempty"`
	Columns     []string   `json:"columns,omitempty"`
	Rows        [][]string `json:"rows,omitempty"`
}

// Button is a button of a buttons block.
type Button struct {
	ActionID string `json:"action_id"`
	Label    string `json:"label"`
	Style    string `json:"style,omitempty"` // primary, danger or empty
}

// Option is an option of a select block.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

var actionIDRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

func short(s string, n int) bool { return s != "" && utf8.RuneCountInString(s) <= n }

// ParseBlocks validates and normalizes blocks; nil or empty input is no
// blocks.
func ParseBlocks(raw json.RawMessage) ([]Block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if len(raw) > MaxBlocksBytes {
		return nil, ErrBadBlocks
	}
	var bs []Block
	if err := json.Unmarshal(raw, &bs); err != nil || len(bs) > MaxBlocks {
		return nil, ErrBadBlocks
	}
	ids := map[string]bool{}
	for i := range bs {
		b := &bs[i]
		switch b.Type {
		case "text":
			if !short(b.Text, 4000) {
				return nil, ErrBadBlocks
			}
			*b = Block{Type: "text", Text: b.Text}
		case "buttons":
			if len(b.Buttons) == 0 || len(b.Buttons) > MaxButtons {
				return nil, ErrBadBlocks
			}
			for _, x := range b.Buttons {
				if !actionIDRe.MatchString(x.ActionID) || !short(x.Label, 40) || ids[x.ActionID] ||
					(x.Style != "" && x.Style != "primary" && x.Style != "danger") {
					return nil, ErrBadBlocks
				}
				ids[x.ActionID] = true
			}
			*b = Block{Type: "buttons", Buttons: b.Buttons}
		case "select":
			if !actionIDRe.MatchString(b.ActionID) || ids[b.ActionID] || len(b.Options) == 0 || len(b.Options) > MaxOptions ||
				utf8.RuneCountInString(b.Placeholder) > 80 {
				return nil, ErrBadBlocks
			}
			for _, o := range b.Options {
				if !short(o.Value, 200) || !short(o.Label, 80) {
					return nil, ErrBadBlocks
				}
			}
			ids[b.ActionID] = true
			*b = Block{Type: "select", ActionID: b.ActionID, Placeholder: b.Placeholder, Options: b.Options}
		case "table":
			if len(b.Columns) == 0 || len(b.Columns) > MaxTableCols || len(b.Rows) > MaxTableRows {
				return nil, ErrBadBlocks
			}
			for _, r := range b.Rows {
				if len(r) != len(b.Columns) {
					return nil, ErrBadBlocks
				}
			}
			*b = Block{Type: "table", Columns: b.Columns, Rows: b.Rows}
		default:
			return nil, ErrBadBlocks
		}
	}
	if len(bs) == 0 {
		return nil, nil
	}
	return bs, nil
}

// FindAction returns the label of a message's button, or of a select's
// chosen option.
func FindAction(blocks []Block, actionID, value string) (string, bool) {
	for _, b := range blocks {
		for _, x := range b.Buttons {
			if x.ActionID == actionID {
				return x.Label, true
			}
		}
		if b.Type == "select" && b.ActionID == actionID {
			for _, o := range b.Options {
				if o.Value == value {
					return o.Label, true
				}
			}
		}
	}
	return "", false
}

// BlocksText is the text form of blocks (the body when none is given).
func BlocksText(blocks []Block) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return "[interactive message]"
}
