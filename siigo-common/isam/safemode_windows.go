//go:build windows

package isam

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

func lockFileExclusive(f *os.File) error {
	var overlapped syscall.Overlapped
	handle := syscall.Handle(f.Fd())
	ret, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("LockFileEx failed: %w", err)
	}
	return nil
}

func unlockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	handle := syscall.Handle(f.Fd())
	ret, _, err := procUnlockFileEx.Call(
		uintptr(handle),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if ret == 0 {
		return fmt.Errorf("UnlockFileEx failed: %w", err)
	}
	return nil
}
