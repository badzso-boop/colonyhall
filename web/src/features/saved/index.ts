// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerSavedEvents } from "./events";
export { useSave, useSavedIndex, savedKeys, type SavedItem } from "./api";
export { SavedEntry } from "./components/SavedEntry";
export { ReminderDialog } from "./components/ReminderDialog";
export { useSaveActions } from "./components/useSaveActions";
export { reminderTime, timeZoneOf, zonedTime, dayAt, zonedDate } from "./model";
export const SavedPage = lazy(() => import("./pages/SavedPage"));
