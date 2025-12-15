package main

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
)

const tokenCacheFile = "onedrive_token.json"

type TokenCache struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type DriveItem struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Size   int64     `json:"size"`
	Folder *struct{} `json:"folder,omitempty"`
}

type DriveItemsResponse struct {
	Value []DriveItem `json:"value"`
}

func main() {
	clientID := "03f96d61-d0cb-4889-acd3-a62ed0e564b9"
	tenantID := "consumers"

	// Get access token (will use cache or prompt for login)
	token, err := getAccessToken(clientID, tenantID)
	if err != nil {
		fmt.Printf("❌ Error getting token: %v\n", err)
		return
	}

	fmt.Println("✅ Authenticated!\n")

	// List files
	err = listFiles(token)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	}
}

func getAccessToken(clientID, tenantID string) (string, error) {
	// Check if we have a cached token
	if cache, err := loadTokenCache(); err == nil {
		// Check if token is still valid (with 5 min buffer)
		if time.Now().Before(cache.ExpiresAt.Add(-5 * time.Minute)) {
			fmt.Println("🔄 Using cached token")
			return cache.AccessToken, nil
		}

		// Try to refresh the token
		if cache.RefreshToken != "" {
			fmt.Println("🔄 Refreshing token...")
			newToken, err := refreshAccessToken(clientID, tenantID, cache.RefreshToken)
			if err == nil {
				return newToken, nil
			}
			fmt.Println("⚠️  Token refresh failed, need to re-authenticate")
		}
	}

	// Need fresh authentication
	fmt.Println("🔐 Authenticating with OneDrive...")
	fmt.Println("You will be prompted to sign in via your browser.\n")

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
		AccessToken:  tokenResp.Token,
		RefreshToken: "", // Device code flow doesn't give us refresh token directly
		ExpiresAt:    tokenResp.ExpiresOn,
	}
	saveTokenCache(cache)

	return tokenResp.Token, nil
}

func refreshAccessToken(clientID, tenantID, refreshToken string) (string, error) {
	// This would require implementing OAuth2 refresh flow manually
	// For now, just return error to trigger re-auth
	return "", fmt.Errorf("refresh not implemented")
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

func listFiles(token string) error {
	client := &http.Client{}

	// Get drive info
	req, _ := http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me/drive", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting drive: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var driveInfo map[string]interface{}
	json.Unmarshal(body, &driveInfo)

	fmt.Printf("📁 Drive: %s\n", driveInfo["name"])
	fmt.Printf("   ID: %s\n\n", driveInfo["id"])

	// List files in root folder
	fmt.Println("📂 Files in your OneDrive root folder:\n")

	req, _ = http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me/drive/root/children", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("error listing files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s", string(body))
	}

	body, _ = io.ReadAll(resp.Body)

	var itemsResp DriveItemsResponse
	err = json.Unmarshal(body, &itemsResp)
	if err != nil {
		return fmt.Errorf("error parsing response: %w", err)
	}

	items := itemsResp.Value
	if len(items) == 0 {
		fmt.Println("   (No files found)")
		return nil
	}

	for i, item := range items {
		if item.Folder != nil {
			fmt.Printf("%d. 📁 %s (folder)\n", i+1, item.Name)
		} else {
			fmt.Printf("%d. 📄 %s (%d bytes)\n", i+1, item.Name, item.Size)
		}
	}

	fmt.Printf("\n✅ Total items: %d\n", len(items))
	return nil
}

func downloadFile(token, filePath, localPath string) error {
	client := &http.Client{}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/content", filePath)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func deleteFile(token, filePath string) error {
	client := &http.Client{}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:", filePath)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", string(body))
	}

	return nil
}
