// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package preview

// Dev templates (round 57): what an agent writes as compose.yaml to show a
// web app live in Colonyhall. The guard sets COLONY_PREVIEW_BASE on every
// service: the path the server's proxy serves the preview on, forwarded
// unchanged, so the dev server must use it as its base. The source is
// bind-mounted (./:/app), so edits show at once; polling makes file
// watching work on WSL's /mnt/* drives, where inotify events do not reach
// the container (drop the two variables on a native Linux filesystem).
// internal/guard/preview_live_test.go checks that the guard accepts them;
// keep the copies there in step.

// ViteTemplate runs a Vite (React, Vue, Svelte, …) dev server with live reload.
const ViteTemplate = `services:
  web:
    image: node:20-alpine
    working_dir: /app
    command: sh -c 'npm install && npm run dev -- --host 0.0.0.0 --port 5173 --base "$$COLONY_PREVIEW_BASE"'
    volumes:
      - ./:/app
      - /app/node_modules
    environment:
      - CHOKIDAR_USEPOLLING=1
      - WATCHPACK_POLLING=true
    ports:
      - "5173:5173"
`

// StaticTemplate serves a folder of HTML, CSS and JavaScript with Caddy.
const StaticTemplate = `services:
  web:
    image: caddy:alpine
    command: sh -c 'mkdir -p "/srv$${COLONY_PREVIEW_BASE%/*/}" && ln -sfn /app "/srv$${COLONY_PREVIEW_BASE%/}" && caddy file-server --listen :8080 --root /srv'
    volumes:
      - ./:/app:ro
    ports:
      - "8080:8080"
`

// TemplateGuide is the part of preview_start's description that tells an
// agent how to write the compose file.
const TemplateGuide = "Always write an explicit compose file in your working folder first. " +
	"The guard sets COLONY_PREVIEW_BASE on every service: Colonyhall serves the app under that path, so the dev server must use it as its base " +
	"(Vite --base \"$$COLONY_PREVIEW_BASE\", Next.js basePath; links must be relative or under it). " +
	"Bind-mount the source (./:/app) for live reload; keep node_modules in an anonymous volume (/app/node_modules); " +
	"CHOKIDAR_USEPOLLING=1 and WATCHPACK_POLLING=true make file watching work on WSL /mnt/* folders (drop them on a native Linux filesystem). " +
	"No host paths outside your folder, no variables in volumes, no privileged settings. " +
	"For a package.json app with Vite:\n" + ViteTemplate +
	"For static HTML:\n" + StaticTemplate
