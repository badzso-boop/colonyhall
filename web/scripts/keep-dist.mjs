// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Vite empties dist/ on every build; recreate the tracked placeholder so
// `go build` keeps working and git does not see it as deleted.
import { writeFileSync } from "node:fs";

writeFileSync(new URL("../dist/.keep", import.meta.url), "");
