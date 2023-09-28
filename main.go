package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
)

// AuthURL:  "https://login.microsoftonline.com/f8cdef31-a31e-4b4a-93e4-5f571e91255a/oauth2/authorize",
// TokenURL: "https://login.microsoftonline.com/f8cdef31-a31e-4b4a-93e4-5f571e91255a/oauth2/token",

func main() {
	// Khai báo thông tin xác thực OAuth2
	config := oauth2.Config{
		ClientID:     "e240abd3-62fa-4378-ae24-9e92f078fc63",
		ClientSecret: "ZSs8Q~nT6zvm19CIE72shXSp-sHelDFYSsz43cBJ",
		RedirectURL:  "http://localhost:9092/oauth2callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		},
		Scopes: []string{"files.readwrite.all"},
	}

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
		uploadFile(accessToken)
		fmt.Fprintln(w, "File uploaded successfully.")
		// fileId := getInfoFileId(accessToken)
		// getInfoFile(accessToken)
	})

	// Khởi động HTTP server
	go http.ListenAndServe(":9092", nil)

	// Tạo URL xác thực và chuyển hướng người dùng đến trang đăng nhập
	authURL := config.AuthCodeURL("", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)
	select {}
}

// /me/drive/items/{item-id}/content
// me/drive/root:/testhodo/test2.txt:/content
func uploadFile(accessToken string) {
	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	uploadURL := "https://graph.microsoft.com/v1.0/me/drive/root:/testhodo/hung.pptx:/content"
	filePath := "testhodo/hung.pptx"
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Kích thước phần nhỏ (ví dụ: 1 MB)
	chunkSize := 1024 * 1024

	// Tạo yêu cầu HTTP PUT để tải lên tệp
	req, err := http.NewRequest("PUT", uploadURL, nil)
	if err != nil {
		fmt.Println("Failed to create HTTP request:", err)
		return
	}
	req.Header.Add("Authorization", "Bearer "+accessToken)

	// Sử dụng HTTP client để gửi yêu cầu
	client := &http.Client{}

	// Đọc và gửi từng phần của tệp
	buffer := make([]byte, chunkSize)
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Failed to read file:", err)
			return
		}

		// Tạo io.Reader từ mảng byte và giới hạn kích thước
		reader := io.LimitReader(bytes.NewReader(buffer[:n]), int64(n))

		// Gửi phần nhỏ lên OneDrive
		req.Body = ioutil.NopCloser(reader)
		req.ContentLength = int64(n)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Failed to upload file:", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to upload file. Status code: %d\n", resp.StatusCode)
			return
		}
	}

	fmt.Println("File uploaded successfully.")
}

func getInfoFile(accessToken string) {
	// Tạo yêu cầu HTTP để tải tệp lên OneDrive
	url := "https://graph.microsoft.com/v1.0/me/drive/root:/testhodo/test2.txt"

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
	} else {
		fmt.Printf("Failed to get file. Status code: %d\n", resp.StatusCode)
	}
	return
}
