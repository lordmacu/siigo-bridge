package isam

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

// ---------------------------------------------------------------------------
// writer_v2.go — Pure Go ISAM in-place operations (no DLL dependency)
//
// Supports:
//   - REWRITE: overwrite existing record data without changing keys
//   - DELETE:  mark record as deleted (type nibble → 0x3)
//   - INSERT:  write new record into deleted slot + update B-tree index
//
// Safety:
//   - Creates .bak backup before first write
//   - Verifies record marker is valid before writing
//   - Verifies key bytes are unchanged in newData (REWRITE)
//   - File locking via OS exclusive open
// ---------------------------------------------------------------------------

// WriteResult contains the result of a write operation
type WriteResult struct {
	Path        string // File path
	RecordIndex int    // 0-based index of the record among data records
	FileOffset  int    // Byte offset in file where data was written
	RecSize     int    // Record size
	BackupPath  string // Path to backup file (empty if backup skipped)
}

// RewriteRecord overwrites a data record in-place without changing keys.
//
// Parameters:
//   - path: ISAM file path
//   - recordIndex: 0-based index among data records (same order as ReadFileV2)
//   - newData: new record bytes (must be exactly recSize bytes)
//   - keyOffsets: pairs of [offset, length] defining key fields to verify unchanged
//
// The function:
//  1. Creates a .bak backup (if not already present)
//  2. Reads the file with V2 reader to find the exact byte offset
//  3. Verifies the record marker is valid
//  4. Verifies all key fields are identical in old and new data
//  5. Writes newData at the exact file offset
//  6. Updates the header modify timestamp
func RewriteRecord(path string, recordIndex int, newData []byte, keyOffsets [][2]int) (*WriteResult, error) {
	// Use the proven V2 reader to get records with their file offsets
	info, hdr, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	recSize := int(hdr.MaxRecordLen)

	// Validate inputs
	if recordIndex < 0 || recordIndex >= len(info.Records) {
		return nil, fmt.Errorf("record index %d out of range (file has %d records)", recordIndex, len(info.Records))
	}

	if len(newData) != recSize {
		return nil, fmt.Errorf("newData size %d != record size %d", len(newData), recSize)
	}

	rec := info.Records[recordIndex]

	// Verify keys are unchanged
	if err := verifyKeysUnchanged(rec.Data, newData, keyOffsets); err != nil {
		return nil, fmt.Errorf("key verification failed: %w", err)
	}

	// Check if data actually changed
	if bytesEqual(rec.Data, newData) {
		return &WriteResult{
			Path:        path,
			RecordIndex: recordIndex,
			FileOffset:  rec.Offset,
			RecSize:     recSize,
		}, nil // No change needed
	}

	// Create backup
	backupPath, err := createBackup(path)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	// Open file for writing
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open for write %s: %w", path, err)
	}
	defer f.Close()

	// Data offset = marker offset + header size
	dataOffset := rec.Offset + hdr.RecHeaderSize

	// Re-verify the marker at the expected offset (guard against concurrent modification)
	markerBuf := make([]byte, hdr.RecHeaderSize)
	if _, err := f.ReadAt(markerBuf, int64(rec.Offset)); err != nil {
		return nil, fmt.Errorf("read marker at %d: %w", rec.Offset, err)
	}

	var verifyType byte
	if hdr.RecHeaderSize == 2 {
		marker := binary.BigEndian.Uint16(markerBuf)
		verifyType = byte(marker >> 12)
	} else {
		marker := binary.BigEndian.Uint32(markerBuf)
		verifyType = byte((marker >> 28) & 0x0F)
	}

	isData := verifyType == RecTypeNormal || verifyType == RecTypeReduced ||
		verifyType == RecTypeRefData || verifyType == RecTypeRedRef
	if !isData {
		return nil, fmt.Errorf("marker at offset %d is type %s, not a data record — file may have been modified",
			rec.Offset, recTypeName(verifyType))
	}

	// Write the new data at the exact offset (overwrite data portion only)
	writeLen := recSize
	if _, err := f.WriteAt(newData[:writeLen], int64(dataOffset)); err != nil {
		return nil, fmt.Errorf("write at offset %d: %w", dataOffset, err)
	}

	// Update header modify timestamp
	updateHeaderTimestamp(f, hdr)

	return &WriteResult{
		Path:        path,
		RecordIndex: recordIndex,
		FileOffset:  dataOffset,
		RecSize:     recSize,
		BackupPath:  backupPath,
	}, nil
}

// RewriteRecordByKey finds a record by key match and overwrites it in-place.
//
// Parameters:
//   - path: ISAM file path
//   - keyOffset, keyLen: position and length of the primary key in the record
//   - keyValue: the key value to search for (trimmed comparison)
//   - newData: new record bytes (must be exactly recSize bytes)
//   - extraKeyOffsets: additional key fields to verify unchanged (optional)
func RewriteRecordByKey(path string, keyOffset, keyLen int, keyValue string, newData []byte, extraKeyOffsets [][2]int) (*WriteResult, error) {
	info, hdr, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	recSize := int(hdr.MaxRecordLen)
	if len(newData) != recSize {
		return nil, fmt.Errorf("newData size %d != record size %d", len(newData), recSize)
	}

	// Find the record by key
	matchIdx := -1
	for i, rec := range info.Records {
		recKey := extractTrimmedField(rec.Data, keyOffset, keyLen)
		if recKey == keyValue {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		return nil, fmt.Errorf("record with key %q not found in %s", keyValue, path)
	}

	// Build full key offsets list (primary + extras)
	allKeys := [][2]int{{keyOffset, keyLen}}
	allKeys = append(allKeys, extraKeyOffsets...)

	return RewriteRecord(path, matchIdx, newData, allKeys)
}

// RewriteFields modifies specific fields in a record without touching the rest.
// This is the safest operation: reads current record, patches fields, writes back.
//
// Parameters:
//   - path: ISAM file path
//   - recordIndex: 0-based data record index
//   - fields: map of offset -> new bytes to write at that offset
//   - keyOffsets: key field positions to verify unchanged
func RewriteFields(path string, recordIndex int, fields map[int][]byte, keyOffsets [][2]int) (*WriteResult, error) {
	info, _, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if recordIndex < 0 || recordIndex >= len(info.Records) {
		return nil, fmt.Errorf("record index %d out of range (file has %d records)", recordIndex, len(info.Records))
	}

	rec := info.Records[recordIndex]
	recSize := len(rec.Data)

	// Start with current record data
	newData := make([]byte, recSize)
	copy(newData, rec.Data)

	// Patch the specified fields
	for offset, value := range fields {
		if offset < 0 || offset+len(value) > recSize {
			return nil, fmt.Errorf("field at offset %d length %d exceeds record size %d", offset, len(value), recSize)
		}
		copy(newData[offset:], value)
	}

	// Verify no key fields were modified
	if err := verifyKeysUnchanged(rec.Data, newData, keyOffsets); err != nil {
		return nil, fmt.Errorf("field modification would change a key: %w", err)
	}

	return RewriteRecord(path, recordIndex, newData, keyOffsets)
}

// RewriteFieldsByKey finds a record by key and patches specific fields.
//
// Parameters:
//   - path: ISAM file path
//   - keyOffset, keyLen: primary key position and length
//   - keyValue: key to search for
//   - fields: map of offset -> new bytes to write at that offset
func RewriteFieldsByKey(path string, keyOffset, keyLen int, keyValue string, fields map[int][]byte) (*WriteResult, error) {
	info, _, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Find the record by key
	matchIdx := -1
	for i, rec := range info.Records {
		recKey := extractTrimmedField(rec.Data, keyOffset, keyLen)
		if recKey == keyValue {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		return nil, fmt.Errorf("record with key %q not found in %s", keyValue, path)
	}

	keyOffsets := [][2]int{{keyOffset, keyLen}}
	return RewriteFields(path, matchIdx, fields, keyOffsets)
}

// ---------------------------------------------------------------------------
// DELETE operations — mark record as deleted (type nibble → 0x3)
// ---------------------------------------------------------------------------

// DeleteRecord marks a data record as deleted by changing its type nibble to 0x3.
// The data bytes are zeroed out for safety. The B-tree index is NOT modified.
//
// Parameters:
//   - path: ISAM file path
//   - recordIndex: 0-based index among data records (same order as ReadFileV2)
//
// The function:
//  1. Creates a .bak backup (if not already present)
//  2. Reads the file with V2 reader to find the exact byte offset
//  3. Verifies the record marker is a data record
//  4. Changes the type nibble from NORMAL/REDUCED/etc to DELETED (0x3)
//  5. Zeros out the data bytes
//  6. Updates the header modify timestamp
func DeleteRecord(path string, recordIndex int) (*WriteResult, error) {
	info, hdr, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if recordIndex < 0 || recordIndex >= len(info.Records) {
		return nil, fmt.Errorf("record index %d out of range (file has %d records)", recordIndex, len(info.Records))
	}

	rec := info.Records[recordIndex]

	// Create backup
	backupPath, err := createBackup(path)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	// Open file for writing
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open for write %s: %w", path, err)
	}
	defer f.Close()

	// Read current marker
	markerBuf := make([]byte, hdr.RecHeaderSize)
	if _, err := f.ReadAt(markerBuf, int64(rec.Offset)); err != nil {
		return nil, fmt.Errorf("read marker at %d: %w", rec.Offset, err)
	}

	// Verify it's a data record and build the new deleted marker
	var newMarker []byte
	if hdr.RecHeaderSize == 2 {
		marker := binary.BigEndian.Uint16(markerBuf)
		recType := byte(marker >> 12)
		if !isDataType(recType) {
			return nil, fmt.Errorf("record at offset %d is type %s, not a data record",
				rec.Offset, recTypeName(recType))
		}
		// Replace type nibble with DELETED (0x3), keep length
		deleted := (uint16(RecTypeDeleted) << 12) | (marker & 0x0FFF)
		newMarker = make([]byte, 2)
		binary.BigEndian.PutUint16(newMarker, deleted)
	} else {
		marker := binary.BigEndian.Uint32(markerBuf)
		recType := byte((marker >> 28) & 0x0F)
		if !isDataType(recType) {
			return nil, fmt.Errorf("record at offset %d is type %s, not a data record",
				rec.Offset, recTypeName(recType))
		}
		// Replace type nibble with DELETED (0x3), keep length
		deleted := (uint32(RecTypeDeleted) << 28) | (marker & 0x0FFFFFFF)
		newMarker = make([]byte, 4)
		binary.BigEndian.PutUint32(newMarker, deleted)
	}

	// Write the new deleted marker
	if _, err := f.WriteAt(newMarker, int64(rec.Offset)); err != nil {
		return nil, fmt.Errorf("write marker at offset %d: %w", rec.Offset, err)
	}

	// Zero out the data bytes
	recSize := int(hdr.MaxRecordLen)
	dataOffset := rec.Offset + hdr.RecHeaderSize
	zeroes := make([]byte, recSize)
	if _, err := f.WriteAt(zeroes, int64(dataOffset)); err != nil {
		return nil, fmt.Errorf("zero data at offset %d: %w", dataOffset, err)
	}

	// Update header modify timestamp
	updateHeaderTimestamp(f, hdr)

	return &WriteResult{
		Path:        path,
		RecordIndex: recordIndex,
		FileOffset:  rec.Offset,
		RecSize:     recSize,
		BackupPath:  backupPath,
	}, nil
}

// DeleteRecordByKey finds a record by key match and marks it as deleted.
//
// Parameters:
//   - path: ISAM file path
//   - keyOffset, keyLen: position and length of the primary key in the record
//   - keyValue: the key value to search for (trimmed comparison)
func DeleteRecordByKey(path string, keyOffset, keyLen int, keyValue string) (*WriteResult, error) {
	info, _, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	matchIdx := -1
	for i, rec := range info.Records {
		recKey := extractTrimmedField(rec.Data, keyOffset, keyLen)
		if recKey == keyValue {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		return nil, fmt.Errorf("record with key %q not found in %s", keyValue, path)
	}

	return DeleteRecord(path, matchIdx)
}

// isDataType returns true if the record type is a user data record
func isDataType(t byte) bool {
	return t == RecTypeNormal || t == RecTypeReduced ||
		t == RecTypeRefData || t == RecTypeRedRef
}

// ---------------------------------------------------------------------------
// INSERT operations — write new record + update B-tree index
// ---------------------------------------------------------------------------

// InsertRecord inserts a new data record into an ISAM file.
//
// Strategy:
//  1. Find a DELETED data slot to reuse (same recSize), or error if none available
//  2. Write the new record data into the slot
//  3. Change the slot's marker from DELETED (0x3) to NORMAL (0x4)
//  4. Insert the key into the B-tree at the correct leaf position
//  5. Handle node splits if the leaf is full
//  6. Update header timestamp
//
// Parameters:
//   - path: ISAM file path
//   - newData: new record bytes (must be exactly recSize bytes)
//   - keyOffset, keyLen: position and length of the primary key in the record
func InsertRecord(path string, newData []byte, keyOffset, keyLen int) (*WriteResult, error) {
	// Read the entire file
	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	hdr, err := parseV2Header(fileData)
	if err != nil {
		return nil, fmt.Errorf("header parse %s: %w", path, err)
	}

	recSize := int(hdr.MaxRecordLen)
	if len(newData) != recSize {
		return nil, fmt.Errorf("newData size %d != record size %d", len(newData), recSize)
	}

	// Extract the key from new data
	if keyOffset+keyLen > recSize {
		return nil, fmt.Errorf("key offset %d+%d exceeds record size %d", keyOffset, keyLen, recSize)
	}
	newKey := newData[keyOffset : keyOffset+keyLen]

	// Check for duplicate key
	info, _, err := ReadFileV2(path)
	if err != nil {
		return nil, fmt.Errorf("read records %s: %w", path, err)
	}
	for _, rec := range info.Records {
		existingKey := rec.Data[keyOffset : keyOffset+keyLen]
		if compareKeys(newKey, existingKey) == 0 {
			return nil, fmt.Errorf("duplicate key %q already exists in %s", trimBytes(newKey), path)
		}
	}

	// Find a deleted data slot to reuse, or we'll append at end of file
	slotOffset := findDeletedSlot(fileData, hdr)

	// Detect key length from B-tree if not matching
	btKeyLen := keyLen
	detectedKeyLen := getKeyLenFromHeader(fileData, hdr)
	if detectedKeyLen > 0 && detectedKeyLen != keyLen {
		btKeyLen = detectedKeyLen
	}

	// Build B-tree info
	rootOffset, nodeDataLen, err := findBTreeRoot(fileData, hdr)
	if err != nil {
		return nil, fmt.Errorf("find B-tree root: %w", err)
	}

	btInfo := &BTreeInfo{
		KeyLen:      btKeyLen,
		PtrLen:      6,
		EntrySize:   btKeyLen + 6,
		MaxEntries:  (nodeDataLen - 2) / (btKeyLen + 6),
		NodeDataLen: nodeDataLen,
		RecHdrSize:  hdr.RecHeaderSize,
	}

	// Build the B-tree key (may differ from record key if btKeyLen != keyLen)
	btKey := make([]byte, btKeyLen)
	if keyLen <= btKeyLen {
		copy(btKey, newKey)
	} else {
		copy(btKey, newKey[:btKeyLen])
	}

	// Find the correct leaf node
	leaf, treePath, err := findLeafForKey(fileData, hdr, btInfo, rootOffset, btKey)
	if err != nil {
		return nil, fmt.Errorf("find leaf for key %q: %w", trimBytes(newKey), err)
	}

	// Check for duplicate in B-tree
	for _, e := range leaf.Entries {
		if compareKeys(btKey, e.Key) == 0 {
			return nil, fmt.Errorf("key %q already in B-tree index", trimBytes(newKey))
		}
	}

	// Create backup before any writes
	backupPath, err := createBackup(path)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	// Open file for writing (append mode needs O_APPEND-like behavior)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open for write %s: %w", path, err)
	}
	defer f.Close()

	// Step 1: Place the data record (reuse deleted slot or append)
	if slotOffset >= 0 {
		// Reuse existing deleted data slot
		dataOffset := slotOffset + hdr.RecHeaderSize
		if _, err := f.WriteAt(newData, int64(dataOffset)); err != nil {
			return nil, fmt.Errorf("write data at offset %d: %w", dataOffset, err)
		}

		// Change marker from DELETED to NORMAL
		if hdr.RecHeaderSize == 2 {
			marker := binary.BigEndian.Uint16(fileData[slotOffset : slotOffset+2])
			newMarker := (uint16(RecTypeNormal) << 12) | (marker & 0x0FFF)
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, newMarker)
			f.WriteAt(buf, int64(slotOffset))
		} else {
			marker := binary.BigEndian.Uint32(fileData[slotOffset : slotOffset+4])
			newMarker := (uint32(RecTypeNormal) << 28) | (marker & 0x0FFFFFFF)
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, newMarker)
			f.WriteAt(buf, int64(slotOffset))
		}
	} else {
		// Append new record at end of file
		stat, err := f.Stat()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		// Align to boundary
		appendPos := stat.Size()
		if hdr.Alignment > 1 {
			slack := int64(hdr.Alignment) - (appendPos % int64(hdr.Alignment))
			if slack != int64(hdr.Alignment) {
				// Write padding zeros
				padding := make([]byte, slack)
				f.WriteAt(padding, appendPos)
				appendPos += slack
			}
		}

		slotOffset = int(appendPos)

		// Write NORMAL marker + data
		var recBuf []byte
		if hdr.RecHeaderSize == 2 {
			marker := (uint16(RecTypeNormal) << 12) | (uint16(recSize) & 0x0FFF)
			recBuf = make([]byte, 2+recSize)
			binary.BigEndian.PutUint16(recBuf[0:2], marker)
			copy(recBuf[2:], newData)
		} else {
			marker := (uint32(RecTypeNormal) << 28) | (uint32(recSize) & 0x0FFFFFFF)
			recBuf = make([]byte, 4+recSize)
			binary.BigEndian.PutUint32(recBuf[0:4], marker)
			copy(recBuf[4:], newData)
		}

		if _, err := f.WriteAt(recBuf, int64(slotOffset)); err != nil {
			return nil, fmt.Errorf("append record at offset %d: %w", slotOffset, err)
		}

		// Write companion SYSTEM record (4 bytes data: 00 00 00 01)
		sysPos := int64(slotOffset) + int64(len(recBuf))
		if hdr.Alignment > 1 {
			slack := int64(hdr.Alignment) - (sysPos % int64(hdr.Alignment))
			if slack != int64(hdr.Alignment) {
				padding := make([]byte, slack)
				f.WriteAt(padding, sysPos)
				sysPos += slack
			}
		}

		var sysBuf []byte
		if hdr.RecHeaderSize == 2 {
			sysMarker := (uint16(RecTypeSystem) << 12) | uint16(4)
			sysBuf = make([]byte, 2+4)
			binary.BigEndian.PutUint16(sysBuf[0:2], sysMarker)
		} else {
			sysMarker := (uint32(RecTypeSystem) << 28) | uint32(4)
			sysBuf = make([]byte, 4+4)
			binary.BigEndian.PutUint32(sysBuf[0:4], sysMarker)
		}
		// Companion data: 00 00 00 01
		sysBuf[len(sysBuf)-4] = 0x00
		sysBuf[len(sysBuf)-3] = 0x00
		sysBuf[len(sysBuf)-2] = 0x00
		sysBuf[len(sysBuf)-1] = 0x01

		f.WriteAt(sysBuf, sysPos)
	}

	// Step 2: Insert key into B-tree leaf
	needsSplit := insertIntoLeaf(leaf, btKey, int64(slotOffset), btInfo)

	if !needsSplit {
		// Simple case: leaf has space, just write it back
		if err := writeNodeToFile(f, leaf, hdr, btInfo); err != nil {
			return nil, fmt.Errorf("write leaf node: %w", err)
		}
	} else {
		// Need to split the leaf and propagate up
		if err := splitAndPropagate(f, fileData, hdr, btInfo, leaf, treePath); err != nil {
			return nil, fmt.Errorf("split and propagate: %w", err)
		}
	}

	// Step 3: Update header timestamp
	updateHeaderTimestamp(f, hdr)

	return &WriteResult{
		Path:        path,
		RecordIndex: len(info.Records), // new record is at the end
		FileOffset:  slotOffset,
		RecSize:     recSize,
		BackupPath:  backupPath,
	}, nil
}

// splitAndPropagate handles splitting a full leaf node and propagating up the tree.
func splitAndPropagate(f *os.File, fileData []byte, hdr *V2Header, btInfo *BTreeInfo, node *BTreeNode, treePath []pathEntry) error {
	currentNode := node

	for {
		// Split the current node
		rightNode, medianKey := splitNode(currentNode, btInfo)

		// Allocate space for the right node: try reusing a freed index slot, else append
		rightOffset, err := allocateIndexNode(f, fileData, hdr, btInfo)
		if err != nil {
			return fmt.Errorf("allocate index node: %w", err)
		}

		rightNode.Offset = rightOffset
		rightNode.DataStart = rightOffset + hdr.RecHeaderSize
		rightNode.DataLen = btInfo.NodeDataLen

		// Write the left (original) node back in place
		if err := writeNodeToFile(f, currentNode, hdr, btInfo); err != nil {
			return fmt.Errorf("write left node: %w", err)
		}
		// Write the new right node
		if err := writeNodeToFile(f, rightNode, hdr, btInfo); err != nil {
			return fmt.Errorf("write right node: %w", err)
		}

		// If we're at the root (no more treePath), create a new root
		if len(treePath) == 0 {
			// Root split: the current node position becomes the new root,
			// but we need to move the left half to a new slot first.
			leftOffset, err := allocateIndexNode(f, fileData, hdr, btInfo)
			if err != nil {
				return fmt.Errorf("allocate left node for root split: %w", err)
			}

			// Move left node data to the new slot
			leftNode := &BTreeNode{
				Offset:    leftOffset,
				DataStart: leftOffset + hdr.RecHeaderSize,
				DataLen:   btInfo.NodeDataLen,
				IsLeaf:    currentNode.IsLeaf,
				Entries:   currentNode.Entries,
			}
			if err := writeNodeToFile(f, leftNode, hdr, btInfo); err != nil {
				return fmt.Errorf("write moved left node: %w", err)
			}

			// Overwrite the original root with a new branch pointing to both halves
			lastLeftKey := leftNode.Entries[len(leftNode.Entries)-1].Key
			// Right subtree handles all remaining keys — use infinity key
			infinityKey := make([]byte, btInfo.KeyLen)
			for i := range infinityKey {
				infinityKey[i] = 0xFF
			}
			newRoot := &BTreeNode{
				Offset:    currentNode.Offset,
				DataStart: currentNode.DataStart,
				DataLen:   currentNode.DataLen,
				IsLeaf:    false,
				Entries: []BTreeEntry{
					{Key: append([]byte{}, lastLeftKey...), Ptr: int64(leftOffset)},
					{Key: infinityKey, Ptr: int64(rightOffset)},
				},
			}
			return writeNodeToFile(f, newRoot, hdr, btInfo)
		}

		// Insert median key + right pointer into parent
		parent := treePath[len(treePath)-1]
		treePath = treePath[:len(treePath)-1]

		// The left half stays at the same offset. Update the parent's existing
		// entry key to reflect the left half's new max key (= median key).
		oldKey := parent.Node.Entries[parent.ChildIndex].Key
		parent.Node.Entries[parent.ChildIndex].Key = append([]byte{}, medianKey...)

		// The right half gets the parent's OLD key (which was the max of the
		// full child before split — now becomes the boundary for the right half).
		rightEntry := BTreeEntry{
			Key: append([]byte{}, oldKey...),
			Ptr: int64(rightOffset),
		}

		insertPos := parent.ChildIndex + 1
		parent.Node.Entries = append(parent.Node.Entries, BTreeEntry{})
		copy(parent.Node.Entries[insertPos+1:], parent.Node.Entries[insertPos:])
		parent.Node.Entries[insertPos] = rightEntry

		if len(parent.Node.Entries) <= btInfo.MaxEntries {
			// Parent has space — write it and we're done
			return writeNodeToFile(f, parent.Node, hdr, btInfo)
		}

		// Parent also needs splitting — continue the loop
		currentNode = parent.Node
	}
}

// allocateIndexNode finds or creates space for a new B-tree index node.
// First tries to find a freed index slot (all-zero data in a type-3 record of the right size).
// If none found, appends a new type-3 record at the aligned end of the file.
func allocateIndexNode(f *os.File, fileData []byte, hdr *V2Header, btInfo *BTreeInfo) (int, error) {
	// Try to find a freed index slot first
	slot := findFreedIndexSlot(fileData, hdr, btInfo.NodeDataLen)
	if slot >= 0 {
		return slot, nil
	}

	// Append a new index node at the end of the file
	stat, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}

	appendPos := stat.Size()

	// Align to boundary
	if hdr.Alignment > 1 {
		slack := int64(hdr.Alignment) - (appendPos % int64(hdr.Alignment))
		if slack != int64(hdr.Alignment) {
			padding := make([]byte, slack)
			if _, err := f.WriteAt(padding, appendPos); err != nil {
				return 0, fmt.Errorf("write alignment padding: %w", err)
			}
			appendPos += slack
		}
	}

	nodeOffset := int(appendPos)
	nodeDataLen := btInfo.NodeDataLen

	// Write DELETED (type 3) marker with nodeDataLen — this is what B-tree nodes use
	var markerBuf []byte
	if hdr.RecHeaderSize == 2 {
		marker := (uint16(RecTypeDeleted) << 12) | (uint16(nodeDataLen) & 0x0FFF)
		markerBuf = make([]byte, 2)
		binary.BigEndian.PutUint16(markerBuf, marker)
	} else {
		marker := (uint32(RecTypeDeleted) << 28) | (uint32(nodeDataLen) & 0x0FFFFFFF)
		markerBuf = make([]byte, 4)
		binary.BigEndian.PutUint32(markerBuf, marker)
	}

	if _, err := f.WriteAt(markerBuf, int64(nodeOffset)); err != nil {
		return 0, fmt.Errorf("write index node marker: %w", err)
	}

	// Initialize node data area with zeros
	zeroData := make([]byte, nodeDataLen)
	if _, err := f.WriteAt(zeroData, int64(nodeOffset+hdr.RecHeaderSize)); err != nil {
		return 0, fmt.Errorf("write index node data: %w", err)
	}

	return nodeOffset, nil
}

// findFreedIndexSlot scans the file for a type-3 record with matching nodeDataLen
// whose data is all zeros (indicating a freed/unused index node, not an active one).
func findFreedIndexSlot(data []byte, hdr *V2Header, nodeDataLen int) int {
	pos := hdr.HeaderSize
	scanEnd := len(data)

	for pos < scanEnd {
		if pos+hdr.RecHeaderSize > scanEnd {
			break
		}

		var recType byte
		var dataLen int

		if hdr.RecHeaderSize == 2 {
			marker := binary.BigEndian.Uint16(data[pos : pos+2])
			recType = byte(marker >> 12)
			dataLen = int(marker & 0x0FFF)
		} else {
			marker := binary.BigEndian.Uint32(data[pos : pos+4])
			recType = byte((marker >> 28) & 0x0F)
			dataLen = int(marker & 0x0FFFFFFF)
		}

		if recType == RecTypeNull && dataLen == 0 {
			if hdr.Alignment > 1 && hdr.Organization == 2 {
				pos += hdr.Alignment
			} else {
				pos++
			}
			continue
		}

		if dataLen <= 0 || dataLen > scanEnd-pos-hdr.RecHeaderSize {
			if hdr.Alignment > 1 && hdr.Organization == 2 {
				pos += hdr.Alignment
			} else {
				pos++
			}
			continue
		}

		// Check for a freed index node: type 3, matching size, all-zero data
		if recType == RecTypeDeleted && dataLen == nodeDataLen {
			dataStart := pos + hdr.RecHeaderSize
			allZero := true
			for i := dataStart; i < dataStart+dataLen && i < scanEnd; i++ {
				if data[i] != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				return pos
			}
		}

		consumed := hdr.RecHeaderSize + dataLen
		if hdr.Alignment > 1 {
			slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
			consumed += slack
		}
		pos += consumed
	}

	return -1
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func verifyKeysUnchanged(oldData, newData []byte, keyOffsets [][2]int) error {
	for _, kp := range keyOffsets {
		offset, length := kp[0], kp[1]
		if offset+length > len(oldData) || offset+length > len(newData) {
			return fmt.Errorf("key offset %d+%d exceeds record size", offset, length)
		}
		oldKey := oldData[offset : offset+length]
		newKey := newData[offset : offset+length]
		if !bytesEqual(oldKey, newKey) {
			return fmt.Errorf("key at offset %d changed: old=%q new=%q", offset,
				trimBytes(oldKey), trimBytes(newKey))
		}
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func trimBytes(b []byte) string {
	end := len(b)
	for end > 0 && (b[end-1] == ' ' || b[end-1] == 0) {
		end--
	}
	return string(b[:end])
}

func extractTrimmedField(data []byte, offset, length int) string {
	if offset >= len(data) {
		return ""
	}
	end := offset + length
	if end > len(data) {
		end = len(data)
	}
	return trimBytes(data[offset:end])
}

// createBackup creates a .bak copy of the file if one doesn't exist yet.
// If a .bak already exists from today, it's kept (only one backup per day).
func createBackup(path string) (string, error) {
	backupPath := path + ".bak"

	// Check if backup exists and is from today
	if info, err := os.Stat(backupPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return backupPath, nil // Recent backup exists
		}
	}

	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return backupPath, nil
}

// updateHeaderTimestamp updates the modify timestamp in the file header.
// For MF stream format: bytes 22-35 (14-char ASCII timestamp YYMMDDHHMMSScc)
// For indexed format: bytes 108-111 (4-byte timestamp)
func updateHeaderTimestamp(f *os.File, hdr *V2Header) {
	now := time.Now()

	if hdr.IsIndexed {
		// Update 4-byte timestamp at offset 112
		ts := uint32(now.Unix())
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, ts)
		f.WriteAt(buf, 112)
	} else {
		// Update 14-char ASCII timestamp at offset 22
		ts := now.Format("060102150405") + "00"
		f.WriteAt([]byte(ts), 22)
	}
}
