package store

import (
	"bufio"
	"cave-archive/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var ErrNoChange = errors.New("无需写入")

type event struct {
	SchemaVersion int             `json:"schemaVersion"`
	Seq           int64           `json:"seq"`
	Type          string          `json:"type"`
	Archive       *domain.Archive `json:"archive,omitempty"`
	PrevHash      string          `json:"prevHash"`
	Hash          string          `json:"hash"`
}

type Store struct {
	mu        sync.RWMutex
	archives  map[string]*domain.Archive
	codeIndex map[string]string
	idem      map[string]any
	path      string
	eventLog  *os.File
	seq       int64
	prevHash  string
}

func New(path string) (*Store, error) {
	s := &Store{archives: map[string]*domain.Archive{}, codeIndex: map[string]string{}, idem: map[string]any{}, path: path}
	if path == "" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := s.replay(); err != nil {
		return nil, err
	}
	s.rebuildIndexes()
	return s, nil
}

// replay re-reads the event log at s.path and re-establishes s.seq, s.prevHash
// and the in-memory archive projections. It is used during initialization and
// after detecting that the active log file has been rotated out from under
// the cached writer handle.
func (s *Store) replay() error {
	s.seq, s.prevHash = 0, ""
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var expectedSeq int64
	var previous string
	for scanner.Scan() {
		var entry event
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || !schemaVersionValid(entry.SchemaVersion) || entry.Seq != expectedSeq+1 || entry.PrevHash != previous {
			return fmt.Errorf("事件日志校验失败")
		}
		stored := entry.Hash
		entry.Hash = ""
		if domain.Hash(entry) != stored {
			return fmt.Errorf("事件哈希链校验失败")
		}
		entry.Hash = stored
		expectedSeq, previous = entry.Seq, entry.Hash
		s.seq, s.prevHash = entry.Seq, entry.Hash
		if entry.Archive != nil {
			initialize(entry.Archive)
			s.archives[entry.Archive.ID] = entry.Archive
		}
	}
	return scanner.Err()
}

func initialize(a *domain.Archive) {
	if a.Revisions == nil {
		a.Revisions = map[string]*domain.Revision{}
	}
	if a.CheckRuns == nil {
		a.CheckRuns = map[string]*domain.CheckRun{}
	}
	if a.Findings == nil {
		a.Findings = map[string]*domain.Finding{}
	}
}

func (s *Store) rebuildIndexes() {
	s.codeIndex = map[string]string{}
	ids := make([]string, 0, len(s.archives))
	for id := range s.archives {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		archive := s.archives[id]
		code := domain.NormalizeArchiveCode(archive.ArchiveCode)
		if s.codeIndex[code] == "" {
			s.codeIndex[code] = id
		}
		if archive.CreateIdempotencyKey != "" {
			s.idem["create:"+archive.CreateIdempotencyKey] = archive.ID
		}
	}
}

func cloneArchive(a *domain.Archive) *domain.Archive {
	if a == nil {
		return nil
	}
	data, _ := json.Marshal(a)
	var out domain.Archive
	_ = json.Unmarshal(data, &out)
	initialize(&out)
	return &out
}

// eventWriter returns a writable handle to the active event log at s.path.
// If the cached handle is stale (the log was rotated out from under us and a
// new file created at the same path), eventWriter closes the old handle,
// replays the new log to re-anchor s.seq and s.prevHash, and opens a fresh
// handle so subsequent writes land on the current active log.
func (s *Store) eventWriter() (*os.File, error) {
	if s.eventLog != nil {
		if same, err := s.sameInode(s.eventLog); err != nil || same {
			if err == nil {
				return s.eventLog, nil
			}
		}
		// The cached handle no longer points at the active file. Close it and
		// re-establish the sequence/hash chain from the current log so that
		// the next event continues a valid chain within this log.
		_ = s.eventLog.Close()
		s.eventLog = nil
		if err := s.replay(); err != nil {
			return nil, err
		}
		s.rebuildIndexes()
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	s.eventLog = f
	return f, nil
}

// sameInode reports whether the cached writer handle still references the file
// currently located at s.path. When the log is rotated (renamed away and a new
// file created at the same path), the inodes differ and the handle is stale.
func (s *Store) sameInode(f *os.File) (bool, error) {
	cached, err := f.Stat()
	if err != nil {
		return false, err
	}
	current, err := os.Stat(s.path)
	if err != nil {
		// Path was removed entirely; treat as stale so the next OpenFile
		// recreates it via O_CREATE.
		return false, nil
	}
	return os.SameFile(cached, current), nil
}

func (s *Store) save(a *domain.Archive, typ string) error {
	if s.path != "" {
		f, err := s.eventWriter()
		if err != nil {
			return err
		}
		// Resolve seq/prevHash after eventWriter, since a detected rotation
		// re-anchors s.seq and s.prevHash from the current active log so the
		// appended event continues a valid chain within that log.
		entry := event{SchemaVersion: currentSchemaVersion, Seq: s.seq + 1, Type: typ, Archive: a, PrevHash: s.prevHash}
		entry.Hash = domain.Hash(entry)
		data, _ := json.Marshal(entry)
		_, err = f.Write(append(data, '\n'))
		if err == nil {
			err = f.Sync()
		}
		if err != nil {
			return err
		}
		s.seq, s.prevHash = entry.Seq, entry.Hash
		snapshot := s.path + ".snapshot"
		temporary := snapshot + ".tmp"
		projection, _ := json.Marshal(a)
		if err := os.WriteFile(temporary, projection, 0644); err != nil {
			return nil
		}
		if f, err := os.OpenFile(temporary, os.O_WRONLY, 0644); err == nil {
			_ = f.Sync()
			_ = f.Close()
		}
		if err := os.Rename(temporary, snapshot); err != nil {
			return nil
		}
		s.seq, s.prevHash = entry.Seq, entry.Hash
		return nil
	}
	entry := event{SchemaVersion: currentSchemaVersion, Seq: s.seq + 1, Type: typ, Archive: a, PrevHash: s.prevHash}
	entry.Hash = domain.Hash(entry)
	s.seq, s.prevHash = entry.Seq, entry.Hash
	return nil
}

func (s *Store) Create(a *domain.Archive, key string) (*domain.Archive, error) {
	digest := domain.ArchiveRequestDigest(a.ArchiveCode, a.CaveName, a.SurveyDate, a.CoordinateDatum)
	created, _, err := s.CreateChecked(a, key, digest)
	return created, err
}

func (s *Store) CreateChecked(a *domain.Archive, key, digest string) (*domain.Archive, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		if raw, ok := s.idem["create:"+key]; ok {
			id, _ := raw.(string)
			existing := s.archives[id]
			if existing != nil && existing.CreateRequestDigest == digest {
				return existing, true, nil
			}
			return nil, false, &domain.BusinessError{Cause: domain.ErrIdempotencyConflict, Code: "idempotency_conflict", Message: "同一idempotencyKey对应的建档参数不同", ExistingArchiveID: id}
		}
	}
	normalizedCode := domain.NormalizeArchiveCode(a.ArchiveCode)
	if id := s.codeIndex[normalizedCode]; id != "" {
		return nil, false, &domain.BusinessError{Cause: domain.ErrArchiveCodeConflict, Code: "archive_code_conflict", Message: "归档编号已被占用", ExistingArchiveID: id}
	}
	candidate := cloneArchive(a)
	candidate.CreateIdempotencyKey = key
	candidate.CreateRequestDigest = digest
	if err := s.save(candidate, "create"); err != nil {
		return nil, false, err
	}
	s.archives[candidate.ID] = candidate
	s.codeIndex[normalizedCode] = candidate.ID
	if key != "" {
		s.idem["create:"+key] = candidate.ID
	}
	return candidate, false, nil
}

func (s *Store) Get(id string) (*domain.Archive, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := s.archives[id]
	if a == nil {
		return nil, domain.ErrNotFound
	}
	return a, nil
}

func (s *Store) Transact(id, typ string, mutate func(*domain.Archive) error) (*domain.Archive, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.archives[id]
	if current == nil {
		return nil, domain.ErrNotFound
	}
	candidate := cloneArchive(current)
	if err := mutate(candidate); errors.Is(err, ErrNoChange) {
		return current, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.save(candidate, typ); err != nil {
		return nil, err
	}
	*current = *candidate
	return current, nil
}

func (s *Store) Update(a *domain.Archive) error {
	_, err := s.Transact(a.ID, "update", func(candidate *domain.Archive) error {
		*candidate = *cloneArchive(a)
		return nil
	})
	return err
}

func (s *Store) List() []*domain.Archive {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Archive, 0, len(s.archives))
	for _, archive := range s.archives {
		out = append(out, cloneArchive(archive))
	}
	return out
}

// ArchivesByCertificate returns detached projections for read-only verification.
func (s *Store) ArchivesByCertificate(ids []string) map[string]*domain.Archive {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	out := make(map[string]*domain.Archive, len(ids))
	for _, archive := range s.archives {
		if archive.Certificate == nil || !wanted[archive.Certificate.CertificateID] {
			continue
		}
		out[archive.Certificate.CertificateID] = cloneArchive(archive)
	}
	return out
}

func (s *Store) Idempotent(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.idem[key]
	return value, ok
}

func (s *Store) PutIdempotent(key string, value any) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idem[key] = value
}
