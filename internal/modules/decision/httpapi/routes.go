package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterForbidden(decision.ErrForbidden)
	httpx.RegisterValidation(decision.ErrTextEmpty)
	httpx.RegisterValidation(decision.ErrTextTooLong)
	httpx.RegisterValidation(decision.ErrRationaleEmpty)
	httpx.RegisterValidation(decision.ErrRationaleTooLong)
	httpx.RegisterValidation(decision.ErrSupersededNotFound)
}

// Register mounts the decision HTTP endpoints on mux.
func Register(mux *http.ServeMux, svc *decision.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/decisions", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindHuman {
			httpx.Error(w, r, decision.ErrForbidden)
			return
		}

		limit := 20
		if q := strings.TrimSpace(r.URL.Query().Get("limit")); q != "" {
			if l, err := strconv.Atoi(q); err == nil && l > 0 {
				limit = l
			}
		}
		query := strings.TrimSpace(r.URL.Query().Get("query"))

		decisionsList, err := svc.ListDecisions(r.Context(), actor, limit, query, false)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		out := apispec.DecisionsResponse{
			Decisions: make([]apispec.Decision, len(decisionsList)),
		}
		for i, d := range decisionsList {
			var trID *uuid.UUID
			if d.ThreadRootID != nil {
				u := uuid.UUID(*d.ThreadRootID)
				trID = &u
			}
			out.Decisions[i] = apispec.Decision{
				Id:              uuid.UUID(d.ID),
				Number:          d.Number,
				FormattedNumber: d.FormattedNumber(),
				WorkspaceId:     uuid.UUID(d.WorkspaceID),
				AuthorId:        uuid.UUID(d.AuthorID),
				ChannelId:       uuid.UUID(d.ChannelID),
				ThreadRootId:    trID,
				Text:            d.Text,
				Rationale:       d.Rationale,
				Supersedes:      d.Supersedes,
				SupersededBy:    d.SupersededBy,
				CreatedAt:       d.CreatedAt,
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
