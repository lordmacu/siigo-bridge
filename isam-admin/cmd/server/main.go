package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"isam-admin/internal/api"
)

func main() {
	port := "4200"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	// Resolve paths relative to executable
	execDir, _ := os.Executable()
	baseDir := filepath.Dir(execDir)

	schemasDir := filepath.Join(baseDir, "data", "schemas")
	frontendDir := filepath.Join(baseDir, "frontend", "dist")

	// Fallback to current directory structure (for development)
	if _, err := os.Stat(schemasDir); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		schemasDir = filepath.Join(cwd, "data", "schemas")
		frontendDir = filepath.Join(cwd, "frontend", "dist")
	}

	os.MkdirAll(schemasDir, 0755)

	router := api.NewRouter(schemasDir, frontendDir)

	fmt.Println("===========================================")
	fmt.Println("  ISAM Admin — phpMyAdmin for ISAM files")
	fmt.Println("===========================================")
	fmt.Printf("  Server:   http://localhost:%s\n", port)
	fmt.Printf("  API:      http://localhost:%s/api/\n", port)
	fmt.Printf("  Schemas:  %s\n", schemasDir)
	fmt.Println("===========================================")

	log.Fatal(http.ListenAndServe(":"+port, router))
}
