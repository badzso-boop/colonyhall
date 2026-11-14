//go:build !unix

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import "os/exec"

// setProcessGroup is a no-op here; ctx cancellation kills the direct child.
func setProcessGroup(*exec.Cmd) {}
