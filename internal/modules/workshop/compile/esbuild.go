// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package compile turns an extension's TypeScript or JavaScript into the
// script the code runner runs (round 56b): esbuild's Go library, no Node.
// The source is one module whose exports become globalThis.__ext; imports
// are refused (a tool has only its ctx).
package compile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Esbuild implements workshop.Compiler.
type Esbuild struct{}

// Compile implements workshop.Compiler.
func (Esbuild) Compile(source string) (string, error) {
	res := api.Transform(source, api.TransformOptions{
		Loader:     api.LoaderTS,
		Format:     api.FormatIIFE,
		GlobalName: "__ext",
		Target:     api.ES2022,
		Sourcefile: "extension.ts",
		Charset:    api.CharsetUTF8,
	})
	if len(res.Errors) > 0 {
		var b strings.Builder
		for i, e := range res.Errors {
			if i == 5 {
				break
			}
			if e.Location != nil {
				fmt.Fprintf(&b, "line %d:%d: ", e.Location.Line, e.Location.Column)
			}
			b.WriteString(e.Text)
			b.WriteString("\n")
		}
		return "", errors.New(strings.TrimSpace(b.String()))
	}
	code := string(res.Code)
	// An IIFE keeps import statements as require() calls; nothing can be
	// required in the runner, so refuse them here with a clear message.
	if strings.Contains(code, "require(") || strings.Contains(code, "__require") {
		return "", errors.New("imports are not supported: a tool uses only its ctx (fetch, store, colony, call, log)")
	}
	return code + "\nglobalThis.__ext = __ext;\n", nil
}
