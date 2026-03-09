package isam

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// query.go — Chainable query builder for ISAM tables (Eloquent-style)
//
// Usage:
//
//	results, _ := isam.Clients.Query().
//	    Where("tipo_doc", "=", "13").
//	    Where("empresa", "=", "001").
//	    OrderBy("nombre", "asc").
//	    Limit(10).
//	    Get()
//
//	first, _ := isam.Clients.Query().Where("nombre", "contains", "FINEAROM").First()
//
//	page, _ := isam.Clients.Query().OrderBy("nombre", "asc").Paginate(1, 20)
//
//	names, _ := isam.Clients.Query().Pluck("nombre")
//
// ---------------------------------------------------------------------------

// Query creates a new query builder for this table.
func (t *Table) Query() *QueryBuilder {
	return &QueryBuilder{table: t}
}

// sortField represents a secondary sort criterion.
type sortField struct {
	field string
	dir   string // "asc" or "desc"
}

// QueryBuilder provides chainable query building for ISAM tables.
type QueryBuilder struct {
	table      *Table
	conditions []condition
	orderField string
	orderDir   string // "asc" or "desc"
	extraSorts []sortField // secondary sorts via ThenBy()
	limitN     int         // 0 = no limit
	offsetN    int         // 0 = no offset
	selectCols []string    // empty = all fields
}

type condition struct {
	field    string
	operator string // "=", "!=", ">", ">=", "<", "<=", "contains", "starts_with", "in"
	value    string
	values   []string // for "in" operator
}

// Select specifies which fields to include in ToMap() results.
// Does not affect Get/First (all bytes are read), but controls ToSelectedMap().
func (q *QueryBuilder) Select(fields ...string) *QueryBuilder {
	q.selectCols = fields
	return q
}

// Where adds a filter condition. Operators: =, !=, >, >=, <, <=, contains, starts_with
func (q *QueryBuilder) Where(field, operator, value string) *QueryBuilder {
	q.conditions = append(q.conditions, condition{
		field:    field,
		operator: strings.ToLower(operator),
		value:    value,
	})
	return q
}

// WhereEquals is a shorthand for Where(field, "=", value)
func (q *QueryBuilder) WhereEquals(field, value string) *QueryBuilder {
	return q.Where(field, "=", value)
}

// WhereIn filters records where field value is in the given list.
func (q *QueryBuilder) WhereIn(field string, values []string) *QueryBuilder {
	q.conditions = append(q.conditions, condition{
		field:    field,
		operator: "in",
		values:   values,
	})
	return q
}

// WhereBetween filters records where field is between min and max (inclusive, string comparison).
func (q *QueryBuilder) WhereBetween(field, min, max string) *QueryBuilder {
	q.conditions = append(q.conditions, condition{field: field, operator: ">=", value: min})
	q.conditions = append(q.conditions, condition{field: field, operator: "<=", value: max})
	return q
}

// OrderBy sets the primary sort field and direction ("asc" or "desc").
func (q *QueryBuilder) OrderBy(field, direction string) *QueryBuilder {
	q.orderField = field
	q.orderDir = strings.ToLower(direction)
	if q.orderDir != "desc" {
		q.orderDir = "asc"
	}
	return q
}

// ThenBy adds a secondary sort field. Must be called after OrderBy().
func (q *QueryBuilder) ThenBy(field, direction string) *QueryBuilder {
	dir := strings.ToLower(direction)
	if dir != "desc" {
		dir = "asc"
	}
	q.extraSorts = append(q.extraSorts, sortField{field: field, dir: dir})
	return q
}

// Limit sets the maximum number of results.
func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limitN = n
	return q
}

// Offset sets the number of records to skip.
func (q *QueryBuilder) Offset(n int) *QueryBuilder {
	q.offsetN = n
	return q
}

// Get executes the query and returns matching records.
func (q *QueryBuilder) Get() ([]*Row, error) {
	all, err := q.table.All()
	if err != nil {
		return nil, err
	}

	// Filter
	filtered := q.applyFilters(all)

	// Sort
	if q.orderField != "" {
		q.applySort(filtered)
	}

	// Offset + Limit
	result := q.applyPagination(filtered)

	return result, nil
}

// GetMaps executes the query and returns results as maps.
// If Select() was used, only selected fields are included.
func (q *QueryBuilder) GetMaps() ([]map[string]interface{}, error) {
	rows, err := q.Get()
	if err != nil {
		return nil, err
	}
	maps := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		if len(q.selectCols) > 0 {
			maps[i] = r.ToSelectedMap(q.selectCols)
		} else {
			maps[i] = r.ToMap()
		}
	}
	return maps, nil
}

// First returns the first matching record, or error if none found.
func (q *QueryBuilder) First() (*Row, error) {
	q.limitN = 1
	results, err := q.Get()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no records found in %s matching query", q.table.Name)
	}
	return results[0], nil
}

// Count returns the number of matching records (without loading all data into result).
func (q *QueryBuilder) Count() (int, error) {
	if len(q.conditions) == 0 {
		return q.table.Count()
	}
	all, err := q.table.All()
	if err != nil {
		return 0, err
	}
	return len(q.applyFilters(all)), nil
}

// Exists returns true if at least one record matches.
func (q *QueryBuilder) Exists() (bool, error) {
	count, err := q.Count()
	return count > 0, err
}

// Pluck returns a slice of string values for a single field.
func (q *QueryBuilder) Pluck(field string) ([]string, error) {
	f := q.table.Field(field)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", field, q.table.Name)
	}

	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	values := make([]string, len(rows))
	for i, r := range rows {
		values[i] = r.Get(field)
	}
	return values, nil
}

// PluckFloat returns a slice of float64 values for a numeric/BCD field.
func (q *QueryBuilder) PluckFloat(field string) ([]float64, error) {
	f := q.table.Field(field)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", field, q.table.Name)
	}

	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	values := make([]float64, len(rows))
	for i, r := range rows {
		values[i] = r.GetFloat(field)
	}
	return values, nil
}

// Sum returns the sum of a numeric field for matching records.
func (q *QueryBuilder) Sum(field string) (float64, error) {
	vals, err := q.PluckFloat(field)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, v := range vals {
		total += v
	}
	return total, nil
}

// Avg returns the average of a numeric field for matching records.
func (q *QueryBuilder) Avg(field string) (float64, error) {
	vals, err := q.PluckFloat(field)
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, nil
	}
	var total float64
	for _, v := range vals {
		total += v
	}
	return total / float64(len(vals)), nil
}

// Min returns the minimum value of a numeric field for matching records.
func (q *QueryBuilder) Min(field string) (float64, error) {
	vals, err := q.PluckFloat(field)
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, nil
	}
	min := math.MaxFloat64
	for _, v := range vals {
		if v < min {
			min = v
		}
	}
	return min, nil
}

// Max returns the maximum value of a numeric field for matching records.
func (q *QueryBuilder) Max(field string) (float64, error) {
	vals, err := q.PluckFloat(field)
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, nil
	}
	max := -math.MaxFloat64
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	return max, nil
}

// GroupBy groups matching records by a field value, returning a map of field value → rows.
//
// Example:
//
//	groups, _ := isam.Clients.Query().GroupBy("tipo_doc")
//	for tipo, rows := range groups {
//	    fmt.Printf("Tipo %s: %d records\n", tipo, len(rows))
//	}
func (q *QueryBuilder) GroupBy(field string) (map[string][]*Row, error) {
	f := q.table.Field(field)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", field, q.table.Name)
	}

	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	groups := make(map[string][]*Row)
	for _, r := range rows {
		key := strings.TrimSpace(r.Get(field))
		groups[key] = append(groups[key], r)
	}
	return groups, nil
}

// GroupByCount groups matching records by a field and returns counts per group.
func (q *QueryBuilder) GroupByCount(field string) (map[string]int, error) {
	groups, err := q.GroupBy(field)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(groups))
	for k, v := range groups {
		counts[k] = len(v)
	}
	return counts, nil
}

// Having filters groups from GroupBy, keeping only those that satisfy a condition.
//
// Example:
//
//	groups, _ := isam.Clients.Query().
//	    GroupBy("tipo_doc").
//	    Having("tipo_doc", func(key string, rows []*Row) bool {
//	        return len(rows) > 5
//	    })
func Having(groups map[string][]*Row, predicate func(key string, rows []*Row) bool) map[string][]*Row {
	result := make(map[string][]*Row)
	for k, v := range groups {
		if predicate(k, v) {
			result[k] = v
		}
	}
	return result
}

// HavingCount filters groups keeping only those with count matching the operator.
// Operators: ">", ">=", "<", "<=", "=", "!="
func HavingCount(groups map[string][]*Row, operator string, count int) map[string][]*Row {
	return Having(groups, func(_ string, rows []*Row) bool {
		n := len(rows)
		switch operator {
		case ">":
			return n > count
		case ">=":
			return n >= count
		case "<":
			return n < count
		case "<=":
			return n <= count
		case "=", "==":
			return n == count
		case "!=", "<>":
			return n != count
		default:
			return true
		}
	})
}

// WithScope applies a named scope registered on the table.
func (q *QueryBuilder) WithScope(name string) *QueryBuilder {
	if fn, ok := getScope(q.table.Path, name); ok {
		fn(q)
	}
	return q
}

// Distinct returns unique values for a field among matching records.
//
// Example:
//
//	tipos, _ := isam.Clients.Query().Distinct("tipo_doc")
//	// → ["13", "51", "11", ...]
func (q *QueryBuilder) Distinct(field string) ([]string, error) {
	f := q.table.Field(field)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", field, q.table.Name)
	}

	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var result []string
	for _, r := range rows {
		val := strings.TrimSpace(r.Get(field))
		if !seen[val] {
			seen[val] = true
			result = append(result, val)
		}
	}
	return result, nil
}

// DistinctCount returns unique values with their counts.
func (q *QueryBuilder) DistinctCount(field string) (map[string]int, error) {
	f := q.table.Field(field)
	if f == nil {
		return nil, fmt.Errorf("field %q not found in table %s", field, q.table.Name)
	}

	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range rows {
		val := strings.TrimSpace(r.Get(field))
		counts[val]++
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Batch operations on QueryBuilder
// ---------------------------------------------------------------------------

// BatchUpdateResult holds results of a batch update operation.
type BatchUpdateResult struct {
	Updated int
	Errors  []error
}

// Update applies field changes to all matching records and saves them.
// The updates map contains field name → new value pairs.
//
// Example:
//
//	result, _ := isam.CodigosDane.Query().
//	    Where("codigo", "starts_with", "050").
//	    Update(map[string]string{"nombre": "UPDATED"})
func (q *QueryBuilder) Update(updates map[string]string) (*BatchUpdateResult, error) {
	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	result := &BatchUpdateResult{}
	for _, r := range rows {
		for field, value := range updates {
			if err := r.Set(field, value); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("record #%d field %s: %w", r.index, field, err))
				continue
			}
		}
		if _, err := r.Save(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("record #%d save: %w", r.index, err))
		} else {
			result.Updated++
		}
	}
	return result, nil
}

// UpdateFunc applies a callback to each matching record and saves it.
// More flexible than Update() — allows custom logic per record.
//
// Example:
//
//	result, _ := isam.Clients.Query().
//	    Where("nombre", "contains", "TEST").
//	    UpdateFunc(func(r *isam.Row) {
//	        r.Set("nombre", strings.ToUpper(r.Get("nombre")))
//	    })
func (q *QueryBuilder) UpdateFunc(fn func(*Row)) (*BatchUpdateResult, error) {
	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	result := &BatchUpdateResult{}
	for _, r := range rows {
		fn(r)
		if _, err := r.Save(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("record #%d: %w", r.index, err))
		} else {
			result.Updated++
		}
	}
	return result, nil
}

// BatchDeleteResult holds results of a batch delete operation.
type BatchDeleteResult struct {
	Deleted int
	Errors  []error
}

// Delete removes all matching records from the ISAM file.
//
// Example:
//
//	result, _ := isam.CodigosDane.Query().
//	    Where("codigo", "starts_with", "99").
//	    Delete()
func (q *QueryBuilder) Delete() (*BatchDeleteResult, error) {
	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	result := &BatchDeleteResult{}
	// Delete in reverse order to avoid index shifts
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		if _, err := r.Delete(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("record #%d: %w", r.index, err))
		} else {
			result.Deleted++
		}
	}
	return result, nil
}

// SoftDeleteAll soft-deletes all matching records (requires EnableSoftDelete).
func (q *QueryBuilder) SoftDeleteAll() (int, error) {
	rows, err := q.Get()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, r := range rows {
		if err := r.SoftDelete(); err == nil {
			count++
		}
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Eager Loading
// ---------------------------------------------------------------------------

// RelationDef defines how to eager-load a relationship.
type RelationDef struct {
	Name         string    // name used to store results (e.g. "saldos")
	Related      Relatable // the related table/model
	ForeignField string    // field on the related table
	LocalField   string    // field on this table
	Type         string    // "has_many" or "belongs_to"
}

// EagerResult wraps a Row with its pre-loaded related records.
type EagerResult struct {
	*Row
	Relations map[string]interface{} // name → []*Row (has_many) or *Row (belongs_to)
}

// With pre-loads related records for all matching rows, avoiding the N+1 problem.
// Pass RelationDefs to specify which relationships to load.
//
// Example:
//
//	rels := []isam.RelationDef{
//	    {Name: "saldos", Related: isam.SaldosTerceros, ForeignField: "nit", LocalField: "codigo", Type: "has_many"},
//	    {Name: "plan", Related: isam.PlanCuentas, ForeignField: "cuenta", LocalField: "cuenta", Type: "belongs_to"},
//	}
//	results, _ := isam.Clients.Query().
//	    Where("nombre", "contains", "A").
//	    With(rels...)
func (q *QueryBuilder) With(relations ...RelationDef) ([]*EagerResult, error) {
	rows, err := q.Get()
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 || len(relations) == 0 {
		results := make([]*EagerResult, len(rows))
		for i, r := range rows {
			results[i] = &EagerResult{Row: r, Relations: map[string]interface{}{}}
		}
		return results, nil
	}

	// Pre-load each relation
	relatedData := make(map[string]map[string]interface{}) // relation name → localValue → data
	for _, rel := range relations {
		relatedTable := rel.Related.GetTable()

		// Collect unique local field values
		localValues := make(map[string]bool)
		for _, r := range rows {
			val := strings.TrimSpace(r.Get(rel.LocalField))
			if val != "" {
				localValues[val] = true
			}
		}

		// Load ALL related records in one pass
		allRelated, err := relatedTable.All()
		if err != nil {
			continue
		}

		// Index related records by their foreign field
		indexed := make(map[string]interface{})
		if rel.Type == "belongs_to" {
			// belongs_to: one record per value
			for _, rr := range allRelated {
				fk := strings.TrimSpace(rr.Get(rel.ForeignField))
				if localValues[fk] {
					indexed[fk] = rr
				}
			}
		} else {
			// has_many: multiple records per value
			groups := make(map[string][]*Row)
			for _, rr := range allRelated {
				fk := strings.TrimSpace(rr.Get(rel.ForeignField))
				if localValues[fk] {
					groups[fk] = append(groups[fk], rr)
				}
			}
			for k, v := range groups {
				indexed[k] = v
			}
		}

		relatedData[rel.Name] = indexed
	}

	// Assemble results
	results := make([]*EagerResult, len(rows))
	for i, r := range rows {
		er := &EagerResult{Row: r, Relations: make(map[string]interface{})}

		for _, rel := range relations {
			localVal := strings.TrimSpace(r.Get(rel.LocalField))
			if data, ok := relatedData[rel.Name]; ok {
				if related, found := data[localVal]; found {
					er.Relations[rel.Name] = related
				} else if rel.Type == "has_many" {
					er.Relations[rel.Name] = []*Row{}
				}
			}
		}

		results[i] = er
	}

	return results, nil
}

// GetRelatedMany returns the has_many relation from an EagerResult by name.
func (er *EagerResult) GetRelatedMany(name string) []*Row {
	if v, ok := er.Relations[name]; ok {
		if rows, ok := v.([]*Row); ok {
			return rows
		}
	}
	return nil
}

// GetRelatedOne returns the belongs_to relation from an EagerResult by name.
func (er *EagerResult) GetRelatedOne(name string) *Row {
	if v, ok := er.Relations[name]; ok {
		if row, ok := v.(*Row); ok {
			return row
		}
	}
	return nil
}

// Chunk processes matching records in batches, calling fn for each batch.
// Returns the total number of records processed.
//
// Example:
//
//	total, _ := isam.CodigosDane.Query().Chunk(100, func(batch []*isam.Row) error {
//	    for _, r := range batch {
//	        // process record
//	    }
//	    return nil // return error to stop chunking
//	})
func (q *QueryBuilder) Chunk(size int, fn func([]*Row) error) (int, error) {
	if size < 1 {
		size = 100
	}

	rows, err := q.Get()
	if err != nil {
		return 0, err
	}

	total := 0
	for i := 0; i < len(rows); i += size {
		end := i + size
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		total += len(batch)
		if err := fn(batch); err != nil {
			return total, err
		}
	}
	return total, nil
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

// PageResult holds paginated query results.
type PageResult struct {
	Data    []*Row `json:"data"`
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	Total   int    `json:"total"`
	Pages   int    `json:"pages"`
}

// Paginate returns a page of results with metadata.
func (q *QueryBuilder) Paginate(page, perPage int) (*PageResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	all, err := q.table.All()
	if err != nil {
		return nil, err
	}

	filtered := q.applyFilters(all)

	if q.orderField != "" {
		q.applySort(filtered)
	}

	total := len(filtered)
	pages := (total + perPage - 1) / perPage
	if pages == 0 {
		pages = 1
	}

	start := (page - 1) * perPage
	if start >= total {
		return &PageResult{Data: []*Row{}, Page: page, PerPage: perPage, Total: total, Pages: pages}, nil
	}
	end := start + perPage
	if end > total {
		end = total
	}

	return &PageResult{
		Data:    filtered[start:end],
		Page:    page,
		PerPage: perPage,
		Total:   total,
		Pages:   pages,
	}, nil
}

// ---------------------------------------------------------------------------
// Relationships
// ---------------------------------------------------------------------------

// Relatable is any type that can provide a *Table (both *Table and *Model satisfy this).
type Relatable interface {
	GetTable() *Table
}

// GetTable implements Relatable for Table.
func (t *Table) GetTable() *Table { return t }

// HasMany returns related records from another table/model where foreignField matches
// this row's value for localField. Like Eloquent's hasMany().
//
// Example:
//
//	client, _ := isam.Clients.Find("00000000002001")
//	cartera, _ := client.HasMany(isam.Cartera, "nit", "codigo")
func (r *Row) HasMany(related Relatable, foreignField, localField string) ([]*Row, error) {
	localValue := strings.TrimSpace(r.Get(localField))
	if localValue == "" {
		return nil, fmt.Errorf("local field %q is empty", localField)
	}

	return related.GetTable().Query().Where(foreignField, "=", localValue).Get()
}

// BelongsTo returns the parent record from another table/model where foreignField matches
// this row's value for localField. Like Eloquent's belongsTo().
//
// Example:
//
//	carteraRec, _ := isam.Cartera.Find("00123")
//	client, _ := carteraRec.BelongsTo(isam.Clients, "codigo", "nit")
func (r *Row) BelongsTo(parent Relatable, foreignField, localField string) (*Row, error) {
	localValue := strings.TrimSpace(r.Get(localField))
	if localValue == "" {
		return nil, fmt.Errorf("local field %q is empty", localField)
	}

	return parent.GetTable().Query().Where(foreignField, "=", localValue).First()
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (q *QueryBuilder) applyFilters(rows []*Row) []*Row {
	if len(q.conditions) == 0 {
		return rows
	}

	var result []*Row
	for _, r := range rows {
		if q.matchesAll(r) {
			result = append(result, r)
		}
	}
	return result
}

func (q *QueryBuilder) matchesAll(r *Row) bool {
	for _, c := range q.conditions {
		if !q.matchCondition(r, c) {
			return false
		}
	}
	return true
}

func (q *QueryBuilder) matchCondition(r *Row, c condition) bool {
	val := strings.TrimSpace(r.Get(c.field))
	target := strings.TrimSpace(c.value)

	switch c.operator {
	case "=", "eq":
		return val == target
	case "!=", "ne", "<>":
		return val != target
	case ">", "gt":
		return val > target
	case ">=", "gte", "ge":
		return val >= target
	case "<", "lt":
		return val < target
	case "<=", "lte", "le":
		return val <= target
	case "contains", "like":
		return strings.Contains(strings.ToUpper(val), strings.ToUpper(target))
	case "starts_with", "startswith":
		return strings.HasPrefix(strings.ToUpper(val), strings.ToUpper(target))
	case "ends_with", "endswith":
		return strings.HasSuffix(strings.ToUpper(val), strings.ToUpper(target))
	case "in":
		for _, v := range c.values {
			if val == strings.TrimSpace(v) {
				return true
			}
		}
		return false
	default:
		return val == target
	}
}

func (q *QueryBuilder) applySort(rows []*Row) {
	f := q.table.Field(q.orderField)
	if f == nil {
		return
	}

	// Build full sort chain: primary + secondary sorts
	type sortSpec struct {
		field     string
		dir       string
		isNumeric bool
	}
	specs := []sortSpec{{
		field:     q.orderField,
		dir:       q.orderDir,
		isNumeric: f.Type == FieldBCD || f.Type == FieldInt || f.Type == FieldFloat,
	}}
	for _, es := range q.extraSorts {
		ef := q.table.Field(es.field)
		if ef != nil {
			specs = append(specs, sortSpec{
				field:     es.field,
				dir:       es.dir,
				isNumeric: ef.Type == FieldBCD || ef.Type == FieldInt || ef.Type == FieldFloat,
			})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		for _, s := range specs {
			var cmp int
			if s.isNumeric {
				vi := rows[i].GetFloat(s.field)
				vj := rows[j].GetFloat(s.field)
				if vi < vj {
					cmp = -1
				} else if vi > vj {
					cmp = 1
				}
			} else {
				vi := strings.TrimSpace(rows[i].Get(s.field))
				vj := strings.TrimSpace(rows[j].Get(s.field))
				if vi < vj {
					cmp = -1
				} else if vi > vj {
					cmp = 1
				}
			}
			if cmp == 0 {
				continue // tie — use next sort field
			}
			if s.dir == "desc" {
				return cmp > 0
			}
			return cmp < 0
		}
		return false // all fields equal
	})
}

func (q *QueryBuilder) applyPagination(rows []*Row) []*Row {
	start := q.offsetN
	if start > len(rows) {
		return nil
	}
	rows = rows[start:]

	if q.limitN > 0 && q.limitN < len(rows) {
		rows = rows[:q.limitN]
	}
	return rows
}
