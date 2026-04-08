package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dataPath := `C:\SIIWI02\`

	files := []string{
		"Z17", "Z49", "Z03", "Z06",
		"Z092016", "Z042016", "Z112016", "Z182016",
		"Z252016", "Z282016", "Z052016", "Z072016",
		"Z272016", "Z262016",
		"Z082016A", "ZDANE", "ZICA", "ZPILA",
	}

	fmt.Println("=================================================================")
	fmt.Println("  ISAM FILE HEADER INSPECTION (128-byte header analysis)")
	fmt.Println("=================================================================")

	for _, name := range files {
		path := filepath.Join(dataPath, name)
		inspectHeader(path)
	}
}

func inspectHeader(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("\n%-12s: ERROR %v\n", filepath.Base(path), err)
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	fileSize := stat.Size()

	// Read first 256 bytes (header + some data)
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	if n < 128 {
		fmt.Printf("\n%-12s: TOO SMALL (%d bytes)\n", filepath.Base(path), n)
		return
	}

	fmt.Printf("\n%-12s (size: %d bytes)\n", filepath.Base(path), fileSize)

	// Magic bytes at offset 0-3
	magic := buf[0:4]
	magicHex := fmt.Sprintf("%02X %02X %02X %02X", magic[0], magic[1], magic[2], magic[3])
	longRecords := false
	magicType := "UNKNOWN"
	if magic[0] == 0x30 && magic[1] == 0x7E && magic[2] == 0x00 && magic[3] == 0x00 {
		magicType = "SHORT_RECORDS (2-byte headers)"
		longRecords = false
	} else if magic[0] == 0x30 && magic[1] == 0x00 && magic[2] == 0x00 && magic[3] == 0x7C {
		magicType = "LONG_RECORDS (4-byte headers)"
		longRecords = true
	} else if magic[0] == 0x33 && magic[1] == 0xFE {
		magicType = "MF_INDEXED (0x33FE)"
	} else if (magic[0] & 0xF0) == 0x30 {
		magicType = fmt.Sprintf("MF_VARIANT (0x%02X%02X)", magic[0], magic[1])
	}
	fmt.Printf("  Magic[0:4]:       %s -> %s\n", magicHex, magicType)
	fmt.Printf("  Long records:     %v\n", longRecords)

	// DB sequence number at offset 4-5
	dbSeq := binary.BigEndian.Uint16(buf[4:6])
	fmt.Printf("  DB seq[4:6]:      %d\n", dbSeq)

	// Integrity flag at offset 6-7
	integrity := binary.BigEndian.Uint16(buf[6:8])
	integrityStr := "OK"
	if integrity != 0 {
		integrityStr = "CORRUPT!"
	}
	fmt.Printf("  Integrity[6:8]:   %d (%s)\n", integrity, integrityStr)

	// Creation date at offset 8-21 (14 chars YYMMDDHHMMSS00)
	creationDate := string(buf[8:22])
	fmt.Printf("  Created[8:22]:    %q\n", creationDate)

	// Reserved/modification date at offset 22-35
	modDate := string(buf[22:36])
	fmt.Printf("  Modified[22:36]:  %q\n", modDate)

	// Reserved marker at offset 36-37 (expected 0x00 0x3E)
	fmt.Printf("  Reserved[36:38]:  %02X %02X (expect 00 3E)\n", buf[36], buf[37])

	// Organization at offset 39
	org := buf[39]
	orgStr := map[byte]string{0: "unknown", 1: "Sequential", 2: "Indexed", 3: "Relative"}[org]
	if orgStr == "" {
		orgStr = fmt.Sprintf("0x%02X", org)
	}
	fmt.Printf("  Org[39]:          %d (%s)\n", org, orgStr)

	// Data compression at offset 41
	compression := buf[41]
	compStr := "None"
	if compression == 1 {
		compStr = "CBLDC001"
	} else if compression >= 0x80 {
		compStr = "User-defined"
	}
	fmt.Printf("  Compression[41]:  %d (%s)\n", compression, compStr)

	// Index type at offset 43 (IDXFORMAT)
	idxType := buf[43]
	fmt.Printf("  IDXFORMAT[43]:    %d\n", idxType)

	// Recording mode at offset 48
	recMode := buf[48]
	modeStr := "Fixed"
	if recMode == 1 {
		modeStr = "Variable"
	}
	fmt.Printf("  RecMode[48]:      %d (%s)\n", recMode, modeStr)

	// Max record length at offset 54-57 (big-endian 32-bit)
	maxRecLen := binary.BigEndian.Uint32(buf[54:58])
	fmt.Printf("  MaxRecLen[54:58]: %d\n", maxRecLen)

	// Min record length at offset 58-61 (big-endian 32-bit)
	minRecLen := binary.BigEndian.Uint32(buf[58:62])
	fmt.Printf("  MinRecLen[58:62]: %d\n", minRecLen)

	// Also check our old offsets for comparison
	oldMagic := binary.BigEndian.Uint16(buf[0:2])
	oldRecSize := binary.BigEndian.Uint16(buf[0x38:0x3A])
	oldExpected := binary.BigEndian.Uint32(buf[0x40:0x44])
	fmt.Printf("  [OLD] magic@0:    0x%04X\n", oldMagic)
	fmt.Printf("  [OLD] recSize@38: %d\n", oldRecSize)
	fmt.Printf("  [OLD] expected@40:%d\n", oldExpected)

	// For indexed files, check offset 108 (handler version)
	if idxType > 0 || org == 2 {
		handlerVer := binary.BigEndian.Uint32(buf[108:112])
		fmt.Printf("  HandlerVer[108]:  %d\n", handlerVer)

		// Logical end at offset 120 (8-byte big-endian)
		logEnd := binary.BigEndian.Uint64(buf[120:128])
		fmt.Printf("  LogicalEnd[120]:  %d\n", logEnd)
	}

	// Hex dump first 128 bytes in rows of 16
	fmt.Println("  --- Header hex dump ---")
	for row := 0; row < 128; row += 16 {
		hexPart := ""
		ascPart := ""
		for col := 0; col < 16 && row+col < 128; col++ {
			b := buf[row+col]
			hexPart += fmt.Sprintf("%02X ", b)
			if b >= 0x20 && b <= 0x7E {
				ascPart += string(b)
			} else {
				ascPart += "."
			}
		}
		fmt.Printf("  %04X: %-48s |%s|\n", row, hexPart, ascPart)
	}

	// Show first few records after header (offset 128+)
	fmt.Println("  --- First bytes after header (128-255) ---")
	postHeader := buf[128:]
	hexPart := ""
	ascPart := ""
	for i := 0; i < len(postHeader) && i < 64; i++ {
		b := postHeader[i]
		hexPart += fmt.Sprintf("%02X ", b)
		if b >= 0x20 && b <= 0x7E {
			ascPart += string(b)
		} else {
			ascPart += "."
		}
		if (i+1)%16 == 0 {
			fmt.Printf("  %04X: %-48s |%s|\n", 128+i-15, hexPart, ascPart)
			hexPart = ""
			ascPart = ""
		}
	}
	if hexPart != "" {
		fmt.Printf("  %04X: %-48s |%s|\n", 128+len(postHeader)-len(strings.Fields(hexPart)), hexPart, ascPart)
	}
}
