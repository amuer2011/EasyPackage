package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type folderShortcut struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}
type placesStore struct {
	mu     sync.Mutex
	file   string
	recent []string
}

func newPlacesStore(file string) *placesStore {
	s := &placesStore{file: file, recent: []string{}}
	if b, err := os.ReadFile(file); err == nil {
		var saved struct {
			Recent []string `json:"recent"`
		}
		if json.Unmarshal(b, &saved) == nil {
			for _, p := range saved.Recent {
				if len(s.recent) == 5 {
					break
				}
				if filepath.IsAbs(p) && !s.contains(p) {
					s.recent = append(s.recent, p)
				}
			}
		}
	}
	return s
}
func (s *placesStore) contains(path string) bool {
	for _, p := range s.recent {
		if sameFolderPath(p, path) {
			return true
		}
	}
	return false
}
func sameFolderPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
func (s *placesStore) Recent() []string {
	if s == nil {
		return []string{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []string{}
	for _, p := range s.recent {
		if st, e := os.Stat(p); e == nil && st.IsDir() {
			result = append(result, p)
		}
	}
	return result
}
func (s *placesStore) Remember(paths ...string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		p, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		p, err = filepath.EvalSymlinks(p)
		if err != nil {
			return err
		}
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("最近使用项必须是文件夹")
		}
		next := []string{p}
		for _, old := range s.recent {
			if !sameFolderPath(old, p) && len(next) < 5 {
				next = append(next, old)
			}
		}
		s.recent = next
	}
	if s.file == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Recent []string `json:"recent"`
	}{s.recent}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.file), ".recent-*.tmp")
	if err != nil {
		return err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.file)
}
func commonFolders() []folderShortcut {
	home, _ := os.UserHomeDir()
	result := []folderShortcut{{"用户目录", home}}
	for _, folder := range platformFolders(home) {
		if st, err := os.Stat(folder.Path); err == nil && st.IsDir() {
			result = append(result, folder)
		}
	}
	return result
}
