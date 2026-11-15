// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/internal/server"
)

func TestOperatorCreateFromTheCommandLine(t *testing.T) {
	k, _ := secretbox.NewKey("k1")
	keys, _ := secretbox.Parse(k)
	clk := clock.NewManual(time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC))
	app, err := server.New(context.Background(), server.Options{Store: "memory", Multitenant: true, BaseDomain: "example.test",
		SecretKeys: keys, Clock: clk})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)

	// The command prints the secret, then reads the code it shows.
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- runOperatorCmd(context.Background(), app.Operator, "operator-create", "ops", "Ops", bufio.NewReader(inR), outW)
		_ = outW.Close()
	}()
	go func() { _, _ = io.WriteString(inW, "a-long-enough-password\n") }()
	var printed bytes.Buffer
	sc := bufio.NewReader(io.TeeReader(outR, &printed))
	var secret string
	for secret == "" {
		l, err := sc.ReadString('\n')
		if err != nil {
			t.Fatalf("no secret printed: %q", printed.String())
		}
		if s, ok := strings.CutPrefix(strings.TrimSpace(l), "secret: "); ok {
			secret = s
		}
	}
	go func() { _, _ = io.WriteString(inW, operator.CodeAt(secret, clk.Now())+"\n") }()
	rest, _ := io.ReadAll(sc)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	codes := regexp.MustCompile(`(?m)^  [A-Z2-9]{5}-[A-Z2-9]{5}$`).FindAllString(string(rest), -1)
	if len(codes) != operator.BackupCodeCount || !strings.Contains(string(rest), "@ops is active") {
		t.Fatalf("output: %s", rest)
	}

	// The operator can log in with the password and a later code.
	clk.Advance(time.Minute)
	if _, _, _, err := app.Operator.Login(context.Background(), operator.LoginInput{Handle: "ops", Password: "a-long-enough-password",
		Code: operator.CodeAt(secret, clk.Now())}); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := runOperatorCmd(context.Background(), app.Operator, "operator-disable", "ops", "", bufio.NewReader(strings.NewReader("")), io.Discard); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if _, _, _, err := app.Operator.Login(context.Background(), operator.LoginInput{Handle: "ops", Password: "a-long-enough-password",
		Code: operator.CodeAt(secret, clk.Now())}); err == nil {
		t.Fatal("a disabled operator logged in")
	}
}
