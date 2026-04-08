package main

import (
	"fmt"
	"siigo-common/isam"
)

func main() {
	recs, stats, err := isam.ReadFileV2WithStats(`C:\SIIWI02\ZDANE`)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	fmt.Printf("Records: %d\n", len(recs))
	fmt.Printf("Stats: total=%d deleted=%d null=%d\n", stats.TotalRecords, stats.DeletedCount, stats.NullCount)
	fmt.Printf("Header: indexed=%v hdrSize=%d alignment=%d recHdr=%d maxRec=%d idxFmt=%d\n",
		stats.Header.IsIndexed, stats.Header.HeaderSize, stats.Header.Alignment,
		stats.Header.RecHeaderSize, stats.Header.MaxRecordLen, stats.Header.IdxFormat)
	fmt.Printf("DataTypes: %v\n", stats.DataTypes)

	// Show first 3 records offset and first bytes
	info, hdr, _ := isam.ReadFileV2(`C:\SIIWI02\ZDANE`)
	fmt.Printf("\nReadFileV2: %d records (indexed=%v hdrSize=%d)\n", len(info.Records), hdr.IsIndexed, hdr.HeaderSize)
	for i := 0; i < 5 && i < len(info.Records); i++ {
		r := info.Records[i]
		fmt.Printf("  [%d] offset=%d hex=% X\n", i, r.Offset, r.Data[:10])
	}
}
