// +build ignore

package main

import (
	"fmt"
	"os"
	"siigo-common/isam"
	"time"
)

func main() {
	dataPath := `C:\SIIWI02`
	year := "2016"

	// Connect the Codigos DANE model (small file, safe to test)
	isam.CodigosDane.Connect(dataPath, year)
	isam.CodigosDane.SafeMode = false // disable safety for test

	fmt.Println("Reading ZDANE file...")
	all, err := isam.CodigosDane.All()
	if err != nil || len(all) == 0 {
		fmt.Println("Error reading or empty:", err)
		os.Exit(1)
	}
	rec := all[0]
	_ = err
	if err != nil {
		fmt.Println("Error reading:", err)
		os.Exit(1)
	}

	originalName := rec.Get("nombre")
	fmt.Printf("Record found: codigo=%s, nombre=%s\n", rec.Get("codigo"), originalName)

	// Modify the record
	testName := "TEST_WATCHER_" + time.Now().Format("150405")
	fmt.Printf("Changing nombre to: %s\n", testName)
	rec.Set("nombre", testName)
	if _, err := rec.Save(); err != nil {
		fmt.Println("Error saving:", err)
		os.Exit(1)
	}
	fmt.Println("Record saved! Watcher should trigger now.")
	fmt.Println("Waiting 3 seconds for watcher to detect...")
	time.Sleep(3 * time.Second)

	// Restore original value
	fmt.Printf("Restoring original nombre: %s\n", originalName)
	rec.Set("nombre", originalName)
	if _, err := rec.Save(); err != nil {
		fmt.Println("Error restoring:", err)
		os.Exit(1)
	}
	fmt.Println("Original value restored.")
}
