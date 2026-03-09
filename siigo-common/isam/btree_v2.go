package isam

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

// ---------------------------------------------------------------------------
// btree_v2.go — Pure Go B-tree reader/writer for MF ISAM IDXFORMAT 8
//
// Supports reading and modifying the B-tree index embedded in ISAM files.
// Used by InsertRecord to maintain index integrity.
//
// B-tree format (0x33FE and variant 0x30xx):
//   - Index nodes are stored as type 3 (DELETED marker) records in the stream
//   - Node data: 2-byte header + N entries of (key + 6-byte pointer)
//   - Header byte[0] & 0x80 = leaf flag
//   - Leaf pointers → data record file offsets
//   - Branch pointers → child index node file offsets
//   - Max entries per node = (nodeDataLen - 2) / entrySize
// ---------------------------------------------------------------------------

// BTreeNode represents a parsed B-tree index node
type BTreeNode struct {
	Offset    int  // File offset of the node's record marker
	DataStart int  // File offset where node data begins (after marker)
	DataLen   int  // Length of node data
	IsLeaf    bool // true if leaf node (byte[0] & 0x80)

	Entries []BTreeEntry // Key-pointer entries
}

// BTreeEntry represents a single key-pointer pair in a B-tree node
type BTreeEntry struct {
	Key    []byte // Key bytes (fixed length, e.g. 5 bytes for ZDANE)
	Ptr    int64  // 6-byte big-endian file offset (leaf→data, branch→child node)
	KeyStr string // Trimmed string representation of key
}

// BTreeInfo holds B-tree metadata for a file
type BTreeInfo struct {
	KeyLen     int // Key length in bytes
	PtrLen     int // Pointer length (always 6)
	EntrySize  int // KeyLen + PtrLen
	MaxEntries int // Max entries per node
	NodeDataLen int // Data length of index nodes
	RecHdrSize int // Record header size (2 or 4)
}

// findBTreeRoot finds the root B-tree node (first type-3 record in the stream)
// For 0x33FE files, the root is the first type-3 record after the header.
// For variant 0x30xx files, the root is the first type-3 record after 0x800.
func findBTreeRoot(data []byte, hdr *V2Header) (int, int, error) {
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

		if recType == 0 && dataLen == 0 {
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

		// Type 3 records are B-tree index nodes
		if recType == RecTypeDeleted {
			return pos, dataLen, nil
		}

		consumed := hdr.RecHeaderSize + dataLen
		if hdr.Alignment > 1 {
			slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
			consumed += slack
		}
		pos += consumed
	}

	return 0, 0, fmt.Errorf("no B-tree root found")
}

// parseBTreeNode parses a B-tree node from file data
func parseBTreeNode(data []byte, offset int, hdr *V2Header, btInfo *BTreeInfo) (*BTreeNode, error) {
	if offset+hdr.RecHeaderSize > len(data) {
		return nil, fmt.Errorf("offset %d beyond file end", offset)
	}

	var dataLen int
	if hdr.RecHeaderSize == 2 {
		marker := binary.BigEndian.Uint16(data[offset : offset+2])
		recType := byte(marker >> 12)
		dataLen = int(marker & 0x0FFF)
		if recType != RecTypeDeleted {
			return nil, fmt.Errorf("not a B-tree node at offset %d (type=%d)", offset, recType)
		}
	} else {
		marker := binary.BigEndian.Uint32(data[offset : offset+4])
		recType := byte((marker >> 28) & 0x0F)
		dataLen = int(marker & 0x0FFFFFFF)
		if recType != RecTypeDeleted {
			return nil, fmt.Errorf("not a B-tree node at offset %d (type=%d)", offset, recType)
		}
	}

	dataStart := offset + hdr.RecHeaderSize
	if dataStart+dataLen > len(data) {
		return nil, fmt.Errorf("node data exceeds file at offset %d", offset)
	}

	nodeData := data[dataStart : dataStart+dataLen]
	if len(nodeData) < 2 {
		return nil, fmt.Errorf("node too small at offset %d", offset)
	}

	node := &BTreeNode{
		Offset:    offset,
		DataStart: dataStart,
		DataLen:   dataLen,
		IsLeaf:    (nodeData[0] & 0x80) != 0,
	}

	// Parse entries: skip 2-byte header, read key+ptr pairs
	pos := 2
	for pos+btInfo.EntrySize <= len(nodeData) {
		key := nodeData[pos : pos+btInfo.KeyLen]
		// Stop at all-zero key (end of entries)
		allZero := true
		for _, b := range key {
			if b != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			break
		}

		ptrBytes := nodeData[pos+btInfo.KeyLen : pos+btInfo.KeyLen+btInfo.PtrLen]
		ptr := int64(0)
		for _, b := range ptrBytes {
			ptr = (ptr << 8) | int64(b)
		}

		node.Entries = append(node.Entries, BTreeEntry{
			Key:    append([]byte{}, key...),
			Ptr:    ptr,
			KeyStr: trimBytes(key),
		})

		pos += btInfo.EntrySize

		// For branch nodes, the infinity key (all 0xFF) is the last valid entry.
		// Any data after it is stale leftover from previous node usage.
		if !node.IsLeaf {
			allFF := true
			for _, b := range key {
				if b != 0xFF {
					allFF = false
					break
				}
			}
			if allFF {
				break
			}
		}
	}

	return node, nil
}

// findLeafForKey navigates the B-tree to find the leaf node where a key should be inserted.
// Returns the leaf node and the path of (node, entryIndex) from root to leaf.
func findLeafForKey(data []byte, hdr *V2Header, btInfo *BTreeInfo, rootOffset int, key []byte) (*BTreeNode, []pathEntry, error) {
	var path []pathEntry
	offset := rootOffset

	for depth := 0; depth < 20; depth++ { // max depth guard
		node, err := parseBTreeNode(data, offset, hdr, btInfo)
		if err != nil {
			return nil, nil, fmt.Errorf("parse node at %d: %w", offset, err)
		}

		if node.IsLeaf {
			return node, path, nil
		}

		// Find which child to follow
		// Entries are sorted by key. Find the first entry where key <= entry.key
		childIdx := len(node.Entries) - 1 // default: last child
		for i, e := range node.Entries {
			if compareKeys(key, e.Key) <= 0 {
				childIdx = i
				break
			}
		}

		path = append(path, pathEntry{Node: node, ChildIndex: childIdx})
		offset = int(node.Entries[childIdx].Ptr)
	}

	return nil, nil, fmt.Errorf("B-tree depth exceeded maximum")
}

type pathEntry struct {
	Node       *BTreeNode
	ChildIndex int // Which entry's pointer was followed
}

// compareKeys compares two key byte slices lexicographically
func compareKeys(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// insertIntoLeaf inserts a key-pointer entry into a leaf node.
// If the node has space, it inserts in-place. Returns true if split is needed.
func insertIntoLeaf(node *BTreeNode, key []byte, ptr int64, btInfo *BTreeInfo) (needsSplit bool) {
	entry := BTreeEntry{
		Key:    append([]byte{}, key...),
		Ptr:    ptr,
		KeyStr: trimBytes(key),
	}

	// Find insertion position (keep sorted order)
	insertPos := sort.Search(len(node.Entries), func(i int) bool {
		return compareKeys(node.Entries[i].Key, key) >= 0
	})

	// Insert at position
	node.Entries = append(node.Entries, BTreeEntry{})
	copy(node.Entries[insertPos+1:], node.Entries[insertPos:])
	node.Entries[insertPos] = entry

	return len(node.Entries) > btInfo.MaxEntries
}

// splitNode splits a full node into two halves.
// Returns the new right node and the median key that goes up to the parent.
func splitNode(node *BTreeNode, btInfo *BTreeInfo) (*BTreeNode, []byte) {
	mid := len(node.Entries) / 2
	medianKey := append([]byte{}, node.Entries[mid].Key...)

	// Right node gets entries after the median
	right := &BTreeNode{
		IsLeaf:  node.IsLeaf,
		Entries: append([]BTreeEntry{}, node.Entries[mid+1:]...),
	}

	// Left node (original) keeps entries up to and including median
	node.Entries = node.Entries[:mid+1]

	return right, medianKey
}

// serializeNode writes a B-tree node's entries back to a byte buffer
func serializeNode(node *BTreeNode, btInfo *BTreeInfo) []byte {
	buf := make([]byte, btInfo.NodeDataLen)

	// Header
	if node.IsLeaf {
		buf[0] = 0x80 | byte(len(node.Entries)&0x7F)
	} else {
		buf[0] = byte(len(node.Entries) & 0xFF)
	}
	buf[1] = byte(len(node.Entries))

	pos := 2
	for _, e := range node.Entries {
		// Write key (pad with zeros if shorter)
		copy(buf[pos:pos+btInfo.KeyLen], e.Key)
		pos += btInfo.KeyLen

		// Write 6-byte big-endian pointer
		for i := btInfo.PtrLen - 1; i >= 0; i-- {
			buf[pos+i] = byte(e.Ptr & 0xFF)
			e.Ptr >>= 8
		}
		pos += btInfo.PtrLen
	}

	return buf
}

// writeNodeToFile writes a serialized node back to the file at the node's offset
func writeNodeToFile(f *os.File, node *BTreeNode, hdr *V2Header, btInfo *BTreeInfo) error {
	nodeBytes := serializeNode(node, btInfo)
	_, err := f.WriteAt(nodeBytes, int64(node.DataStart))
	return err
}

// findDeletedSlot finds a DELETED data record slot that can be reused.
// Returns the file offset of the deleted record's marker, or -1 if none found.
func findDeletedSlot(data []byte, hdr *V2Header) int {
	pos := hdr.HeaderSize
	scanEnd := len(data)
	recSize := int(hdr.MaxRecordLen)

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

		if recType == 0 && dataLen == 0 {
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

		// Look for deleted data records (type 3 with dataLen == recSize)
		// B-tree nodes are also type 3 but have a different size (1022 typically)
		if recType == RecTypeDeleted && dataLen == recSize {
			return pos
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

// getKeyLenFromHeader attempts to determine the key length from the B-tree structure.
// It reads the first leaf node and compares its entries with data records.
func getKeyLenFromHeader(data []byte, hdr *V2Header) int {
	// For known file types, we can calculate from the index node size
	// Node data = 1022 (common), entry_size = key_len + 6
	// We need to figure out key_len from the node structure

	// Find root node
	rootOffset, _, err := findBTreeRoot(data, hdr)
	if err != nil {
		return 0
	}

	// Read root node header to get node data length
	var nodeDataLen int
	if hdr.RecHeaderSize == 2 {
		marker := binary.BigEndian.Uint16(data[rootOffset : rootOffset+2])
		nodeDataLen = int(marker & 0x0FFF)
	} else {
		marker := binary.BigEndian.Uint32(data[rootOffset : rootOffset+4])
		nodeDataLen = int(marker & 0x0FFFFFFF)
	}

	nodeData := data[rootOffset+hdr.RecHeaderSize : rootOffset+hdr.RecHeaderSize+nodeDataLen]

	// Walk entries to find key length by checking where pointers make sense
	// Try key lengths from 1 to 50
	for tryKeyLen := 1; tryKeyLen <= 50; tryKeyLen++ {
		entrySize := tryKeyLen + 6
		if (nodeDataLen-2)%entrySize != 0 && (nodeDataLen-2)/entrySize < 2 {
			continue
		}

		// Check first few entries
		valid := true
		checked := 0
		pos := 2
		for pos+entrySize <= nodeDataLen && checked < 3 {
			key := nodeData[pos : pos+tryKeyLen]
			allZero := true
			for _, b := range key {
				if b != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				break
			}

			ptrBytes := nodeData[pos+tryKeyLen : pos+tryKeyLen+6]
			ptr := int64(0)
			for _, b := range ptrBytes {
				ptr = (ptr << 8) | int64(b)
			}

			// Pointer should be within file bounds and point to a valid record
			if ptr < int64(hdr.HeaderSize) || ptr >= int64(len(data)-2) {
				valid = false
				break
			}

			// Check that the target has a valid marker
			if hdr.RecHeaderSize == 2 {
				m := binary.BigEndian.Uint16(data[ptr : ptr+2])
				rt := byte(m >> 12)
				if rt == 0 || rt > 8 {
					valid = false
					break
				}
			}

			checked++
			pos += entrySize
		}

		if valid && checked >= 2 {
			return tryKeyLen
		}
	}

	return 0
}
