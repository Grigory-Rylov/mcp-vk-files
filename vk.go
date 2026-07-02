package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	VKAPIVersion = "5.200"
	VKBaseURL    = "https://api.vk.com/method/"
)

// VKClient handles VK API interactions
type VKClient struct {
	token string
	http  *http.Client
}

// NewVKClient creates a new VK client
func NewVKClient(token string) *VKClient {
	return &VKClient{
		token: token,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// apiRequest makes a GET request to the VK API
func (c *VKClient) apiRequest(method string, params url.Values) ([]byte, error) {
	return c.apiRequestWithMethod("GET", method, params)
}

// apiRequestPost makes a POST request to the VK API
func (c *VKClient) apiRequestPost(method string, params url.Values) ([]byte, error) {
	return c.apiRequestWithMethod("POST", method, params)
}

// apiRequestWithMethod makes a request to the VK API with the specified HTTP method
func (c *VKClient) apiRequestWithMethod(httpMethod, apiMethod string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}

	params.Set("access_token", c.token)
	params.Set("v", VKAPIVersion)

	var resp *http.Response
	var err error

	if httpMethod == "POST" {
		urlStr := fmt.Sprintf("%s%s", VKBaseURL, apiMethod)
		resp, err = c.http.PostForm(urlStr, params)
	} else {
		urlStr := fmt.Sprintf("%s%s?%s", VKBaseURL, apiMethod, params.Encode())
		resp, err = c.http.Get(urlStr)
	}

	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			ErrorCode    int    `json:"error_code"`
			ErrorMsg     string `json:"error_msg"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("VK API error %d: %s", apiResp.Error.ErrorCode, apiResp.Error.ErrorMsg)
	}

	return apiResp.Response, nil
}

// uploadFile uploads a file to the given URL using multipart/form-data
func (c *VKClient) uploadFile(uploadURL, filePath, filename string) ([]byte, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if filename == "" {
		filename = filepath.Base(filePath)
	}

	// Build multipart form
	boundary := fmt.Sprintf("----WebKitFormBoundary%d", rand.Int63n(9000000000000000)+1000000000000000)
	contentType := fmt.Sprintf("multipart/form-data; boundary=%s", boundary)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename))
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	buf.Write(fileData)
	buf.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload failed: HTTP %d - %s", resp.StatusCode, body)
	}

	return body, nil
}

// parseSendResponse extracts message_id from the messages.send response
func parseSendResponse(data []byte) (int, error) {
	var msgID int
	if err := json.Unmarshal(data, &msgID); err == nil {
		return msgID, nil
	}

	var sendResp struct {
		MessageID int `json:"message_id"`
	}
	if err := json.Unmarshal(data, &sendResp); err == nil {
		return sendResp.MessageID, nil
	}

	var raw float64
	if err := json.Unmarshal(data, &raw); err == nil {
		return int(raw), nil
	}

	return 0, fmt.Errorf("failed to parse send response: %s", data)
}

// extractUploadParams converts upload result JSON fields into url.Values
func extractUploadParams(data []byte) (url.Values, error) {
	var uploadData map[string]interface{}
	if err := json.Unmarshal(data, &uploadData); err != nil {
		return nil, fmt.Errorf("failed to parse upload result: %w", err)
	}

	params := url.Values{}
	for k, v := range uploadData {
		switch val := v.(type) {
		case string:
			params.Set(k, val)
		case float64:
			params.Set(k, fmt.Sprintf("%.0f", val))
		case json.Number:
			params.Set(k, val.String())
		}
	}
	return params, nil
}

// SendFile uploads a file and sends it to a VK peer as a document
func (c *VKClient) SendFile(peerID int, filePath, filename, caption string) (int, error) {
	log.Printf("Starting file upload: file=%s, peerID=%d", filePath, peerID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, fmt.Errorf("file not found: %s", filePath)
	}

	log.Printf("Step 1: Getting upload server URL")
	params := url.Values{
		"type":    {"doc"},
		"peer_id": {fmt.Sprintf("%d", peerID)},
	}

	uploadURLJSON, err := c.apiRequest("docs.getMessagesUploadServer", params)
	if err != nil {
		return 0, fmt.Errorf("failed to get upload server: %w", err)
	}

	var serverResp struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(uploadURLJSON, &serverResp); err != nil {
		return 0, fmt.Errorf("failed to parse upload server response: %w", err)
	}

	log.Printf("Upload URL: %s", serverResp.UploadURL)

	log.Printf("Step 2: Uploading file to server")
	uploadResult, err := c.uploadFile(serverResp.UploadURL, filePath, filename)
	if err != nil {
		return 0, fmt.Errorf("failed to upload file: %w", err)
	}

	log.Printf("Upload successful")

	log.Printf("Step 3: Saving document")
	saveParams, err := extractUploadParams(uploadResult)
	if err != nil {
		return 0, err
	}

	saveJSON, err := c.apiRequestPost("docs.save", saveParams)
	if err != nil {
		return 0, fmt.Errorf("failed to save document: %w", err)
	}

	var saveResp struct {
		Doc struct {
			ID      int    `json:"id"`
			OwnerID int    `json:"owner_id"`
			Title   string `json:"title"`
		} `json:"doc"`
	}
	if err := json.Unmarshal(saveJSON, &saveResp); err != nil {
		return 0, fmt.Errorf("failed to parse save response: %w", err)
	}

	docID := saveResp.Doc.ID
	docOwnerID := saveResp.Doc.OwnerID
	attachment := fmt.Sprintf("doc%d_%d", docOwnerID, docID)

	log.Printf("Document saved: %s (ID=%d, OwnerID=%d)", attachment, docID, docOwnerID)

	log.Printf("Step 4: Sending message with attachment")
	sendParams := url.Values{
		"peer_id":    {fmt.Sprintf("%d", peerID)},
		"attachment": {attachment},
		"random_id":  {fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))},
	}

	if caption != "" {
		sendParams.Set("message", caption)
	}

	sendJSON, err := c.apiRequestPost("messages.send", sendParams)
	if err != nil {
		return 0, fmt.Errorf("failed to send message: %w", err)
	}

	msgID, err := parseSendResponse(sendJSON)
	if err != nil {
		return 0, err
	}

	log.Printf("File sent! Message ID: %d", msgID)
	return msgID, nil
}

// SendAudioMessage uploads an audio file and sends it to a VK peer as an audio message
func (c *VKClient) SendAudioMessage(peerID int, filePath, filename, caption string) (int, error) {
	log.Printf("Starting audio upload: file=%s, peerID=%d", filePath, peerID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, fmt.Errorf("file not found: %s", filePath)
	}

	if filename == "" {
		filename = filepath.Base(filePath)
	}

	log.Printf("Step 1: Getting audio_message upload server URL")
	params := url.Values{
		"type":    {"audio_message"},
		"peer_id": {fmt.Sprintf("%d", peerID)},
	}

	uploadURLJSON, err := c.apiRequest("docs.getMessagesUploadServer", params)
	if err != nil {
		return 0, fmt.Errorf("failed to get upload server: %w", err)
	}

	var serverResp struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(uploadURLJSON, &serverResp); err != nil {
		return 0, fmt.Errorf("failed to parse upload server response: %w", err)
	}

	log.Printf("Upload URL: %s", serverResp.UploadURL)

	log.Printf("Step 2: Uploading audio file to server")
	uploadResult, err := c.uploadFile(serverResp.UploadURL, filePath, filename)
	if err != nil {
		return 0, fmt.Errorf("failed to upload file: %w", err)
	}

	log.Printf("Upload successful")

	log.Printf("Step 3: Saving audio document")
	saveParams, err := extractUploadParams(uploadResult)
	if err != nil {
		return 0, err
	}

	saveJSON, err := c.apiRequestPost("docs.save", saveParams)
	if err != nil {
		return 0, fmt.Errorf("failed to save document: %w", err)
	}

	var saveResp struct {
		AudioMessage struct {
			ID        int    `json:"id"`
			OwnerID   int    `json:"owner_id"`
			AccessKey string `json:"access_key"`
		} `json:"audio_message"`
	}
	if err := json.Unmarshal(saveJSON, &saveResp); err != nil {
		return 0, fmt.Errorf("failed to parse save response: %w", err)
	}

	audioID := saveResp.AudioMessage.ID
	audioOwnerID := saveResp.AudioMessage.OwnerID
	audioAccessKey := saveResp.AudioMessage.AccessKey
	attachment := fmt.Sprintf("audio_message%d_%d %s", audioOwnerID, audioID, audioAccessKey)

	log.Printf("Audio saved: %s (ID=%d, OwnerID=%d)", attachment, audioID, audioOwnerID)

	log.Printf("Step 4: Sending message with audio attachment")
	sendParams := url.Values{
		"peer_id":    {fmt.Sprintf("%d", peerID)},
		"attachment": {attachment},
		"random_id":  {fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))},
	}

	if caption != "" {
		sendParams.Set("message", caption)
	}

	sendJSON, err := c.apiRequestPost("messages.send", sendParams)
	if err != nil {
		return 0, fmt.Errorf("failed to send message: %w", err)
	}

	msgID, err := parseSendResponse(sendJSON)
	if err != nil {
		return 0, err
	}

	log.Printf("Audio sent! Message ID: %d", msgID)
	return msgID, nil
}
