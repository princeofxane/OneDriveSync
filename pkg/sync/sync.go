package sync

import (
	"fmt"
	"log"
	"strings"

	"github.com/OneDriveSync/pkg/config"
	od "github.com/OneDriveSync/pkg/onedrive"
	wp "github.com/OneDriveSync/pkg/workerpool"
)

const Workers = 4

func Synchronize(c *config.Config) {
	client, err := od.NewOneDriveClient(c.ODrive.ClientID, c.ODrive.TenantID)
	if err != nil {
		log.Fatalf("failed to create OneDrive client: %v", err)
	}

	switch c.App.SyncMode {
	case config.Downld:
		Download(client, c.DirMap, c.Storage.Root, false)
	case config.DownldDel:
		Download(client, c.DirMap, c.Storage.Root, true)
	}

}

// Download downloads files from OneDrive to local storage based on the provided mappings.
// If delSrc is true, it deletes the source files from OneDrive after downloading.
func Download(client *od.OneDriveClient, mappings []string, destPath string, delSrc bool) {
	for _, mapping := range mappings {

		// Each mapping is in the format "source:destination"
		parts := strings.Split(mapping, ":")

		// Validate mapping format
		if len(parts) != 2 {
			log.Fatal("invalid mapping format, expected 'source:destination'")
		}

		// Extract source and destination paths
		srcDir := parts[0]
		destDir := destPath + "/" + parts[1]

		// Fetch the list of files from the source folder
		files, err := client.ListFiles(srcDir)
		if err != nil {
			log.Printf("error during fetching the file list: %v", err)
		}

		// Set up a worker pool for concurrent downloads with 4 workers
		pool := wp.NewPool(Workers, len(files))

		for _, file := range files {
			src := fmt.Sprintf("%s/%s", srcDir, file.Name)
			dst := fmt.Sprintf("%s/%s", destDir, file.Name)

			job := wp.Job{
				Task: func() error {
					err := client.DownloadFile(src, dst)
					if err != nil {
						return err
					}
					if delSrc {
						err = client.DeleteFile(src)
						if err != nil {
							return err
						}
					}
					return nil
				},
			}

			pool.AddJob(job)
		}

		// Close the pool and execute the jobs
		pool.Close()
		pool.Execute()
	}
}
