// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi is the operator's REST API (round 51), served only on
// admin.<base domain> in multi-tenant mode.
package httpapi

import (
	"context"
	"crypto/subtle"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/clientip"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterStatus(http.StatusUnauthorized, operator.ErrBadCredentials, operator.ErrUnauthorized)
	httpx.RegisterValidation(operator.ErrInvalidHandle, operator.ErrInvalidPlan, operator.ErrInvalidBreakGlass)
	httpx.RegisterProblemType(http.StatusConflict, "plan_over_quota", operator.ErrPlanOverQuota)
	httpx.RegisterProblemType(http.StatusConflict, "plan_archived", operator.ErrPlanArchived)
	httpx.RegisterProblemType(http.StatusConflict, "plan_name_taken", operator.ErrPlanNameTaken)
}

// CookieName is the operator session cookie; HostCookieName over HTTPS
// (the __Host- prefix keeps it on admin.<domain> exactly).
const (
	CookieName     = "colony_operator"
	HostCookieName = "__Host-colony_operator"
)

// Options configures the operator API.
type Options struct {
	// SecureCookies: HTTPS (the __Host- cookie, https:// invite links).
	SecureCookies bool
	// Database reports the whole database (round 50's card).
	Database func(ctx context.Context) (apispec.OperatorDatabase, error)
}

type ctxKey struct{}

type authed struct {
	op   operator.Operator
	sess operator.Session
}

func from(r *http.Request) authed {
	a, _ := r.Context().Value(ctxKey{}).(authed)
	return a
}

// Register adds the operator routes to mux (the admin host's own mux).
func Register(mux *http.ServeMux, svc *operator.Service, opt Options) {
	cookieName := CookieName
	if opt.SecureCookies {
		cookieName = HostCookieName
	}
	setCookie := func(w http.ResponseWriter, value string, expires time.Time) {
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", Expires: expires,
			HttpOnly: true, Secure: opt.SecureCookies, SameSite: http.SameSiteStrictMode})
	}
	auth := func(h http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil {
				httpx.Error(w, r, operator.ErrUnauthorized)
				return
			}
			o, sess, err := svc.Authenticate(r.Context(), c.Value)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if !csrfMatches(sess.CSRFToken, r.Header.Get(httpx.CSRFHeader)) {
					httpx.Error(w, r, httpx.ErrCSRF)
					return
				}
			}
			h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, authed{o, sess})))
		})
	}
	remote := func(r *http.Request) string { return clientip.String(clientip.From(r.Context())) }
	sessionInfo := func(ctx context.Context, o operator.Operator, sess operator.Session) (apispec.OperatorSessionInfo, error) {
		left, err := svc.BackupCodesLeft(ctx, o)
		return apispec.OperatorSessionInfo{Operator: accountToAPI(o), CsrfToken: sess.CSRFToken, ExpiresAt: sess.ExpiresAt,
			DevAccess: svc.DevAccess, BackupCodesLeft: left}, err
	}

	mux.HandleFunc("POST /api/v1/operator/auth/login", func(w http.ResponseWriter, r *http.Request) {
		// JSON only: blocks cross-site HTML form posts (login CSRF).
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "Content-Type must be application/json"})
			return
		}
		var in apispec.OperatorLoginRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		o, sess, secret, err := svc.Login(r.Context(), operator.LoginInput{Handle: in.Handle, Password: in.Password, Code: in.Code,
			RemoteAddr: remote(r), UserAgent: r.UserAgent()})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		info, err := sessionInfo(r.Context(), o, sess)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		setCookie(w, secret, sess.ExpiresAt)
		httpx.JSON(w, http.StatusOK, info)
	})

	mux.Handle("POST /api/v1/operator/auth/logout", auth(func(w http.ResponseWriter, r *http.Request) {
		a := from(r)
		if err := svc.Logout(r.Context(), a.op, a.sess, remote(r)); err != nil {
			httpx.Error(w, r, err)
			return
		}
		setCookie(w, "", time.Unix(0, 0))
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/operator/auth/session", auth(func(w http.ResponseWriter, r *http.Request) {
		a := from(r)
		info, err := sessionInfo(r.Context(), a.op, a.sess)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, info)
	}))

	mux.Handle("POST /api/v1/operator/me/backup-codes", auth(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.OperatorReauthRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		codes, err := svc.RenewBackupCodes(r.Context(), from(r).op, in.Password, in.Code, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.OperatorBackupCodes{Codes: codes})
	}))

	mux.Handle("GET /api/v1/operator/workspaces", auth(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListCustomers(r.Context())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]apispec.Customer, 0, len(list))
		for _, c := range list {
			out = append(out, customerToAPI(c))
		}
		httpx.JSON(w, http.StatusOK, apispec.CustomerList{Customers: out})
	}))

	mux.Handle("POST /api/v1/operator/workspaces", auth(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateCustomerRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, link, err := svc.CreateCustomer(r.Context(), from(r).op, in.Name, strings.ToLower(strings.TrimSpace(in.Slug)), remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.CreatedCustomer{Customer: customerToAPI(c), Invite: inviteToAPI(r, opt, c.Slug, link)})
	}))

	mux.Handle("GET /api/v1/operator/workspaces/{workspaceId}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		c, err := svc.Customer(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, customerToAPI(c))
	}))

	mux.Handle("POST /api/v1/operator/workspaces/{workspaceId}/invite", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		c, err := svc.Customer(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		link, err := svc.Invite(r.Context(), from(r).op, id, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, inviteToAPI(r, opt, c.Slug, link))
	}))

	suspend := func(on bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, ok := workspaceID(w, r)
			if !ok {
				return
			}
			reason := ""
			if on {
				var in apispec.SuspendCustomerRequest
				if err := httpx.Decode(r, &in); err != nil {
					httpx.Error(w, r, err)
					return
				}
				reason = in.Reason
			}
			c, err := svc.SetSuspended(r.Context(), from(r).op, id, on, reason, remote(r))
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			httpx.JSON(w, http.StatusOK, customerToAPI(c))
		}
	}
	mux.Handle("POST /api/v1/operator/workspaces/{workspaceId}/suspend", auth(suspend(true)))
	mux.Handle("POST /api/v1/operator/workspaces/{workspaceId}/restore", auth(suspend(false)))

	// Round 51b: plans.
	mux.Handle("GET /api/v1/operator/plans", auth(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListPlans(r.Context())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]apispec.Plan, 0, len(list))
		for _, p := range list {
			out = append(out, planToAPI(p))
		}
		httpx.JSON(w, http.StatusOK, apispec.PlanList{Plans: out})
	}))

	mux.Handle("POST /api/v1/operator/plans", auth(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.PlanRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		p, err := svc.CreatePlan(r.Context(), from(r).op, operator.Plan{Name: in.Name, StorageBytes: in.StorageBytes,
			CPUCores: in.CpuCores, MemoryBytes: in.MemoryBytes, ExtCallsMonth: httpx.Deref(in.ExtCallsMonth, 0)}, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, planToAPI(p))
	}))

	mux.Handle("PATCH /api/v1/operator/plans/{planId}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("planId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "planId must be a UUID"})
			return
		}
		var in apispec.PlanPatchRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		p, err := svc.UpdatePlan(r.Context(), from(r).op, id, operator.PlanPatch{Name: in.Name, StorageBytes: in.StorageBytes,
			CPUCores: in.CpuCores, MemoryBytes: in.MemoryBytes, ExtCallsMonth: in.ExtCallsMonth, Archived: in.Archived,
			ConfirmOverQuota: in.ConfirmOverQuota != nil && *in.ConfirmOverQuota}, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, planToAPI(p))
	}))

	mux.Handle("PUT /api/v1/operator/workspaces/{workspaceId}/plan", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		var in apispec.AssignPlanRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var planID *ids.ID
		if in.PlanId != nil {
			p := *in.PlanId
			planID = &p
		}
		c, err := svc.AssignPlan(r.Context(), from(r).op, id, planID, in.ConfirmOverQuota != nil && *in.ConfirmOverQuota, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, customerToAPI(c))
	}))

	mux.Handle("GET /api/v1/operator/workspaces/{workspaceId}/plan-check", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		planID, err := ids.Parse(r.URL.Query().Get("plan_id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "plan_id must be a UUID"})
			return
		}
		chk, err := svc.CheckPlan(r.Context(), id, planID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.PlanCheck{UsedBytes: chk.UsedBytes, NewQuotaBytes: chk.NewQuotaBytes, Fits: chk.Fits,
			StateAfter: apispec.StorageState(chk.StateAfter)})
	}))

	// Round 51b: break-glass access.
	mux.Handle("GET /api/v1/operator/workspaces/{workspaceId}/break-glass", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListBreakGlass(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.BreakGlassList{BreakGlass: make([]apispec.BreakGlass, 0, len(list))}
		for _, b := range list {
			out.BreakGlass = append(out.BreakGlass, breakGlassToAPI(b))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/operator/workspaces/{workspaceId}/break-glass", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		var in apispec.BreakGlassRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.Customer(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		link, err := svc.OpenBreakGlass(r.Context(), from(r).op, id, httpx.Deref(in.Reason, ""), in.Minutes, remote(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.BreakGlassOpened{BreakGlass: breakGlassToAPI(link.BreakGlass),
			Url: customerURL(r, opt, c.Slug, "/app/break-glass#"+link.Token), HandoffExpiresAt: link.HandoffExpiresAt})
	}))

	mux.Handle("DELETE /api/v1/operator/workspaces/{workspaceId}/break-glass/{breakGlassId}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspaceID(w, r)
		if !ok {
			return
		}
		bg, err := ids.Parse(r.PathValue("breakGlassId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "breakGlassId must be a UUID"})
			return
		}
		if err := svc.CloseBreakGlass(r.Context(), from(r).op, id, bg, remote(r)); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/operator/audit", auth(func(w http.ResponseWriter, r *http.Request) {
		var before time.Time
		if v := r.URL.Query().Get("before"); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "before must be an RFC 3339 time"})
				return
			}
			before = t
		}
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "limit must be 1–200"})
				return
			}
			limit = n
		}
		list, err := svc.Audit(r.Context(), before, limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]apispec.OperatorAuditEntry, 0, len(list))
		for _, e := range list {
			x := apispec.OperatorAuditEntry{Id: e.ID, Action: e.Action, Payload: e.Payload, RemoteAddr: e.RemoteAddr, CreatedAt: e.CreatedAt}
			if x.Payload == nil {
				x.Payload = map[string]any{}
			}
			if e.OperatorID != ids.Nil {
				x.OperatorId = &e.OperatorID
			}
			if e.WorkspaceID != ids.Nil {
				x.WorkspaceId = &e.WorkspaceID
			}
			out = append(out, x)
		}
		httpx.JSON(w, http.StatusOK, apispec.OperatorAuditList{Entries: out})
	}))

	mux.Handle("GET /api/v1/operator/system/database", auth(func(w http.ResponseWriter, r *http.Request) {
		out, err := opt.Database(r.Context())
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}

func csrfMatches(want, got string) bool {
	return want != "" && got != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func workspaceID(w http.ResponseWriter, r *http.Request) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue("workspaceId"))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: "workspaceId must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}

// inviteToAPI builds the invite link on the customer's subdomain: the
// request came to admin.<domain>[:port], the link goes to <slug>.<domain>
// with the same port.
func inviteToAPI(r *http.Request, opt Options, slug string, link operator.InviteLink) apispec.InviteLink {
	return apispec.InviteLink{Url: customerURL(r, opt, slug, "/app/invite#"+link.Token), ExpiresAt: link.ExpiresAt}
}

// customerURL is a link on the customer's subdomain: the request came to
// admin.<domain>[:port], the link goes to <slug>.<domain> with the same port.
func customerURL(r *http.Request, opt Options, slug, path string) string {
	scheme := "http"
	if opt.SecureCookies {
		scheme = "https"
	}
	host := strings.TrimPrefix(strings.ToLower(r.Host), "admin.")
	return scheme + "://" + slug + "." + host + path
}

func breakGlassToAPI(b operator.BreakGlass) apispec.BreakGlass {
	x := apispec.BreakGlass{Id: b.ID, OperatorHandle: b.OperatorHandle, Reason: b.Reason, DevMode: b.DevMode,
		OpenedAt: b.OpenedAt, ExpiresAt: b.ExpiresAt, Open: b.Open, ClosedAt: b.ClosedAt}
	if b.ClosedBy != "" {
		x.ClosedBy = &b.ClosedBy
	}
	return x
}

func accountToAPI(o operator.Operator) apispec.OperatorAccount {
	return apispec.OperatorAccount{Id: o.ID, Handle: o.Handle, DisplayName: o.DisplayName, LastLoginAt: o.LastLoginAt}
}

func customerToAPI(c operator.Customer) apispec.Customer {
	out := apispec.Customer{Id: c.ID, Name: c.Name, Slug: c.Slug, State: apispec.CustomerState(c.State), CreatedAt: c.CreatedAt,
		HasOwner: c.HasOwner, Users: c.Users, Agents: c.Agents, Hosts: c.Hosts, HostsOnline: c.HostsOnline, LastActivityAt: c.LastActivity,
		SuspendedAt: c.SuspendedAt,
		Storage: apispec.CustomerStorage{UsedBytes: c.Storage.UsedBytes, QuotaBytes: c.Storage.QuotaBytes, FilesBytes: c.Storage.FilesBytes,
			DbBytes: c.Storage.DBBytes, State: apispec.CustomerStorageState(c.Storage.State)}}
	if c.SuspendedReason != "" {
		out.SuspendedReason = &c.SuspendedReason
	}
	out.BreakGlassOpen = c.BreakGlassOpen
	if c.PlanID != ids.Nil {
		id := c.PlanID
		out.PlanId = &id
	}
	if c.Invite != nil {
		out.Invite = &apispec.CustomerInvite{CreatedAt: c.Invite.CreatedAt, ExpiresAt: c.Invite.ExpiresAt, Used: c.Invite.Used, Revoked: c.Invite.Revoked}
	}
	return out
}

func planToAPI(p operator.PlanWithUse) apispec.Plan {
	return apispec.Plan{Id: p.ID, Name: p.Name, StorageBytes: p.StorageBytes, CpuCores: p.CPUCores, MemoryBytes: p.MemoryBytes,
		ExtCallsMonth: httpx.Ptr(p.ExtCallsMonth), Workspaces: p.Workspaces, ArchivedAt: p.ArchivedAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}
