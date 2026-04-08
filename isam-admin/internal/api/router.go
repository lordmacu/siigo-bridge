package api

import (
	"net/http"
	"strings"

	"isam-admin/internal/handlers"
	"isam-admin/internal/middleware"
	"isam-admin/internal/schemas"
)

// NewRouter creates the HTTP router with all API routes
func NewRouter(schemasDir, frontendDir string) http.Handler {
	mux := http.NewServeMux()

	// Schema manager
	schemaManager := schemas.NewManager(schemasDir)
	schemaHandlers := handlers.NewSchemaHandlers(schemaManager)

	// Table manager
	tableManager := handlers.NewTableManager(schemaManager)

	// === File browser ===
	mux.HandleFunc("/api/files/browse", handlers.FileBrowseHandler)
	mux.HandleFunc("/api/files/info", handlers.FileInfoHandler)
	mux.HandleFunc("/api/files/hex", handlers.FileHexHandler)
	mux.HandleFunc("/api/files/detect", handlers.FieldDetectHandler)

	// === Tables (open/close/list) ===
	mux.HandleFunc("/api/tables", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			tableManager.ListTablesHandler(w, r)
		case "POST":
			tableManager.OpenTableHandler(w, r)
		case "DELETE":
			tableManager.CloseTableHandler(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})

	// === Records ===
	mux.HandleFunc("/api/records", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			tableManager.RecordsHandler(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/record", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET", "PUT", "DELETE":
			tableManager.RecordCRUDHandler(w, r)
		case "POST":
			tableManager.InsertRecordHandler(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})

	// === Query ===
	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			tableManager.QueryHandler(w, r)
		} else if r.Method == "GET" {
			tableManager.QuickQueryHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", 405)
		}
	})

	// === Schemas ===
	mux.HandleFunc("/api/schemas", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if r.URL.Query().Get("name") != "" {
				schemaHandlers.GetHandler(w, r)
			} else {
				schemaHandlers.ListHandler(w, r)
			}
		case "POST":
			schemaHandlers.SaveHandler(w, r)
		case "DELETE":
			schemaHandlers.DeleteHandler(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})

	// === Create file from schema ===
	mux.HandleFunc("/api/schemas/create-file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			schemaHandlers.CreateFileHandler(w, r)
		} else {
			http.Error(w, "Method not allowed", 405)
		}
	})

	// === SPA fallback: serve frontend ===
	fs := http.FileServer(http.Dir(frontendDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// API routes already handled above
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Try serving static file
		fs.ServeHTTP(w, r)
	})

	// Apply middleware
	var handler http.Handler = mux
	handler = middleware.CORS(handler)

	return handler
}
