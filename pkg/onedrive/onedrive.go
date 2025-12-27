package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	u "github.com/OneDriveSync/pkg/utility"
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
	File             *FileInfo `json:"file,omitempty"`
	LastModifiedTime string    `json:"lastModifiedDateTime"`
}

type FileInfo struct {
	Hashes FileHashes `json:"hashes"`
}

type FileHashes struct {
	QuickXorHash string `json:"quickXorHash,omitempty"`
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
		httpClient: &http.Client{Timeout: 30 * time.Minute},
	}, nil
}

func getAccessToken(clientID, tenantID string) (string, error) {
	// If useCache is true check if we have a cached token
	if cache, err := loadTokenCache(); err == nil {
		// Check if token is still valid (with 5 min buffer)
		if time.Now().Before(cache.ExpiresAt.Add(-5 * time.Minute)) {
			fmt.Println("Using cached token")
			return cache.AccessToken, nil
		}
	}

	// Need fresh authentication
	fmt.Println("Authenticating with OneDrive...")
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
		Scopes: []string{
			"Files.ReadWrite.All",
			"User.Read",
		},
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
	var cache TokenCache

	data, err := os.ReadFile(tokenCacheFile)
	if err != nil {
		return TokenCache{}, err
	}

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
	// Check if file exists
	exist := u.FileExists(localPath)
	if exist {
		fmt.Printf("File already exists, skipping: %s\n", localPath)
		return nil
	}

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

	// Create local file
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	fmt.Printf("Downloaded: %s → %s\n", remotePath, localPath)
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

	fmt.Printf("Deleted: %s\n", filePath)
	return nil
}
