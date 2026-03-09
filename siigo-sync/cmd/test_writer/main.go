package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"siigo-common/isam"
)

func main() {
	// Test REWRITE on a copy of ZDANE (small file, 256-byte records)
	// ZDANE: key=codigo@0(5), nombre@5(40)
	srcPath := `C:\DEMOS01\ZDANE`
	testPath := `C:\tmp\ZDANE_TEST`

	// Step 1: Copy ZDANE to temp
	fmt.Println("=== ISAM REWRITE TEST (Pure Go) ===")
	fmt.Println()

	if err := copyFile(srcPath, testPath); err != nil {
		fmt.Printf("ERROR copying: %v\n", err)
		return
	}
	defer os.Remove(testPath)
	defer os.Remove(testPath + ".bak")
	fmt.Printf("1. Copied %s -> %s\n", srcPath, testPath)

	// Step 2: Read with V2 reader
	info, hdr, err := isam.ReadFileV2(testPath)
	if err != nil {
		fmt.Printf("ERROR reading: %v\n", err)
		return
	}
	fmt.Printf("2. V2 Reader: %d records (recSize=%d, indexed=%v, recHdrSize=%d)\n",
		len(info.Records), hdr.MaxRecordLen, hdr.IsIndexed, hdr.RecHeaderSize)

	if len(info.Records) < 3 {
		fmt.Println("ERROR: not enough records")
		return
	}

	// Pick record #2 (3rd record)
	targetIdx := 2
	rec := info.Records[targetIdx]
	recSize := int(hdr.MaxRecordLen)

	// For ZDANE indexed format, the first byte might not be printable ASCII
	// Let's show hex of the first 10 bytes to understand the format
	fmt.Printf("3. Target record #%d (offset=%d):\n", targetIdx, rec.Offset)
	fmt.Printf("   first 10 bytes hex: % X\n", rec.Data[:10])
	fmt.Printf("   first 50 bytes str: %q\n", string(rec.Data[:50]))

	// ZDANE: codigo@0(5) nombre@5(40) — but check if this is the right layout
	// From earlier analysis: ZDANE records start with codigo(5 ASCII digits) + nombre(40 chars)
	oldCodigo := strings.TrimRight(string(rec.Data[0:5]), " \x00")
	oldNombre := strings.TrimRight(string(rec.Data[5:45]), " \x00")
	fmt.Printf("   codigo=%q nombre=%q\n", oldCodigo, oldNombre)

	// Step 3: Modify the nombre field (offset 5, length 40)
	newNombre := "TEST REWRITE GO PURO"
	newData := make([]byte, recSize)
	copy(newData, rec.Data)

	// Pad nombre with spaces (COBOL-style)
	nombreBytes := make([]byte, 40)
	for i := range nombreBytes {
		nombreBytes[i] = ' '
	}
	copy(nombreBytes, []byte(newNombre))
	copy(newData[5:45], nombreBytes)

	// Verify key is unchanged
	newCodigo := strings.TrimRight(string(newData[0:5]), " \x00")
	fmt.Printf("4. New data: codigo=%q (unchanged=%v) nombre=%q\n",
		newCodigo, newCodigo == oldCodigo, newNombre)

	// Step 4: Rewrite!
	keyOffsets := [][2]int{{0, 5}} // codigo at offset 0, length 5
	result, err := isam.RewriteRecord(testPath, targetIdx, newData, keyOffsets)
	if err != nil {
		fmt.Printf("ERROR rewrite: %v\n", err)
		return
	}
	fmt.Printf("5. Rewrite OK! fileOffset=%d backup=%s\n", result.FileOffset, result.BackupPath)

	// Step 5: Re-read and verify
	info2, _, err := isam.ReadFileV2(testPath)
	if err != nil {
		fmt.Printf("ERROR re-read: %v\n", err)
		return
	}

	verifyRec := info2.Records[targetIdx]
	verifyCodigo := strings.TrimRight(string(verifyRec.Data[0:5]), " \x00")
	verifyNombre := strings.TrimRight(string(verifyRec.Data[5:45]), " \x00")
	fmt.Printf("6. Verify after rewrite:\n")
	fmt.Printf("   codigo=%q nombre=%q\n", verifyCodigo, verifyNombre)

	if verifyCodigo != oldCodigo {
		fmt.Printf("   FAIL: codigo changed!\n")
	} else if verifyNombre != newNombre {
		fmt.Printf("   FAIL: nombre not updated (got %q)\n", verifyNombre)
	} else {
		fmt.Printf("   PASS: record updated correctly!\n")
	}

	// Step 6: Verify other records are untouched
	fmt.Println("7. Verifying other records untouched...")
	unchanged := 0
	changed := 0
	for i := range info2.Records {
		if i == targetIdx {
			continue
		}
		if i < len(info.Records) {
			if string(info.Records[i].Data) == string(info2.Records[i].Data) {
				unchanged++
			} else {
				changed++
				if changed <= 3 {
					fmt.Printf("   WARNING: record #%d changed!\n", i)
				}
			}
		}
	}
	fmt.Printf("   %d unchanged, %d changed (expected 0 changed)\n", unchanged, changed)

	// Step 7: Test RewriteFields (patch individual fields)
	fmt.Println("8. Testing RewriteFields (field-level patch)...")
	patchNombre := make([]byte, 40)
	for i := range patchNombre {
		patchNombre[i] = ' '
	}
	copy(patchNombre, []byte("CAMPO PARCHEADO"))

	fields := map[int][]byte{
		5: patchNombre, // nombre at offset 5
	}
	result2, err := isam.RewriteFields(testPath, targetIdx, fields, keyOffsets)
	if err != nil {
		fmt.Printf("   ERROR: %v\n", err)
		return
	}

	info3, _, _ := isam.ReadFileV2(testPath)
	patchedNombre := strings.TrimRight(string(info3.Records[targetIdx].Data[5:45]), " \x00")
	fmt.Printf("   After patch: nombre=%q (offset=%d)\n", patchedNombre, result2.FileOffset)
	if patchedNombre == "CAMPO PARCHEADO" {
		fmt.Printf("   PASS: field patch works!\n")
	} else {
		fmt.Printf("   FAIL: expected 'CAMPO PARCHEADO', got %q\n", patchedNombre)
	}

	// Step 8: Test key change prevention
	fmt.Println("9. Testing key change prevention...")
	badData := make([]byte, recSize)
	copy(badData, info3.Records[targetIdx].Data)
	copy(badData[0:5], []byte("99999")) // Change the key!
	_, err = isam.RewriteRecord(testPath, targetIdx, badData, keyOffsets)
	if err != nil {
		fmt.Printf("   PASS: correctly rejected key change: %v\n", err)
	} else {
		fmt.Printf("   FAIL: should have rejected key change!\n")
	}

	// Step 9: Verify with V2 reader that record count is preserved
	fmt.Println("10. Verifying file integrity...")
	records, _, v2err := isam.ReadFileV2WithStats(testPath)
	if v2err != nil {
		fmt.Printf("   FAIL: V2 reader error: %v\n", v2err)
	} else {
		fmt.Printf("   PASS: V2 reads %d records (original had %d)\n", len(records), len(info.Records))
	}

	// Step 10: Test RewriteFieldsByKey
	fmt.Println("11. Testing RewriteFieldsByKey...")
	patchNombre2 := make([]byte, 40)
	for i := range patchNombre2 {
		patchNombre2[i] = ' '
	}
	copy(patchNombre2, []byte("BY KEY LOOKUP"))
	fields2 := map[int][]byte{5: patchNombre2}
	_, err = isam.RewriteFieldsByKey(testPath, 0, 5, oldCodigo, fields2)
	if err != nil {
		fmt.Printf("   ERROR: %v\n", err)
	} else {
		info4, _, _ := isam.ReadFileV2(testPath)
		n := strings.TrimRight(string(info4.Records[targetIdx].Data[5:45]), " \x00")
		if n == "BY KEY LOOKUP" {
			fmt.Printf("   PASS: RewriteFieldsByKey works! nombre=%q\n", n)
		} else {
			fmt.Printf("   FAIL: got %q\n", n)
		}
	}

	// Step 11: Diff vs original
	origInfo, _, _ := isam.ReadFileV2(srcPath)
	info5, _, _ := isam.ReadFileV2(testPath)
	diffCount := 0
	for i := 0; i < len(origInfo.Records) && i < len(info5.Records); i++ {
		if string(origInfo.Records[i].Data) != string(info5.Records[i].Data) {
			diffCount++
		}
	}
	fmt.Printf("12. Diff vs original: %d records differ (expected 1)\n", diffCount)

	// =====================================================================
	// DELETE TESTS
	// =====================================================================
	fmt.Println("\n=== ISAM DELETE TEST (Pure Go) ===")
	fmt.Println()

	// Fresh copy for delete tests
	testPathDel := `C:\tmp\ZDANE_DEL_TEST`
	if err := copyFile(srcPath, testPathDel); err != nil {
		fmt.Printf("ERROR copying for delete: %v\n", err)
		return
	}
	defer os.Remove(testPathDel)
	defer os.Remove(testPathDel + ".bak")

	// Read original state
	infoD, _, err := isam.ReadFileV2(testPathDel)
	if err != nil {
		fmt.Printf("ERROR reading: %v\n", err)
		return
	}
	origCount := len(infoD.Records)
	fmt.Printf("13. Fresh copy: %d records\n", origCount)

	// Pick record #5 to delete
	delIdx := 5
	delRec := infoD.Records[delIdx]
	delCodigo := strings.TrimRight(string(delRec.Data[0:5]), " \x00")
	delNombre := strings.TrimRight(string(delRec.Data[5:45]), " \x00")
	fmt.Printf("14. Target for delete: record #%d codigo=%q nombre=%q\n", delIdx, delCodigo, delNombre)

	// Delete by index
	delResult, err := isam.DeleteRecord(testPathDel, delIdx)
	if err != nil {
		fmt.Printf("   ERROR delete: %v\n", err)
		return
	}
	fmt.Printf("15. DeleteRecord OK! offset=%d backup=%s\n", delResult.FileOffset, delResult.BackupPath)

	// Verify: record count should be origCount-1
	infoD2, _, err := isam.ReadFileV2(testPathDel)
	if err != nil {
		fmt.Printf("   ERROR re-read: %v\n", err)
		return
	}
	newCount := len(infoD2.Records)
	if newCount == origCount-1 {
		fmt.Printf("16. PASS: record count %d → %d (deleted 1)\n", origCount, newCount)
	} else {
		fmt.Printf("16. FAIL: expected %d records, got %d\n", origCount-1, newCount)
	}

	// Verify: deleted record should not appear in results
	found := false
	for _, r := range infoD2.Records {
		c := strings.TrimRight(string(r.Data[0:5]), " \x00")
		if c == delCodigo {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("17. PASS: codigo=%q no longer in records\n", delCodigo)
	} else {
		fmt.Printf("17. FAIL: codigo=%q still found after delete!\n", delCodigo)
	}

	// Verify: all other records unchanged
	fmt.Println("18. Verifying other records untouched after delete...")
	unchangedDel := 0
	changedDel := 0
	origIdx := 0
	for _, r := range infoD2.Records {
		// Skip the deleted record's original position
		if origIdx == delIdx {
			origIdx++
		}
		if origIdx < len(infoD.Records) {
			if string(r.Data) == string(infoD.Records[origIdx].Data) {
				unchangedDel++
			} else {
				changedDel++
				if changedDel <= 3 {
					c := strings.TrimRight(string(r.Data[0:5]), " \x00")
					fmt.Printf("   WARNING: record codigo=%q changed!\n", c)
				}
			}
		}
		origIdx++
	}
	fmt.Printf("   %d unchanged, %d changed (expected 0 changed)\n", unchangedDel, changedDel)

	// Verify: V2WithStats should show increased deleted count
	_, statsD, _ := isam.ReadFileV2WithStats(testPathDel)
	_, statsOrig, _ := isam.ReadFileV2WithStats(srcPath)
	if statsD.DeletedCount > statsOrig.DeletedCount {
		fmt.Printf("19. PASS: deleted count increased %d → %d\n", statsOrig.DeletedCount, statsD.DeletedCount)
	} else {
		fmt.Printf("19. FAIL: deleted count %d vs original %d\n", statsD.DeletedCount, statsOrig.DeletedCount)
	}

	// Test: Delete by key
	fmt.Println("20. Testing DeleteRecordByKey...")
	// Pick another record to delete by key
	delKey2 := strings.TrimRight(string(infoD2.Records[10].Data[0:5]), " \x00")
	delName2 := strings.TrimRight(string(infoD2.Records[10].Data[5:45]), " \x00")
	fmt.Printf("    Target: codigo=%q nombre=%q\n", delKey2, delName2)

	_, err = isam.DeleteRecordByKey(testPathDel, 0, 5, delKey2)
	if err != nil {
		fmt.Printf("    ERROR: %v\n", err)
	} else {
		infoD3, _, _ := isam.ReadFileV2(testPathDel)
		if len(infoD3.Records) == origCount-2 {
			fmt.Printf("    PASS: DeleteRecordByKey works! count %d → %d\n", origCount, len(infoD3.Records))
		} else {
			fmt.Printf("    FAIL: expected %d records, got %d\n", origCount-2, len(infoD3.Records))
		}
		// Verify key gone
		foundK := false
		for _, r := range infoD3.Records {
			if strings.TrimRight(string(r.Data[0:5]), " \x00") == delKey2 {
				foundK = true
				break
			}
		}
		if !foundK {
			fmt.Printf("    PASS: codigo=%q no longer in records\n", delKey2)
		} else {
			fmt.Printf("    FAIL: codigo=%q still found!\n", delKey2)
		}
	}

	// Test: Double-delete should fail (record already deleted)
	fmt.Println("21. Testing double-delete prevention...")
	// The deleted record is no longer in the records list, so trying
	// to delete by key should return "not found"
	_, err = isam.DeleteRecordByKey(testPathDel, 0, 5, delCodigo)
	if err != nil {
		fmt.Printf("    PASS: correctly rejected: %v\n", err)
	} else {
		fmt.Printf("    FAIL: should have rejected double delete!\n")
	}

	// =====================================================================
	// INSERT TESTS
	// =====================================================================
	fmt.Println("\n=== ISAM INSERT TEST (Pure Go) ===")
	fmt.Println()

	// Fresh copy for insert tests
	testPathIns := `C:\tmp\ZDANE_INS_TEST`
	if err := copyFile(srcPath, testPathIns); err != nil {
		fmt.Printf("ERROR copying for insert: %v\n", err)
		return
	}
	defer os.Remove(testPathIns)
	defer os.Remove(testPathIns + ".bak")

	// Read original state
	infoI, hdrI, err := isam.ReadFileV2(testPathIns)
	if err != nil {
		fmt.Printf("ERROR reading: %v\n", err)
		return
	}
	origCountI := len(infoI.Records)
	fmt.Printf("22. Fresh copy: %d records (recSize=%d)\n", origCountI, hdrI.MaxRecordLen)

	// First, delete a record to create a data slot with matching recSize
	// (The file already has 95 deleted slots from its original state)
	_, statsI, _ := isam.ReadFileV2WithStats(testPathIns)
	fmt.Printf("23. Deleted slots available: %d\n", statsI.DeletedCount)

	// Create new record data
	newRecSize := int(hdrI.MaxRecordLen)
	newRecData := make([]byte, newRecSize)
	// ZDANE: codigo@0(5), nombre@5(40)
	copy(newRecData[0:5], []byte("99999"))
	insNombre := make([]byte, 40)
	for i := range insNombre {
		insNombre[i] = ' '
	}
	copy(insNombre, []byte("MUNICIPIO TEST INSERT"))
	copy(newRecData[5:45], insNombre)

	fmt.Printf("24. Inserting: codigo=%q nombre=%q\n",
		strings.TrimRight(string(newRecData[0:5]), " \x00"),
		strings.TrimRight(string(newRecData[5:45]), " \x00"))

	// Insert!
	insResult, err := isam.InsertRecord(testPathIns, newRecData, 0, 5)
	if err != nil {
		fmt.Printf("    ERROR insert: %v\n", err)
		fmt.Println("\n=== INSERT TEST INCOMPLETE (error above) ===")
		fmt.Println("\n=== ALL TESTS COMPLETE ===")
		return
	}
	fmt.Printf("25. InsertRecord OK! offset=%d backup=%s\n", insResult.FileOffset, insResult.BackupPath)

	// Verify: record count should be origCount+1
	infoI2, _, err := isam.ReadFileV2(testPathIns)
	if err != nil {
		fmt.Printf("    ERROR re-read: %v\n", err)
		return
	}
	newCountI := len(infoI2.Records)
	if newCountI == origCountI+1 {
		fmt.Printf("26. PASS: record count %d -> %d (inserted 1)\n", origCountI, newCountI)
	} else {
		fmt.Printf("26. FAIL: expected %d records, got %d\n", origCountI+1, newCountI)
	}

	// Verify: new record should appear in results
	foundIns := false
	for _, r := range infoI2.Records {
		c := strings.TrimRight(string(r.Data[0:5]), " \x00")
		if c == "99999" {
			n := strings.TrimRight(string(r.Data[5:45]), " \x00")
			fmt.Printf("27. PASS: found inserted record codigo=%q nombre=%q\n", c, n)
			foundIns = true
			break
		}
	}
	if !foundIns {
		fmt.Printf("27. FAIL: codigo=\"99999\" not found in records\n")
	}

	// Verify: all original records still present
	fmt.Println("28. Verifying original records untouched...")
	missingOrig := 0
	for _, origRec := range infoI.Records {
		origKey := strings.TrimRight(string(origRec.Data[0:5]), " \x00")
		foundOrig := false
		for _, newRec := range infoI2.Records {
			newKey := strings.TrimRight(string(newRec.Data[0:5]), " \x00")
			if origKey == newKey {
				if string(origRec.Data) == string(newRec.Data) {
					foundOrig = true
				}
				break
			}
		}
		if !foundOrig {
			missingOrig++
			if missingOrig <= 3 {
				fmt.Printf("   WARNING: record codigo=%q missing or changed\n", origKey)
			}
		}
	}
	if missingOrig == 0 {
		fmt.Printf("   PASS: all %d original records intact\n", origCountI)
	} else {
		fmt.Printf("   FAIL: %d original records missing or changed\n", missingOrig)
	}

	// Verify: duplicate key rejection
	fmt.Println("29. Testing duplicate key rejection...")
	_, err = isam.InsertRecord(testPathIns, newRecData, 0, 5)
	if err != nil {
		fmt.Printf("    PASS: correctly rejected duplicate: %v\n", err)
	} else {
		fmt.Printf("    FAIL: should have rejected duplicate key!\n")
	}

	// =====================================================================
	// NODE SPLIT STRESS TEST — insert many records to force B-tree leaf split
	// =====================================================================
	fmt.Println("\n=== ISAM INSERT NODE SPLIT TEST ===")
	fmt.Println()

	testPathSplit := `C:\tmp\ZDANE_SPLIT_TEST`
	if err := copyFile(srcPath, testPathSplit); err != nil {
		fmt.Printf("ERROR copying for split test: %v\n", err)
		return
	}
	defer os.Remove(testPathSplit)
	defer os.Remove(testPathSplit + ".bak")

	infoS, hdrS, err := isam.ReadFileV2(testPathSplit)
	if err != nil {
		fmt.Printf("ERROR reading: %v\n", err)
		return
	}
	origCountS := len(infoS.Records)
	fmt.Printf("30. Fresh copy: %d records (recSize=%d)\n", origCountS, hdrS.MaxRecordLen)

	// Insert 100 records with sequential keys to stress the B-tree
	// ZDANE leaf max entries = 92 (with 5-byte key + 6-byte ptr = 11 bytes, 1020/11 = 92)
	// So inserting 100 records should trigger at least one split
	insertCount := 100
	successCount := 0
	splitErrors := 0
	fmt.Printf("31. Inserting %d records to force node split...\n", insertCount)

	for i := 0; i < insertCount; i++ {
		rec := make([]byte, int(hdrS.MaxRecordLen))
		// Keys: 80001, 80002, ..., 80100 (unlikely to collide with existing DANE codes)
		key := fmt.Sprintf("%05d", 80001+i)
		copy(rec[0:5], []byte(key))
		nom := make([]byte, 40)
		for j := range nom {
			nom[j] = ' '
		}
		copy(nom, []byte(fmt.Sprintf("SPLIT TEST %d", i+1)))
		copy(rec[5:45], nom)

		_, err := isam.InsertRecord(testPathSplit, rec, 0, 5)
		if err != nil {
			if i < 5 || i == insertCount-1 {
				fmt.Printf("    insert #%d (key=%s): ERROR %v\n", i+1, key, err)
			}
			splitErrors++
		} else {
			successCount++
		}
	}

	fmt.Printf("32. Inserted %d/%d records (%d errors)\n", successCount, insertCount, splitErrors)

	// Verify all records readable
	infoS2, _, err := isam.ReadFileV2(testPathSplit)
	if err != nil {
		fmt.Printf("33. FAIL: V2 reader error after inserts: %v\n", err)
	} else {
		expectedCount := origCountS + successCount
		actualCount := len(infoS2.Records)
		if actualCount == expectedCount {
			fmt.Printf("33. PASS: record count %d → %d (expected %d)\n", origCountS, actualCount, expectedCount)
		} else {
			fmt.Printf("33. FAIL: expected %d records, got %d\n", expectedCount, actualCount)
		}

		// Verify originals intact
		missingS := 0
		for _, origRec := range infoS.Records {
			origKey := strings.TrimRight(string(origRec.Data[0:5]), " \x00")
			foundS := false
			for _, newRec := range infoS2.Records {
				newKey := strings.TrimRight(string(newRec.Data[0:5]), " \x00")
				if origKey == newKey {
					if string(origRec.Data) == string(newRec.Data) {
						foundS = true
					}
					break
				}
			}
			if !foundS {
				missingS++
			}
		}
		if missingS == 0 {
			fmt.Printf("34. PASS: all %d original records intact after %d inserts\n", origCountS, successCount)
		} else {
			fmt.Printf("34. FAIL: %d original records missing or changed\n", missingS)
		}

		// Verify inserted records findable
		foundNew := 0
		for i := 0; i < successCount; i++ {
			key := fmt.Sprintf("%05d", 80001+i)
			for _, r := range infoS2.Records {
				if strings.TrimRight(string(r.Data[0:5]), " \x00") == key {
					foundNew++
					break
				}
			}
		}
		fmt.Printf("35. Found %d/%d inserted records in re-read\n", foundNew, successCount)
	}

	// =====================================================================
	// MULTI-FORMAT TESTS — Z06 (4-byte markers), Z49 (large), Z032016 (large), Z17
	// =====================================================================
	fmt.Println("\n=== MULTI-FORMAT WRITE TESTS ===")
	fmt.Println()

	type fileTest struct {
		name     string
		src      string
		keyOff   int
		keyLen   int
		recHdr   int // expected recHeaderSize
		newKey   string
		newField string
		fieldOff int
		fieldLen int
	}

	multiTests := []fileTest{
		{"Z06", `C:\DEMOS01\Z06`, 2, 7, 4, "ZZ99999", "TEST Z06 REWRITE", 31, 20},
		{"Z49", `C:\DEMOS01\Z49`, 1, 3, 2, "ZZ9", "TEST Z49 REWRITE NOMBRE", 15, 35},
		{"Z032016", `C:\DEMOS01\Z032016`, 3, 9, 2, "999999999", "TEST Z03 REWRITE", 25, 70},
		{"Z17", `C:\DEMOS01\Z17`, 4, 14, 2, "99999999999999", "TEST Z17 REWRITE", 36, 40},
	}

	stepNum := 36
	for _, ft := range multiTests {
		fmt.Printf("\n--- %s (recHdr=%d) ---\n", ft.name, ft.recHdr)
		testP := fmt.Sprintf(`C:\tmp\%s_WRITE_TEST`, ft.name)

		if err := copyFile(ft.src, testP); err != nil {
			fmt.Printf("%d. ERROR copying %s: %v\n", stepNum, ft.name, err)
			stepNum++
			continue
		}
		defer os.Remove(testP)
		defer os.Remove(testP + ".bak")

		infoM, hdrM, err := isam.ReadFileV2(testP)
		if err != nil {
			fmt.Printf("%d. ERROR reading %s: %v\n", stepNum, ft.name, err)
			stepNum++
			continue
		}
		fmt.Printf("%d. %s: %d records, recSize=%d, recHdr=%d, hdrSize=%d\n",
			stepNum, ft.name, len(infoM.Records), hdrM.MaxRecordLen, hdrM.RecHeaderSize, hdrM.HeaderSize)
		stepNum++

		if hdrM.RecHeaderSize != ft.recHdr {
			fmt.Printf("   WARNING: expected recHdr=%d, got %d\n", ft.recHdr, hdrM.RecHeaderSize)
		}

		origCountM := len(infoM.Records)

		// --- REWRITE TEST ---
		if origCountM >= 3 {
			targetM := 2
			recM := infoM.Records[targetM]
			oldKeyM := strings.TrimRight(string(recM.Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00")
			oldFieldM := strings.TrimRight(string(recM.Data[ft.fieldOff:ft.fieldOff+ft.fieldLen]), " \x00")

			newDataM := make([]byte, int(hdrM.MaxRecordLen))
			copy(newDataM, recM.Data)

			fieldBytes := make([]byte, ft.fieldLen)
			for i := range fieldBytes {
				fieldBytes[i] = ' '
			}
			copy(fieldBytes, []byte(ft.newField))
			copy(newDataM[ft.fieldOff:ft.fieldOff+ft.fieldLen], fieldBytes)

			keyOffs := [][2]int{{ft.keyOff, ft.keyLen}}
			_, err := isam.RewriteRecord(testP, targetM, newDataM, keyOffs)
			if err != nil {
				fmt.Printf("%d. REWRITE FAIL: %v\n", stepNum, err)
			} else {
				infoM2, _, _ := isam.ReadFileV2(testP)
				verKey := strings.TrimRight(string(infoM2.Records[targetM].Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00")
				verField := strings.TrimRight(string(infoM2.Records[targetM].Data[ft.fieldOff:ft.fieldOff+ft.fieldLen]), " \x00")

				if verKey == oldKeyM && verField == ft.newField {
					fmt.Printf("%d. REWRITE PASS: key=%q field changed %q → %q\n", stepNum, verKey, oldFieldM, verField)
				} else {
					fmt.Printf("%d. REWRITE FAIL: key=%q (expected %q) field=%q (expected %q)\n",
						stepNum, verKey, oldKeyM, verField, ft.newField)
				}

				// Verify count preserved
				if len(infoM2.Records) != origCountM {
					fmt.Printf("   FAIL: count changed %d → %d\n", origCountM, len(infoM2.Records))
				}
			}
			stepNum++
		}

		// --- DELETE TEST ---
		delIdxM := 5
		if delIdxM >= origCountM {
			delIdxM = origCountM / 2
		}
		delKeyM := strings.TrimRight(string(infoM.Records[delIdxM].Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00")

		// Count how many records have this key before delete (may have duplicates)
		dupsBeforeDel := 0
		for _, r := range infoM.Records {
			if strings.TrimRight(string(r.Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00") == delKeyM {
				dupsBeforeDel++
			}
		}

		_, err = isam.DeleteRecord(testP, delIdxM)
		if err != nil {
			fmt.Printf("%d. DELETE FAIL: %v\n", stepNum, err)
		} else {
			infoM3, _, _ := isam.ReadFileV2(testP)
			if len(infoM3.Records) == origCountM-1 {
				// Count key occurrences after delete
				dupsAfterDel := 0
				for _, r := range infoM3.Records {
					if strings.TrimRight(string(r.Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00") == delKeyM {
						dupsAfterDel++
					}
				}
				if dupsAfterDel == dupsBeforeDel-1 {
					fmt.Printf("%d. DELETE PASS: key=%q count %d→%d (dups %d→%d)\n",
						stepNum, delKeyM, origCountM, len(infoM3.Records), dupsBeforeDel, dupsAfterDel)
				} else {
					fmt.Printf("%d. DELETE FAIL: key=%q dups %d→%d (expected %d)\n",
						stepNum, delKeyM, dupsBeforeDel, dupsAfterDel, dupsBeforeDel-1)
				}
			} else {
				fmt.Printf("%d. DELETE FAIL: count %d → %d (expected %d)\n",
					stepNum, origCountM, len(infoM3.Records), origCountM-1)
			}
		}
		stepNum++

		// --- INSERT TEST ---
		recSizeM := int(hdrM.MaxRecordLen)
		newRecM := make([]byte, recSizeM)
		// Write key
		keyBytesM := make([]byte, ft.keyLen)
		for i := range keyBytesM {
			keyBytesM[i] = ' '
		}
		copy(keyBytesM, []byte(ft.newKey))
		copy(newRecM[ft.keyOff:ft.keyOff+ft.keyLen], keyBytesM)
		// Write field
		fieldBytesM := make([]byte, ft.fieldLen)
		for i := range fieldBytesM {
			fieldBytesM[i] = ' '
		}
		copy(fieldBytesM, []byte("INSERTED RECORD"))
		copy(newRecM[ft.fieldOff:ft.fieldOff+ft.fieldLen], fieldBytesM)

		_, err = isam.InsertRecord(testP, newRecM, ft.keyOff, ft.keyLen)
		if err != nil {
			fmt.Printf("%d. INSERT FAIL: %v\n", stepNum, err)
		} else {
			infoM4, _, _ := isam.ReadFileV2(testP)
			// After rewrite(-0) + delete(-1) + insert(+1) = origCount
			expectedM := origCountM
			if len(infoM4.Records) == expectedM {
				// Find inserted record
				foundInsM := false
				for _, r := range infoM4.Records {
					rk := strings.TrimRight(string(r.Data[ft.keyOff:ft.keyOff+ft.keyLen]), " \x00")
					if rk == strings.TrimRight(ft.newKey, " ") {
						rf := strings.TrimRight(string(r.Data[ft.fieldOff:ft.fieldOff+ft.fieldLen]), " \x00")
						fmt.Printf("%d. INSERT PASS: key=%q field=%q count=%d\n", stepNum, rk, rf, len(infoM4.Records))
						foundInsM = true
						break
					}
				}
				if !foundInsM {
					fmt.Printf("%d. INSERT FAIL: key=%q not found in records\n", stepNum, ft.newKey)
				}
			} else {
				fmt.Printf("%d. INSERT FAIL: count=%d expected=%d\n", stepNum, len(infoM4.Records), expectedM)
			}
		}
		stepNum++
	}

	// =====================================================================
	// DELETE + INSERT COMBO — replace a record by changing key
	// =====================================================================
	fmt.Println("\n=== DELETE + INSERT COMBO TEST ===")
	fmt.Println()

	testPathCombo := `C:\tmp\ZDANE_COMBO_TEST`
	if err := copyFile(srcPath, testPathCombo); err != nil {
		fmt.Printf("%d. ERROR copying: %v\n", stepNum, err)
	} else {
		defer os.Remove(testPathCombo)
		defer os.Remove(testPathCombo + ".bak")

		infoC, _, err := isam.ReadFileV2(testPathCombo)
		if err != nil {
			fmt.Printf("%d. ERROR reading: %v\n", stepNum, err)
		} else {
			origCountC := len(infoC.Records)

			// Pick a record to "replace"
			replaceIdx := 10
			oldKeyC := strings.TrimRight(string(infoC.Records[replaceIdx].Data[0:5]), " \x00")
			oldNameC := strings.TrimRight(string(infoC.Records[replaceIdx].Data[5:45]), " \x00")
			fmt.Printf("%d. Target for replace: idx=%d key=%q name=%q\n", stepNum, replaceIdx, oldKeyC, oldNameC)
			stepNum++

			// Step 1: Delete the old record
			_, err = isam.DeleteRecord(testPathCombo, replaceIdx)
			if err != nil {
				fmt.Printf("%d. DELETE step FAIL: %v\n", stepNum, err)
			} else {
				fmt.Printf("%d. DELETE step OK: key=%q removed\n", stepNum, oldKeyC)
			}
			stepNum++

			// Step 2: Insert new record with different key but same-ish data
			newRecC := make([]byte, 256)
			copy(newRecC[0:5], []byte("77777"))
			nomC := make([]byte, 40)
			for i := range nomC {
				nomC[i] = ' '
			}
			copy(nomC, []byte("REPLACED "+oldNameC))
			if len(nomC) > 40 {
				nomC = nomC[:40]
			}
			copy(newRecC[5:45], nomC)

			_, err = isam.InsertRecord(testPathCombo, newRecC, 0, 5)
			if err != nil {
				fmt.Printf("%d. INSERT step FAIL: %v\n", stepNum, err)
			} else {
				fmt.Printf("%d. INSERT step OK: key=%q\n", stepNum, "77777")
			}
			stepNum++

			// Verify final state
			infoC2, _, _ := isam.ReadFileV2(testPathCombo)
			if len(infoC2.Records) == origCountC {
				fmt.Printf("%d. COMBO PASS: count preserved %d\n", stepNum, origCountC)
			} else {
				fmt.Printf("%d. COMBO FAIL: count %d → %d\n", stepNum, origCountC, len(infoC2.Records))
			}
			stepNum++

			// Old key gone, new key present
			foundOld := false
			foundNew := false
			for _, r := range infoC2.Records {
				k := strings.TrimRight(string(r.Data[0:5]), " \x00")
				if k == oldKeyC {
					foundOld = true
				}
				if k == "77777" {
					n := strings.TrimRight(string(r.Data[5:45]), " \x00")
					fmt.Printf("%d. Found replacement: key=%q name=%q\n", stepNum, k, n)
					foundNew = true
				}
			}
			stepNum++

			if !foundOld && foundNew {
				fmt.Printf("%d. COMBO PASS: old key gone, new key present\n", stepNum)
			} else {
				fmt.Printf("%d. COMBO FAIL: oldFound=%v newFound=%v\n", stepNum, foundOld, foundNew)
			}
			stepNum++
		}
	}

	fmt.Println("\n=== ALL TESTS COMPLETE ===")
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	return err
}
