// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package question

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

// States of a question.
const (
	StateOpen      = "open"
	StateAnswered  = "answered"
	StateEscalated = "escalated"
	StateWithdrawn = "withdrawn"
	StateExpired   = "expired"
)

// Question is a question asked by an agent or human.
type Question struct {
	ID            ids.ID
	Number        int
	WorkspaceID   ids.ID
	AskedBy       ids.ID
	ToActorID     *ids.ID
	ToHuman       bool
	AllowExpire   bool
	ChannelID     ids.ID
	ThreadRootID  ids.ID
	Text          string
	Options       []string
	DefaultOption *string
	Deadline      *time.Time
	State         string
	Answer        *string
	AnsweredBy    *ids.ID
	CreatedAt     time.Time
	AnsweredAt    *time.Time
	EscalatedAt   *time.Time
	WithdrawnAt   *time.Time
	ExpiredAt     *time.Time
}

// FormattedNumber returns "Q-12".
func (q Question) FormattedNumber() string {
	return FormatNumber(q.Number)
}

// FormatNumber returns "Q-12".
func FormatNumber(n int) string {
	return fmt.Sprintf("Q-%d", n)
}

// ParseReference parses a question identifier which can be either "Q-12", "q-12",
// an integer "12", or a UUID string. It returns (id, number, isNumber, err).
func ParseReference(ref string) (id ids.ID, number int, isNumber bool, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ids.Nil, 0, false, errors.New("empty question identifier")
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "q-") {
		numStr := strings.TrimPrefix(lower, "q-")
		n, err := strconv.Atoi(numStr)
		if err != nil || n <= 0 {
			return ids.Nil, 0, false, fmt.Errorf("invalid question number: %s", ref)
		}
		return ids.Nil, n, true, nil
	}
	if n, err := strconv.Atoi(ref); err == nil && n > 0 {
		return ids.Nil, n, true, nil
	}
	parsedID, err := ids.Parse(ref)
	if err != nil {
		return ids.Nil, 0, false, fmt.Errorf("invalid question reference (expected Q-<n> or UUID): %s", ref)
	}
	return parsedID, 0, false, nil
}

// Errors.
var (
	ErrNotFound            = errors.New("question not found")
	ErrForbidden           = errors.New("not allowed")
	ErrQuestionNotOpen     = errors.New("question is not open")
	ErrQuestionAlreadyOpen = errors.New("thread already has an open question from you, withdraw it first")
	ErrEmptyText           = errors.New("question text cannot be empty")
	ErrEmptyAnswer         = errors.New("answer cannot be empty")
	ErrInvalidOption       = errors.New("default option must be one of the choices")
	ErrBadDeadline         = errors.New("deadline must be in the future")
)

// MaxDeadline is the longest a question may wait for its deadline.
const MaxDeadline = 30 * 24 * time.Hour
