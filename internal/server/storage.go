// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
)

// storageBotHandle is the system actor that writes the storage warnings to
// the owner (round 54), like the Gitea bot writes webhook summaries.
const storageBotHandle = "colonyhall-storage"

// storageNotifier sends the owner a DM from the system when the workspace
// storage crosses 80%, 95% or fills up. It names numbers, never people or
// files (Norbi, 2026-11-14: the owner never sees who uploaded what).
type storageNotifier struct {
	identity     *identity.Service
	conversation *conversation.Service
	logger       *slog.Logger
}

func (n *storageNotifier) StorageWarning(ctx context.Context, workspaceID ids.ID, u files.Usage) {
	ctx = db.WithWorkspace(context.WithoutCancel(ctx), workspaceID)
	if err := n.send(ctx, workspaceID, u); err != nil {
		n.logger.Warn("storage warning not sent", "workspace", workspaceID, "state", u.State(), "err", err)
	}
}

func (n *storageNotifier) send(ctx context.Context, workspaceID ids.ID, u files.Usage) error {
	admins, err := n.identity.Admins(ctx, workspaceID)
	if err != nil {
		return err
	}
	var owner *identity.Actor
	for i := range admins {
		if admins[i].Role == identity.RoleOwner && admins[i].Kind == identity.KindHuman {
			owner = &admins[i]
			break
		}
	}
	if owner == nil {
		return nil // no active owner to tell; the banner still shows
	}
	bot, err := n.identity.EnsureSystemActor(ctx, workspaceID, storageBotHandle, "Colonyhall")
	if err != nil {
		return err
	}
	// Never write as somebody who took the handle before round 54.
	if bot.Kind != identity.KindAgent || bot.OwnerID != ids.Nil || bot.ID == owner.ID {
		return fmt.Errorf("handle %q is taken by another actor", storageBotHandle)
	}
	dm, err := n.conversation.OpenDM(ctx, bot, []ids.ID{owner.ID})
	if err != nil {
		return err
	}
	_, err = n.conversation.SendMessage(ctx, bot, conversation.SendInput{ChannelID: dm.ID, Body: storageWarningText(u)})
	return err
}

// storageWarningText is the warning in the owner's language (Hungarian, like
// the other server-written notes).
func storageWarningText(u files.Usage) string {
	used, quota := humanBytes(u.Used()), humanBytes(u.QuotaBytes)
	if u.State() == files.StorageFull {
		return fmt.Sprintf("⛔ A munkaterület tárhelye megtelt: %s / %s foglalt. Új fájlt nem lehet feltölteni, "+
			"és az őrök nem ébresztik az agenteket, amíg nincs hely. Üzenetet továbbra is lehet írni. "+
			"Mindenki a saját fájljait törölheti a „Fájljaim” oldalon; a keretet a Rendszerállapot oldalon emelheted.", used, quota)
	}
	pct := 80
	if u.State() == files.StorageWarning95 {
		pct = 95
	}
	return fmt.Sprintf("⚠️ A munkaterület tárhelye elérte a %d%%-ot: %s / %s foglalt. "+
		"Ha megtelik, a feltöltés hibát ad, és az agentek szünetelnek. "+
		"Mindenki a saját fájljait törölheti a „Fájljaim” oldalon; a keretet a Rendszerállapot oldalon emelheted.", pct, used, quota)
}

// humanBytes formats a size with a decimal comma: "1,5 GB".
func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", f, units[i]), ".", ",", 1)
}

// storageStates gives the hosts module the workspace storage state for the
// guards' host config.
type storageStates struct{ files *files.Service }

func (s storageStates) HostStorage(ctx context.Context, workspaceID ids.ID) (hosts.HostStorage, error) {
	u, err := s.files.StorageState(ctx, workspaceID)
	if err != nil {
		return hosts.HostStorage{}, err
	}
	return hosts.HostStorage{State: string(u.State()), UsedBytes: u.Used(), QuotaBytes: u.QuotaBytes}, nil
}

// maintainStorage refreshes every workspace's database share and recounts
// its files (round 54), then removes contents no live file references. The
// estimate runs scoped to each workspace; the sweep in system mode (a
// content may be shared across workspaces).
func (a *App) maintainStorage(ctx context.Context) {
	ctx = system.Context(ctx)
	list, ok, err := a.DBStats.Workspaces(ctx)
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Warn("storage refresh: workspace list failed", "err", err)
		}
		return
	}
	if ok {
		only := len(list) == 1
		for _, ws := range list {
			wctx := db.WithWorkspace(ctx, ws)
			est, err := a.DBStats.WorkspaceEstimate(wctx, ws, only)
			if err == nil {
				_, err = a.Files.RefreshUsage(wctx, ws, files.DBEstimate{Total: est.Total, Chat: est.Chat})
			}
			if err != nil && ctx.Err() == nil {
				a.logger.Warn("storage refresh failed", "workspace", ws, "err", err)
			}
		}
	}
	n, err := a.Files.SweepContents(ctx)
	if err != nil && ctx.Err() == nil {
		a.logger.Warn("file content sweep failed", "err", err)
	}
	if n > 0 {
		a.logger.Info("removed unreferenced file contents", "count", n)
	}
}
