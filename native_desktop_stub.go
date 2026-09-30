//go:build (darwin || linux) && !cgo

package main

import "fmt"

func platformRun() {
	fmt.Println("EasyPackage 的原生界面构建需要 CGO；请使用对应系统的构建脚本。")
}
func platformSetText(int, string)               {}
func platformGetText(int) string                { return "" }
func platformChecked(int) bool                  { return false }
func platformSetSecret(int, bool)               {}
func platformSetEnabled(int, bool)              {}
func platformSetStatus(string, int, bool)       {}
func platformSetBusy(bool)                      {}
func platformSetPage(int)                       {}
func platformSetOptions(int, []string, int)     {}
func platformGetSelected(int) string            { return "" }
func platformChoose(int, string) string         { return "" }
func platformMessage(string, string, bool) bool { return false }
func platformQuit()                             {}
func platformOpenPath(string)                   {}
