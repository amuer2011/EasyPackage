//go:build cgo && (darwin || linux)

package main

/*
 */
import "C"

//export easyPackageGoReady
func easyPackageGoReady() { desktopReady() }

//export easyPackageGoAction
func easyPackageGoAction(id C.int) { desktopAction(int(id)) }

//export easyPackageGoRefresh
func easyPackageGoRefresh() { desktopRefresh() }
