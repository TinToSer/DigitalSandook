//go:build windows

package fsjail

import (
	"os"
	"syscall"
)

const fsctlSetSparse = 0x000900C4

// SetSparse marks f as a sparse file on Windows NTFS so Truncate(size) does
// not trigger synchronous zero-filling when out-of-order upload chunks arrive.
func SetSparse(f *os.File) {
	var returned uint32
	_ = syscall.DeviceIoControl(syscall.Handle(f.Fd()), fsctlSetSparse, nil, 0, nil, 0, &returned, nil)
}
