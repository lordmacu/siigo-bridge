package isam

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/text/encoding/charmap"
)

// ---------------------------------------------------------------------------
// EXTFH opcodes (Micro Focus Callable File Handler)
// Reference: xfhfcd3.cpy, Micro Focus Visual COBOL documentation
// ---------------------------------------------------------------------------
const (
	// Special opcodes (0x00xx)
	OpGetInfo      = 0x0006 // Query file attributes without opening
	OpFlush        = 0x000C // Flush file buffers
	OpUnlockRecord = 0x000F // Unlock a specific record

	// Standard opcodes (0xFAxx)
	OpOpenInput  = 0xFA00 // OPEN INPUT (read-only)
	OpOpenOutput = 0xFA01 // OPEN OUTPUT (write, creates file)
	OpOpenIO     = 0xFA02 // OPEN I-O (read-write)
	OpOpenExtend = 0xFA03 // OPEN EXTEND (append)
	OpClose      = 0xFA80 // CLOSE file
	OpCloseLock  = 0xFA84 // CLOSE WITH LOCK
	OpUnlock     = 0xFA0F // UNLOCK records

	// Sequential operations
	OpReadSeq  = 0xFAF5 // READ NEXT (sequential)
	OpReadPrev = 0xFAF8 // READ PREVIOUS

	// Random operations
	OpReadRan = 0xFAF6 // READ by key (random)
	OpWrite   = 0xFAF3 // WRITE record
	OpRewrite = 0xFAF4 // REWRITE record
	OpDelete  = 0xFAF7 // DELETE record

	// Positioning (STEP = low-level cursor movement)
	OpStepFirst = 0xFACC // Position to first record
	OpStepNext  = 0xFACA // Step to next record
	OpStepPrev  = 0xFACB // Step to previous record

	// START operations (position cursor for subsequent READ)
	OpStartEQ = 0xFAE9 // START = (equal)
	OpStartGE = 0xFAEB // START >= (greater or equal)
	OpStartGT = 0xFAE5 // START > (greater than)
	OpStartLE = 0xFAD9 // START <= (less or equal)
	OpStartLT = 0xFADA // START < (less than)

	// Transaction control
	OpCommit   = 0xFADC // COMMIT transaction
	OpRollback = 0xFADD // ROLLBACK transaction
)

// File organizations
const (
	OrgLineSeq  = 0
	OrgSeq      = 1
	OrgIndexed  = 2
	OrgRelative = 3
)

// Access modes
const (
	AccessSeq     = 0
	AccessRandom  = 4
	AccessDynamic = 8
)

// File format constants (IDXFORMAT values)
const (
	FormatDefault = 0  // Auto-detect
	FormatCISAM   = 1  // C-ISAM
	FormatLIIV1   = 2  // Level II v1
	FormatCOBOL   = 3  // IDXFORMAT "3"
	FormatIDX4    = 4  // IDXFORMAT "4"
	FormatIDX8    = 8  // IDXFORMAT "8" (Siigo uses this)
	FormatDISAM   = 16 // D-ISAM
	FormatVBISAM  = 17 // VBISAM
)

// Configuration flags (ConfFlags field)
const (
	ConfWRTHRU  = 0x80 // Write through to disk
	ConfRELADRS = 0x40 // Use relative byte address
	ConfUPPTR   = 0x20 // Update current record pointer
	ConfREC64   = 0x10 // Use 64-bit record address
)

// ---------------------------------------------------------------------------
// KDB (Key Definition Block) structures
// ---------------------------------------------------------------------------

// KDB_KEY is a key definition (16 bytes per key)
type KDB_KEY struct {
	Count     [2]byte // Number of components in this key
	Offset    [2]byte // Offset to component definitions
	KeyFlags  byte    // Flags: 0x10=primary, 0x40=duplicates, 0x02=sparse
	CompFlags byte    // Compression flags
	Sparse    byte    // Sparse character
	Reserved  [9]byte
}

// Key flag constants
const (
	KeyPrimary    = 0x10 // Primary key
	KeyDuplicates = 0x40 // Duplicates allowed
	KeySparse     = 0x02 // Sparse key
)

// KDB is the Key Definition Block header
type KDB struct {
	KdbLen  [2]byte
	Filler  [4]byte
	Nkeys   [2]byte
	Filler2 [6]byte
	Keys    [1]KDB_KEY // First key; for multi-key, allocate larger buffer
}

// ---------------------------------------------------------------------------
// FCD3 - File Control Description version 3 (64-bit)
// Binary-compatible with Micro Focus's definition (280 bytes)
// ---------------------------------------------------------------------------
type FCD3 struct {
	FileStatus   [2]byte
	FcdLen       [2]byte
	FcdVer       byte
	FileOrg      byte
	AccessFlags  byte
	OpenMode     byte
	RecordMode   byte
	FileFormat   byte
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
	FnameLen     [2]byte
	IdxNameLen   [2]byte
	RetryCnt     [2]byte
	RefKey       [2]byte
	LineCount    [2]byte
	UseFiles     byte
	GiveFiles    byte
	EffKeyLen    [2]byte
	Res5         [14]byte
	Eop          [2]byte
	Opt          [4]byte
	CurRecLen    [4]byte
	MinRecLen    [4]byte
	MaxRecLen    [4]byte
	Fsv2SessId   [4]byte
	Res6         [24]byte
	RelByteAdrs  [8]byte
	MaxRelKey    [8]byte
	RelKey       [8]byte
	FileHandle   [8]byte
	RecPtr       [8]byte
	FnamePtr     [8]byte
	IdxNamePtr   [8]byte
	KdbPtr       [8]byte
	ColPtr       [8]byte
	FileDefPtr   [8]byte
	DfSortPtr    [8]byte
}

// ---------------------------------------------------------------------------
// FileStatus - EXTFH file status code with rich error info
// Reference: Chapter 31 of Micro Focus COBOL docs
// ---------------------------------------------------------------------------
type FileStatus struct {
	S1, S2 byte
}

func (fs FileStatus) IsOK() bool      { return fs.S1 == '0' && (fs.S2 == '0' || fs.S2 == 0) }
func (fs FileStatus) IsEOF() bool     { return fs.S1 == '1' && fs.S2 == '0' }
func (fs FileStatus) IsNotFound() bool { return fs.S1 == '2' && fs.S2 == '3' }
func (fs FileStatus) IsDupKey() bool   { return fs.S1 == '0' && fs.S2 == '2' }

// IsLocked returns true if the file or record is locked
func (fs FileStatus) IsLocked() bool {
	// Extended: 9/065 (file locked), 9/068 (record locked)
	if fs.S1 == '9' {
		code := int(fs.S2)
		return code == 65 || code == 68
	}
	return false
}

// IsPermError returns true for permanent/fatal I/O errors
func (fs FileStatus) IsPermError() bool {
	return fs.S1 == '3' && fs.S2 == '0'
}

// IsAttrConflict returns true for attribute mismatch (status 39)
func (fs FileStatus) IsAttrConflict() bool {
	return fs.S1 == '3' && fs.S2 == '9'
}

// ExtendedCode returns the 9/nnn code if this is an extended status, or 0
func (fs FileStatus) ExtendedCode() int {
	if fs.S1 == '9' {
		return int(fs.S2)
	}
	return 0
}

// Error returns a human-readable error message
func (fs FileStatus) Error() string {
	desc := fs.Description()
	if desc != "" {
		return fmt.Sprintf("status %c%c: %s", fs.S1, fs.S2, desc)
	}
	return fmt.Sprintf("status %c%c (0x%02x/0x%02x)", fs.S1, fs.S2, fs.S1, fs.S2)
}

// Description returns a human-readable description of the status code
func (fs FileStatus) Description() string {
	key := [2]byte{fs.S1, fs.S2}

	// Standard ANSI codes
	ansi := map[[2]byte]string{
		{'0', '0'}: "successful",
		{'0', 0}:   "successful",
		{'0', '2'}: "duplicate key (allowed)",
		{'0', '4'}: "wrong record length",
		{'0', '5'}: "optional file not present",
		{'1', '0'}: "end of file",
		{'1', '4'}: "relative record number too large",
		{'2', '1'}: "key sequence error",
		{'2', '2'}: "duplicate key (not allowed)",
		{'2', '3'}: "record not found",
		{'2', '4'}: "boundary violation / disk full",
		{'3', '0'}: "permanent I/O error",
		{'3', '4'}: "boundary violation (sequential)",
		{'3', '5'}: "file not found",
		{'3', '7'}: "open mode not supported",
		{'3', '8'}: "file closed with lock",
		{'3', '9'}: "fixed file attribute conflict",
		{'4', '1'}: "file already open",
		{'4', '2'}: "file not open",
		{'4', '3'}: "DELETE/REWRITE without prior READ",
		{'4', '4'}: "record size boundary violation",
		{'4', '6'}: "no valid next record",
		{'4', '7'}: "READ on OUTPUT/EXTEND file",
		{'4', '8'}: "WRITE on INPUT file",
		{'4', '9'}: "DELETE/REWRITE on non-I-O file",
	}

	if desc, ok := ansi[key]; ok {
		return desc
	}

	// Extended 9/nnn codes (S2 is binary nnn)
	if fs.S1 == '9' {
		code := int(fs.S2)
		extended := map[int]string{
			1:   "insufficient buffer/memory (RT001)",
			2:   "file not open (RT002)",
			3:   "serial mode error (RT003)",
			4:   "illegal file name (RT004)",
			5:   "illegal device (RT005)",
			6:   "write to INPUT file (RT006)",
			7:   "disk space exhausted (RT007)",
			8:   "read from OUTPUT file (RT008)",
			9:   "directory not found (RT009)",
			10:  "file name not supplied (RT010)",
			12:  "file already open (RT012)",
			13:  "file not found (RT013)",
			14:  "too many files open (RT014)",
			15:  "too many indexed files open (RT015)",
			17:  "record error: zero length (RT017)",
			18:  "read part record: EOF before EOR (RT018)",
			19:  "rewrite error: wrong mode (RT019)",
			20:  "device or resource busy (RT020)",
			21:  "file is a directory (RT021)",
			22:  "illegal access mode (RT022)",
			24:  "disk I/O error (RT024)",
			30:  "file system is read-only (RT030)",
			35:  "incorrect access permission (RT035)",
			37:  "file access denied (RT037)",
			39:  "file not compatible (RT039)",
			41:  "corrupt index file (RT041)",
			43:  "file info missing for indexed file (RT043)",
			47:  "index structure overflow (RT047)",
			65:  "file locked by another process (RT065)",
			66:  "duplicate record key (RT066)",
			68:  "record locked by another process (RT068)",
			69:  "illegal argument to ISAM module (RT069)",
			71:  "bad indexed file format (RT071)",
			72:  "end of indexed file (RT072)",
			100: "invalid file operation (RT100)",
			105: "memory allocation error (RT105)",
			138: "file closed with lock (RT138)",
			139: "record length or key inconsistency (RT139)",
			141: "file already open (RT141)",
			142: "file not open (RT142)",
			146: "no current record for sequential read (RT146)",
			147: "wrong mode for READ/START (RT147)",
			148: "wrong mode for WRITE (RT148)",
			149: "wrong mode for REWRITE/DELETE (RT149)",
			161: "file header not found/corrupted (RT161)",
			194: "file size too large (RT194)",
			210: "file closed with lock (RT210)",
			213: "too many locks (RT213)",
			218: "sharing conflict (RT218)",
			219: "OS shared file limit exceeded (RT219)",
		}
		if desc, ok := extended[code]; ok {
			return fmt.Sprintf("9/%03d %s", code, desc)
		}
		return fmt.Sprintf("9/%03d unknown extended status", code)
	}

	return ""
}

// ---------------------------------------------------------------------------
// KeyInfo describes a discovered key from the file
// ---------------------------------------------------------------------------
type KeyInfo struct {
	Index     int  // Key number (0=primary, 1+=alternate)
	IsPrimary bool // True if primary key
	AllowDups bool // True if duplicates allowed
	IsSparse  bool // True if sparse key
	CompCount int  // Number of components
}

// ---------------------------------------------------------------------------
// IsamFile represents an open ISAM file via EXTFH
// ---------------------------------------------------------------------------
type IsamFile struct {
	path   string
	fcd    FCD3
	kdb    []byte // Raw KDB buffer (dynamic size for multi-key)
	kdbHdr *KDB   // Pointer into kdb buffer
	recBuf []byte
	fname  []byte
	opened bool

	// File metadata (populated after Open)
	RecSize  int       // Max record size
	MinRec   int       // Min record size (for variable-length)
	Format   int       // IDXFORMAT (0=default, 8=large, etc.)
	NumKeys  int       // Number of keys
	Keys     []KeyInfo // All key definitions
	IsVarLen bool      // True if variable-length records

	// Diagnostics
	LastStatus FileStatus // Last EXTFH call status
	CallCount  int        // Total EXTFH calls made on this file
}

// DLL management
var (
	extfhDLL  *syscall.LazyDLL
	extfhProc *syscall.LazyProc
	dllOnce   sync.Once
	dllErr    error
	dllAvail  bool
	dllPath   string
)

// ExtfhDebug enables verbose EXTFH logging when true
var ExtfhDebug = false

func initDLL() {
	paths := []string{
		`C:\Microfocus\bin64\cblrtsm.dll`,
		`C:\Microfocus\bin\cblrtsm.dll`,
	}

	// Also check COBDIR environment variable
	if cobdir := os.Getenv("COBDIR"); cobdir != "" {
		paths = append([]string{
			cobdir + `\bin64\cblrtsm.dll`,
			cobdir + `\bin\cblrtsm.dll`,
		}, paths...)
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		extfhDLL = syscall.NewLazyDLL(p)
		extfhProc = extfhDLL.NewProc("EXTFH")
		if err := extfhProc.Find(); err != nil {
			continue
		}
		dllAvail = true
		dllPath = p
		return
	}
	dllErr = fmt.Errorf("cblrtsm.dll not found (checked COBDIR, Microfocus bin/bin64)")
}

// ExtfhAvailable returns true if the Micro Focus EXTFH DLL is loaded
func ExtfhAvailable() bool {
	dllOnce.Do(initDLL)
	return dllAvail
}

// ExtfhDLLPath returns the path of the loaded DLL, or "" if not loaded
func ExtfhDLLPath() string {
	dllOnce.Do(initDLL)
	return dllPath
}

// opcodeName returns a human-readable name for an opcode (for debug logging)
func opcodeName(opcode uint16) string {
	names := map[uint16]string{
		OpGetInfo: "GETINFO", OpFlush: "FLUSH",
		OpOpenInput: "OPEN_INPUT", OpOpenOutput: "OPEN_OUTPUT",
		OpOpenIO: "OPEN_IO", OpOpenExtend: "OPEN_EXTEND",
		OpClose: "CLOSE", OpCloseLock: "CLOSE_LOCK",
		OpReadSeq: "READ_SEQ", OpReadPrev: "READ_PREV", OpReadRan: "READ_RAN",
		OpWrite: "WRITE", OpRewrite: "REWRITE", OpDelete: "DELETE",
		OpStepFirst: "STEP_FIRST", OpStepNext: "STEP_NEXT", OpStepPrev: "STEP_PREV",
		OpStartEQ: "START_EQ", OpStartGE: "START_GE", OpStartGT: "START_GT",
		OpStartLE: "START_LE", OpStartLT: "START_LT",
		OpCommit: "COMMIT", OpRollback: "ROLLBACK",
	}
	if name, ok := names[opcode]; ok {
		return name
	}
	return fmt.Sprintf("0x%04X", opcode)
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

func setPointer(field *[8]byte, ptr unsafe.Pointer) {
	*(*uintptr)(unsafe.Pointer(field)) = uintptr(ptr)
}

// ---------------------------------------------------------------------------
// OpenIsamFile opens an ISAM file for reading via EXTFH.
// Always call Close() when done (use defer).
// ---------------------------------------------------------------------------
func OpenIsamFile(path string) (*IsamFile, error) {
	if !ExtfhAvailable() {
		return nil, dllErr
	}

	f := &IsamFile{
		path:   path,
		fname:  append([]byte(path), 0),
		recBuf: make([]byte, 65536),
	}

	// Allocate KDB large enough for up to 32 keys (matching EXTFH.CFG INDEXCOUNT=32)
	// KDB header (14 bytes) + 32 keys * 16 bytes = 526 bytes
	f.kdb = make([]byte, 14+32*16)
	f.kdbHdr = (*KDB)(unsafe.Pointer(&f.kdb[0]))
	binary.BigEndian.PutUint16(f.kdbHdr.KdbLen[:], uint16(len(f.kdb)))
	binary.BigEndian.PutUint16(f.kdbHdr.Nkeys[:], 1)
	f.kdbHdr.Keys[0].KeyFlags = KeyPrimary

	// Initialize FCD3 - struct is zero-valued by default (critical per MF docs)
	f.fcd = FCD3{}

	f.fcd.FcdVer = 1 // 64-bit FCD3
	f.fcd.FileOrg = OrgIndexed
	f.fcd.AccessFlags = AccessDynamic
	f.fcd.OpenMode = 128 // OPEN_NOT_OPEN
	f.fcd.FileFormat = FormatDefault
	f.fcd.FstatusType = 0x80
	f.fcd.OtherFlags = 0x80
	f.fcd.ConfFlags2 = 0x08

	binary.BigEndian.PutUint16(f.fcd.FcdLen[:], uint16(unsafe.Sizeof(f.fcd)))
	binary.BigEndian.PutUint16(f.fcd.FnameLen[:], uint16(len(f.fname)-1))
	binary.BigEndian.PutUint32(f.fcd.MaxRecLen[:], 65536)
	binary.BigEndian.PutUint32(f.fcd.MinRecLen[:], 0)

	setPointer(&f.fcd.RecPtr, unsafe.Pointer(&f.recBuf[0]))
	setPointer(&f.fcd.FnamePtr, unsafe.Pointer(&f.fname[0]))
	setPointer(&f.fcd.KdbPtr, unsafe.Pointer(&f.kdb[0]))

	// GETINFO: discover file attributes before opening
	st := callEXTFH(OpGetInfo, &f.fcd)
	if !st.IsOK() {
		if ExtfhDebug {
			log.Printf("[EXTFH] GETINFO returned %s for %s, continuing...", st.Error(), path)
		}
		f.fcd.FileStatus[0] = '0'
		f.fcd.FileStatus[1] = '0'
	}

	// OPEN INPUT (read-only)
	st = callEXTFH(OpOpenInput, &f.fcd)
	if !st.IsOK() {
		return nil, fmt.Errorf("open %s: %s", path, st.Error())
	}

	f.opened = true
	f.LastStatus = st

	// Read discovered attributes
	f.RecSize = int(binary.BigEndian.Uint32(f.fcd.MaxRecLen[:]))
	f.MinRec = int(binary.BigEndian.Uint32(f.fcd.MinRecLen[:]))
	f.Format = int(f.fcd.FileFormat)
	f.NumKeys = int(binary.BigEndian.Uint16(f.kdbHdr.Nkeys[:]))
	f.IsVarLen = f.MinRec > 0 && f.MinRec != f.RecSize

	// Validate IDXFORMAT
	if f.Format != 0 && f.Format != FormatIDX8 {
		log.Printf("[EXTFH] WARNING: %s has IDXFORMAT=%d, expected 8", path, f.Format)
	}

	// Parse key definitions from KDB
	f.parseKeys()

	if ExtfhDebug {
		log.Printf("[EXTFH] Opened %s: recSize=%d minRec=%d format=%d keys=%d varLen=%v",
			path, f.RecSize, f.MinRec, f.Format, f.NumKeys, f.IsVarLen)
		for _, k := range f.Keys {
			log.Printf("[EXTFH]   Key[%d]: primary=%v dups=%v sparse=%v components=%d",
				k.Index, k.IsPrimary, k.AllowDups, k.IsSparse, k.CompCount)
		}
	}

	return f, nil
}

// parseKeys extracts key definitions from the KDB buffer
func (f *IsamFile) parseKeys() {
	f.Keys = nil
	nkeys := f.NumKeys
	if nkeys <= 0 || nkeys > 32 {
		return
	}

	for i := 0; i < nkeys; i++ {
		offset := 14 + i*16
		if offset+16 > len(f.kdb) {
			break
		}

		kk := (*KDB_KEY)(unsafe.Pointer(&f.kdb[offset]))
		compCount := int(binary.BigEndian.Uint16(kk.Count[:]))
		if compCount <= 0 {
			compCount = 1
		}

		f.Keys = append(f.Keys, KeyInfo{
			Index:     i,
			IsPrimary: kk.KeyFlags&KeyPrimary != 0,
			AllowDups: kk.KeyFlags&KeyDuplicates != 0,
			IsSparse:  kk.KeyFlags&KeySparse != 0,
			CompCount: compCount,
		})
	}
}

// ---------------------------------------------------------------------------
// Close closes the ISAM file. Safe to call multiple times.
// ---------------------------------------------------------------------------
func (f *IsamFile) Close() {
	if !f.opened {
		return
	}
	st := callEXTFH(OpClose, &f.fcd)
	f.opened = false
	f.LastStatus = st
	f.CallCount++
}

// ---------------------------------------------------------------------------
// Sequential read operations
// ---------------------------------------------------------------------------

// ReadFirst reads the first record (by primary key order).
func (f *IsamFile) ReadFirst() ([]byte, error) {
	if !f.opened {
		return nil, fmt.Errorf("file not open")
	}
	st := callEXTFH(OpStepFirst, &f.fcd)
	f.LastStatus = st
	f.CallCount++
	if st.IsEOF() {
		return nil, nil
	}
	if !st.IsOK() {
		return nil, fmt.Errorf("read first: %s", st.Error())
	}
	return f.copyRecord(), nil
}

// ReadNext reads the next record sequentially. Returns nil, nil at EOF.
func (f *IsamFile) ReadNext() ([]byte, error) {
	if !f.opened {
		return nil, fmt.Errorf("file not open")
	}
	st := callEXTFH(OpStepNext, &f.fcd)
	f.LastStatus = st
	f.CallCount++
	if st.IsEOF() {
		return nil, nil
	}
	if !st.IsOK() {
		return nil, fmt.Errorf("read next: %s", st.Error())
	}
	return f.copyRecord(), nil
}

// ReadPrev reads the previous record. Returns nil, nil at beginning of file.
func (f *IsamFile) ReadPrev() ([]byte, error) {
	if !f.opened {
		return nil, fmt.Errorf("file not open")
	}
	st := callEXTFH(OpStepPrev, &f.fcd)
	f.LastStatus = st
	f.CallCount++
	if st.IsEOF() {
		return nil, nil
	}
	if !st.IsOK() {
		return nil, fmt.Errorf("read prev: %s", st.Error())
	}
	return f.copyRecord(), nil
}

// ---------------------------------------------------------------------------
// Key-based read operations
// ---------------------------------------------------------------------------

// ReadByKey reads a record by exact key match.
// keyNum is 0 for primary key, 1+ for alternate keys.
func (f *IsamFile) ReadByKey(key []byte, keyNum int) ([]byte, error) {
	if !f.opened {
		return nil, fmt.Errorf("file not open")
	}
	if keyNum < 0 || keyNum >= f.NumKeys {
		return nil, fmt.Errorf("invalid key number %d (file has %d keys)", keyNum, f.NumKeys)
	}

	binary.BigEndian.PutUint16(f.fcd.RefKey[:], uint16(keyNum))
	copy(f.recBuf, key)
	binary.BigEndian.PutUint16(f.fcd.EffKeyLen[:], uint16(len(key)))

	st := callEXTFH(OpReadRan, &f.fcd)
	f.LastStatus = st
	f.CallCount++

	if st.IsNotFound() {
		return nil, nil
	}
	if !st.IsOK() && !st.IsDupKey() {
		return nil, fmt.Errorf("read by key: %s", st.Error())
	}
	return f.copyRecord(), nil
}

// ---------------------------------------------------------------------------
// START operations (position cursor, then use ReadNext/ReadPrev)
// ---------------------------------------------------------------------------

func (f *IsamFile) startOp(opcode uint16, key []byte, keyNum int) error {
	if !f.opened {
		return fmt.Errorf("file not open")
	}
	if keyNum < 0 || keyNum >= f.NumKeys {
		return fmt.Errorf("invalid key number %d (file has %d keys)", keyNum, f.NumKeys)
	}

	binary.BigEndian.PutUint16(f.fcd.RefKey[:], uint16(keyNum))
	copy(f.recBuf, key)
	binary.BigEndian.PutUint16(f.fcd.EffKeyLen[:], uint16(len(key)))

	st := callEXTFH(opcode, &f.fcd)
	f.LastStatus = st
	f.CallCount++

	if st.IsEOF() || st.IsNotFound() {
		return fmt.Errorf("no records matching criteria (%s)", opcodeName(opcode))
	}
	if !st.IsOK() {
		return fmt.Errorf("%s: %s", opcodeName(opcode), st.Error())
	}
	return nil
}

// StartEQ positions to the first record = key.
func (f *IsamFile) StartEQ(key []byte, keyNum int) error {
	return f.startOp(OpStartEQ, key, keyNum)
}

// StartGE positions to the first record >= key.
func (f *IsamFile) StartGE(key []byte, keyNum int) error {
	return f.startOp(OpStartGE, key, keyNum)
}

// StartGT positions to the first record > key.
func (f *IsamFile) StartGT(key []byte, keyNum int) error {
	return f.startOp(OpStartGT, key, keyNum)
}

// StartLE positions to the last record <= key.
func (f *IsamFile) StartLE(key []byte, keyNum int) error {
	return f.startOp(OpStartLE, key, keyNum)
}

// StartLT positions to the last record < key.
func (f *IsamFile) StartLT(key []byte, keyNum int) error {
	return f.startOp(OpStartLT, key, keyNum)
}

// ---------------------------------------------------------------------------
// Convenience methods
// ---------------------------------------------------------------------------

func (f *IsamFile) copyRecord() []byte {
	curLen := int(binary.BigEndian.Uint32(f.fcd.CurRecLen[:]))
	if curLen <= 0 || curLen > len(f.recBuf) {
		curLen = f.RecSize
	}
	if curLen <= 0 || curLen > len(f.recBuf) {
		curLen = len(f.recBuf)
	}
	rec := make([]byte, curLen)
	copy(rec, f.recBuf[:curLen])
	return rec
}

// ReadAll reads all records sequentially.
func (f *IsamFile) ReadAll() ([][]byte, error) {
	rec, err := f.ReadFirst()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}

	var records [][]byte
	records = append(records, rec)

	for {
		rec, err = f.ReadNext()
		if err != nil {
			return records, err
		}
		if rec == nil {
			break
		}
		records = append(records, rec)
	}
	return records, nil
}

// ForEach iterates all records calling fn for each.
// Return false from fn to stop early.
func (f *IsamFile) ForEach(fn func(rec []byte) bool) error {
	rec, err := f.ReadFirst()
	if err != nil {
		return err
	}
	if rec == nil {
		return nil
	}
	if !fn(rec) {
		return nil
	}

	for {
		rec, err = f.ReadNext()
		if err != nil {
			return err
		}
		if rec == nil {
			return nil
		}
		if !fn(rec) {
			return nil
		}
	}
}

// ForEachByKey iterates records starting from a key position.
// Uses START GE + READ NEXT. Return false from fn to stop early.
func (f *IsamFile) ForEachByKey(startKey []byte, keyNum int, fn func(rec []byte) bool) error {
	if err := f.StartGE(startKey, keyNum); err != nil {
		return err
	}

	for {
		rec, err := f.ReadNext()
		if err != nil {
			return err
		}
		if rec == nil {
			return nil
		}
		if !fn(rec) {
			return nil
		}
	}
}

// Count counts all records in the file (full sequential scan).
func (f *IsamFile) Count() (int, error) {
	count := 0
	err := f.ForEach(func(rec []byte) bool {
		count++
		return true
	})
	return count, err
}

// Path returns the file path
func (f *IsamFile) Path() string {
	return f.path
}

// IsOpen returns true if the file is currently open
func (f *IsamFile) IsOpen() bool {
	return f.opened
}

// ---------------------------------------------------------------------------
// Unified API: works with EXTFH or falls back to binary reader
// ---------------------------------------------------------------------------

// ReadIsamFile reads all records from an ISAM file.
// Uses EXTFH if available, falls back to binary reader.
func ReadIsamFile(path string) ([][]byte, int, error) {
	if ExtfhAvailable() {
		f, err := OpenIsamFile(path)
		if err != nil {
			return nil, 0, err
		}
		defer f.Close()

		records, err := f.ReadAll()
		return records, f.RecSize, err
	}

	// Fallback: binary reader
	info, err := ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	records := make([][]byte, len(info.Records))
	for i, r := range info.Records {
		records[i] = r.Data
	}
	return records, info.RecordSize, nil
}

// ---------------------------------------------------------------------------
// Field extraction and decoding
// ---------------------------------------------------------------------------

// DecodeExtfhField extracts and decodes a field from a record
func DecodeExtfhField(rec []byte, offset, length int) string {
	return ExtractField(rec, offset, length)
}

// DecodeField extracts and decodes a Windows-1252 field with trimming
func DecodeField(rec []byte, offset, length int) string {
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

// DecodeFieldTrimLeft extracts a field and trims leading zeros/spaces
func DecodeFieldTrimLeft(rec []byte, offset, length int) string {
	s := DecodeField(rec, offset, length)
	return strings.TrimLeft(s, "0 ")
}

// ---------------------------------------------------------------------------
// Legacy compatibility
// ---------------------------------------------------------------------------

// ExtfhRecord represents a record read via EXTFH (legacy)
type ExtfhRecord struct {
	Data []byte
}

// ReadFileExtfh reads an ISAM file using EXTFH (legacy API).
// Prefer ReadIsamFile or OpenIsamFile for new code.
func ReadFileExtfh(path string) ([]ExtfhRecord, error) {
	f, err := OpenIsamFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := f.ReadAll()
	if err != nil {
		return nil, err
	}

	result := make([]ExtfhRecord, len(records))
	for i, r := range records {
		result[i] = ExtfhRecord{Data: r}
	}
	return result, nil
}
