package isam

import (
	"fmt"
	"testing"
)

func TestReadFileExtfh(t *testing.T) {
	records, err := ReadFileExtfh(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("ReadFileExtfh failed: %v", err)
	}

	fmt.Printf("Total records read via EXTFH: %d\n", len(records))

	for i, rec := range records {
		if i >= 10 {
			break
		}
		// Try multiple offsets to find the name
		for _, off := range []int{20, 30, 35, 38, 40, 42, 45, 50} {
			name := DecodeExtfhField(rec.Data, off, 40)
			if len(name) > 5 {
				fmt.Printf("Record %d off=%d: %q\n", i, off, name)
			}
		}
		fmt.Println("---")
	}
}

func TestReadMultipleFiles(t *testing.T) {
	files := []string{`C:\DEMOS01\Z17`, `C:\DEMOS01\Z06`, `C:\DEMOS01\Z49`, `C:\DEMOS01\Z092016`}
	for _, f := range files {
		records, err := ReadFileExtfh(f)
		if err != nil {
			fmt.Printf("%-20s ERROR: %v\n", f, err)
			continue
		}
		fmt.Printf("%-20s %d records\n", f, len(records))
	}
}

func TestCompareReaders(t *testing.T) {
	// Read with original binary reader
	info, err := ReadFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Read with EXTFH
	extRecords, err := ReadFileExtfh(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("ReadFileExtfh failed: %v", err)
	}

	fmt.Printf("Binary reader: %d records\n", len(info.Records))
	fmt.Printf("EXTFH reader:  %d records\n", len(extRecords))

	// Show first 5 from each
	fmt.Println("\n=== Binary Reader ===")
	for i := 0; i < 5 && i < len(info.Records); i++ {
		name := ExtractField(info.Records[i].Data, 38, 40)
		fmt.Printf("  %d: %q\n", i, name)
	}
	fmt.Println("\n=== EXTFH Reader ===")
	for i := 0; i < 5 && i < len(extRecords); i++ {
		name := DecodeExtfhField(extRecords[i].Data, 38, 40)
		fmt.Printf("  %d: %q\n", i, name)
	}
}
