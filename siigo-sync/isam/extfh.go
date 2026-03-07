package isam

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/text/encoding/charmap"
)

// EXTFH opcodes
const (
	OpOpenInput  = 0xFA00
	OpClose      = 0xFA80
	OpReadSeq    = 0xFAF5 // READ NEXT
	OpReadRan    = 0xFAF6 // READ by key
	OpStepFirst  = 0xFACC // STEP FIRST
	OpStepNext   = 0xFACA // STEP NEXT
)

// File organizations
const (
	OrgIndexed = 2
)

// Access modes
const (
	AccessSeq     = 0
	AccessRandom  = 4
	AccessDynamic = 8
)

// KDB_KEY is a key definition (16 bytes)
type KDB_KEY struct {
	Count    [2]byte
	Offset   [2]byte
	KeyFlags byte
	CompFlags byte
	Sparse   byte
	Reserved [9]byte
}

// KDB is the Key Definition Block
type KDB struct {
	KdbLen  [2]byte
	Filler  [4]byte
	Nkeys   [2]byte
	Filler2 [6]byte
	Keys    [1]KDB_KEY // at least 1 key
}

// FCD3 is the File Control Description version 3 (64-bit)
// This struct must be binary-compatible with Micro Focus's definition
type FCD3 struct {
	FileStatus   [2]byte  // I/O completion status
	FcdLen       [2]byte  // length of FCD
	FcdVer       byte     // FCD version (1 = 64-bit)
	FileOrg      byte     // file organization
	AccessFlags  byte     // access flags
	OpenMode     byte     // open mode
	RecordMode   byte     // recording mode
	FileFormat   byte     // file format
	DeviceFlag   byte
	LockAction   byte
	CompType     byte
	Blocking     byte
	IdxCacheSz   byte
	Percent      byte
	BlockSize    byte
	Flags1       byte
	Flags2       byte
	MvsFlags     byte
	FstatusType  byte
	OtherFlags   byte
	TransLog     byte
	LockTypes    byte
	FsFlags      byte
	ConfFlags    byte
	MiscFlags    byte
	ConfFlags2   byte
	LockMode     byte
	Fsv2Flags    byte
	IdxCacheArea byte
	FcdInternal1 byte
	FcdInternal2 byte
	Res3         [14]byte
	GcFlags      byte
	NlsId        [2]byte
	Fsv2FileId   [2]byte
	RetryOpenCnt [2]byte
	FnameLen     [2]byte  // file name length
	IdxNameLen   [2]byte
	RetryCnt     [2]byte
	RefKey       [2]byte  // key of reference
	LineCount    [2]byte
	UseFiles     byte
	GiveFiles    byte
	EffKeyLen    [2]byte  // effective key length
	Res5         [14]byte
	Eop          [2]byte
	Opt          [4]byte
	CurRecLen    [4]byte  // current record length
	MinRecLen    [4]byte  // min record length
	MaxRecLen    [4]byte  // max record length
	Fsv2SessId   [4]byte
	Res6         [24]byte
	RelByteAdrs  [8]byte
	MaxRelKey    [8]byte
	RelKey       [8]byte
	FileHandle   [8]byte  // pointer to file handle (8 bytes for 64-bit)
	RecPtr       [8]byte  // pointer to record area
	FnamePtr     [8]byte  // pointer to file name
	IdxNamePtr   [8]byte  // pointer to index name
	KdbPtr       [8]byte  // pointer to KDB
	ColPtr       [8]byte  // pointer to collating sequence
	FileDefPtr   [8]byte  // pointer to filedef
	DfSortPtr    [8]byte  // pointer to DFSORT
}

var (
	cblrtsDLL  *syscall.LazyDLL
	extfhProc  *syscall.LazyProc
	dllLoaded  bool
)

func loadDLL() error {
	if dllLoaded {
		return nil
	}
	// Try to find the 64-bit DLL
	paths := []string{
		`C:\Microfocus\bin64\cblrtsm.dll`,
		`C:\Microfocus\bin\cblrtsm.dll`,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			cblrtsDLL = syscall.NewLazyDLL(p)
			extfhProc = cblrtsDLL.NewProc("EXTFH")
			if err := extfhProc.Find(); err != nil {
				continue
			}
			dllLoaded = true
			return nil
		}
	}
	return fmt.Errorf("cblrtsm.dll not found")
}

func callEXTFH(opcode uint16, fcd *FCD3) error {
	op := [2]byte{byte(opcode >> 8), byte(opcode & 0xFF)}
	ret, _, _ := extfhProc.Call(
		uintptr(unsafe.Pointer(&op[0])),
		uintptr(unsafe.Pointer(fcd)),
	)
	if ret != 0 {
		return fmt.Errorf("EXTFH returned %d, status=%c%c", ret, fcd.FileStatus[0], fcd.FileStatus[1])
	}
	// Check file status
	if fcd.FileStatus[0] != '0' || fcd.FileStatus[1] != '0' {
		return fmt.Errorf("file status: %c%c (0x%02x 0x%02x)", fcd.FileStatus[0], fcd.FileStatus[1], fcd.FileStatus[0], fcd.FileStatus[1])
	}
	return nil
}

func setPointer(field *[8]byte, ptr unsafe.Pointer) {
	*(*uintptr)(unsafe.Pointer(field)) = uintptr(ptr)
}

// ExtfhRecord represents a record read via EXTFH
type ExtfhRecord struct {
	Data []byte
}

// ReadFileExtfh reads an ISAM file using the Micro Focus EXTFH DLL
func ReadFileExtfh(path string) ([]ExtfhRecord, error) {
	if err := loadDLL(); err != nil {
		return nil, err
	}

	// Prepare file name (null-terminated)
	fname := append([]byte(path), 0)

	// Allocate record buffer (max 32KB)
	recBuf := make([]byte, 32768)

	// Prepare KDB with 1 key
	kdb := &KDB{}
	binary.BigEndian.PutUint16(kdb.KdbLen[:], uint16(unsafe.Sizeof(*kdb)))
	binary.BigEndian.PutUint16(kdb.Nkeys[:], 1)
	kdb.Keys[0].KeyFlags = 0x10 // KEY_PRIMARY

	// Initialize FCD3 - all zeros first
	var fcd FCD3
	fcd.FcdVer = 1 // FCD_VER_64Bit
	fcd.FileOrg = OrgIndexed
	fcd.AccessFlags = AccessDynamic
	fcd.OpenMode = 128 // OPEN_NOT_OPEN
	fcd.RecordMode = 0 // REC_MODE_FIXED
	fcd.FileFormat = 0 // MF_FF_DEFAULT - let EXTFH detect the format
	fcd.FstatusType = 0x80 // MF_FST_COBOL85 - use COBOL 85 status codes
	fcd.OtherFlags = 0x80 // OTH_OPTIONAL
	fcd.ConfFlags2 = 0x08 // MF_CF2_IGN_MIN_LEN

	binary.BigEndian.PutUint16(fcd.FcdLen[:], uint16(unsafe.Sizeof(fcd)))
	binary.BigEndian.PutUint16(fcd.FnameLen[:], uint16(len(fname)-1))
	binary.BigEndian.PutUint32(fcd.MaxRecLen[:], 32768)
	binary.BigEndian.PutUint32(fcd.MinRecLen[:], 0)

	// Set pointers
	setPointer(&fcd.RecPtr, unsafe.Pointer(&recBuf[0]))
	setPointer(&fcd.FnamePtr, unsafe.Pointer(&fname[0]))
	setPointer(&fcd.KdbPtr, unsafe.Pointer(kdb))

	// GETINFO first to discover file attributes
	err := callEXTFH(0x0006, &fcd) // OP_GETINFO
	if err != nil {
		// Reset status for open
		fcd.FileStatus[0] = '0'
		fcd.FileStatus[1] = '0'
	}

	// OPEN INPUT
	err = callEXTFH(OpOpenInput, &fcd)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	// Get actual record size from FCD after open
	recSize := int(binary.BigEndian.Uint32(fcd.MaxRecLen[:]))
	if recSize <= 0 || recSize > 32768 {
		recSize = int(binary.BigEndian.Uint32(fcd.CurRecLen[:]))
	}

	var records []ExtfhRecord

	// READ first record using STEP FIRST
	err = callEXTFH(OpStepFirst, &fcd)
	if err != nil {
		// If step first fails, try READ SEQ
		err = callEXTFH(OpReadSeq, &fcd)
	}

	for err == nil {
		curLen := int(binary.BigEndian.Uint32(fcd.CurRecLen[:]))
		if curLen <= 0 {
			curLen = recSize
		}
		rec := make([]byte, curLen)
		copy(rec, recBuf[:curLen])
		records = append(records, ExtfhRecord{Data: rec})

		// READ NEXT
		err = callEXTFH(OpStepNext, &fcd)
	}

	// Close file (ignore error on close)
	_ = callEXTFH(OpClose, &fcd)

	// Status "10" means end of file - that's OK
	if fcd.FileStatus[0] == '1' && fcd.FileStatus[1] == '0' {
		err = nil
	}

	return records, nil
}

// DecodeExtfhField extracts and decodes a field from an EXTFH record
func DecodeExtfhField(rec []byte, offset, length int) string {
	if offset >= len(rec) {
		return ""
	}
	end := offset + length
	if end > len(rec) {
		end = len(rec)
	}
	field := rec[offset:end]

	trimEnd := len(field)
	for trimEnd > 0 && (field[trimEnd-1] == ' ' || field[trimEnd-1] == 0) {
		trimEnd--
	}

	decoder := charmap.Windows1252.NewDecoder()
	result, err := decoder.Bytes(field[:trimEnd])
	if err != nil {
		return string(field[:trimEnd])
	}
	return string(result)
}
