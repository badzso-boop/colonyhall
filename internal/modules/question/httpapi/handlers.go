// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes questions over REST.
package httpapi

import (
	"context"
	"net/http"
	"strings"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/db"
)

func init() {
	httpx.RegisterForbidden(question.ErrForbidden)
	httpx.RegisterValidation(question.ErrEmptyText, question.ErrEmptyAnswer, question.ErrInvalidOption, question.ErrBadDeadline)
	httpx.RegisterStatus(http.StatusConflict, question.ErrQuestionAlreadyOpen, question.ErrQuestionNotOpen)
	httpx.RegisterStatus(http.StatusNotFound, question.ErrNotFound, db.ErrNotFound)
}

// Register mounts the question routes on mux.
func Register(mux *http.ServeMux, svc *question.Service, dir identity.Directory, conv conversation.Reader, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/questions", wrap(func(w http.ResponseWriter, r *http.Request) {
		var stateFilter *string
		if st := strings.TrimSpace(r.URL.Query().Get("state")); st != "" {
			stateFilter = &st
		}
		list, err := svc.List(r.Context(), httpx.Actor(r), stateFilter)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		specQuestions := make([]apispec.Question, 0, len(list))
		for _, q := range list {
			specQuestions = append(specQuestions, toSpec(r.Context(), q, dir, conv))
		}
		httpx.JSON(w, http.StatusOK, apispec.QuestionList{Questions: specQuestions})
	}))

	mux.Handle("POST /api/v1/questions/{questionId}/answer", wrap(func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("questionId")
		var in apispec.AnswerQuestionRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		updated, err := svc.Answer(r.Context(), httpx.Actor(r), ref, in.Answer)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSpec(r.Context(), updated, dir, conv))
	}))
}

func toSpec(ctx context.Context, q question.Question, dir identity.Directory, conv conversation.Reader) apispec.Question {
	var askedByHandle string
	if a, err := dir.ActorByID(ctx, q.AskedBy); err == nil {
		askedByHandle = a.Handle
	}
	var toActorHandle *string
	if q.ToActorID != nil {
		if a, err := dir.ActorByID(ctx, *q.ToActorID); err == nil {
			h := a.Handle
			toActorHandle = &h
		}
	}
	var answeredByHandle *string
	if q.AnsweredBy != nil {
		if a, err := dir.ActorByID(ctx, *q.AnsweredBy); err == nil {
			h := a.Handle
			answeredByHandle = &h
		}
	}
	var channelName string
	if ch, err := conv.ChannelByID(ctx, q.ChannelID); err == nil {
		channelName = ch.Name
	}
	opts := q.Options
	if opts == nil {
		opts = []string{}
	}
	return apispec.Question{
		Id:               q.ID,
		Number:           q.Number,
		FormattedNumber:  q.FormattedNumber(),
		WorkspaceId:      q.WorkspaceID,
		AskedBy:          q.AskedBy,
		AskedByHandle:    askedByHandle,
		ToActorId:        q.ToActorID,
		ToActorHandle:    toActorHandle,
		ToHuman:          q.ToHuman,
		ChannelId:        q.ChannelID,
		ChannelName:      channelName,
		ThreadRootId:     q.ThreadRootID,
		Text:             q.Text,
		Options:          opts,
		DefaultOption:    q.DefaultOption,
		Deadline:         q.Deadline,
		State:            apispec.QuestionState(q.State),
		Answer:           q.Answer,
		AnsweredBy:       q.AnsweredBy,
		AnsweredByHandle: answeredByHandle,
		CreatedAt:        q.CreatedAt,
		AnsweredAt:       q.AnsweredAt,
		EscalatedAt:      q.EscalatedAt,
		WithdrawnAt:      q.WithdrawnAt,
		ExpiredAt:        q.ExpiredAt,
	}
}
