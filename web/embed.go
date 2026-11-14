// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package web embeds the built PWA (React + Vite, see web/package.json)
// into the server binary. `make web` fills dist/; the tracked dist/.keep
// keeps the directory non-empty, so the server also builds without Node
// and then answers /app/ with 503.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
