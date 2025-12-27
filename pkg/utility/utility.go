package utility

import (
	"errors"
	"os"

	"github.com/OneDriveSync/pkg/config"
)

func SyncMode(m string) config.SyncMode {
	switch m {
	case "download":
		return config.Downld
	case "steal":
		return config.DownldDel
	case "validate":
		return config.Validte
	default:
		return config.Downld
	}
}

func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	if err == nil {
		return true // File exists
	}
	if errors.Is(err, os.ErrNotExist) {
		return false // File does not exist
	}
	// File may or may not exist (permission error, etc.)
	return false
}
