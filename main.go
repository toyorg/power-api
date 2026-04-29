package main

import (
	"log"

	powerapi "power-api/src"
)

func main() {
	if err := powerapi.Run(); err != nil {
		log.Fatalf("Failed to start power-api server: %T: %v", err, err)
	}
}
