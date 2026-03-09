package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"siigo-common/isam"
)

func main() {
	fmt.Println("=== ISAM ELOQUENT-STYLE ORM TEST ===")
	fmt.Println()

	// =====================================================================
	// Connect all models at once (like Laravel's DB config)
	// =====================================================================
	isam.ConnectAll(`C:\DEMOS01`, "2016")

	// Show available models
	available := isam.AvailableModels()
	fmt.Printf("1. Available models: %d\n", len(available))
	for _, name := range available {
		model := isam.GetModel(name)
		count, _ := model.Count()
		fmt.Printf("   %-25s %4d records  (file: %s)\n", name, count, model.FileName())
	}

	// =====================================================================
	// READ TESTS — using models directly (like Eloquent)
	// =====================================================================
	fmt.Println("\n--- READ TESTS (Eloquent-style) ---")

	// isam.CodigosDane.All() — like DaneCode::all()
	all, err := isam.CodigosDane.All()
	if err != nil {
		fmt.Printf("2. ERROR All(): %v\n", err)
		return
	}
	fmt.Printf("2. CodigosDane.All(): %d records\n", len(all))

	for i := 0; i < 3 && i < len(all); i++ {
		r := all[i]
		fmt.Printf("   [%d] codigo=%q nombre=%q hash=%s\n",
			i, r.Get("codigo"), r.Get("nombre"), r.Hash()[:8])
	}

	// isam.CodigosDane.Find("05001") — like DaneCode::find("05001")
	rec, err := isam.CodigosDane.Find("05001")
	if err != nil {
		fmt.Printf("3. ERROR Find(): %v\n", err)
	} else {
		fmt.Printf("3. CodigosDane.Find(\"05001\"): codigo=%q nombre=%q\n",
			rec.Get("codigo"), rec.Get("nombre"))
	}

	// isam.CodigosDane.Where(...) — like DaneCode::where(...)
	results, err := isam.CodigosDane.Where("codigo", func(v string) bool {
		return strings.HasPrefix(v, "050")
	})
	if err != nil {
		fmt.Printf("4. ERROR Where(): %v\n", err)
	} else {
		fmt.Printf("4. CodigosDane.Where(codigo starts with \"050\"): %d records\n", len(results))
	}

	// Count
	count, _ := isam.CodigosDane.Count()
	fmt.Printf("5. CodigosDane.Count(): %d\n", count)

	// ToMap
	if rec != nil {
		m := rec.ToMap()
		fmt.Printf("6. ToMap(): %v\n", m)
	}

	// isam.Clients.All() — like Client::all()
	if isam.Clients.Exists() {
		allC, err := isam.Clients.All()
		if err != nil {
			fmt.Printf("7. ERROR Clients.All(): %v\n", err)
		} else {
			fmt.Printf("7. Clients.All(): %d records\n", len(allC))
			for i := 0; i < 3 && i < len(allC); i++ {
				r := allC[i]
				fmt.Printf("   [%d] codigo=%q nombre=%q tipo_doc=%q\n",
					i, r.Get("codigo"), r.Get("nombre"), r.Get("tipo_doc"))
			}
		}
	}

	// isam.SaldosTerceros — BCD fields
	if isam.SaldosTerceros.Exists() {
		allS, err := isam.SaldosTerceros.All()
		if err != nil {
			fmt.Printf("8. ERROR SaldosTerceros.All(): %v\n", err)
		} else {
			fmt.Printf("8. SaldosTerceros.All(): %d records\n", len(allS))
			for i := 0; i < 3 && i < len(allS); i++ {
				r := allS[i]
				fmt.Printf("   [%d] cuenta=%q nit=%q saldo=%.2f deb=%.2f cred=%.2f\n",
					i, r.Get("cuenta"), r.Get("nit"),
					r.GetFloat("saldo_anterior"), r.GetFloat("debito"), r.GetFloat("credito"))
			}
		}
	}

	// =====================================================================
	// WRITE TESTS (on a copy)
	// =====================================================================
	fmt.Println("\n--- WRITE TESTS (on copy) ---")

	// Copy ZDANE to temp
	srcPath := `C:\DEMOS01\ZDANE`
	testPath := `C:\tmp\ZDANE_ELOQUENT_TEST`
	copyFile(srcPath, testPath)
	defer os.Remove(testPath)
	defer os.Remove(testPath + ".bak")

	// Create a standalone model for the copy (like a test model)
	daneCopy := isam.DefineModel("dane_test_eloquent", "ZDANE", false, "", 256, func(m *isam.Model) {
		m.Key("codigo", 0, 5)
		m.String("nombre", 5, 40)
	})
	daneCopy.Table.Path = testPath // point to copy

	origCount, _ := daneCopy.Count()
	fmt.Printf("9. Test file: %d records\n", origCount)

	// UPDATE: Find and modify — like $rec = DaneCode::find("05001"); $rec->nombre = "..."; $rec->save()
	rec2, err := daneCopy.Find("05001")
	if err != nil {
		fmt.Printf("10. ERROR Find: %v\n", err)
		return
	}
	oldName := rec2.Get("nombre")
	rec2.Set("nombre", "ELOQUENT UPDATE")
	result, err := rec2.Save()
	if err != nil {
		fmt.Printf("10. REWRITE FAIL: %v\n", err)
	} else {
		verify, _ := daneCopy.Find("05001")
		if verify.Get("nombre") == "ELOQUENT UPDATE" {
			fmt.Printf("10. REWRITE PASS: %q → %q (offset=%d)\n", oldName, verify.Get("nombre"), result.FileOffset)
		} else {
			fmt.Printf("10. REWRITE FAIL: got %q\n", verify.Get("nombre"))
		}
	}

	// UPDATE via UpdateByKey — like DaneCode::where('key', '05002')->update([...])
	_, err = daneCopy.UpdateByKey("05002", func(r *isam.Row) {
		r.Set("nombre", "ELOQUENT CALLBACK")
	})
	if err != nil {
		fmt.Printf("11. UpdateByKey FAIL: %v\n", err)
	} else {
		v, _ := daneCopy.Find("05002")
		if v.Get("nombre") == "ELOQUENT CALLBACK" {
			fmt.Printf("11. UpdateByKey PASS: nombre=%q\n", v.Get("nombre"))
		} else {
			fmt.Printf("11. UpdateByKey FAIL: got %q\n", v.Get("nombre"))
		}
	}

	// DELETE — like $rec->delete()
	rec3, _ := daneCopy.Find("05004")
	delName := rec3.Get("nombre")
	_, err = rec3.Delete()
	if err != nil {
		fmt.Printf("12. DELETE FAIL: %v\n", err)
	} else {
		newCount, _ := daneCopy.Count()
		if newCount == origCount-1 {
			fmt.Printf("12. DELETE PASS: removed %q, count %d → %d\n", delName, origCount, newCount)
		} else {
			fmt.Printf("12. DELETE FAIL: count %d → %d\n", origCount, newCount)
		}
	}

	// DELETE by key — like DaneCode::destroy("05021")
	_, err = daneCopy.DeleteByKey("05021")
	if err != nil {
		fmt.Printf("13. DeleteByKey FAIL: %v\n", err)
	} else {
		newCount2, _ := daneCopy.Count()
		if newCount2 == origCount-2 {
			fmt.Printf("13. DeleteByKey PASS: count %d → %d\n", origCount, newCount2)
		} else {
			fmt.Printf("13. DeleteByKey FAIL: count %d → %d\n", origCount, newCount2)
		}
	}

	// INSERT — like $rec = new DaneCode(); $rec->codigo = "99998"; $rec->save()
	newRec := daneCopy.New()
	newRec.Set("codigo", "99998")
	newRec.Set("nombre", "ELOQUENT INSERT")
	_, err = newRec.Save()
	if err != nil {
		fmt.Printf("14. INSERT FAIL: %v\n", err)
	} else {
		v, _ := daneCopy.Find("99998")
		if v != nil && v.Get("nombre") == "ELOQUENT INSERT" {
			fmt.Printf("14. INSERT PASS: codigo=%q nombre=%q\n", v.Get("codigo"), v.Get("nombre"))
		} else {
			fmt.Printf("14. INSERT FAIL: not found after insert\n")
		}
	}

	// REPLACE — like delete old + create new
	replacer := daneCopy.New()
	replacer.Set("codigo", "77777")
	replacer.Set("nombre", "ELOQUENT REPLACE")
	_, err = daneCopy.ReplaceByKey("05030", replacer)
	if err != nil {
		fmt.Printf("15. ReplaceByKey FAIL: %v\n", err)
	} else {
		_, errOld := daneCopy.Find("05030")
		vNew, errNew := daneCopy.Find("77777")
		if errOld != nil && errNew == nil && vNew.Get("nombre") == "ELOQUENT REPLACE" {
			fmt.Printf("15. ReplaceByKey PASS: old gone, new=%q\n", vNew.Get("nombre"))
		} else {
			fmt.Printf("15. ReplaceByKey FAIL: oldErr=%v newErr=%v\n", errOld, errNew)
		}
	}

	// Final integrity check
	finalCount, _ := daneCopy.Count()
	expected := origCount - 3 + 2 // -3 deletes +2 inserts
	if finalCount == expected {
		fmt.Printf("16. INTEGRITY PASS: final count %d (expected %d)\n", finalCount, expected)
	} else {
		fmt.Printf("16. INTEGRITY FAIL: final count %d (expected %d)\n", finalCount, expected)
	}

	// Double-delete protection
	_, err = rec3.Delete()
	if err != nil {
		fmt.Printf("17. Double-delete protection PASS: %v\n", err)
	} else {
		fmt.Printf("17. Double-delete protection FAIL\n")
	}

	// Save-after-delete protection
	_, err = rec3.Save()
	if err != nil {
		fmt.Printf("18. Save-after-delete protection PASS: %v\n", err)
	} else {
		fmt.Printf("18. Save-after-delete protection FAIL\n")
	}

	fmt.Println("\n=== ALL ELOQUENT ORM TESTS COMPLETE ===")
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
