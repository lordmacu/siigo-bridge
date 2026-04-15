//go:build !windows

package isam

import "fmt"

func initDLL() {
	dllErr = fmt.Errorf("EXTFH DLL not available on this platform (Windows only)")
}

func callEXTFH(_ uint16, fcd *FCD3) FileStatus {
	return FileStatus{'9', '9'}
}
