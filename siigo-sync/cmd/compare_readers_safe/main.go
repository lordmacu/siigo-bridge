package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"siigo-common/isam"
)

type fileSpec struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type readerRun struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	Count      int    `json:"count,omitempty"`
	RecSize    int    `json:"rec_size,omitempty"`
	FirstHash  string `json:"first_hash,omitempty"`
	LastHash   string `json:"last_hash,omitempty"`
	IdxFormat  int    `json:"idx_format,omitempty"`
	NumKeys    int    `json:"num_keys,omitempty"`
	IsVarLen   bool   `json:"is_var_len,omitempty"`
	License247 bool   `json:"license_247,omitempty"`
}

type compareFlags struct {
	CountMatch bool `json:"count_match"`
	FirstMatch bool `json:"first_match"`
	LastMatch  bool `json:"last_match"`
}

type fileResult struct {
	Name   string       `json:"name"`
	Desc   string       `json:"desc"`
	Path   string       `json:"path"`
	Exists bool         `json:"exists"`
	V1     readerRun    `json:"v1"`
	V2     readerRun    `json:"v2"`
	Extfh  readerRun    `json:"extfh"`
	V1VsV2 compareFlags `json:"v1_vs_v2"`
	V1VsE  compareFlags `json:"v1_vs_extfh"`
	V2VsE  compareFlags `json:"v2_vs_extfh"`
}

type summary struct {
	FilesTotal            int `json:"files_total"`
	FilesPresent          int `json:"files_present"`
	ExtfhSuccess          int `json:"extfh_success"`
	ExtfhErrors           int `json:"extfh_errors"`
	ExtfhLicense247       int `json:"extfh_license_247"`
	V1V2CountMatches      int `json:"v1_v2_count_matches"`
	V1VsExtfhCountMatches int `json:"v1_vs_extfh_count_matches"`
	V2VsExtfhCountMatches int `json:"v2_vs_extfh_count_matches"`
}

type runReport struct {
	GeneratedAt string       `json:"generated_at"`
	DataPath    string       `json:"data_path"`
	ExtfhDLL    string       `json:"extfh_dll"`
	Results     []fileResult `json:"results"`
	Summary     summary      `json:"summary"`
}

type probePayload struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Count     int    `json:"count,omitempty"`
	RecSize   int    `json:"rec_size,omitempty"`
	FirstHash string `json:"first_hash,omitempty"`
	LastHash  string `json:"last_hash,omitempty"`
	IdxFormat int    `json:"idx_format,omitempty"`
	NumKeys   int    `json:"num_keys,omitempty"`
	IsVarLen  bool   `json:"is_var_len,omitempty"`
}

func defaultFiles() []fileSpec {
	return []fileSpec{
		{"Z17", "Terceros"},
		{"Z06", "Maestros"},
		{"Z49", "Movimientos"},
		{"Z032016", "Plan Cuentas"},
		{"Z042016", "Inventario"},
		{"Z092016", "Cartera"},
		{"Z112016", "Documentos"},
		{"Z182016", "Historial"},
		{"Z252016", "Saldos Terceros"},
		{"Z262016", "Periodos"},
		{"Z272016", "Activos Fijos"},
		{"Z282016", "Saldos Consol."},
		{"Z052016", "Cond. Pago"},
		{"Z072016", "Libros Auxiliares"},
		{"Z07T", "Trans. Detalle"},
		{"Z082016A", "Terceros Amp."},
		{"ZDANE", "DANE Municipios"},
		{"ZICA", "ICA"},
		{"ZPILA", "PILA"},
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	// Short hash is enough for fast comparisons in reports.
	return hex.EncodeToString(sum[:8])
}

func toRunFromRaw(records [][]byte, recSize int) readerRun {
	rr := readerRun{
		OK:      true,
		Count:   len(records),
		RecSize: recSize,
	}
	if len(records) > 0 {
		rr.FirstHash = hashBytes(records[0])
		rr.LastHash = hashBytes(records[len(records)-1])
	}
	return rr
}

func runV1(path string) readerRun {
	info, err := isam.ReadFile(path)
	if err != nil {
		return readerRun{Error: err.Error()}
	}
	raw := make([][]byte, len(info.Records))
	for i := range info.Records {
		raw[i] = info.Records[i].Data
	}
	return toRunFromRaw(raw, info.RecordSize)
}

func runV2(path string) readerRun {
	recs, stats, err := isam.ReadFileV2WithStats(path)
	if err != nil {
		return readerRun{Error: err.Error()}
	}
	rr := toRunFromRaw(recs, int(stats.Header.MaxRecordLen))
	rr.IdxFormat = int(stats.Header.IdxFormat)
	return rr
}

func probeExtfhSubprocess(path string) readerRun {
	exe, err := os.Executable()
	if err != nil {
		return readerRun{Error: fmt.Sprintf("cannot resolve executable: %v", err)}
	}
	cmd := exec.Command(exe, "-probe-extfh", path)
	cmd.Env = os.Environ()
	out, runErr := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	// Try JSON payload first.
	var payload probePayload
	if output != "" && json.Unmarshal([]byte(output), &payload) == nil {
		rr := readerRun{
			OK:        payload.OK,
			Error:     payload.Error,
			Count:     payload.Count,
			RecSize:   payload.RecSize,
			FirstHash: payload.FirstHash,
			LastHash:  payload.LastHash,
			IdxFormat: payload.IdxFormat,
			NumKeys:   payload.NumKeys,
			IsVarLen:  payload.IsVarLen,
		}
		if strings.Contains(strings.ToLower(rr.Error), "error code: 247") {
			rr.License247 = true
		}
		return rr
	}

	// Non-JSON output usually means runtime/loader failure.
	errText := output
	if errText == "" && runErr != nil {
		errText = runErr.Error()
	}
	rr := readerRun{Error: errText}
	if strings.Contains(strings.ToLower(errText), "error code: 247") {
		rr.License247 = true
	}
	return rr
}

func compare(a, b readerRun) compareFlags {
	if !a.OK || !b.OK {
		return compareFlags{}
	}
	return compareFlags{
		CountMatch: a.Count == b.Count,
		FirstMatch: a.FirstHash != "" && a.FirstHash == b.FirstHash,
		LastMatch:  a.LastHash != "" && a.LastHash == b.LastHash,
	}
}

func extfhProbeMain(path string) int {
	resp := probePayload{}
	if !isam.ExtfhAvailable() {
		resp.Error = "EXTFH DLL not available"
		_ = json.NewEncoder(os.Stdout).Encode(resp)
		return 2
	}

	f, err := isam.OpenIsamFile(path)
	if err != nil {
		resp.Error = err.Error()
		_ = json.NewEncoder(os.Stdout).Encode(resp)
		return 2
	}
	defer f.Close()

	recs, err := f.ReadAll()
	if err != nil {
		resp.Error = err.Error()
		_ = json.NewEncoder(os.Stdout).Encode(resp)
		return 2
	}

	resp.OK = true
	resp.Count = len(recs)
	resp.RecSize = f.RecSize
	resp.IdxFormat = f.Format
	resp.NumKeys = f.NumKeys
	resp.IsVarLen = f.IsVarLen
	if len(recs) > 0 {
		resp.FirstHash = hashBytes(recs[0])
		resp.LastHash = hashBytes(recs[len(recs)-1])
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
	return 0
}

func fileListFromArg(arg string) []fileSpec {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return defaultFiles()
	}
	names := strings.Split(arg, ",")
	out := make([]fileSpec, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		out = append(out, fileSpec{Name: n, Desc: n})
	}
	if len(out) == 0 {
		return defaultFiles()
	}
	return out
}

func main() {
	var (
		dataPath   string
		reportPath string
		fileArg    string
		probeOnly  string
	)
	flag.StringVar(&dataPath, "data", `C:\SIIWI02`, "Directorio de datos ISAM")
	flag.StringVar(&reportPath, "report", "compare_readers_safe.json", "Ruta del reporte JSON")
	flag.StringVar(&fileArg, "files", "", "CSV list of files to compare (e.g.: Z17,Z06,Z49)")
	flag.StringVar(&probeOnly, "probe-extfh", "", "Internal use: test EXTFH for a single file")
	flag.Parse()

	if probeOnly != "" {
		os.Exit(extfhProbeMain(probeOnly))
		return
	}

	files := fileListFromArg(fileArg)
	rep := runReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DataPath:    dataPath,
		ExtfhDLL:    isam.ExtfhDLLPath(),
		Results:     make([]fileResult, 0, len(files)),
	}
	rep.Summary.FilesTotal = len(files)

	licenseBlocked := false
	for _, fs := range files {
		path := filepath.Join(dataPath, fs.Name)
		res := fileResult{
			Name: fs.Name,
			Desc: fs.Desc,
			Path: path,
		}
		if _, err := os.Stat(path); err != nil {
			rep.Results = append(rep.Results, res)
			continue
		}
		res.Exists = true
		rep.Summary.FilesPresent++

		res.V1 = runV1(path)
		res.V2 = runV2(path)

		if licenseBlocked {
			res.Extfh = readerRun{
				Error:      "skipped due previous EXTFH license 247 failure",
				License247: true,
			}
		} else {
			res.Extfh = probeExtfhSubprocess(path)
			if res.Extfh.License247 {
				licenseBlocked = true
			}
		}

		res.V1VsV2 = compare(res.V1, res.V2)
		res.V1VsE = compare(res.V1, res.Extfh)
		res.V2VsE = compare(res.V2, res.Extfh)

		if res.V1VsV2.CountMatch {
			rep.Summary.V1V2CountMatches++
		}
		if res.Extfh.OK {
			rep.Summary.ExtfhSuccess++
			if res.V1VsE.CountMatch {
				rep.Summary.V1VsExtfhCountMatches++
			}
			if res.V2VsE.CountMatch {
				rep.Summary.V2VsExtfhCountMatches++
			}
		} else {
			rep.Summary.ExtfhErrors++
			if res.Extfh.License247 {
				rep.Summary.ExtfhLicense247++
			}
		}

		rep.Results = append(rep.Results, res)
	}

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Printf("error serializando reporte: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(reportPath, b, 0o644); err != nil {
		fmt.Printf("error escribiendo reporte %s: %v\n", reportPath, err)
		os.Exit(1)
	}

	fmt.Println("====================================================================")
	fmt.Println("  ISAM SAFE COMPARISON: EXTFH (subprocess) vs V1 vs V2")
	fmt.Println("====================================================================")
	fmt.Printf("  Data path: %s\n", rep.DataPath)
	fmt.Printf("  EXTFH DLL: %s\n", rep.ExtfhDLL)
	fmt.Printf("  Files present: %d/%d\n", rep.Summary.FilesPresent, rep.Summary.FilesTotal)
	fmt.Printf("  EXTFH success/errors: %d/%d (247=%d)\n",
		rep.Summary.ExtfhSuccess, rep.Summary.ExtfhErrors, rep.Summary.ExtfhLicense247)
	fmt.Printf("  Count matches: V1~V2=%d, V1~EXTFH=%d, V2~EXTFH=%d\n",
		rep.Summary.V1V2CountMatches, rep.Summary.V1VsExtfhCountMatches, rep.Summary.V2VsExtfhCountMatches)
	fmt.Printf("  Report: %s\n", reportPath)
	fmt.Println("====================================================================")

	var lines []string
	for _, r := range rep.Results {
		if !r.Exists {
			continue
		}
		v1 := "ERR"
		if r.V1.OK {
			v1 = fmt.Sprintf("%d", r.V1.Count)
		}
		v2 := "ERR"
		if r.V2.OK {
			v2 = fmt.Sprintf("%d", r.V2.Count)
		}
		ext := "ERR"
		if r.Extfh.OK {
			ext = fmt.Sprintf("%d", r.Extfh.Count)
		}
		line := fmt.Sprintf("%-10s V1=%-6s V2=%-6s EXTFH=%-8s", r.Name, v1, v2, ext)
		if r.Extfh.License247 {
			line += "  [247]"
		}
		lines = append(lines, line)
	}
	fmt.Println(strings.Join(lines, "\n"))
}
