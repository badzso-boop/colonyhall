// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package mcpremote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/egress"
)

// MaxFetchBody bounds an answer an extension gets (docs/25 13.4).
const MaxFetchBody = 5 << 20

// Fetcher implements workshop.Fetcher: the extensions' outbound HTTP,
// through the egress policy. Authentication headers go only to the
// request's own origin (a redirect elsewhere drops them).
type Fetcher struct {
	Policy egress.Policy
	// HTTP replaces the egress client (tests only).
	HTTP *http.Client
}

// Fetch implements workshop.Fetcher.
func (f *Fetcher) Fetch(ctx context.Context, r workshop.FetchRequest) (workshop.FetchResponse, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return workshop.FetchResponse{}, errors.New("invalid URL")
	}
	hc := f.HTTP
	if hc == nil {
		hc = f.Policy.Client(egress.Options{Timeout: workshop.CallTimeout, Hosts: r.Hosts})
	}
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	client := &http.Client{Timeout: hc.Timeout, CheckRedirect: hc.CheckRedirect,
		Transport: withHeader{header: r.Auth, origin: u.Scheme + "://" + u.Host, next: next}}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return workshop.FetchResponse{}, errors.New("invalid request")
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return workshop.FetchResponse{}, clean(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxFetchBody+1))
	if err != nil {
		return workshop.FetchResponse{}, clean(err)
	}
	if len(body) > MaxFetchBody {
		return workshop.FetchResponse{}, errors.New("the answer is larger than 5 MB")
	}
	ct := res.Header.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	return workshop.FetchResponse{Status: res.StatusCode, ContentType: strings.TrimSpace(ct), Body: body}, nil
}
