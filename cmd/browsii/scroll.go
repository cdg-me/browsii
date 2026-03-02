package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var (
	scrollDown   bool
	scrollUp     bool
	scrollTop    bool
	scrollBottom bool
	scrollPixels int
)

func init() {
	scrollCmd := &cobra.Command{
		Use:   "scroll",
		Short: "Scrolls the active page",
		Run: func(cmd *cobra.Command, args []string) {
			var direction string
			switch {
			case scrollDown:
				direction = "down"
			case scrollUp:
				direction = "up"
			case scrollTop:
				direction = "top"
			case scrollBottom:
				direction = "bottom"
			default:
				// If pixels provided as positional arg, default to down
				if len(args) > 0 {
					px, err := strconv.Atoi(args[0])
					if err == nil {
						scrollPixels = px
						direction = "down"
					}
				}
				if direction == "" {
					log.Fatal("Specify a direction: --down, --up, --top, or --bottom")
				}
			}

			payload := map[string]interface{}{
				"direction": direction,
				"pixels":    scrollPixels,
			}

			_, err := client.SendCommand(port, "scroll", payload)
			if err != nil {
				log.Fatalf("Scroll failed: %v", err)
			}

			fmt.Printf("Successfully scrolled %s", direction)
			if direction == "down" || direction == "up" {
				fmt.Printf(" %dpx", scrollPixels)
			}
			fmt.Println()
		},
	}

	scrollCmd.Flags().BoolVar(&scrollDown, "down", false, "Scroll down")
	scrollCmd.Flags().BoolVar(&scrollUp, "up", false, "Scroll up")
	scrollCmd.Flags().BoolVar(&scrollTop, "top", false, "Scroll to the top of the page")
	scrollCmd.Flags().BoolVar(&scrollBottom, "bottom", false, "Scroll to the bottom of the page")
	scrollCmd.Flags().IntVar(&scrollPixels, "pixels", 300, "Number of pixels to scroll")

	rootCmd.AddCommand(scrollCmd)
}
