package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/files"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type msgFileLink struct {
	fileID   ids.ID
	position int
}

// Store is an in-memory implementation of files.Store.
type Store struct {
	mu           sync.RWMutex
	files        map[ids.ID]files.File
	messageFiles map[ids.ID][]msgFileLink // messageID -> []msgFileLink
	requests     map[ids.ID]files.FileFetchRequest
	settings     map[ids.ID]files.FileFetchSettings
}

// NewStore creates a new in-memory file store.
func NewStore() *Store {
	return &Store{
		files:        make(map[ids.ID]files.File),
		messageFiles: make(map[ids.ID][]msgFileLink),
		requests:     make(map[ids.ID]files.FileFetchRequest),
		settings:     make(map[ids.ID]files.FileFetchSettings),
	}
}

// Insert saves a new file's metadata.
func (s *Store) Insert(_ context.Context, f files.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[f.ID] = f
	return nil
}

// ByID loads a file by ID.
func (s *Store) ByID(_ context.Context, id ids.ID) (files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.files[id]
	if !ok {
		return files.File{}, db.ErrNotFound
	}
	return f, nil
}

// ByIDs loads multiple files by IDs.
func (s *Store) ByIDs(_ context.Context, idList []ids.ID) ([]files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]files.File, 0, len(idList))
	for _, id := range idList {
		if f, ok := s.files[id]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}

// ListByChannel returns files uploaded in channel, newest first.
func (s *Store) ListByChannel(_ context.Context, channelID ids.ID) ([]files.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]files.File, 0)
	for _, f := range s.files {
		if f.ChannelID == channelID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// AttachToMessage links a file to a message with a position index.
func (s *Store) AttachToMessage(_ context.Context, messageID, fileID ids.ID, position int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	links := s.messageFiles[messageID]
	// Check if already attached
	for _, l := range links {
		if l.fileID == fileID {
			return nil
		}
	}
	s.messageFiles[messageID] = append(links, msgFileLink{fileID: fileID, position: position})
	return nil
}

// AttachmentsForMessage returns attachments for a single message.
func (s *Store) AttachmentsForMessage(_ context.Context, messageID ids.ID) ([]files.Attachment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	links := s.messageFiles[messageID]
	if len(links) == 0 {
		return []files.Attachment{}, nil
	}
	sorted := make([]msgFileLink, len(links))
	copy(sorted, links)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].position < sorted[j].position
	})

	out := make([]files.Attachment, 0, len(sorted))
	for _, l := range sorted {
		if f, ok := s.files[l.fileID]; ok {
			out = append(out, files.Attachment{
				ID:   f.ID,
				Name: f.Name,
				Mime: f.Mime,
				Size: f.Size,
			})
		}
	}
	return out, nil
}

// AttachmentsForMessages returns attachments grouped by message ID.
func (s *Store) AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]files.Attachment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[ids.ID][]files.Attachment, len(messageIDs))
	for _, mID := range messageIDs {
		links := s.messageFiles[mID]
		if len(links) == 0 {
			res[mID] = []files.Attachment{}
			continue
		}
		sorted := make([]msgFileLink, len(links))
		copy(sorted, links)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].position < sorted[j].position
		})
		atts := make([]files.Attachment, 0, len(sorted))
		for _, l := range sorted {
			if f, ok := s.files[l.fileID]; ok {
				atts = append(atts, files.Attachment{
					ID:   f.ID,
					Name: f.Name,
					Mime: f.Mime,
					Size: f.Size,
				})
			}
		}
		res[mID] = atts
	}
	return res, nil
}

// InsertFileFetchRequest creates a new file fetch request.
func (s *Store) InsertFileFetchRequest(_ context.Context, req files.FileFetchRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[req.ID] = req
	return nil
}

// FileFetchRequestByID loads a file fetch request by ID.
func (s *Store) FileFetchRequestByID(_ context.Context, id ids.ID) (files.FileFetchRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.requests[id]
	if !ok {
		return files.FileFetchRequest{}, db.ErrNotFound
	}
	return req, nil
}

// FileFetchRequestByMessageAndPath loads the most recent request for a message and path.
func (s *Store) FileFetchRequestByMessageAndPath(_ context.Context, workspaceID, messageID ids.ID, path string) (files.FileFetchRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []files.FileFetchRequest
	for _, r := range s.requests {
		if r.WorkspaceID == workspaceID && r.MessageID == messageID && r.Path == path {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return files.FileFetchRequest{}, db.ErrNotFound
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})
	return matches[0], nil
}

// ListFileFetchRequestsByAgent lists requests for an agent with a given state.
func (s *Store) ListFileFetchRequestsByAgent(_ context.Context, workspaceID, agentID ids.ID, state files.FileFetchState) ([]files.FileFetchRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []files.FileFetchRequest
	for _, r := range s.requests {
		if r.WorkspaceID == workspaceID && r.AgentID == agentID && r.State == state {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// ListPendingFileFetchRequestsBefore lists pending requests created on or before a given time.
func (s *Store) ListPendingFileFetchRequestsBefore(_ context.Context, before time.Time) ([]files.FileFetchRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []files.FileFetchRequest
	for _, r := range s.requests {
		if r.State == files.FileFetchPending && (r.CreatedAt.Before(before) || r.CreatedAt.Equal(before)) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// UpdateFileFetchRequestCompleted marks a request as done with its stored file ID.
func (s *Store) UpdateFileFetchRequestCompleted(_ context.Context, id ids.ID, fileID ids.ID, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.requests[id]
	if !ok {
		return db.ErrNotFound
	}
	if req.State != files.FileFetchPending {
		return db.ErrConflict
	}
	req.State = files.FileFetchDone
	req.FileID = fileID
	req.CompletedAt = &completedAt
	s.requests[id] = req
	return nil
}

// UpdateFileFetchRequestFailed marks a request as failed with an error code.
func (s *Store) UpdateFileFetchRequestFailed(_ context.Context, id ids.ID, errorCode files.FileFetchErrorCode, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.requests[id]
	if !ok {
		return db.ErrNotFound
	}
	if req.State != files.FileFetchPending {
		return db.ErrConflict
	}
	req.State = files.FileFetchFailed
	req.ErrorCode = errorCode
	req.CompletedAt = &completedAt
	s.requests[id] = req
	return nil
}

// GetFileFetchSettings loads workspace file fetch settings.
func (s *Store) GetFileFetchSettings(_ context.Context, workspaceID ids.ID) (files.FileFetchSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.settings[workspaceID]
	if !ok {
		return files.FileFetchSettings{
			WorkspaceID: workspaceID,
			MaxImageMB:  files.DefaultMaxImageMB,
			MaxFileMB:   files.DefaultMaxFileMB,
		}, nil
	}
	return st, nil
}

// UpsertFileFetchSettings creates or updates workspace file fetch settings.
func (s *Store) UpsertFileFetchSettings(_ context.Context, st files.FileFetchSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[st.WorkspaceID] = st
	return nil
}

var _ files.Store = (*Store)(nil)
