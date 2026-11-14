// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { FileBrowserPane } from "./components/FileBrowserPane";
export { MessageFileSheet } from "./components/MessageFileSheet";
export { StorageBanner } from "./components/StorageBanner";
export { registerFilesEvents, storageFull, useSetQuota, useWorkspaceStorage, type StorageState, type WorkspaceStorage } from "./storage";
export const MyFilesPage = lazy(() => import("./pages/MyFilesPage"));
