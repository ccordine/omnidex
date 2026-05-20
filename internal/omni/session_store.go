package omni

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SessionStore struct {
	RootDir string
}

func NewSessionStore(rootDir string) SessionStore {
	if strings.TrimSpace(rootDir) != "" {
		return SessionStore{RootDir: rootDir}
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return SessionStore{RootDir: ".omni/sessions"}
	}

	return SessionStore{RootDir: filepath.Join(home, ".omni", "sessions")}
}

func (s SessionStore) LoadOrCreate(workspacePath string) (*Session, bool, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, false, errors.New("workspace path is required")
	}

	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, false, fmt.Errorf("resolve workspace: %w", err)
	}

	if err := os.MkdirAll(s.RootDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("create session root: %w", err)
	}

	hash := workspaceHash(absWorkspace)
	sessionPath := filepath.Join(s.RootDir, hash+".json")
	now := nowUTC()

	blob, err := os.ReadFile(sessionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			session := &Session{
				Version:             sessionVersion,
				WorkspacePath:       absWorkspace,
				WorkspaceHash:       hash,
				ActiveDirectoryPath: absWorkspace,
				Permission:          PermissionAsk,
				CreatedAt:           now,
				UpdatedAt:           now,
				Messages:            []Message{},
				Memories:            []SessionMemory{},
				Turns:               []Turn{},
			}
			if err := s.Save(session); err != nil {
				return nil, false, err
			}
			return session, false, nil
		}
		return nil, false, fmt.Errorf("read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(blob, &session); err != nil {
		return nil, false, fmt.Errorf("decode session file: %w", err)
	}

	if session.Version == "" {
		session.Version = sessionVersion
	}
	if session.WorkspacePath == "" {
		session.WorkspacePath = absWorkspace
	}
	if session.WorkspaceHash == "" {
		session.WorkspaceHash = hash
	}
	if strings.TrimSpace(session.ActiveDirectoryPath) == "" {
		session.ActiveDirectoryPath = session.WorkspacePath
	}
	if session.Permission != PermissionAsk && session.Permission != PermissionFull {
		session.Permission = PermissionAsk
	}
	if session.CreatedAt == "" {
		session.CreatedAt = now
	}
	if session.Memories == nil {
		session.Memories = []SessionMemory{}
	}
	session.UpdatedAt = now

	return &session, true, nil
}

func (s SessionStore) Save(session *Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if strings.TrimSpace(session.WorkspacePath) == "" {
		return errors.New("session workspace path is required")
	}
	if strings.TrimSpace(session.ActiveDirectoryPath) == "" {
		session.ActiveDirectoryPath = session.WorkspacePath
	}

	if session.WorkspaceHash == "" {
		session.WorkspaceHash = workspaceHash(session.WorkspacePath)
	}

	if session.Permission != PermissionAsk && session.Permission != PermissionFull {
		return fmt.Errorf("session contains invalid permission mode %q", session.Permission)
	}

	session.Version = sessionVersion
	session.UpdatedAt = nowUTC()

	if err := os.MkdirAll(s.RootDir, 0o755); err != nil {
		return fmt.Errorf("create session root: %w", err)
	}

	path := filepath.Join(s.RootDir, session.WorkspaceHash+".json")
	tempPath := path + ".tmp"

	blob, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}

	if err := os.WriteFile(tempPath, blob, 0o644); err != nil {
		return fmt.Errorf("write temp session: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace session file: %w", err)
	}

	return nil
}

func workspaceHash(workspacePath string) string {
	normalized := filepath.Clean(strings.TrimSpace(workspacePath))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:20]
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
