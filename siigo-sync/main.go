package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"siigo-common/api"
	"siigo-common/config"
	"siigo-common/parsers"
	gosync "siigo-sync/sync"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("=== Siigo Sync Middleware ===")

	// Load config
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("Config not found at %s, creating default...", cfgPath)
		cfg = config.Default()
		if err := cfg.Save(cfgPath); err != nil {
			log.Fatalf("Cannot save default config: %v", err)
		}
		log.Printf("Default config saved to %s — edit it and restart", cfgPath)
		return
	}

	log.Printf("Config loaded: data_path=%s, api=%s, interval=%ds",
		cfg.Siigo.DataPath, cfg.Finearom.BaseURL, cfg.Sync.IntervalSeconds)

	// Load sync state
	state, err := gosync.LoadState(cfg.Sync.StatePath)
	if err != nil {
		log.Fatalf("Cannot load state: %v", err)
	}
	log.Printf("State loaded: %d files tracked", len(state.Files))

	// Create API client
	client := api.NewClient(cfg.Finearom.BaseURL, cfg.Finearom.Email, cfg.Finearom.Password)

	// Login to Finearom
	if err := client.Login(); err != nil {
		log.Printf("WARNING: Cannot login to Finearom API: %v", err)
		log.Println("Will retry on next sync cycle...")
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Start polling loop
	interval := time.Duration(cfg.Sync.IntervalSeconds) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	log.Printf("Starting sync loop (every %s)...", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run first sync immediately
	runSync(cfg, state, client)

	for {
		select {
		case <-ticker.C:
			runSync(cfg, state, client)
		case <-stop:
			log.Println("Shutting down...")
			if err := state.Save(cfg.Sync.StatePath); err != nil {
				log.Printf("Error saving state: %v", err)
			}
			log.Println("State saved. Goodbye!")
			return
		}
	}
}

func runSync(cfg *config.Config, state *gosync.SyncState, client *api.Client) {
	log.Println("--- Sync cycle start ---")

	for _, file := range cfg.Sync.Files {
		result, err := gosync.DetectChanges(cfg.Siigo.DataPath, file, state)
		if err != nil {
			log.Printf("[%s] Error detecting changes: %v", file, err)
			continue
		}

		if !result.HasChanges {
			log.Printf("[%s] No changes (%d records)", file, result.RecordCount)
			continue
		}

		newCount := 0
		updCount := 0
		delCount := 0
		for _, c := range result.Changes {
			switch c.Type {
			case gosync.ChangeNew:
				newCount++
			case gosync.ChangeUpdated:
				updCount++
			case gosync.ChangeDeleted:
				delCount++
			}
		}
		log.Printf("[%s] Changes detected: %d new, %d updated, %d deleted",
			file, newCount, updCount, delCount)

		// Send changes to Finearom API
		if client.IsAuthenticated() {
			syncErr := sendChanges(cfg, file, result, client)
			if syncErr != nil {
				log.Printf("[%s] Error sending changes: %v", file, syncErr)
			}
		} else {
			log.Printf("[%s] Skipping API sync (not authenticated)", file)
			// Try to re-login
			if err := client.Login(); err != nil {
				log.Printf("Re-login failed: %v", err)
			}
		}

		// Update state regardless (we track changes even if API is down)
		state.UpdateFileState(file, 0, result.NewHashes, result.RecordCount)
	}

	// Save state after each cycle
	if err := state.Save(cfg.Sync.StatePath); err != nil {
		log.Printf("Error saving state: %v", err)
	}

	log.Println("--- Sync cycle end ---")
}

func sendChanges(cfg *config.Config, file string, result *gosync.DetectResult, client *api.Client) error {
	switch {
	case file == "Z17":
		return sendTercerosChanges(cfg, result, client)
	case file == "Z04" || (len(file) >= 3 && file[:3] == "Z04"):
		return sendProductosChanges(cfg, result, client)
	case file == "Z49":
		return sendMovimientosChanges(cfg, result, client)
	case len(file) >= 3 && file[:3] == "Z09":
		anio := ""
		if len(file) > 3 {
			anio = file[3:]
		}
		return sendCarteraChanges(cfg, anio, result, client)
	}
	return nil
}

func sendTercerosChanges(cfg *config.Config, result *gosync.DetectResult, client *api.Client) error {
	// Re-parse to get full records for changed items
	clientes, err := parsers.ParseTercerosClientes(cfg.Siigo.DataPath)
	if err != nil {
		return err
	}

	changedKeys := make(map[string]bool)
	for _, c := range result.Changes {
		if c.Type != gosync.ChangeDeleted {
			changedKeys[c.Key] = true
		}
	}

	sent := 0
	for _, t := range clientes {
		key := t.TipoClave + "-" + t.Empresa + "-" + t.Codigo
		if !changedKeys[key] {
			continue
		}
		data := t.ToFinearomClient()
		nit := t.NumeroDoc
		if err := client.Sync("clients", "add", nit, data); err != nil {
			log.Printf("[Z17] Error syncing client %s (%s): %v", t.Nombre, t.NumeroDoc, err)
			continue
		}
		sent++
	}
	log.Printf("[Z17] Sent %d clients to Finearom", sent)
	return nil
}

func sendProductosChanges(cfg *config.Config, result *gosync.DetectResult, client *api.Client) error {
	productos, _, err := parsers.ParseInventario(cfg.Siigo.DataPath)
	if err != nil {
		return err
	}

	changedKeys := make(map[string]bool)
	for _, c := range result.Changes {
		if c.Type != gosync.ChangeDeleted {
			changedKeys[c.Key] = true
		}
	}

	sent := 0
	for _, p := range productos {
		key := p.Codigo
		if key == "" {
			key = p.Hash
		}
		if !changedKeys[key] {
			continue
		}
		data := p.ToFinearomProduct()
		if err := client.Sync("products", "add", key, data); err != nil {
			log.Printf("[Z04] Error syncing product %s: %v", p.Nombre, err)
			continue
		}
		sent++
	}
	log.Printf("[Z04] Sent %d products to Finearom", sent)
	return nil
}

func sendMovimientosChanges(cfg *config.Config, result *gosync.DetectResult, client *api.Client) error {
	movimientos, err := parsers.ParseMovimientos(cfg.Siigo.DataPath)
	if err != nil {
		return err
	}

	changedKeys := make(map[string]bool)
	for _, c := range result.Changes {
		if c.Type != gosync.ChangeDeleted {
			changedKeys[c.Key] = true
		}
	}

	sent := 0
	for _, m := range movimientos {
		key := m.TipoComprobante + "-" + m.NumeroDoc + "-" + m.NombreTercero
		if key == "--" {
			key = m.Hash
		}
		if !changedKeys[key] {
			continue
		}

		desc := m.Descripcion
		if m.Descripcion2 != "" {
			desc = desc + " " + m.Descripcion2
		}
		data := map[string]interface{}{
			"tipo_comprobante": m.TipoComprobante,
			"numero_doc":      m.NumeroDoc,
			"nombre_tercero":  m.NombreTercero,
			"descripcion":     desc,
			"siigo_sync_hash": m.Hash,
		}
		movKey := m.TipoComprobante + "-" + m.NumeroDoc
		if err := client.Sync("movements", "add", movKey, data); err != nil {
			log.Printf("[Z49] Error syncing movement %s: %v", m.NumeroDoc, err)
			continue
		}
		sent++
	}
	log.Printf("[Z49] Sent %d movements to Finearom", sent)

	return nil
}

func sendCarteraChanges(cfg *config.Config, anio string, result *gosync.DetectResult, client *api.Client) error {
	cartera, err := parsers.ParseCartera(cfg.Siigo.DataPath, anio)
	if err != nil {
		return err
	}

	changedKeys := make(map[string]bool)
	for _, c := range result.Changes {
		if c.Type != gosync.ChangeDeleted {
			changedKeys[c.Key] = true
		}
	}

	sent := 0
	for _, c := range cartera {
		key := c.TipoRegistro + "-" + c.Empresa + "-" + c.Secuencia
		if !changedKeys[key] {
			continue
		}
		data := c.ToFinearomCartera()
		carteraKey := c.TipoRegistro + "-" + c.Empresa + "-" + c.Secuencia
		if err := client.Sync("cartera", "add", carteraKey, data); err != nil {
			log.Printf("[Z09%s] Error syncing cartera %s (%s): %v", anio, c.Secuencia, c.NitTercero, err)
			continue
		}
		sent++
	}
	log.Printf("[Z09%s] Sent %d cartera entries to Finearom", anio, sent)
	return nil
}

// formatFechaISO converts YYYYMMDD to YYYY-MM-DD
func formatFechaISO(fecha string) string {
	if len(fecha) != 8 {
		return fecha
	}
	return fecha[:4] + "-" + fecha[4:6] + "-" + fecha[6:8]
}

func init() {
	// Prevent "declared and not used" for fmt
	_ = fmt.Sprintf
}
