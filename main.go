package main

import (
	"dbf-sync/cmd"
	"log"
	"os"
)

// Version info — injected at build time via ldflags
var (
	version   = "dev"
	buildDate = "unknown"
	commit    = "unknown"
)

func main() {
	// Pass version info to cmd package
	cmd.SetVersionInfo(version, buildDate, commit)

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
