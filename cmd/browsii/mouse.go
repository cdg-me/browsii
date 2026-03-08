package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var dragSteps int

func init() {
	mouseCmd := &cobra.Command{
		Use:   "mouse",
		Short: "Mouse operations (move, drag, right-click, double-click)",
	}

	moveCmd := &cobra.Command{
		Use:   "move <x> <y>",
		Short: "Moves the mouse to absolute pixel coordinates",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			x, err := strconv.ParseFloat(args[0], 64)
			if err != nil {
				log.Fatalf("Invalid X coordinate: %v", err)
			}
			y, err := strconv.ParseFloat(args[1], 64)
			if err != nil {
				log.Fatalf("Invalid Y coordinate: %v", err)
			}

			payload := map[string]float64{"x": x, "y": y}
			_, err = client.SendCommand(port, "mouse/move", payload)
			if err != nil {
				log.Fatalf("Mouse move failed: %v", err)
			}
			fmt.Printf("Mouse moved to (%v, %v)\n", x, y)
		},
	}

	dragCmd := &cobra.Command{
		Use:   "drag <x1> <y1> <x2> <y2>",
		Short: "Drags from (x1,y1) to (x2,y2) with interpolated steps for smooth drawing",
		Args:  cobra.ExactArgs(4),
		Run: func(cmd *cobra.Command, args []string) {
			x1, _ := strconv.ParseFloat(args[0], 64)
			y1, _ := strconv.ParseFloat(args[1], 64)
			x2, _ := strconv.ParseFloat(args[2], 64)
			y2, _ := strconv.ParseFloat(args[3], 64)

			payload := map[string]interface{}{
				"x1": x1, "y1": y1, "x2": x2, "y2": y2, "steps": dragSteps,
			}
			_, err := client.SendCommand(port, "mouse/drag", payload)
			if err != nil {
				log.Fatalf("Mouse drag failed: %v", err)
			}
			fmt.Printf("Dragged from (%v,%v) to (%v,%v) in %d steps\n", x1, y1, x2, y2, dragSteps)
		},
	}
	dragCmd.Flags().IntVar(&dragSteps, "steps", 10, "Number of interpolation steps for smooth drawing")

	rightClickCmd := &cobra.Command{
		Use:   "right-click <selector>",
		Short: "Right-clicks a DOM element (triggers contextmenu)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"selector": args[0]}
			_, err := client.SendCommand(port, "mouse/rightclick", payload)
			if err != nil {
				log.Fatalf("Right-click failed: %v", err)
			}
			fmt.Printf("Right-clicked %s\n", args[0])
		},
	}

	doubleClickCmd := &cobra.Command{
		Use:   "double-click <selector>",
		Short: "Double-clicks a DOM element",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]string{"selector": args[0]}
			_, err := client.SendCommand(port, "mouse/doubleclick", payload)
			if err != nil {
				log.Fatalf("Double-click failed: %v", err)
			}
			fmt.Printf("Double-clicked %s\n", args[0])
		},
	}

	mouseCmd.AddCommand(moveCmd)
	mouseCmd.AddCommand(dragCmd)
	mouseCmd.AddCommand(rightClickCmd)
	mouseCmd.AddCommand(doubleClickCmd)

	rootCmd.AddCommand(mouseCmd)
}
