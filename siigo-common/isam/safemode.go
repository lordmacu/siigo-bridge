package isam

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// safemode.go — Write protection for ISAM files
//
// Prevents corruption by checking two conditions before any write:
//   1. Siigo process is not running (cblrtsm.exe / siaborto.exe)
//   2. The ISAM file is not locked by another process
//
// Safe mode is enabled by default on all tables. Disable with:
//   table.SafeMode = false
//
// ---------------------------------------------------------------------------

// SafeMode controls write protection. When true (default), all write
// operations check that Siigo is not running and the file is not locked.
// Set to false on Table to disable for that specific table.

// siigoProcesses are the executables that indicate Siigo is actively using files
var siigoProcesses = []string{"cblrtsm.exe", "siaborto.exe", "siaborto32.exe", "siigo.exe"}

// cachedSiigoCheck avoids calling tasklist on every write
var (
	siigoCheckMu     sync.Mutex
	siigoCheckResult bool
	siigoCheckTime   time.Time
	siigoCheckTTL    = 2 * time.Second // cache result for 2s
)

// ErrSiigoRunning is returned when a write is attempted while Siigo is open
var ErrSiigoRunning = fmt.Errorf("siigo is running — write operations blocked to prevent corruption")

// ErrFileLocked is returned when the ISAM file is locked by another process
var ErrFileLocked = fmt.Errorf("ISAM file is locked by another process")

// SiigoIsRunning checks if any Siigo process is active.
// Results are cached for 2 seconds to avoid expensive tasklist calls.
func SiigoIsRunning() bool {
	siigoCheckMu.Lock()
	defer siigoCheckMu.Unlock()

	if time.Since(siigoCheckTime) < siigoCheckTTL {
		return siigoCheckResult
	}

	siigoCheckResult = checkSiigoProcesses()
	siigoCheckTime = time.Now()
	return siigoCheckResult
}

func checkSiigoProcesses() bool {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false // can't check — assume safe
	}
	lower := strings.ToLower(string(out))
	for _, proc := range siigoProcesses {
		if strings.Contains(lower, proc) {
			return true
		}
	}
	return false
}

// TryLockFile attempts to get an exclusive lock on an ISAM file.
// Returns the locked file handle (caller must close it after writing)
// or an error if the file is already locked.
func TryLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}

	err = lockFileExclusive(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: %s", ErrFileLocked, path)
	}
	return f, nil
}

// UnlockFile releases the lock and closes the file handle.
func UnlockFile(f *os.File) error {
	if f == nil {
		return nil
	}
	unlockFile(f)
	return f.Close()
}

// CheckWriteSafe verifies it's safe to write to a specific ISAM file.
// Checks: (1) Siigo not running, (2) file not locked by another process.
func CheckWriteSafe(path string) error {
	// Check 1: Siigo process
	if SiigoIsRunning() {
		return ErrSiigoRunning
	}

	// Check 2: File lock test (try lock, immediately release)
	f, err := TryLockFile(path)
	if err != nil {
		return err
	}
	UnlockFile(f)
	return nil
}

// lock constants used by safemode_windows.go
const (
	lockfileExclusiveLock   = 0x02
	lockfileFailImmediately = 0x01
)
