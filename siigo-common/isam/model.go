package isam

import (
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// model.go — Eloquent-style model layer for ISAM tables
//
// Each model is a package-level variable that declares its own file, schema,
// and fields. Call ConnectAll() once to resolve file paths, then use models
// directly for CRUD operations.
//
// Usage:
//
//	isam.ConnectAll(`C:\SIIWI02`, "2016")
//
//	all, _ := isam.Clients.All()
//	rec, _ := isam.Clients.Find("00000000002001")
//	rec.Set("nombre", "NUEVO NOMBRE")
//	rec.Save()
//
//	newRec := isam.Clients.New()
//	newRec.Set("codigo", "99999999999999")
//	newRec.Save()
//
// ---------------------------------------------------------------------------

// Model wraps a Table with connection and self-describing metadata.
type Model struct {
	*Table
	file    string
	hasYear bool
	suffix  string // e.g. "A" for Z08YYYYA
}

// registeredModels holds all defined models for ConnectAll
var registeredModels = map[string]*Model{}

// DefineModel creates a self-describing model. Called at package init time.
func DefineModel(name, file string, hasYear bool, suffix string, recSize int, configure func(*Model)) *Model {
	m := &Model{
		Table:   NewTable(name, "", recSize),
		file:    file,
		hasYear: hasYear,
		suffix:  suffix,
	}
	configure(m)
	registeredModels[name] = m
	return m
}

// ConnectAll resolves file paths for all registered models.
// Call once at startup with the Siigo data directory and year.
func ConnectAll(dataDir, year string) {
	for _, m := range registeredModels {
		m.resolve(dataDir, year)
	}
}

// Connect resolves this model's file path individually.
func (m *Model) Connect(dataDir, year string) *Model {
	m.resolve(dataDir, year)
	return m
}

func (m *Model) resolve(dataDir, year string) {
	name := m.file
	if m.hasYear {
		name += year
	}
	name += m.suffix
	m.Table.Path = filepath.Join(dataDir, name)
}

// Exists returns true if the model's ISAM file exists on disk.
func (m *Model) Exists() bool {
	_, err := os.Stat(m.Table.Path)
	return err == nil
}

// FileName returns the base file name (e.g. "Z17", "Z08").
func (m *Model) FileName() string {
	return m.file
}

// GetTable implements Relatable interface, allowing Models to be used in HasMany/BelongsTo.
func (m *Model) GetTable() *Table {
	return m.Table
}

// AllModels returns all registered model names.
func AllModels() []string {
	names := make([]string, 0, len(registeredModels))
	for k := range registeredModels {
		names = append(names, k)
	}
	return names
}

// GetModel returns a registered model by name, or nil.
func GetModel(name string) *Model {
	return registeredModels[name]
}

// AllMultiYear reads records from year-based files filtered by minYear.
// e.g. AllMultiYear(dataDir, "2025") reads Z092025 + Z092026 + ...
// Use "" for minYear to read all years.
func (m *Model) AllMultiYear(dataDir string, minYear ...string) ([]*Row, error) {
	if !m.hasYear {
		return m.All()
	}

	pattern := filepath.Join(dataDir, m.file+"*"+m.suffix)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	yearFloor := "2010"
	if len(minYear) > 0 && minYear[0] != "" {
		yearFloor = minYear[0]
	}

	var allRows []*Row
	for _, path := range matches {
		base := filepath.Base(path)
		rest := base[len(m.file):]
		if m.suffix != "" {
			rest = strings.TrimSuffix(rest, m.suffix)
		}
		if len(rest) != 4 {
			continue
		}
		if rest < yearFloor || rest > "2099" {
			continue
		}

		// Temporarily set path to this year's file
		origPath := m.Table.Path
		m.Table.Path = path
		rows, err := m.All()
		m.Table.Path = origPath
		if err != nil {
			continue
		}
		allRows = append(allRows, rows...)
	}

	return allRows, nil
}

// AvailableYears returns all years that have ISAM files for this model.
func (m *Model) AvailableYears(dataDir string) []string {
	if !m.hasYear {
		return nil
	}
	pattern := filepath.Join(dataDir, m.file+"*"+m.suffix)
	matches, _ := filepath.Glob(pattern)
	var years []string
	for _, path := range matches {
		base := filepath.Base(path)
		rest := base[len(m.file):]
		if m.suffix != "" {
			rest = strings.TrimSuffix(rest, m.suffix)
		}
		if len(rest) == 4 && rest >= "2010" && rest <= "2099" {
			years = append(years, rest)
		}
	}
	return years
}

// AvailableModels returns only models whose ISAM files exist on disk.
func AvailableModels() []string {
	var available []string
	for name, m := range registeredModels {
		if m.Exists() {
			available = append(available, name)
		}
	}
	return available
}
