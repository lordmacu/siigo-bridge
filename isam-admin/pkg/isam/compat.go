package isam

import "time"

// ---------------------------------------------------------------------------
// compat.go — Compatibility stubs for isam-admin
//
// Provides constants and functions that were in extfh.go (DLL-dependent)
// but are needed by reader.go and other files.
// Since isam-admin is a standalone tool, we always use the V2 binary reader.
// ---------------------------------------------------------------------------

// MaxLockRetries is the number of times to retry when a file is locked
var MaxLockRetries = 3

// LockRetryDelay is the delay between lock retries
var LockRetryDelay = 200 * time.Millisecond

// ExtfhAvailable returns false — isam-admin always uses the pure Go V2 reader
func ExtfhAvailable() bool {
	return false
}

// ReadIsamFile reads an ISAM file using the V2 binary reader.
// This is the main entry point for reading any ISAM file.
func ReadIsamFile(path string) (*FileInfo, *V2Header, error) {
	return ReadFileV2(path)
}
