package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	records, recSize, err := isam.ReadIsamFile(`C:\SIIWI02\Z90PO`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Z90PO: records=%d recSize=%d EXTFH=%v\n\n", len(records), recSize, isam.ExtfhAvailable())

	shown := 0
	for idx, rec := range records {
		if shown >= 10 {
			break
		}
		empty := true
		for _, b := range rec {
			if b != 0 && b != ' ' {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		shown++

		s := strings.TrimRight(string(rec), "\x00 ")
		fmt.Printf("[%3d] %q\n", idx, s)
	}
}
