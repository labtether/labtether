package installstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const currentSchemaVersion = 1

type Metadata struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type Secrets struct {
	OwnerToken       string `json:"owner_token,omitempty"`
	APIToken         string `json:"api_token,omitempty"` // #nosec G117 -- Persisted runtime secret, not a hardcoded credential.
	EncryptionKey    string `json:"encryption_key,omitempty"`
	PostgresPassword string `json:"postgres_password,omitempty"` // #nosec G117 -- Persisted runtime secret, not a hardcoded credential.
}

type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Store) statePath() string {
	return filepath.Join(s.root, "state.json")
}

func (s *Store) secretsPath() string {
	return filepath.Join(s.root, "secrets.json")
}

func (s *Store) Load() (Metadata, Secrets, bool, error) {
	var meta Metadata
	var secrets Secrets
	if s == nil || s.root == "" {
		return meta, secrets, false, errors.New("install state root is required")
	}

	stateExists := false
	if err := readJSONFile(s.statePath(), &meta); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Metadata{}, Secrets{}, false, fmt.Errorf("read install state metadata: %w", err)
		}
	} else {
		stateExists = true
	}

	secretsExists := false
	if err := readJSONFile(s.secretsPath(), &secrets); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Metadata{}, Secrets{}, false, fmt.Errorf("read install state secrets: %w", err)
		}
	} else {
		secretsExists = true
	}

	exists := stateExists || secretsExists
	return meta, secrets, exists, nil
}

func (s *Store) Save(meta Metadata, secrets Secrets) error {
	if s == nil || s.root == "" {
		return errors.New("install state root is required")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("mkdir install state root: %w", err)
	}

	meta.SchemaVersion = currentSchemaVersion
	if err := writeJSONFileAtomic(s.statePath(), meta, 0o600); err != nil {
		return fmt.Errorf("write install state metadata: %w", err)
	}
	if err := writeJSONFileAtomic(s.secretsPath(), secrets, 0o600); err != nil {
		return fmt.Errorf("write install state secrets: %w", err)
	}
	return nil
}

func readJSONFile(path string, dest any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- Path is derived from the install-state root managed by this package.
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSONFileAtomic(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpName); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("installstate: remove temp file %s: %v", tmpName, err)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			return fmt.Errorf("write temp file: %w (close temp file: %v)", err, closeErr)
		}
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Chmod(perm); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			return fmt.Errorf("chmod temp file: %w (close temp file: %v)", err, closeErr)
		}
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil { // #nosec G703 -- Temp and destination paths are both derived from the package-managed install-state root.
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
