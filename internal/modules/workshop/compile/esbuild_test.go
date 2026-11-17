// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package compile

import (
	"strings"
	"testing"
)

func TestCompile(t *testing.T) {
	js, err := Esbuild{}.Compile(`type In = { name: string }
export async function hello(input: In, ctx: any) { return "Hello " + input.name; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "__ext") || strings.Contains(js, ": In") {
		t.Fatalf("compiled: %s", js)
	}
	if _, err := (Esbuild{}).Compile(`export const f = (`); err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("syntax error: %v", err)
	}
	if _, err := (Esbuild{}).Compile(`import fs from "fs"; export const f = () => fs;`); err == nil {
		t.Fatal("an import was accepted")
	}
}
