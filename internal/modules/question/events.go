// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package question

// Event types.
const (
	EventAsked     = "question.asked"
	EventAnswered  = "question.answered"
	EventEscalated = "question.escalated"
	EventWithdrawn = "question.withdrawn"
	EventExpired   = "question.expired"
)

// Event is the payload of question events in the audit log.
type Event struct {
	QuestionID      string   `json:"question_id"`
	Number          int      `json:"number"`
	FormattedNumber string   `json:"formatted_number"`
	AskedBy         string   `json:"asked_by"`
	ToActorID       string   `json:"to_actor_id,omitempty"`
	ToHuman         bool     `json:"to_human"`
	ChannelID       string   `json:"channel_id"`
	ThreadRootID    string   `json:"thread_root_id"`
	Text            string   `json:"text,omitempty"`
	Options         []string `json:"options,omitempty"`
	DefaultOption   string   `json:"default_option,omitempty"`
	Deadline        string   `json:"deadline,omitempty"`
	State           string   `json:"state"`
	Answer          string   `json:"answer,omitempty"`
	AnsweredBy      string   `json:"answered_by,omitempty"`
	AnswerMessageID string   `json:"answer_message_id,omitempty"`
}
