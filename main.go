package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const tokenCacheFile = "onedrive_token.json"

type TokenCache struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type DriveItem struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Size             int64     `json:"size"`
	Folder           *struct{} `json:"folder,omitempty"`
	LastModifiedTime string    `json:"lastModifiedDateTime"`
}

type DriveItemsResponse struct {
	Value []DriveItem `json:"value"`
}

type OneDriveClient struct {
	token      string
	httpClient *http.Client
}

func NewOneDriveClient(clientID, tenantID string) (*OneDriveClient, error) {
	token, err := getAccessToken(clientID, tenantID)
	if err != nil {
		return nil, err
	}

	return &OneDriveClient{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func getAccessToken(clientID, tenantID string) (string, error) {
	// Check if we have a cached token
	if cache, err := loadTokenCache(); err == nil {
		// Check if token is still valid (with 5 min buffer)
		if time.Now().Before(cache.ExpiresAt.Add(-5 * time.Minute)) {
			fmt.Println("🔄 Using cached token")
			return cache.AccessToken, nil
		}
	}

	// Need fresh authentication
	fmt.Println("🔐 Authenticating with OneDrive...")
	fmt.Printf("You will be prompted to sign in via your browser.\n")

	ctx := context.Background()
	cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		ClientID: clientID,
		TenantID: tenantID,
		UserPrompt: func(ctx context.Context, message azidentity.DeviceCodeMessage) error {
			fmt.Println(message.Message)
			return nil
		},
	})
	if err != nil {
		return "", err
	}

	tokenResp, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://graph.microsoft.com/Files.ReadWrite.All", "https://graph.microsoft.com/User.Read"},
	})
	if err != nil {
		return "", err
	}

	// Save token to cache
	cache := TokenCache{
		AccessToken: tokenResp.Token,
		ExpiresAt:   tokenResp.ExpiresOn,
	}
	saveTokenCache(cache)

	return tokenResp.Token, nil
}

func loadTokenCache() (TokenCache, error) {
	data, err := os.ReadFile(tokenCacheFile)
	if err != nil {
		return TokenCache{}, err
	}

	var cache TokenCache
	err = json.Unmarshal(data, &cache)
	return cache, err
}

func saveTokenCache(cache TokenCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(tokenCacheFile, data, 0600)
}

// ListFiles lists files in a folder (use "" or "/" for root)
func (c *OneDriveClient) ListFiles(folderPath string) ([]DriveItem, error) {
	var url string
	if folderPath == "" || folderPath == "/" {
		url = "https://graph.microsoft.com/v1.0/me/drive/root/children"
	} else {
		url = fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/children", folderPath)
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var itemsResp DriveItemsResponse
	err = json.Unmarshal(body, &itemsResp)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return itemsResp.Value, nil
}

// DownloadFile downloads a file from OneDrive to local path
func (c *OneDriveClient) DownloadFile(remotePath, localPath string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/content", remotePath)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	fmt.Printf("✅ Downloaded: %s → %s\n", remotePath, localPath)
	return nil
}

// CopyFile copies a file within OneDrive
func (c *OneDriveClient) CopyFile(sourcePath, destinationFolder, newName string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/copy", sourcePath)

	requestBody := map[string]interface{}{
		"parentReference": map[string]string{
			"path": "/drive/root:/" + destinationFolder,
		},
	}

	if newName != "" {
		requestBody["name"] = newName
	}

	jsonBody, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("copy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("copy failed %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Copied: %s → %s/%s\n", sourcePath, destinationFolder, newName)
	return nil
}

// DeleteFile deletes a file from OneDrive
func (c *OneDriveClient) DeleteFile(filePath string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:", filePath)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Deleted: %s\n", filePath)
	return nil
}

// MonitorFolder watches a folder for new files
func (c *OneDriveClient) MonitorFolder(folderPath string, knownFiles map[string]bool) ([]DriveItem, error) {
	items, err := c.ListFiles(folderPath)
	if err != nil {
		return nil, err
	}

	var newFiles []DriveItem
	for _, item := range items {
		if !knownFiles[item.ID] {
			newFiles = append(newFiles, item)
			knownFiles[item.ID] = true
		}
	}

	return newFiles, nil
}

func main() {
	// Configuration
	clientID := "CLIENT_ID_HERE" // Replace with your Azure app client ID
	tenantID := "consumers"

	// Folder to monitor
	watchFolder := "Videos/Xbox Game DVR" // Change this to your folder, or "" for root
	downloadDir := "./downloads"

	// Create download directory if it doesn't exist
	os.MkdirAll(downloadDir, 0755)

	// Create OneDrive client
	client, err := NewOneDriveClient(clientID, tenantID)
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		return
	}

	fmt.Printf("\n✅ Connected to OneDrive!\n")

	// Example 1: List files in a folder
	fmt.Printf("📂 Files in '%s':\n", watchFolder)
	files, err := client.ListFiles(watchFolder)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	for i, file := range files {
		if file.Folder != nil {
			fmt.Printf("%d. 📁 %s\n", i+1, file.Name)
		} else {
			fmt.Printf("%d. 📄 %s (%d bytes)\n", i+1, file.Name, file.Size)
		}
	}
	fmt.Println()

	// Example 2: Download a specific file
	// Uncomment and modify as needed:
	/*
		err = client.DownloadFile("Documents/example.txt", "./downloads/example.txt")
		if err != nil {
			fmt.Printf("❌ Download error: %v\n", err)
		}
	*/

	// Example 3: Copy a file
	// Uncomment and modify as needed:
	/*
		err = client.CopyFile("Documents/example.txt", "Documents/Backup", "example_backup.txt")
		if err != nil {
			fmt.Printf("❌ Copy error: %v\n", err)
		}
	*/

	// Example 4: Delete a file
	// Uncomment and modify as needed:
	/*
		err = client.DeleteFile("Documents/old_file.txt")
		if err != nil {
			fmt.Printf("❌ Delete error: %v\n", err)
		}
	*/

	// Example 5: Monitor for new files (automation use case)
	fmt.Println("\n--- Monitoring Example ---")
	knownFiles := make(map[string]bool)

	// Initial scan - mark all current files as known
	initialFiles, err := client.ListFiles(watchFolder)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	for _, file := range initialFiles {
		knownFiles[file.ID] = true
	}
	fmt.Printf("📊 Tracking %d existing files\n", len(knownFiles))

	// Check for new files (in production, you'd run this in a loop with time.Sleep)
	newFiles, err := client.MonitorFolder(watchFolder, knownFiles)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	if len(newFiles) > 0 {
		fmt.Printf("\n🆕 Found %d new file(s):\n", len(newFiles))
		for _, file := range newFiles {
			fmt.Printf("  - %s\n", file.Name)

			// Auto-download new files
			remotePath := watchFolder + "/" + file.Name
			localPath := downloadDir + "/" + file.Name

			err = client.DownloadFile(remotePath, localPath)
			if err != nil {
				fmt.Printf("    ❌ Download failed: %v\n", err)
			}
		}
	} else {
		fmt.Println("\n✓ No new files found")
	}
}
