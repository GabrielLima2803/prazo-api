package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

var validKey = regexp.MustCompile(`^[a-z0-9_]+$`)

type FileKV struct {
	dir    string
	sealer *Sealer
}

func NewFileKV(dir string, sealer *Sealer) (*FileKV, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("criar diretório de dados: %w", err)
	}
	return &FileKV{dir: dir, sealer: sealer}, nil
}

func (s *FileKV) Get(_ context.Context, key string) ([]byte, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	ct, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	pt, err := s.sealer.Open(ct, []byte(key))
	if err != nil {
		return nil, fmt.Errorf("%s: %w (PRAZO_MASTER_KEY mudou?)", key, err)
	}
	return pt, nil
}

// Put grava num arquivo temporário e renomeia, para que uma queda no meio
// nunca deixe um segredo pela metade.
func (s *FileKV) Put(_ context.Context, key string, value []byte) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "."+key+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(s.sealer.Seal(value, []byte(key))); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (s *FileKV) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *FileKV) path(key string) (string, error) {
	if !validKey.MatchString(key) {
		return "", fmt.Errorf("chave inválida %q", key)
	}
	return filepath.Join(s.dir, key+".enc"), nil
}
