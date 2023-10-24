package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	"golang.org/x/oauth2"
)

var (
	clientId     = "e240abd3-62fa-4378-ae24-9e92f078fc63"
	clientSecret = "ZSs8Q~nT6zvm19CIE72shXSp-sHelDFYSsz43cBJ"
	redirectURL  = "http://localhost:9092/oauth2callback"
	scopes       = []string{"files.readwrite.all", "offline_access"}
	// Khai báo thông tin xác thực OAuth2
	config = oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		},
		Scopes: scopes,
	}
	handler *multipart.FileHeader
)

type respInfoFile struct {
	Id string `json:"id"`
}

type ShareLinkRequest struct {
	Type  string `json:"type"`
	Scope string `json:"scope"`
}

type ShareLinkResponse struct {
	Link WebUrl `json:"link"`
}

type WebUrl struct {
	WebUrl string `json:"webUrl"`
}

func startOAuthFlow() {
	// Tạo URL xác thực và chuyển hướng người dùng đến trang đăng nhập
	authURL := config.AuthCodeURL("", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)
	// Mở trình duyệt tự động với URL xác thực
	if err := open.Run(authURL); err != nil {
		log.Fatalf("Failed to open browser: %v", err)
	}

	// Đợi cho phép người dùng đăng nhập
	fmt.Println("Waiting for authorization...")

	time.Sleep(1 * time.Second)

	fmt.Println("Authorization process complete.")
}

func uploadHttpFile(w http.ResponseWriter, r *http.Request) {
	// Parse the request to get the file
	err := r.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Get the file from the form data
	file, handlerFlie, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Unable to get file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()
	handler = handlerFlie

	// Create a new file on the server to store the uploaded file
	f, err := os.OpenFile(handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		http.Error(w, "Unable to create file on server", http.StatusInternalServerError)
		return
	}
	// Copy the file from the request to the server file
	_, err = io.Copy(f, file)
	if err != nil {
		http.Error(w, "Unable to copy file", http.StatusInternalServerError)
		return
	}
	go startOAuthFlow()
	fmt.Fprintf(w, "File uploaded successfully: %s", handler.Filename)
}

func main() {
	// Set up an HTTP server to handle file uploads
	http.HandleFunc("/upload", uploadHttpFile)

	// Tạo HTTP server để lắng nghe callback từ OneDrive
	http.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		ctx := context.Background()
		token, err := config.Exchange(ctx, code)
		if err != nil {
			log.Fatalf("Failed to exchange token: %v", err)
		}

		// Sử dụng AccessToken để tải tệp lên OneDrive
		accessToken := token.AccessToken
		refreshToken := token.RefreshToken
		newAccessToken, err := uploadFile(accessToken, refreshToken)
		if err != nil {
			log.Fatalf("Failed uploadFile: %v", err)
		}
		fmt.Fprintln(w, "File uploaded successfully.")
		if newAccessToken == "" {
			getInfoFile(accessToken)
		} else {
			getInfoFile(newAccessToken)
		}
	})

	// Khởi động HTTP server
	go http.ListenAndServe(":9092", nil)

	select {}
}

func uploadFile(accessToken, refreshToken string) (string, error) {
	filePath := handler.Filename
	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	uploadURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/content", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	baseReq, err := http.NewRequest("PUT", uploadURL, file)
	if err != nil {
		return "", err
	}
	baseReq.Header.Add("Authorization", "Bearer "+accessToken)

	// Sử dụng HTTP client để gửi yêu cầu cơ bản
	client := &http.Client{}
	resp, err := client.Do(baseReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		newAccessToken, err := uploadFileRefreshToken(filePath, refreshToken)
		if err != nil {
			return "", err
		}
		return newAccessToken, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		err = fmt.Errorf("failed to upload file. Status code: %v", resp.StatusCode)
		return "", err
	}

	return "", nil
}

func getInfoFile(accessToken string) {
	filePath := handler.Filename
	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	url := "https://graph.microsoft.com/v1.0/me/drive/root:/" + filePath

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to get HTTP request: %v", err)
	}
	req.Header.Add("Content-Type", "text/plain")
	req.Header.Add("Authorization", "Bearer "+accessToken)

	// Sử dụng HTTP client để gửi yêu cầu
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to get file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(body))
		// Phân tích phản hồi JSON để lấy itemId
		var webUrl map[string]interface{}
		if err := json.Unmarshal(body, &webUrl); err != nil {
			fmt.Printf("Failed to parse JSON response: %v", err)
		}

		// Trích xuất itemId
		webUrlValue, ok := webUrl["webUrl"].(string)
		if !ok {
			fmt.Printf("webUrl not found in response.")
		} else {
			fmt.Printf("webUrl : %v\n", webUrlValue)
		}

		var dataId respInfoFile
		json.Unmarshal(body, &dataId)
		createLink(dataId.Id, accessToken)

	} else {
		fmt.Printf("Failed to get file. Status code: %d\n", resp.StatusCode)
	}
}

func createLink(itemId, accessToken string) {
	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	urlLink := "https://graph.microsoft.com/v1.0/me/drive/items/" + itemId + "/createLink"
	shareLinkReq := ShareLinkRequest{
		Type:  "embed", // Loại liên kết (ví dụ: xem, chỉnh sửa, v.v.)
		Scope: "anonymous",
	}
	jsonData, err := json.Marshal(shareLinkReq)
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}
	req, err := http.NewRequest("POST", urlLink, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Failed to get HTTP request: %v", err)
	}
	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	// Sử dụng HTTP client để gửi yêu cầu
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to get file: %v", err)
	}
	bodys, _ := ioutil.ReadAll(resp.Body)

	// In liên kết chia sẻ
	fmt.Println("Share Link:", string(bodys))
}

func uploadFileRefreshToken(filePath, refreshToken string) (string, error) {
	// Tạo yêu cầu HTTP để làm mới AccessToken
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", clientId)
	data.Set("client_secret", clientSecret)
	data.Set("scope", "files.readwrite.all offline_access")
	req, err := http.NewRequest("POST", "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to refresh AccessToken. Status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tokenResponse map[string]interface{}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", err
	}

	accessToken, ok := tokenResponse["access_token"].(string)
	if !ok {
		return "", errors.New("access token not found in response")
	}

	_, ok = tokenResponse["refresh_token"].(string)
	if !ok {
		return "", errors.New("refresh token not found in response")
	}

	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	uploadURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/content", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	baseReqUpload, err := http.NewRequest("PUT", uploadURL, file)
	if err != nil {
		return "", err
	}
	baseReqUpload.Header.Add("Authorization", "Bearer "+accessToken)

	// Sử dụng HTTP client để gửi yêu cầu cơ bản
	clientUpload := &http.Client{}
	respUpload, err := clientUpload.Do(baseReqUpload)
	if err != nil {
		return "", err
	}
	defer respUpload.Body.Close()

	if respUpload.StatusCode != http.StatusOK && respUpload.StatusCode != http.StatusCreated {
		err = fmt.Errorf("failed to upload file. Status code: %v", respUpload.StatusCode)
		return "", err
	}

	return accessToken, nil
}
