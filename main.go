package main

import (
	"flag"
	"log"

	"github.com/OneDriveSync/pkg/config"
	"github.com/OneDriveSync/pkg/sync"
	u "github.com/OneDriveSync/pkg/utility"
)

func main() {
	mode := flag.String("mode", "download", "Sync mode: download, steal, or validate")
	flag.Parse()

	c, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	c.App.SyncMode = u.SyncMode(*mode)

	sync.Synchronize(c)
}
