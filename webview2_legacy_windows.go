//go:build windows && !amd64

package main

// Other Windows architectures retain the legacy desktop window until a
// matching WebView2Loader redistributable is provided for them.
func runWindowsWebViewUI() { runLegacyWindowsUI() }
