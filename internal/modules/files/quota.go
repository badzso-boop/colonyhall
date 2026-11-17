// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

// StorageState is how full a workspace's storage is (round 54).
type StorageState string

const (
	StorageOK        StorageState = "ok"
	StorageWarning80 StorageState = "warning_80"
	StorageWarning95 StorageState = "warning_95"
	StorageFull      StorageState = "full"
)

// Warning levels in percent of the quota; LevelFull is the full state.
const (
	LevelNone = 0
	Level80   = 80
	Level95   = 95
	LevelFull = 100
)

// MaxQuotaBytes bounds a quota (1 PiB): anything above is a typo.
const MaxQuotaBytes int64 = 1 << 50

// Usage is a workspace's storage: its live files (each content once), its
// share of the database (the hourly estimate) and its quota. Never per person.
type Usage struct {
	WorkspaceID ids.ID
	// QuotaBytes 0 is unlimited.
	QuotaBytes int64
	FilesBytes int64
	// DBBytes is the database share; ChatBytes is the part of it that is
	// chat and search data (messages, events, search chunks).
	DBBytes      int64
	ChatBytes    int64
	DBMeasuredAt *time.Time
	// WarnedLevel is the level last announced (LevelNone, Level80, …).
	WarnedLevel int
	UpdatedAt   time.Time
	// Plan is the operator's plan for the workspace (round 51b), a copy of
	// its numbers; nil without a plan (only QuotaBytes counts, as before).
	Plan *Plan
}

// Plan is what the workspace's plan gives (round 51b): the storage it
// limits, and the CPU cores and memory a hosted guard will get (stored and
// shown; not enforced yet).
type Plan struct {
	ID           ids.ID
	Name         string
	StorageBytes int64
	CPUCores     int
	MemoryBytes  int64
	// ExtCallsMonth is the Workshop's monthly call budget (round 56b); 0
	// is unlimited.
	ExtCallsMonth int
}

// Limit is the effective quota (round 51b): the plan's storage, or the
// owner's own quota when that is lower and set; 0 is unlimited (no plan
// and no quota).
func (u Usage) Limit() int64 {
	if u.Plan == nil || u.Plan.StorageBytes <= 0 {
		return u.QuotaBytes
	}
	if u.QuotaBytes > 0 && u.QuotaBytes < u.Plan.StorageBytes {
		return u.QuotaBytes
	}
	return u.Plan.StorageBytes
}

// Used is the total that counts towards the quota.
func (u Usage) Used() int64 { return u.FilesBytes + u.DBBytes }

// Level is the warning level the usage is at: LevelFull when the quota is
// reached, LevelNone without a quota.
func (u Usage) Level() int { return levelOf(u.Used(), u.Limit()) }

func levelOf(used, quota int64) int {
	if quota <= 0 {
		return LevelNone
	}
	switch {
	case used >= quota:
		return LevelFull
	// used*100 >= quota*95 without overflow for quotas up to MaxQuotaBytes.
	case used >= quota/100*95+quota%100*95/100:
		return Level95
	case used >= quota/100*80+quota%100*80/100:
		return Level80
	}
	return LevelNone
}

// State is the level as the API names it.
func (u Usage) State() StorageState { return stateOf(u.Level()) }

func stateOf(level int) StorageState {
	switch level {
	case LevelFull:
		return StorageFull
	case Level95:
		return StorageWarning95
	case Level80:
		return StorageWarning80
	}
	return StorageOK
}

// Breakdown splits the usage by kind; never by person.
type Breakdown struct {
	Attachments int64
	ChatSearch  int64
	Other       int64
}

// Breakdown returns the usage by kind.
func (u Usage) Breakdown() Breakdown {
	chat := min(u.ChatBytes, u.DBBytes)
	return Breakdown{Attachments: u.FilesBytes, ChatSearch: chat, Other: u.DBBytes - chat}
}

// DBEstimate is a workspace's share of the database, measured hourly.
type DBEstimate struct {
	Total int64
	Chat  int64
	At    time.Time
}

// ErrStorageFull: the upload would take the workspace over its quota.
var ErrStorageFull = errors.New("workspace storage is full")

// FullError is ErrStorageFull with the numbers the client shows.
type FullError struct {
	Used  int64
	Quota int64
}

func (e *FullError) Error() string {
	return fmt.Sprintf("The workspace storage is full: %d of %d bytes used. Delete files you no longer need in My files, or ask the owner to raise the quota.", e.Used, e.Quota)
}

// Is makes errors.Is(err, ErrStorageFull) true.
func (e *FullError) Is(target error) bool { return target == ErrStorageFull }

// ProblemDetail keeps the explanation and the numbers in the 507 answer
// (5xx details are hidden otherwise): they hold no secret, and the person
// needs them to make room.
func (e *FullError) ProblemDetail() (string, map[string]any) {
	return e.Error(), map[string]any{"used_bytes": e.Used, "quota_bytes": e.Quota}
}

// Deleting files (round 54).
var (
	// ErrGone: the file was deleted; its download answers 410.
	ErrGone = errors.New("file was deleted")
	// ErrNotUploader: only the uploader (or the owner of the uploading
	// agent) may delete a file; an admin or the owner is no exception.
	ErrNotUploader = errors.New("only the uploader can delete a file")
	// ErrNotOwner: only the workspace owner sets the quota.
	ErrNotOwner = errors.New("only the workspace owner can set the storage quota")
	// ErrNotAdmin: only the owner and admins see the storage usage.
	ErrNotAdmin = errors.New("only the owner and admins can see the storage usage")
	// ErrInvalidQuota: a negative or absurd quota.
	ErrInvalidQuota = errors.New("quota must be between 0 (unlimited) and 1 PiB")
	// ErrQuotaAbovePlan: the owner's quota may lower the plan's storage,
	// never raise it (round 51b).
	ErrQuotaAbovePlan = errors.New("the quota cannot be above the workspace's plan")
	// ErrPlanOverQuota: the plan would leave the workspace full; the
	// operator must confirm it (round 51b).
	ErrPlanOverQuota = errors.New("the plan is below the workspace's current usage")
)

// OverQuotaError is ErrPlanOverQuota with the numbers the operator sees.
type OverQuotaError struct {
	Used  int64
	Quota int64
}

func (e *OverQuotaError) Error() string {
	return fmt.Sprintf("The workspace uses %d bytes, the plan gives %d: it would be full and its agents would pause until it shrinks. Confirm to go ahead.", e.Used, e.Quota)
}

// Is makes errors.Is(err, ErrPlanOverQuota) true.
func (e *OverQuotaError) Is(target error) bool { return target == ErrPlanOverQuota }

// Uploader picks whose files "My files" lists.
type Uploader string

const (
	// UploaderMe lists the files the person uploaded.
	UploaderMe Uploader = "me"
	// UploaderAgents lists the files the person's agents uploaded or fetched.
	UploaderAgents Uploader = "agents"
)

// MyFilesQuery filters and pages "My files".
type MyFilesQuery struct {
	WorkspaceID ids.ID
	// UploaderIDs are the people or agents whose files are listed.
	UploaderIDs []ids.ID
	ChannelID   ids.ID // ids.Nil: every channel
	// Kind filters by type: "image", "document", "other" or "" (all).
	Kind string
	// Before pages: files strictly older than (BeforeAt, BeforeID).
	BeforeAt *time.Time
	BeforeID ids.ID
	Limit    int
}

// File kinds for the "My files" filter.
const (
	KindImage    = "image"
	KindDocument = "document"
	KindOther    = "other"
)

// KindOf classifies a MIME type for the "My files" filter; the Postgres
// query uses the same rule.
func KindOf(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return KindImage
	case strings.HasPrefix(mime, "text/"), strings.HasPrefix(mime, "application/vnd."),
		mime == "application/pdf", mime == "application/json", mime == "application/msword", mime == "application/rtf":
		return KindDocument
	}
	return KindOther
}

// MyFile is one row of "My files".
type MyFile struct {
	File
	// MessageID, ThreadRootID: the message the file is attached to (ids.Nil
	// when it was never sent); the service fills ThreadRootID.
	MessageID    ids.ID
	ThreadRootID ids.ID
	// SharedCount is how many other live files of the same uploaders (the
	// caller and their agents) have the same content (deleting this one
	// keeps the content for them). Other people's copies are not counted:
	// that would tell the caller who else holds a given file.
	SharedCount int
}

// Paging of "My files".
const (
	DefaultMyFilesLimit = 50
	MaxMyFilesLimit     = 200
)

// SweepBatch bounds how many contents one sweep looks at.
const SweepBatch = 500
