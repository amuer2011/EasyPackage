package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRecentFiveFoldersPersistAndDeduplicate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config", "preferences.json")
	s := newPlacesStore(file)
	folders := []string{}
	for i := 0; i < 7; i++ {
		p := filepath.Join(dir, fmt.Sprintf("folder-%d", i))
		if e := os.Mkdir(p, 0755); e != nil {
			t.Fatal(e)
		}
		folders = append(folders, p)
		if e := s.Remember(p); e != nil {
			t.Fatal(e)
		}
	}
	if got := s.Recent(); len(got) != 5 || got[0] != folders[6] || got[4] != folders[2] {
		t.Fatalf("wrong order: %v", got)
	}
	if e := s.Remember(folders[3]); e != nil {
		t.Fatal(e)
	}
	loaded := newPlacesStore(file)
	got := loaded.Recent()
	if len(got) != 5 || got[0] != folders[3] || got[1] != folders[6] {
		t.Fatalf("persistence/reorder failed: %v", got)
	}
	os.Remove(folders[3])
	got = loaded.Recent()
	if len(got) != 4 {
		t.Fatal("missing folders should not be displayed")
	}
	invalid := filepath.Join(dir, "file.txt")
	os.WriteFile(invalid, []byte("x"), 0600)
	if e := loaded.Remember(invalid); e == nil {
		t.Fatal("file stored as directory")
	}
}
func TestCommonFolders(t *testing.T) {
	home, _ := os.UserHomeDir()
	folders := commonFolders()
	if len(folders) == 0 || folders[0].Path != home {
		t.Fatal("missing home shortcut")
	}
	for _, folder := range folders {
		if !filepath.IsAbs(folder.Path) {
			t.Fatal("relative known folder")
		}
	}
}
