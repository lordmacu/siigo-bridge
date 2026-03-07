package isam

import (
	"fmt"
	"testing"
)

func TestOpenAndIterate(t *testing.T) {
	f, err := OpenIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("OpenIsamFile failed: %v", err)
	}
	defer f.Close()

	fmt.Printf("Z17: recSize=%d minRec=%d format=%d keys=%d varLen=%v\n",
		f.RecSize, f.MinRec, f.Format, f.NumKeys, f.IsVarLen)

	for _, k := range f.Keys {
		fmt.Printf("  Key[%d]: primary=%v dups=%v sparse=%v components=%d\n",
			k.Index, k.IsPrimary, k.AllowDups, k.IsSparse, k.CompCount)
	}

	count := 0
	err = f.ForEach(func(rec []byte) bool {
		if count < 5 {
			name := ExtractField(rec, 43, 40)
			nit := ExtractField(rec, 24, 10)
			fmt.Printf("  [%d] NIT=%s Name=%q\n", count, nit, name)
		}
		count++
		return true
	})
	if err != nil {
		t.Fatalf("ForEach failed: %v", err)
	}
	fmt.Printf("Total: %d records (%d EXTFH calls)\n", count, f.CallCount)
}

func TestReadIsamFileUnified(t *testing.T) {
	files := []string{`C:\DEMOS01\Z17`, `C:\DEMOS01\Z06`, `C:\DEMOS01\Z49`, `C:\DEMOS01\Z092016`}
	for _, path := range files {
		records, recSize, err := ReadIsamFile(path)
		if err != nil {
			fmt.Printf("%-20s ERROR: %v\n", path, err)
			continue
		}
		fmt.Printf("%-20s %d records (recSize=%d)\n", path, len(records), recSize)
	}
}

func TestCompareReaders(t *testing.T) {
	info, err := ReadFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	f, err := OpenIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("OpenIsamFile failed: %v", err)
	}
	defer f.Close()
	extRecords, _ := f.ReadAll()

	fmt.Printf("Binary: %d records | EXTFH: %d records\n", len(info.Records), len(extRecords))

	max := 5
	if max > len(info.Records) {
		max = len(info.Records)
	}
	for i := 0; i < max; i++ {
		binName := ExtractField(info.Records[i].Data, 43, 40)
		extName := ExtractField(extRecords[i], 43, 40)
		match := "OK"
		if binName != extName {
			match = "MISMATCH"
		}
		fmt.Printf("  %d: bin=%q ext=%q [%s]\n", i, binName, extName, match)
	}
}

func TestForEachEarlyStop(t *testing.T) {
	f, err := OpenIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("OpenIsamFile failed: %v", err)
	}
	defer f.Close()

	count := 0
	f.ForEach(func(rec []byte) bool {
		count++
		return count < 3
	})

	if count != 3 {
		t.Fatalf("expected 3, got %d", count)
	}
	fmt.Printf("ForEach stopped at %d records\n", count)
}

func TestFallbackReader(t *testing.T) {
	records, recSize, err := ReadIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("ReadIsamFile failed: %v", err)
	}
	if len(records) != 73 {
		t.Fatalf("expected 73 records, got %d", len(records))
	}
	fmt.Printf("ReadIsamFile: %d records, recSize=%d, extfh=%v\n",
		len(records), recSize, ExtfhAvailable())
}

func TestReadPrev(t *testing.T) {
	f, err := OpenIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("OpenIsamFile failed: %v", err)
	}
	defer f.Close()

	var names []string
	rec, _ := f.ReadFirst()
	if rec != nil {
		names = append(names, ExtractField(rec, 43, 40))
	}
	for i := 0; i < 2; i++ {
		rec, _ = f.ReadNext()
		if rec != nil {
			names = append(names, ExtractField(rec, 43, 40))
		}
	}

	rec, err = f.ReadPrev()
	if err != nil {
		t.Fatalf("ReadPrev failed: %v", err)
	}
	if rec != nil {
		prevName := ExtractField(rec, 43, 40)
		fmt.Printf("Forward: %v\n", names)
		fmt.Printf("Prev after 3rd: %q\n", prevName)
	}
}

func TestCount(t *testing.T) {
	f, err := OpenIsamFile(`C:\DEMOS01\Z17`)
	if err != nil {
		t.Fatalf("OpenIsamFile failed: %v", err)
	}
	defer f.Close()

	count, err := f.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 73 {
		t.Fatalf("expected 73, got %d", count)
	}
	fmt.Printf("Count: %d records\n", count)
}

func TestFileStatusDescriptions(t *testing.T) {
	tests := []struct {
		s1, s2 byte
		want   string
	}{
		{'0', '0', "successful"},
		{'1', '0', "end of file"},
		{'2', '3', "record not found"},
		{'3', '5', "file not found"},
		{'3', '9', "fixed file attribute conflict"},
		{'9', 65, "9/065 file locked by another process (RT065)"},
		{'9', 68, "9/068 record locked by another process (RT068)"},
		{'9', 71, "9/071 bad indexed file format (RT071)"},
	}

	for _, tt := range tests {
		fs := FileStatus{tt.s1, tt.s2}
		desc := fs.Description()
		if desc != tt.want {
			t.Errorf("FileStatus{%c,%v}.Description() = %q, want %q", tt.s1, tt.s2, desc, tt.want)
		}
	}
	fmt.Println("All FileStatus descriptions match")
}

func TestDecodeFieldTrimLeft(t *testing.T) {
	rec := []byte("00001234  NOMBRE TEST           ")
	result := DecodeFieldTrimLeft(rec, 0, 8)
	if result != "1234" {
		t.Fatalf("expected '1234', got %q", result)
	}
	fmt.Printf("DecodeFieldTrimLeft: %q\n", result)
}

func TestExtfhDLLPath(t *testing.T) {
	path := ExtfhDLLPath()
	avail := ExtfhAvailable()
	fmt.Printf("EXTFH available=%v DLL=%s\n", avail, path)
	if avail && path == "" {
		t.Fatal("available but no path")
	}
}
