//go:build !windows

package fsjail

import "os"

// SetSparse is a no-op on Unix, where ftruncate creates sparse files by default.
func SetSparse(f *os.File) {}
