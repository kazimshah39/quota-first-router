//go:build windows

package main

import "os"

// Windows warmup stays unavailable until private ACL and durable replacement
// support have been verified there. The default-off scheduler still works.
func warmupPrivateMode(os.FileMode, bool) bool           { return false }
func replaceWarmupLedger(*os.Root, string, string) error { return errWarmupLedger }
