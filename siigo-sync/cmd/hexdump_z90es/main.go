package main

import (
	"fmt"
	"siigo-common/isam"
	"strings"
)

func main() {
	records, recSize, err := isam.ReadIsamFile(`C:\DEMOS01\Z90ES`)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	fmt.Printf("Z90ES: records=%d recSize=%d\n\n", len(records), recSize)

	shown := 0
	for idx, rec := range records {
		if shown >= 15 {
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
		if len(s) > 120 {
			s = s[:120]
		}
		fmt.Printf("[%3d] %q\n", idx, s)
	}
}
