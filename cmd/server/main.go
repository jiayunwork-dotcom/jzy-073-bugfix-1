// Command isa-service serves the International Standard Atmosphere model
// (0-20 km) over HTTP. Nothing is persisted: every request is computed
// independently.
package main

import (
	"log"
	"os"

	"isa-service/internal/httpapi"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := httpapi.NewRouter()
	log.Printf("ISA service listening on :%s (model range 0 m - 20000 m)", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
