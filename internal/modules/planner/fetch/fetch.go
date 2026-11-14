// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package fetch downloads subscribed calendars from public https hosts only.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"colonyhall/internal/modules/planner"
)

// Safe fetches public https URLs only: every dial resolves the host
// and connects to a checked public IP (no DNS rebinding), every redirect is
// checked again, and the body is capped.
type Safe struct {
	Resolver *net.Resolver
}

// Fetch implements Fetcher.
func (f Safe) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	resolver := f.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if planner.PublicIP(ip.IP) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				}
			}
			return nil, planner.ErrCalendarURL
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: planner.FetchTimeout,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		Timeout:   planner.FetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > planner.MaxRedirects {
				return errors.New("too many redirects")
			}
			if _, err := planner.NormalizeCalendarURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/calendar, */*;q=0.5")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, planner.MaxCalendarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > planner.MaxCalendarBytes {
		return nil, errors.New("calendar is larger than 5 MB")
	}
	return body, nil
}
