package main

import (
	"log"

	"power-api/internal/powerapi"
)

func main() {
	if err := powerapi.Run(); err != nil {
		log.Fatalf("Failed to start power-api server: %T: %v", err, err)
	}
}
