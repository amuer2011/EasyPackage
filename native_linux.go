//go:build linux && cgo

package main

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
#include <gtk/gtk.h>
void ep_run(void);
void ep_set_text(int id, const char *text);
const char *ep_get_text(int id);
int ep_checked(int id);
void ep_set_secret(int id, int secret);
void ep_set_enabled(int id, int enabled);
void ep_set_status(const char *text, int percent, int marquee);
void ep_set_busy(int busy);
void ep_set_page(int page);
void ep_set_options(int id, char **items, int count, int active);
int ep_selected(int id);
const char *ep_get_selected_text(int id);
int ep_choose(int kind, const char *current, char **selected);
int ep_message(const char *title, const char *message, int confirm);
void ep_quit(void);
*/
import "C"

import (
	"os/exec"
	"unsafe"
)

func platformRun() { C.ep_run() }
func platformSetText(id int, value string) {
	text := C.CString(value)
	C.ep_set_text(C.int(id), text)
	C.free(unsafe.Pointer(text))
}
func platformGetText(id int) string { return C.GoString(C.ep_get_text(C.int(id))) }
func platformChecked(id int) bool   { return C.ep_checked(C.int(id)) != 0 }
func platformSetSecret(id int, secret bool) {
	C.ep_set_secret(C.int(id), boolC(secret))
}
func platformSetEnabled(id int, enabled bool) {
	C.ep_set_enabled(C.int(id), boolC(enabled))
}
func platformSetStatus(text string, percent int, marquee bool) {
	value := C.CString(text)
	C.ep_set_status(value, C.int(percent), boolC(marquee))
	C.free(unsafe.Pointer(value))
}
func platformSetBusy(busy bool) { C.ep_set_busy(boolC(busy)) }
func platformSetPage(page int)  { C.ep_set_page(C.int(page)) }
func platformSetOptions(id int, items []string, active int) {
	values := C.malloc(C.size_t(len(items)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	defer C.free(values)
	array := unsafe.Slice((**C.char)(values), len(items))
	for i, item := range items {
		array[i] = C.CString(item)
		defer C.free(unsafe.Pointer(array[i]))
	}
	C.ep_set_options(C.int(id), (**C.char)(values), C.int(len(items)), C.int(active))
}
func platformGetSelected(id int) string {
	if id == dIDUnit {
		value := C.ep_get_selected_text(C.int(id))
		if value == nil {
			return ""
		}
		defer C.free(unsafe.Pointer(value))
		return C.GoString(value)
	}
	index := int(C.ep_selected(C.int(id)))
	if index < 0 || index >= len(activeDesktop.quickFolders) {
		return ""
	}
	return activeDesktop.quickFolders[index].Path
}
func platformChoose(kind int, current string) string {
	value := C.CString(current)
	var selected *C.char
	result := C.ep_choose(C.int(kind), value, &selected)
	C.free(unsafe.Pointer(value))
	if result == 0 || selected == nil {
		return ""
	}
	path := C.GoString(selected)
	C.free(unsafe.Pointer(selected))
	return path
}
func platformMessage(title, message string, confirm bool) bool {
	cTitle, cMessage := C.CString(title), C.CString(message)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cMessage))
	return C.ep_message(cTitle, cMessage, boolC(confirm)) != 0
}
func platformQuit() { C.ep_quit() }
func platformOpenPath(path string) {
	_ = exec.Command("xdg-open", path).Start()
}
func platformSetFolderOptions(id int, paths []string, active int) {
	labels := make([]string, len(paths))
	for i, path := range paths {
		labels[i] = path
	}
	platformSetOptions(id, labels, active)
}
func boolC(value bool) C.int {
	if value {
		return 1
	}
	return 0
}
