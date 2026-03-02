//go:build ignore

// Package main demonstrates basic browsii Go client usage.
// Run with: go run examples/go/01_basics.go
package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/client"
)

func main() {
	c, err := client.Start(client.Options{}) // headful by default
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()

	fmt.Printf("Daemon running on port %d\n", c.Port())

	if err := c.Navigate("https://example.com"); err != nil {
		log.Fatal(err)
	}

	text, err := c.Scrape(client.Markdown)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)

	tabs, err := c.TabList()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d tab(s) open\n", len(tabs))
}
