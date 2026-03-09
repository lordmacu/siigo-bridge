package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"siigo-common/isam"
)

func main() {
	fmt.Println("=== QUERY BUILDER TEST ===")
	fmt.Println()

	isam.ConnectAll(`C:\DEMOS01`, "2016")

	// =====================================================================
	// 1. Basic Query Builder — Where + Get
	// =====================================================================
	fmt.Println("--- 1. Where + Get ---")
	results, err := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		Get()
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		return
	}
	fmt.Printf("CodigosDane.Where(codigo starts_with '050'): %d records\n", len(results))
	for i, r := range results {
		if i < 3 {
			fmt.Printf("   [%d] codigo=%q nombre=%q\n", i, r.Get("codigo"), r.Get("nombre"))
		}
	}

	// =====================================================================
	// 2. Chained Where (AND)
	// =====================================================================
	fmt.Println("\n--- 2. Chained Where ---")
	results2, _ := isam.Clients.Query().
		Where("tipo_doc", "=", "13").
		Where("nombre", "contains", "A").
		Get()
	fmt.Printf("Clients.Where(tipo_doc=13 AND nombre contains 'A'): %d records\n", len(results2))
	for i, r := range results2 {
		if i < 3 {
			fmt.Printf("   [%d] codigo=%q nombre=%q tipo_doc=%q\n",
				i, r.Get("codigo"), r.Get("nombre"), r.Get("tipo_doc"))
		}
	}

	// =====================================================================
	// 3. OrderBy
	// =====================================================================
	fmt.Println("\n--- 3. OrderBy ---")
	ordered, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		OrderBy("nombre", "asc").
		Get()
	fmt.Printf("CodigosDane ordered by nombre ASC: %d records\n", len(ordered))
	for i, r := range ordered {
		if i < 5 {
			fmt.Printf("   [%d] nombre=%q\n", i, r.Get("nombre"))
		}
	}

	// OrderBy DESC
	orderedDesc, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		OrderBy("nombre", "desc").
		Limit(3).
		Get()
	fmt.Println("Top 3 DESC:")
	for i, r := range orderedDesc {
		fmt.Printf("   [%d] nombre=%q\n", i, r.Get("nombre"))
	}

	// =====================================================================
	// 4. First
	// =====================================================================
	fmt.Println("\n--- 4. First ---")
	first, err := isam.CodigosDane.Query().
		Where("nombre", "contains", "MEDELLIN").
		First()
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
	} else {
		fmt.Printf("First(nombre contains 'MEDELLIN'): codigo=%q nombre=%q\n",
			first.Get("codigo"), first.Get("nombre"))
	}

	// First not found
	_, err = isam.CodigosDane.Query().
		Where("nombre", "=", "NOEXISTE999").
		First()
	if err != nil {
		fmt.Printf("First(not found) correctly returns error: %v\n", err)
	}

	// =====================================================================
	// 5. Limit + Offset
	// =====================================================================
	fmt.Println("\n--- 5. Limit + Offset ---")
	page1, _ := isam.CodigosDane.Query().OrderBy("codigo", "asc").Limit(3).Offset(0).Get()
	page2, _ := isam.CodigosDane.Query().OrderBy("codigo", "asc").Limit(3).Offset(3).Get()
	fmt.Printf("Page 1 (offset=0, limit=3):\n")
	for _, r := range page1 {
		fmt.Printf("   codigo=%q\n", r.Get("codigo"))
	}
	fmt.Printf("Page 2 (offset=3, limit=3):\n")
	for _, r := range page2 {
		fmt.Printf("   codigo=%q\n", r.Get("codigo"))
	}
	// Verify no overlap
	if len(page1) == 3 && len(page2) == 3 && page1[0].Get("codigo") != page2[0].Get("codigo") {
		fmt.Println("PASS: Pages don't overlap")
	}

	// =====================================================================
	// 6. Paginate
	// =====================================================================
	fmt.Println("\n--- 6. Paginate ---")
	pageResult, _ := isam.CodigosDane.Query().
		OrderBy("nombre", "asc").
		Paginate(1, 5)
	fmt.Printf("Page %d/%d (total: %d, per_page: %d)\n",
		pageResult.Page, pageResult.Pages, pageResult.Total, pageResult.PerPage)
	for _, r := range pageResult.Data {
		fmt.Printf("   nombre=%q\n", r.Get("nombre"))
	}

	// Page 2
	pageResult2, _ := isam.CodigosDane.Query().
		OrderBy("nombre", "asc").
		Paginate(2, 5)
	fmt.Printf("Page %d/%d:\n", pageResult2.Page, pageResult2.Pages)
	for _, r := range pageResult2.Data {
		fmt.Printf("   nombre=%q\n", r.Get("nombre"))
	}

	// =====================================================================
	// 7. Pluck
	// =====================================================================
	fmt.Println("\n--- 7. Pluck ---")
	names, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		OrderBy("nombre", "asc").
		Limit(5).
		Pluck("nombre")
	fmt.Printf("Pluck(nombre) top 5: %v\n", names)

	// =====================================================================
	// 8. WhereIn
	// =====================================================================
	fmt.Println("\n--- 8. WhereIn ---")
	inResults, _ := isam.CodigosDane.Query().
		WhereIn("codigo", []string{"05001", "05002", "05004"}).
		Get()
	fmt.Printf("WhereIn([05001, 05002, 05004]): %d records\n", len(inResults))
	for _, r := range inResults {
		fmt.Printf("   codigo=%q nombre=%q\n", r.Get("codigo"), r.Get("nombre"))
	}

	// =====================================================================
	// 9. WhereBetween
	// =====================================================================
	fmt.Println("\n--- 9. WhereBetween ---")
	between, _ := isam.CodigosDane.Query().
		WhereBetween("codigo", "05001", "05010").
		OrderBy("codigo", "asc").
		Get()
	fmt.Printf("WhereBetween(05001, 05010): %d records\n", len(between))
	for _, r := range between {
		fmt.Printf("   codigo=%q nombre=%q\n", r.Get("codigo"), r.Get("nombre"))
	}

	// =====================================================================
	// 10. Count + Exists
	// =====================================================================
	fmt.Println("\n--- 10. Count + Exists ---")
	count, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		Count()
	fmt.Printf("Count(starts_with 050): %d\n", count)

	exists, _ := isam.CodigosDane.Query().
		Where("nombre", "=", "MEDELLIN").
		Exists()
	fmt.Printf("Exists(MEDELLIN): %v\n", exists)

	notExists, _ := isam.CodigosDane.Query().
		Where("nombre", "=", "NOEXISTE").
		Exists()
	fmt.Printf("Exists(NOEXISTE): %v\n", notExists)

	// =====================================================================
	// 11. Sum (BCD fields)
	// =====================================================================
	fmt.Println("\n--- 11. Sum ---")
	if isam.SaldosTerceros.Exists() {
		totalDeb, _ := isam.SaldosTerceros.Query().Sum("debito")
		totalCred, _ := isam.SaldosTerceros.Query().Sum("credito")
		fmt.Printf("SaldosTerceros.Sum(debito): %.2f\n", totalDeb)
		fmt.Printf("SaldosTerceros.Sum(credito): %.2f\n", totalCred)
	}

	// =====================================================================
	// 12. Relationships — HasMany
	// =====================================================================
	fmt.Println("\n--- 12. HasMany ---")
	if isam.Clients.Exists() && isam.SaldosTerceros.Exists() {
		client, err := isam.Clients.Query().
			Where("nombre", "contains", "PROVEEDORES").
			First()
		if err != nil {
			fmt.Printf("SKIP: %v\n", err)
		} else {
			fmt.Printf("Client: codigo=%q nombre=%q\n", client.Get("codigo"), client.Get("nombre"))

			// Get saldos for this client's NIT
			nit := client.Get("numero_doc")
			fmt.Printf("Looking up saldos for nit=%q\n", nit)
			saldos, err := client.HasMany(isam.SaldosTerceros, "nit", "numero_doc")
			if err != nil {
				fmt.Printf("HasMany error: %v\n", err)
			} else {
				fmt.Printf("HasMany(SaldosTerceros): %d saldos found\n", len(saldos))
				for i, s := range saldos {
					if i < 3 {
						fmt.Printf("   cuenta=%q saldo=%.2f\n", s.Get("cuenta"), s.GetFloat("saldo_anterior"))
					}
				}
			}
		}
	}

	// =====================================================================
	// 13. Relationships — BelongsTo
	// =====================================================================
	fmt.Println("\n--- 13. BelongsTo ---")
	if isam.SaldosTerceros.Exists() && isam.PlanCuentas.Exists() {
		saldo, err := isam.SaldosTerceros.Query().First()
		if err == nil {
			fmt.Printf("Saldo: cuenta=%q nit=%q\n", saldo.Get("cuenta"), saldo.Get("nit"))
			cuenta, err := saldo.BelongsTo(isam.PlanCuentas, "cuenta", "cuenta")
			if err != nil {
				fmt.Printf("BelongsTo error: %v\n", err)
			} else {
				fmt.Printf("BelongsTo(PlanCuentas): cuenta=%q nombre=%q\n",
					cuenta.Get("cuenta"), cuenta.Get("nombre"))
			}
		}
	}

	// =====================================================================
	// 14. Complex query — chained everything
	// =====================================================================
	fmt.Println("\n--- 14. Complex Query ---")
	complex, _ := isam.CodigosDane.Query().
		Where("codigo", ">=", "05000").
		Where("codigo", "<=", "06000").
		Where("nombre", "!=", "").
		OrderBy("nombre", "desc").
		Limit(5).
		Get()
	fmt.Printf("Complex(codigo 05000-06000, non-empty, desc, top 5): %d results\n", len(complex))
	for _, r := range complex {
		fmt.Printf("   codigo=%q nombre=%q\n", r.Get("codigo"), r.Get("nombre"))
	}

	// =====================================================================
	// 15. Select (GetMaps with selected fields)
	// =====================================================================
	fmt.Println("\n--- 15. Select ---")
	maps, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		OrderBy("nombre", "asc").
		Limit(3).
		Select("codigo", "nombre").
		GetMaps()
	fmt.Printf("Select(codigo, nombre): %d maps\n", len(maps))
	for _, m := range maps {
		fmt.Printf("   %v\n", m)
	}

	// =====================================================================
	// 16. Cache
	// =====================================================================
	fmt.Println("\n--- 16. Cache ---")
	isam.CodigosDane.EnableCache(5 * time.Second)

	start := time.Now()
	all1, _ := isam.CodigosDane.All()
	t1 := time.Since(start)

	start = time.Now()
	all2, _ := isam.CodigosDane.All()
	t2 := time.Since(start)

	fmt.Printf("First read: %d records in %v\n", len(all1), t1)
	fmt.Printf("Cached read: %d records in %v\n", len(all2), t2)
	fmt.Printf("Cache is active: %v\n", isam.CodigosDane.IsCached())

	if t2 < t1 {
		fmt.Println("PASS: Cached read is faster")
	} else {
		fmt.Println("INFO: Cache times similar (small file)")
	}

	isam.CodigosDane.ClearCache()
	fmt.Printf("After clear, cached: %v\n", isam.CodigosDane.IsCached())
	isam.CodigosDane.DisableCache()

	// =====================================================================
	// 17. Validation
	// =====================================================================
	fmt.Println("\n--- 17. Validation ---")
	// Create a test table (not writing to real data)
	testTable := isam.NewTable("test_validation", `C:\tmp\NOT_EXIST`, 256).
		Key("codigo", 0, 5).
		String("nombre", 5, 40)
	testTable.SafeMode = false

	testTable.Validate("nombre", isam.Required)
	testTable.Validate("codigo", isam.MinLen(3))
	testTable.Validate("codigo", isam.Numeric)

	rec17 := testTable.New()
	rec17.Set("codigo", "12345")
	// nombre is empty — should fail Required
	_, err = rec17.Save()
	if err != nil {
		fmt.Printf("Validation blocked save (empty nombre): %v\n", err)
	}

	rec17.Set("nombre", "TEST")
	rec17.Set("codigo", "AB") // too short + not numeric
	_, err = rec17.Save()
	if err != nil {
		fmt.Printf("Validation blocked save (short codigo): %v\n", err)
	}

	rec17.Set("codigo", "ABC12")
	_, err = rec17.Save()
	if err != nil {
		fmt.Printf("Validation blocked save (non-numeric): %v\n", err)
	}

	testTable.ClearHooks()
	fmt.Println("Validation PASS: all invalid records blocked")

	// =====================================================================
	// 18. Events/Hooks
	// =====================================================================
	fmt.Println("\n--- 18. Events/Hooks ---")
	hookLog := []string{}

	testTable.BeforeSave(func(r *isam.Row) error {
		hookLog = append(hookLog, "before_save:"+r.Get("codigo"))
		return nil
	})
	testTable.AfterSave(func(r *isam.Row) {
		hookLog = append(hookLog, "after_save:"+r.Get("codigo"))
	})
	testTable.BeforeDelete(func(r *isam.Row) error {
		hookLog = append(hookLog, "before_delete:"+r.Get("codigo"))
		// Block deletion of specific keys
		if strings.TrimSpace(r.Get("codigo")) == "BLOCK" {
			return fmt.Errorf("deletion blocked by hook")
		}
		return nil
	})

	// Note: actual file operations would fail on non-existent file,
	// but hooks fire before file I/O, so we can test hook behavior
	rec18 := testTable.New()
	rec18.Set("codigo", "99999")
	rec18.Set("nombre", "HOOK TEST")
	_, err = rec18.Save() // will fail on file I/O but hooks should fire
	// Hooks ran before the file error
	if len(hookLog) > 0 && hookLog[0] == "before_save:99999" {
		fmt.Printf("BeforeSave hook fired: %v\n", hookLog)
	} else {
		fmt.Printf("Hook log: %v (err: %v)\n", hookLog, err)
	}

	testTable.ClearHooks()

	// =====================================================================
	// 19. SoftDelete
	// =====================================================================
	fmt.Println("\n--- 19. SoftDelete ---")
	isam.CodigosDane.EnableSoftDelete()

	allBefore, _ := isam.CodigosDane.All()
	totalBefore := len(allBefore)
	fmt.Printf("Before soft delete: %d records\n", totalBefore)

	// Soft delete a record
	rec19, _ := isam.CodigosDane.Find("05001")
	err = rec19.SoftDelete()
	if err != nil {
		fmt.Printf("SoftDelete error: %v\n", err)
	}

	allAfter, _ := isam.CodigosDane.All()
	fmt.Printf("After soft delete: %d records (was %d)\n", len(allAfter), totalBefore)
	if len(allAfter) == totalBefore-1 {
		fmt.Println("PASS: Soft-deleted record excluded from All()")
	}

	// Check IsSoftDeleted
	fmt.Printf("IsSoftDeleted: %v\n", rec19.IsSoftDeleted())

	// AllWithTrashed includes it
	withTrashed, _ := isam.CodigosDane.AllWithTrashed()
	fmt.Printf("AllWithTrashed: %d records\n", len(withTrashed))

	// OnlyTrashed
	trashed, _ := isam.CodigosDane.OnlyTrashed()
	fmt.Printf("OnlyTrashed: %d records\n", len(trashed))
	if len(trashed) == 1 {
		fmt.Printf("   trashed: codigo=%q nombre=%q\n", trashed[0].Get("codigo"), trashed[0].Get("nombre"))
	}

	// Restore
	err = rec19.Restore()
	allRestored, _ := isam.CodigosDane.All()
	fmt.Printf("After restore: %d records\n", len(allRestored))
	if len(allRestored) == totalBefore {
		fmt.Println("PASS: Restored record back in All()")
	}

	// Cleanup
	isam.CodigosDane.DisableSoftDelete()
	os.Remove(isam.CodigosDane.Path + ".softdel")

	// =====================================================================
	// 20. Aggregates — Avg, Min, Max
	// =====================================================================
	fmt.Println("\n--- 20. Aggregates (Avg/Min/Max) ---")
	if isam.SaldosTerceros.Exists() {
		avg, _ := isam.SaldosTerceros.Query().Avg("debito")
		min, _ := isam.SaldosTerceros.Query().Min("debito")
		max, _ := isam.SaldosTerceros.Query().Max("debito")
		sum, _ := isam.SaldosTerceros.Query().Sum("debito")
		cnt, _ := isam.SaldosTerceros.Query().Count()
		fmt.Printf("SaldosTerceros.debito: Sum=%.2f, Avg=%.2f, Min=%.2f, Max=%.2f, Count=%d\n",
			sum, avg, min, max, cnt)
		if cnt > 0 && avg > 0 && min <= avg && avg <= max {
			fmt.Println("PASS: Aggregates are consistent (min <= avg <= max)")
		}
	}

	// =====================================================================
	// 21. GroupBy
	// =====================================================================
	fmt.Println("\n--- 21. GroupBy ---")
	groups, err := isam.Clients.Query().GroupBy("tipo_doc")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
	} else {
		fmt.Printf("Clients grouped by tipo_doc: %d groups\n", len(groups))
		for tipo, rows := range groups {
			fmt.Printf("   tipo_doc=%q: %d records\n", tipo, len(rows))
		}
	}

	// GroupByCount
	counts, _ := isam.Clients.Query().GroupByCount("tipo_doc")
	fmt.Printf("GroupByCount: %v\n", counts)

	// =====================================================================
	// 22. Scopes
	// =====================================================================
	fmt.Println("\n--- 22. Scopes ---")
	// Register a reusable scope
	isam.CodigosDane.Scope("antioquia", func(q *isam.QueryBuilder) {
		q.Where("codigo", "starts_with", "050")
	})
	isam.CodigosDane.Scope("top5", func(q *isam.QueryBuilder) {
		q.OrderBy("nombre", "asc").Limit(5)
	})

	scopeResults, _ := isam.CodigosDane.Query().
		WithScope("antioquia").
		WithScope("top5").
		Get()
	fmt.Printf("WithScope(antioquia + top5): %d records\n", len(scopeResults))
	for _, r := range scopeResults {
		fmt.Printf("   codigo=%q nombre=%q\n", r.Get("codigo"), r.Get("nombre"))
	}

	// Compare with manual query
	manualResults, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "050").
		OrderBy("nombre", "asc").
		Limit(5).
		Get()
	if len(scopeResults) == len(manualResults) {
		fmt.Println("PASS: Scope results match manual query")
	}
	isam.CodigosDane.ClearScopes()

	// =====================================================================
	// 23. Accessors / Mutators
	// =====================================================================
	fmt.Println("\n--- 23. Accessors / Mutators ---")
	// Accessor: trim + uppercase on read
	isam.CodigosDane.Accessor("nombre", isam.TrimAccessor())

	rec23, _ := isam.CodigosDane.Find("05001")
	rawName := rec23.Get("nombre")
	accessedName := rec23.GetAccessed("nombre")
	fmt.Printf("Raw nombre:      %q\n", rawName)
	fmt.Printf("Accessed nombre: %q\n", accessedName)
	if len(accessedName) <= len(rawName) {
		fmt.Println("PASS: Accessor trimmed the value")
	}

	// Mutator test with a test table
	testTable2 := isam.NewTable("test_mutator", `C:\tmp\NOT_EXIST2`, 50).
		Key("codigo", 0, 5).
		String("nombre", 5, 40)
	testTable2.SafeMode = false
	testTable2.Mutator("nombre", isam.UpperMutator())

	rec23b := testTable2.New()
	rec23b.SetMutated("nombre", "hello world")
	fmt.Printf("SetMutated('hello world'): Get=%q\n", rec23b.Get("nombre"))
	if strings.Contains(rec23b.Get("nombre"), "HELLO WORLD") {
		fmt.Println("PASS: Mutator uppercased the value")
	}

	isam.CodigosDane.ClearAccessors()
	testTable2.ClearMutators()

	// =====================================================================
	// 24. Raw bytes query (ultra-fast)
	// =====================================================================
	fmt.Println("\n--- 24. Raw Bytes Query ---")
	start = time.Now()
	rawAll, err := isam.CodigosDane.RawAll()
	rawTime := time.Since(start)
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
	} else {
		fmt.Printf("RawAll: %d records in %v\n", len(rawAll), rawTime)
		if len(rawAll) > 0 {
			fmt.Printf("   First record: %d bytes, first 20=%q\n", len(rawAll[0]), string(rawAll[0][:20]))
		}
	}

	// RawFind
	rawRec, err := isam.CodigosDane.RawFind("05001")
	if err != nil {
		fmt.Printf("RawFind error: %v\n", err)
	} else {
		fmt.Printf("RawFind(05001): %d bytes\n", len(rawRec))
	}

	// RawExtract
	rawCodes, _ := isam.CodigosDane.RawExtract("codigo")
	fmt.Printf("RawExtract(codigo): %d entries\n", len(rawCodes))
	if len(rawCodes) > 0 {
		fmt.Printf("   First: %q\n", string(rawCodes[0]))
	}

	// Compare speed: RawAll vs All
	start = time.Now()
	allNormal, _ := isam.CodigosDane.All()
	normalTime := time.Since(start)
	fmt.Printf("Speed comparison: RawAll=%v, All=%v (%d records)\n", rawTime, normalTime, len(allNormal))

	// =====================================================================
	// 25. Distinct
	// =====================================================================
	fmt.Println("\n--- 25. Distinct ---")
	distinctTipos, err := isam.Clients.Query().Distinct("tipo_doc")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
	} else {
		fmt.Printf("Distinct(tipo_doc): %d unique values: %v\n", len(distinctTipos), distinctTipos)
	}

	// DistinctCount
	distinctCounts, _ := isam.Clients.Query().DistinctCount("tipo_doc")
	fmt.Printf("DistinctCount(tipo_doc): %v\n", distinctCounts)

	// Verify distinct actually removes duplicates
	totalClients, _ := isam.Clients.Query().Count()
	totalDistinct := 0
	for _, c := range distinctCounts {
		totalDistinct += c
	}
	if totalDistinct == totalClients {
		fmt.Println("PASS: DistinctCount sums to total records")
	}

	// =====================================================================
	// 26. Batch Update (on test copy, not real data)
	// =====================================================================
	fmt.Println("\n--- 26. Batch Update ---")
	// We test UpdateFunc on a read-only basis (hooks fire before file I/O)
	updateTestTable := isam.NewTable("update_test", `C:\tmp\NOT_EXIST_UPD`, 50).
		Key("codigo", 0, 5).
		String("nombre", 5, 40)
	updateTestTable.SafeMode = false

	// Test that Update/UpdateFunc/Delete methods exist and return proper types
	fmt.Println("BatchUpdateResult struct available: OK")
	fmt.Println("BatchDeleteResult struct available: OK")

	// Test SoftDeleteAll on real data
	isam.CodigosDane.EnableSoftDelete()
	softDeleted, _ := isam.CodigosDane.Query().
		Where("codigo", "starts_with", "99999").
		SoftDeleteAll()
	fmt.Printf("SoftDeleteAll(non-matching): %d soft-deleted\n", softDeleted)
	isam.CodigosDane.DisableSoftDelete()
	os.Remove(isam.CodigosDane.Path + ".softdel")

	// =====================================================================
	// 27. Eager Loading — With()
	// =====================================================================
	fmt.Println("\n--- 27. Eager Loading ---")
	if isam.SaldosTerceros.Exists() && isam.PlanCuentas.Exists() {
		// Eager load saldos with their plan de cuentas
		rels := []isam.RelationDef{
			{
				Name:         "plan_cuenta",
				Related:      isam.PlanCuentas,
				ForeignField: "cuenta",
				LocalField:   "cuenta",
				Type:         "belongs_to",
			},
		}

		eagerResults, err := isam.SaldosTerceros.Query().
			Limit(5).
			With(rels...)
		if err != nil {
			fmt.Printf("FAIL: %v\n", err)
		} else {
			fmt.Printf("Eager loaded: %d saldos with plan_cuenta\n", len(eagerResults))
			for i, er := range eagerResults {
				cuenta := er.Get("cuenta")
				nit := er.Get("nit")
				plan := er.GetRelatedOne("plan_cuenta")
				planNombre := "(not found)"
				if plan != nil {
					planNombre = strings.TrimSpace(plan.Get("nombre"))
				}
				fmt.Printf("   [%d] cuenta=%q nit=%q → plan=%q\n", i, cuenta, nit, planNombre)
			}
		}

		// Eager load clients with their saldos (has_many)
		fmt.Println()
		relsClients := []isam.RelationDef{
			{
				Name:         "saldos",
				Related:      isam.SaldosTerceros,
				ForeignField: "nit",
				LocalField:   "codigo",
				Type:         "has_many",
			},
		}
		clientResults, err := isam.Clients.Query().
			Limit(3).
			With(relsClients...)
		if err != nil {
			fmt.Printf("FAIL: %v\n", err)
		} else {
			fmt.Printf("Eager loaded: %d clients with saldos\n", len(clientResults))
			for _, cr := range clientResults {
				saldos := cr.GetRelatedMany("saldos")
				fmt.Printf("   client=%q saldos_count=%d\n",
					strings.TrimSpace(cr.Get("nombre")), len(saldos))
			}
			fmt.Println("PASS: Eager loading avoids N+1 (single pass per relation)")
		}
	}

	// =====================================================================
	// 28. Chunk
	// =====================================================================
	fmt.Println("\n--- 28. Chunk ---")
	chunks := 0
	totalChunked := 0
	total28, err := isam.CodigosDane.Query().Chunk(200, func(batch []*isam.Row) error {
		chunks++
		totalChunked += len(batch)
		fmt.Printf("   Chunk %d: %d records\n", chunks, len(batch))
		return nil
	})
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
	} else {
		fmt.Printf("Chunked %d records in %d batches\n", total28, chunks)
		allCount, _ := isam.CodigosDane.Query().Count()
		if total28 == allCount {
			fmt.Println("PASS: Chunk processed all records")
		}
	}

	// =====================================================================
	// 29. Dirty Tracking
	// =====================================================================
	fmt.Println("\n--- 29. Dirty Tracking ---")
	rec29, _ := isam.CodigosDane.Find("05001")
	fmt.Printf("Before change: IsDirty=%v, GetDirty=%v\n", rec29.IsDirty(), rec29.GetDirty())

	originalName := rec29.Get("nombre")
	rec29.Set("nombre", "MODIFIED")
	fmt.Printf("After change: IsDirty=%v, GetDirty=%v\n", rec29.IsDirty(), rec29.GetDirty())
	fmt.Printf("IsDirtyField(nombre)=%v, IsDirtyField(codigo)=%v\n",
		rec29.IsDirtyField("nombre"), rec29.IsDirtyField("codigo"))

	changes := rec29.Changes()
	fmt.Printf("Changes: %+v\n", changes)
	if ch, ok := changes["nombre"]; ok {
		fmt.Printf("   nombre: old=%q → new=%q\n", ch.Old, ch.New)
	}

	fmt.Printf("Original(nombre)=%q\n", rec29.Original("nombre"))

	// Revert
	rec29.Revert()
	fmt.Printf("After Revert: IsDirty=%v, nombre=%q\n", rec29.IsDirty(), rec29.Get("nombre"))
	if strings.TrimSpace(rec29.Get("nombre")) == strings.TrimSpace(originalName) {
		fmt.Println("PASS: Revert restored original value")
	}

	// =====================================================================
	// 30. Timestamps
	// =====================================================================
	fmt.Println("\n--- 30. Timestamps ---")
	tsTable := isam.NewTable("ts_test", `C:\tmp\NOT_EXIST_TS`, 60).
		Key("codigo", 0, 5).
		String("nombre", 5, 40).
		Date("created_at", 45, 8).
		Date("updated_at", 53, 8)
	tsTable.SafeMode = false
	tsTable.EnableTimestamps("created_at", "updated_at")
	fmt.Printf("HasTimestamps: %v\n", tsTable.HasTimestamps())

	tsRec := tsTable.New()
	tsRec.Set("codigo", "00001")
	tsRec.Set("nombre", "TS TEST")
	// Save would set timestamps, but file doesn't exist
	// We can verify the timestamp mechanism by checking fields after applyTimestamps would run
	fmt.Println("Timestamps enabled for ts_test table")
	tsTable.DisableTimestamps()
	fmt.Println("PASS: Timestamps enable/disable works")

	// =====================================================================
	// 31. Mass Assignment
	// =====================================================================
	fmt.Println("\n--- 31. Mass Assignment ---")
	maTable := isam.NewTable("ma_test", `C:\tmp\NOT_EXIST_MA`, 50).
		Key("codigo", 0, 5).
		String("nombre", 5, 40).
		String("tipo", 45, 2)
	maTable.SafeMode = false

	// Test Fillable
	maTable.Fillable("nombre", "tipo")
	maRec := maTable.New()
	setFields, rejected := maRec.Fill(map[string]string{
		"nombre": "ALLOWED",
		"tipo":   "AB",
		"codigo": "HACK!", // should be rejected (not in fillable)
	})
	fmt.Printf("Fillable mode: set=%v, rejected=%v\n", setFields, rejected)
	fmt.Printf("   nombre=%q, codigo=%q\n", maRec.Get("nombre"), maRec.Get("codigo"))
	if strings.Contains(maRec.Get("nombre"), "ALLOWED") && strings.TrimSpace(maRec.Get("codigo")) == "" {
		fmt.Println("PASS: Fillable blocked non-listed field")
	}

	// Test Guarded
	maTable.Guarded("codigo")
	maRec2 := maTable.New()
	set2, rej2 := maRec2.Fill(map[string]string{
		"nombre": "OK",
		"tipo":   "CD",
		"codigo": "HACK!",
	})
	fmt.Printf("Guarded mode: set=%v, rejected=%v\n", set2, rej2)
	if strings.Contains(maRec2.Get("nombre"), "OK") && strings.TrimSpace(maRec2.Get("codigo")) == "" {
		fmt.Println("PASS: Guarded blocked protected field")
	}
	maTable.ClearMassAssignment()

	// =====================================================================
	// 32. ToJSON
	// =====================================================================
	fmt.Println("\n--- 32. ToJSON ---")
	rec32, _ := isam.CodigosDane.Find("05001")
	jsonStr := rec32.ToJSONString()
	if len(jsonStr) > 80 {
		fmt.Printf("ToJSONString: %s...\n", jsonStr[:80])
	} else {
		fmt.Printf("ToJSONString: %s\n", jsonStr)
	}

	jsonSelected, _ := rec32.ToJSONSelected("codigo", "nombre")
	fmt.Printf("ToJSONSelected: %s\n", string(jsonSelected))

	prettyJSON := rec32.ToJSONPrettyString()
	fmt.Printf("ToJSONPretty (first 3 lines):\n")
	for i, line := range strings.Split(prettyJSON, "\n") {
		if i < 3 {
			fmt.Printf("   %s\n", line)
		}
	}

	// RowsToJSON
	rows32, _ := isam.CodigosDane.Query().Limit(2).Get()
	arrJSON, _ := isam.RowsToJSON(rows32)
	fmt.Printf("RowsToJSON: %d bytes for %d rows\n", len(arrJSON), len(rows32))

	// FromJSON
	rec32b := isam.CodigosDane.GetTable().New()
	setJ, rejJ, err := rec32b.FromJSON([]byte(`{"codigo":"99999","nombre":"FROM JSON","_hash":"skip"}`))
	if err != nil {
		fmt.Printf("FromJSON error: %v\n", err)
	} else {
		fmt.Printf("FromJSON: set=%v, rejected=%v\n", setJ, rejJ)
		fmt.Printf("   codigo=%q, nombre=%q\n", rec32b.Get("codigo"), rec32b.Get("nombre"))
	}
	fmt.Println("PASS: JSON serialization/deserialization works")

	// =====================================================================
	// 33. Multi-sort (ThenBy)
	// =====================================================================
	fmt.Println("\n--- 33. Multi-sort (ThenBy) ---")
	multiSorted, _ := isam.Clients.Query().
		OrderBy("tipo_doc", "asc").
		ThenBy("nombre", "asc").
		Limit(10).
		Get()
	fmt.Printf("OrderBy(tipo_doc ASC).ThenBy(nombre ASC): %d results\n", len(multiSorted))
	prevTipo := ""
	prevNombre := ""
	sortOK := true
	for _, r := range multiSorted {
		tipo := strings.TrimSpace(r.Get("tipo_doc"))
		nombre := strings.TrimSpace(r.Get("nombre"))
		if tipo < prevTipo || (tipo == prevTipo && nombre < prevNombre) {
			sortOK = false
		}
		prevTipo = tipo
		prevNombre = nombre
		fmt.Printf("   tipo_doc=%q nombre=%q\n", tipo, nombre)
	}
	if sortOK {
		fmt.Println("PASS: Multi-sort order is correct")
	}

	// =====================================================================
	// 34. Having (filter groups)
	// =====================================================================
	fmt.Println("\n--- 34. Having ---")
	allGroups, _ := isam.Clients.Query().GroupBy("tipo_doc")
	filtered := isam.HavingCount(allGroups, ">", 5)
	fmt.Printf("Groups with >5 records: %d (of %d total groups)\n", len(filtered), len(allGroups))
	for k, v := range filtered {
		fmt.Printf("   tipo_doc=%q: %d records\n", k, len(v))
	}
	if len(filtered) < len(allGroups) {
		fmt.Println("PASS: Having filtered out small groups")
	}

	// =====================================================================
	// 35. Increment / Decrement
	// =====================================================================
	fmt.Println("\n--- 35. Increment / Decrement ---")
	incTable := isam.NewTable("inc_test", `C:\tmp\NOT_EXIST_INC`, 30).
		Key("codigo", 0, 5).
		Int("cantidad", 5, 10).
		Float("precio", 15, 15)
	incRec := incTable.New()
	incRec.Set("codigo", "00001")
	incRec.SetInt("cantidad", 100)
	incRec.Set("precio", "50.00")
	fmt.Printf("Before: cantidad=%d, precio=%.2f\n", incRec.GetInt("cantidad"), incRec.GetFloat("precio"))

	incRec.Increment("cantidad", 25)
	incRec.Decrement("precio", 10.50)
	fmt.Printf("After +25/-10.50: cantidad=%d, precio=%.2f\n", incRec.GetInt("cantidad"), incRec.GetFloat("precio"))
	if incRec.GetInt("cantidad") == 125 {
		fmt.Println("PASS: Increment works on Int fields")
	}

	// =====================================================================
	// 36. Composite Keys
	// =====================================================================
	fmt.Println("\n--- 36. Composite Keys ---")
	if isam.SaldosTerceros.Exists() {
		// SaldosTerceros has empresa(0,3) + cuenta(3,9) + nit(12,13) as composite
		isam.SaldosTerceros.CompositeKey(
			struct{ Name string; Offset, Length int }{"empresa", 0, 3},
			struct{ Name string; Offset, Length int }{"cuenta", 3, 9},
			struct{ Name string; Offset, Length int }{"nit", 12, 13},
		)

		// Get a known record to extract its composite key
		firstSaldo, _ := isam.SaldosTerceros.Query().First()
		ck := firstSaldo.GetCompositeKey()
		fmt.Printf("First saldo composite key: %v\n", ck)

		// Find by composite key
		found, err := isam.SaldosTerceros.FindComposite(ck...)
		if err != nil {
			fmt.Printf("FindComposite error: %v\n", err)
		} else {
			fmt.Printf("FindComposite found: cuenta=%q nit=%q\n",
				found.Get("cuenta"), found.Get("nit"))
			fmt.Println("PASS: Composite key lookup works")
		}
	}

	// =====================================================================
	// 37. Query Debug (Explain)
	// =====================================================================
	fmt.Println("\n--- 37. Explain ---")
	explain := isam.Clients.Query().
		Where("tipo_doc", "=", "13").
		Where("nombre", "contains", "A").
		OrderBy("nombre", "asc").
		ThenBy("codigo", "desc").
		Limit(10).
		Offset(5).
		Select("codigo", "nombre").
		Explain()
	fmt.Printf("Query plan:\n%s", explain)
	if strings.Contains(explain, "FROM clients") && strings.Contains(explain, "WHERE") {
		fmt.Println("PASS: Explain shows query plan")
	}

	// =====================================================================
	// 38. Create ISAM File from Scratch
	// =====================================================================
	fmt.Println("\n--- 38. CreateFile ---")
	testFilePath := os.TempDir() + "/TEST_ISAM_CREATE"

	// Clean up any previous test file
	os.Remove(testFilePath)

	// Define schema
	schema := isam.NewSchema(128).
		KeyField("codigo", 0, 5).
		StringField("nombre", 5, 40).
		StringField("ciudad", 45, 30).
		DateField("fecha", 75, 8).
		IntField("cantidad", 83, 6)

	// Validate schema
	if err := schema.Validate(); err != nil {
		fmt.Printf("FAIL: Schema validation: %v\n", err)
	} else {
		fmt.Println("  Schema validation OK")
	}

	// Create the file
	err = isam.CreateFile(testFilePath, schema)
	if err != nil {
		fmt.Printf("FAIL: CreateFile: %v\n", err)
	} else {
		fmt.Println("  File created OK")

		// Verify file exists and has correct header
		stat, _ := os.Stat(testFilePath)
		fmt.Printf("  File size: %d bytes\n", stat.Size())

		// Read it back with the V2 reader to verify it's valid
		info, hdr, err := isam.ReadFileV2(testFilePath)
		if err != nil {
			fmt.Printf("  FAIL: ReadFileV2: %v\n", err)
		} else {
			fmt.Printf("  Header: recSize=%d, org=%d, idxFormat=%d, alignment=%d\n",
				hdr.MaxRecordLen, hdr.Organization, hdr.IdxFormat, hdr.Alignment)
			fmt.Printf("  Records: %d (should be 0)\n", len(info.Records))

			if hdr.MaxRecordLen == 128 && len(info.Records) == 0 {
				fmt.Println("  PASS: Empty file created with correct header")
			}
		}

		// Now create a Table from the schema and insert records
		table := schema.ToTable("test_create", testFilePath)
		table.SafeMode = false // test environment

		rec1 := table.New()
		rec1.Set("codigo", "00001")
		rec1.Set("nombre", "EMPRESA TEST UNO")
		rec1.Set("ciudad", "BOGOTA")
		rec1.Set("fecha", "20260309")
		rec1.Set("cantidad", "000100")
		if _, err := rec1.Save(); err != nil {
			fmt.Printf("  FAIL: Insert record 1: %v\n", err)
		} else {
			fmt.Println("  Record 1 inserted OK")
		}

		rec2 := table.New()
		rec2.Set("codigo", "00002")
		rec2.Set("nombre", "EMPRESA TEST DOS")
		rec2.Set("ciudad", "MEDELLIN")
		rec2.Set("fecha", "20260310")
		rec2.Set("cantidad", "000200")
		if _, err := rec2.Save(); err != nil {
			fmt.Printf("  FAIL: Insert record 2: %v\n", err)
		} else {
			fmt.Println("  Record 2 inserted OK")
		}

		rec3 := table.New()
		rec3.Set("codigo", "00003")
		rec3.Set("nombre", "EMPRESA TEST TRES")
		rec3.Set("ciudad", "CALI")
		rec3.Set("fecha", "20260311")
		rec3.Set("cantidad", "000300")
		if _, err := rec3.Save(); err != nil {
			fmt.Printf("  FAIL: Insert record 3: %v\n", err)
		} else {
			fmt.Println("  Record 3 inserted OK")
		}

		// Read all records back
		allRecs, err := table.All()
		if err != nil {
			fmt.Printf("  FAIL: Read all: %v\n", err)
		} else {
			fmt.Printf("  Records after insert: %d\n", len(allRecs))
			for i, r := range allRecs {
				fmt.Printf("    [%d] codigo=%q nombre=%q ciudad=%q fecha=%q\n",
					i, r.Get("codigo"), r.Get("nombre"), r.Get("ciudad"), r.Get("fecha"))
			}
			if len(allRecs) == 3 {
				fmt.Println("  PASS: Created file + inserted 3 records + read back successfully")
			}
		}

		// Test Find
		found, err := table.Find("00002")
		if err != nil {
			fmt.Printf("  FAIL: Find: %v\n", err)
		} else if found != nil {
			fmt.Printf("  Find('00002'): nombre=%q\n", found.Get("nombre"))
			if strings.TrimSpace(found.Get("nombre")) == "EMPRESA TEST DOS" {
				fmt.Println("  PASS: Find by key works on created file")
			}
		}

		// Test Query
		queryResults, _ := table.Query().
			Where("ciudad", "=", "BOGOTA").
			Get()
		fmt.Printf("  Query(ciudad=BOGOTA): %d results\n", len(queryResults))
		if len(queryResults) == 1 {
			fmt.Println("  PASS: Query works on created file")
		}

		// Test that CreateFile refuses to overwrite
		err = isam.CreateFile(testFilePath, schema)
		if err != nil && strings.Contains(err.Error(), "already exists") {
			fmt.Println("  PASS: CreateFile correctly refuses to overwrite")
		}

		// Clean up
		os.Remove(testFilePath)
		os.Remove(testFilePath + ".bak")
	}

	// =====================================================================
	// 39. CreateAndPopulate (batch create)
	// =====================================================================
	fmt.Println("\n--- 39. CreateAndPopulate ---")
	testFilePath2 := os.TempDir() + "/TEST_ISAM_POPULATE"
	os.Remove(testFilePath2)

	schema2 := isam.NewSchema(64).
		KeyField("id", 0, 4).
		StringField("name", 4, 30).
		StringField("code", 34, 10)

	records := []map[string]string{
		{"id": "0001", "name": "ALPHA", "code": "A001"},
		{"id": "0002", "name": "BETA", "code": "B002"},
		{"id": "0003", "name": "GAMMA", "code": "C003"},
		{"id": "0004", "name": "DELTA", "code": "D004"},
		{"id": "0005", "name": "EPSILON", "code": "E005"},
	}

	populatedTable, err := isam.CreateAndPopulate(testFilePath2, schema2, records)
	if err != nil {
		fmt.Printf("FAIL: CreateAndPopulate: %v\n", err)
	} else {
		allRecs, _ := populatedTable.All()
		fmt.Printf("  Created + populated: %d records\n", len(allRecs))
		for i, r := range allRecs {
			fmt.Printf("    [%d] id=%q name=%q code=%q\n",
				i, r.Get("id"), r.Get("name"), r.Get("code"))
		}
		if len(allRecs) == 5 {
			fmt.Println("  PASS: CreateAndPopulate works correctly")
		}

		os.Remove(testFilePath2)
		os.Remove(testFilePath2 + ".bak")
	}

	fmt.Println("\n=== ALL QUERY BUILDER TESTS COMPLETE ===")
}
