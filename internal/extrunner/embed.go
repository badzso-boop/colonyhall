// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import _ "embed"

// qjs.wasm is QuickJS-NG compiled to WebAssembly, copied from
// github.com/fastschema/qjs v0.0.6 (MIT; the package does not export its
// own copy). Update it together with the module version; QJSWasmSHA256
// and the test keep the two in step.
//
//go:embed qjs.wasm
var embeddedQJS []byte

// QJSWasmSHA256 is the hash of qjs.wasm of fastschema/qjs v0.0.6.
const QJSWasmSHA256 = "88fcbeead33cca3ec8394feb0fb3eec8047ec9ac5062aba7a63799cee11989b7"

func qjsWasm() ([]byte, error) { return embeddedQJS, nil }
