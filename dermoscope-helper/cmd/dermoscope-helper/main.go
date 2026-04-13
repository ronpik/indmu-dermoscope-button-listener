// Package main is the entry point for the Dermoscope Button Helper application.
// This application monitors supported dermoscope devices for button press events
// and simulates F9 keypresses to trigger image capture in the TrichoAI web application.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/trichoai/dermoscope-helper/internal/app"
	"github.com/trichoai/dermoscope-helper/internal/logger"
	"github.com/trichoai/dermoscope-helper/internal/usb"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	listProfiles := flag.Bool("list-profiles", false, "List supported device profiles and exit")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Handle --version
	if *version {
		fmt.Printf("dermoscope-helper version %s\n", Version)
		return
	}

	// Initialize logger
	log := logger.New(*debug)

	// Handle --list-profiles
	if *listProfiles {
		registry := usb.NewProfileRegistry()
		fmt.Println("Supported Device Profiles:")
		fmt.Println("==========================")
		for _, p := range registry.GetAll() {
			fmt.Printf("\n  ID:           %s\n", p.ID)
			fmt.Printf("  Name:         %s\n", p.Name)
			fmt.Printf("  Manufacturer: %s\n", p.Manufacturer)
			fmt.Printf("  VID:PID:      %04X:%04X\n", p.VendorID, p.ProductID)
			if p.Notes != "" {
				fmt.Printf("  Notes:        %s\n", p.Notes)
			}
		}
		fmt.Printf("\nTotal: %d profile(s)\n", registry.Count())
		return
	}

	// Load configuration
	config := app.DefaultConfig()
	if *configPath != "" {
		var err error
		config, err = app.LoadConfig(*configPath)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load config")
		}
	}

	// Create application
	application, err := app.New(config, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create application")
	}

	// Log profile info in debug mode
	if *debug {
		registry := application.GetRegistry()
		log.Debug().Int("count", registry.Count()).Msg("Loaded device profiles")
		for _, p := range registry.GetAll() {
			log.Debug().
				Str("id", p.ID).
				Str("name", p.Name).
				Str("vidpid", fmt.Sprintf("%04X:%04X", p.VendorID, p.ProductID)).
				Msg("Profile registered")
		}
	}

	// Handle OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info().Msg("Received shutdown signal")
		application.Stop()
	}()

	// Run application (blocks until stopped)
	log.Info().Str("version", Version).Msg("Starting Dermoscope Button Helper")
	if err := application.Run(); err != nil {
		log.Fatal().Err(err).Msg("Application error")
	}

	log.Info().Msg("Application stopped")
}
