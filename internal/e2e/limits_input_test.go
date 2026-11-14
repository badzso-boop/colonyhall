// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"math"
	"testing"

	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/server"
)

// Round 40: a huge deadline does not overflow into the past.
func TestQuestionDeadlineIsBounded(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	agent, err := e.app.Identity.Authenticate(ctx, server.DevAgentToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx = db.WithWorkspace(ctx, agent.WorkspaceID)
	huge := math.MaxInt
	q, err := e.app.Question.Ask(ctx, agent, question.AskInput{ChannelID: mustID(e.generalID()), To: "norbi", Text: "deploy?", DeadlineMinutes: &huge})
	if err != nil {
		t.Fatal(err)
	}
	now := e.clock.Now()
	if q.Deadline == nil || !q.Deadline.After(now) || q.Deadline.After(now.Add(question.MaxDeadline)) {
		t.Fatalf("deadline %v, now %v", q.Deadline, now)
	}
}
