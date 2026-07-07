package main

import (
	"log"

	"myai-url-shortener/cmd/cobar"
)

func main() {
	if err := cobar.Execute(); err != nil {
		log.Fatal(err)
	}
}
