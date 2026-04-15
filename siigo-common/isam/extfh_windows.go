//go:build windows

package isam

import (
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Windows-only: DLL handle and EXTFH procedure
var (
	extfhDLL  *syscall.LazyDLL
	extfhProc *syscall.LazyProc
)

func initDLL() {
	var paths []string

	if cobdir := os.Getenv("COBDIR"); cobdir != "" {
		paths = append(paths,
			cobdir+`\bin64\cblrtsm.dll`,
			cobdir+`\bin\cblrtsm.dll`,
		)
	}

	paths = append(paths, `C:\Siigo\cblrtsm.dll`)
	paths = append(paths,
		`C:\Microfocus\bin64\cblrtsm.dll`,
		`C:\Microfocus\bin\cblrtsm.dll`,
	)

	for _, pf := range []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	} {
		if pf == "" {
			continue
		}
		paths = append(paths,
			pf+`\Micro Focus\Visual COBOL\bin64\cblrtsm.dll`,
			pf+`\Micro Focus\Visual COBOL\bin\cblrtsm.dll`,
			pf+`\Micro Focus\COBOL Server\bin64\cblrtsm.dll`,
			pf+`\Micro Focus\COBOL Server\bin\cblrtsm.dll`,
		)
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		dllDir := p[:strings.LastIndex(p, `\`)]
		setupEnvironment(dllDir)

		extfhDLL = syscall.NewLazyDLL(p)
		extfhProc = extfhDLL.NewProc("EXTFH")
		if err := extfhProc.Find(); err != nil {
			continue
		}
		dllAvail = true
		dllPath = p
		return
	}
	dllErr = fmt.Errorf("cblrtsm.dll not found (checked COBDIR, Siigo, Microfocus, Program Files)")
}

func callEXTFH(opcode uint16, fcd *FCD3) FileStatus {
	op := [2]byte{byte(opcode >> 8), byte(opcode & 0xFF)}
	extfhProc.Call(
		uintptr(unsafe.Pointer(&op[0])),
		uintptr(unsafe.Pointer(fcd)),
	)
	st := FileStatus{fcd.FileStatus[0], fcd.FileStatus[1]}
	if ExtfhDebug {
		log.Printf("[EXTFH] %s -> %s", opcodeName(opcode), st.Error())
	}
	return st
}
