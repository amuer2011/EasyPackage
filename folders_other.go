//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func platformFolders(home string) []folderShortcut {
	paths := map[string]string{"DESKTOP": filepath.Join(home, "Desktop"), "DOCUMENTS": filepath.Join(home, "Documents"), "DOWNLOAD": filepath.Join(home, "Downloads")}
	if runtime.GOOS == "linux" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		if data, err := os.ReadFile(filepath.Join(config, "user-dirs.dirs")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				if !ok {
					continue
				}
				key = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(key), "XDG_"), "_DIR")
				if _, ok = paths[key]; !ok {
					continue
				}
				value, err = strconv.Unquote(strings.TrimSpace(value))
				if err != nil {
					continue
				}
				value = strings.ReplaceAll(value, "${HOME}", home)
				value = strings.ReplaceAll(value, "$HOME", home)
				if filepath.IsAbs(value) {
					paths[key] = value
				}
			}
		}
	}
	return []folderShortcut{{"桌面", paths["DESKTOP"]}, {"文档", paths["DOCUMENTS"]}, {"下载", paths["DOWNLOAD"]}}
}
