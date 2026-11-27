//go:build !windows

package main

import "os"

func warmupPrivateMode(mode os.FileMode, dir bool) bool {
	return mode.Perm()&0077 == 0 && (!dir || mode.Perm()&0700 == 0700)
}

func replaceWarmupLedger(root *os.Root, from, to string) error {
	if err := root.Rename(from, to); err != nil {
		return err
	}
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
