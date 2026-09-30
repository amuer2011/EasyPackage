package main

import (
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

type folderGUID struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

// The Windows Known Folder API follows OneDrive redirection and relocated folders.
func platformFolders(home string) []folderShortcut {
	known := syscall.NewLazyDLL("shell32.dll").NewProc("SHGetKnownFolderPath")
	free := syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
	entries := []struct {
		label, fallback string
		id              folderGUID
	}{
		{"桌面", "Desktop", folderGUID{0xB4BFCC3A, 0xDB2C, 0x424C, [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}},
		{"文档", "Documents", folderGUID{0xFDD39AD0, 0x238F, 0x46AF, [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7}}},
		{"下载", "Downloads", folderGUID{0x374DE290, 0x123F, 0x4565, [8]byte{0x91, 0x64, 0x39, 0xC4, 0x92, 0x5E, 0x46, 0x7B}}},
	}
	result := []folderShortcut{}
	for _, e := range entries {
		path := filepath.Join(home, e.fallback)
		var ptr *uint16
		hr, _, _ := known.Call(uintptr(unsafe.Pointer(&e.id)), 0, 0, uintptr(unsafe.Pointer(&ptr)))
		if hr == 0 && ptr != nil {
			units := []uint16{}
			for i := 0; i < 32768; i++ {
				value := *(*uint16)(unsafe.Add(unsafe.Pointer(ptr), i*2))
				if value == 0 {
					break
				}
				units = append(units, value)
			}
			if len(units) > 0 {
				path = string(utf16.Decode(units))
			}
			free.Call(uintptr(unsafe.Pointer(ptr)))
		}
		result = append(result, folderShortcut{e.label, path})
	}
	return result
}
