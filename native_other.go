//go:build !windows

package main

// runNativeWindowsUI exists only because the shared entry point retains a
// Windows dispatch case; the function is never called on other platforms.
func runNativeWindowsUI() {}

// Windows launches the WebView2 front end; only the Windows build defines it.
func runWindowsWebViewUI() {}
