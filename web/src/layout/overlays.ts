// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { create } from "zustand";

/** Which app-level overlay is open: the command palette or the shortcut help. */
export const useAppOverlay = create<{ open: "palette" | "help" | null }>(() => ({ open: null }));
export const openOverlay = (o: "palette" | "help" | null) => useAppOverlay.setState({ open: o });
