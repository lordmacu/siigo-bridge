package main

import (
	"fmt"
	"siigo-common/storage"
)

func main() {
	db, err := storage.NewDB(`C:\Users\lordmacu\siigo\siigo-web\siigo_web.db`)
	if err != nil {
		fmt.Println("Error abriendo DB:", err)
		return
	}
	defer db.Close()

	err = db.ClearAll()
	if err != nil {
		fmt.Println("Error limpiando:", err)
		return
	}
	fmt.Println("Base de datos vaciada exitosamente.")
}
