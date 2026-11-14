// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { NotesWindow } from "./components/NotesWindow";
export { useNotesBadge } from "./badges";
export const NotesPage = lazy(() => import("./pages/NotesPage"));
