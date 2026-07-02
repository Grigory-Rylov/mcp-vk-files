package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// JSONRPCMessage is a JSON-RPC 2.0 message
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "mcp" {
		RunMCP()
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Environment variables override config file
	if token := os.Getenv("VK_TOKEN"); token != "" {
		cfg.VkToken = token
	}

	if cfg.VkToken == "" {
		log.Fatal("VK_TOKEN is required")
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[mcp-vk-files] ")
	log.Printf("Starting server")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := newMCPServer(cfg)

	if err := s.run(ctx); err != nil && err != io.EOF {
		log.Fatal(err)
	}
}

// mcpServer handles MCP protocol messages
type mcpServer struct {
	vkToken        string
	peerID         int
	thinkingPeerID int
}

func newMCPServer(cfg *Config) *mcpServer {
	return &mcpServer{
		vkToken:        cfg.VkToken,
		peerID:         cfg.PeerID,
		thinkingPeerID: cfg.ThinkingPeerID,
	}
}

func (s *mcpServer) run(ctx context.Context) error {
	stdinReader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := readMessage(stdinReader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			log.Printf("Error reading message: %v", err)
			continue
		}

		log.Printf("Received: method=%s", msg.Method)

		if len(msg.ID) > 0 {
			switch msg.Method {
			case "initialize":
				s.respondInitialize(msg)
			case "ping":
				s.respondPing(msg)
			case "tools/list":
				s.respondToolsList(msg)
			case "tools/call":
				s.respondToolCall(msg)
			default:
				s.respondError(msg, -32601, "Method not found")
			}
		} else {
			// Notification (no response expected)
			log.Printf("Notification: %s", msg.Method)
		}
	}
}

// readMessage reads a JSON-RPC message from the reader
func readMessage(r *bufio.Reader) (*JSONRPCMessage, error) {
	var raw json.RawMessage
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}

	msg := &JSONRPCMessage{JSONRPC: "2.0"}
	
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}

	if idRaw, ok := obj["id"]; ok {
		msg.ID = idRaw
	}
	if methodRaw, ok := obj["method"]; ok {
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return nil, err
		}
		msg.Method = method
	}
	if paramsRaw, ok := obj["params"]; ok {
		msg.Params = paramsRaw
	}

	return msg, nil
}

// send sends a JSON-RPC response
func (s *mcpServer) send(msg *JSONRPCMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}
	if _, err := fmt.Fprintf(os.Stdout, "%s\n", data); err != nil {
		log.Printf("Error writing response: %v", err)
	}
	os.Stdout.Sync()
}

// respondError sends an error response
func (s *mcpServer) respondError(req *JSONRPCMessage, code int, message string) {
	resp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	s.send(resp)
}

// respondInitialize handles the initialize request
func (s *mcpServer) respondInitialize(req *JSONRPCMessage) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "mcp-vk-files",
			"version": "0.1.0",
		},
	}

	resultJSON, _ := json.Marshal(result)

	resp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
	s.send(resp)

	// Send initialized notification
	notif := &JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	s.send(notif)
}

// respondPing handles the ping request
func (s *mcpServer) respondPing(req *JSONRPCMessage) {
	resp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage("{}"),
	}
	s.send(resp)
}

// respondToolsList handles tools/list
func (s *mcpServer) respondToolsList(req *JSONRPCMessage) {
	tools := []map[string]interface{}{
		{
			"name":        "send_file",
			"description": "Send a file to a VK peer (user or chat) as a document attachment. Uploads the file and sends it via VK Messages API.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"peer_name": map[string]interface{}{
						"type":        "string",
						"description": "Peer identifier: key name from config.peers, or numeric peer_id directly",
					},
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file on disk",
					},
					"filename": map[string]interface{}{
						"type":        "string",
						"description": "Custom filename in the chat (default: original filename from path)",
					},
					"caption": map[string]interface{}{
						"type":        "string",
						"description": "Optional text message to include with the file",
					},
				},
				"required": []string{"peer_name", "file_path"},
			},
		},
		{
			"name":        "send_audio",
			"description": "Send an audio file to a VK peer (user or chat) as an audio message. The file will appear as a playable audio clip in VK Messenger.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"peer_name": map[string]interface{}{
						"type":        "string",
						"description": "Peer identifier: key name from config.peers, or numeric peer_id directly",
					},
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the audio file on disk (.mp3, .ogg, .m4a)",
					},
					"filename": map[string]interface{}{
						"type":        "string",
						"description": "Custom filename in the chat (default: original filename from path)",
					},
					"caption": map[string]interface{}{
						"type":        "string",
						"description": "Optional text message to include with the audio",
					},
				},
				"required": []string{"peer_name", "file_path"},
			},
		},
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"tools": tools,
	})

	resp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
	s.send(resp)
}

// respondToolCall handles tools/call
func (s *mcpServer) respondToolCall(req *JSONRPCMessage) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respondError(req, -32602, "Invalid params: "+err.Error())
		return
	}

	switch strings.ToLower(params.Name) {
	case "send_file":
		s.handleSendFile(req, params.Arguments)
	case "send_audio":
		s.handleSendAudio(req, params.Arguments)
	default:
		s.respondError(req, -32601, fmt.Sprintf("Unknown tool: %s", params.Name))
	}
}

func (s *mcpServer) handleSendFile(req *JSONRPCMessage, args map[string]interface{}) {
	peerName, ok := args["peer_name"].(string)
	if !ok || peerName == "" {
		s.toolResponse(req, "Error: peer_name is required", true)
		return
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		s.toolResponse(req, "Error: file_path is required", true)
		return
	}

	peerID := s.resolvePeerID(peerName)
	if peerID == 0 {
		s.toolResponse(req, fmt.Sprintf("Error: unknown peer '%s'. Available peers: default, thinking", peerName), true)
		return
	}

	filename, _ := args["filename"].(string)
	caption, _ := args["caption"].(string)

	vk := NewVKClient(s.vkToken)
	msgID, err := vk.SendFile(peerID, filePath, filename, caption)
	if err != nil {
		s.toolResponse(req, fmt.Sprintf("Error: %v", err), true)
		return
	}

	s.toolResponse(req, fmt.Sprintf(
		"File sent successfully!\n\nMessage ID: %d\nPeer ID: %d\nFile: %s",
		msgID, peerID, filePath,
	), false)
}

func (s *mcpServer) handleSendAudio(req *JSONRPCMessage, args map[string]interface{}) {
	peerName, ok := args["peer_name"].(string)
	if !ok || peerName == "" {
		s.toolResponse(req, "Error: peer_name is required", true)
		return
	}

	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		s.toolResponse(req, "Error: file_path is required", true)
		return
	}

	peerID := s.resolvePeerID(peerName)
	if peerID == 0 {
		s.toolResponse(req, fmt.Sprintf("Error: unknown peer '%s'. Available peers: default, thinking", peerName), true)
		return
	}

	filename, _ := args["filename"].(string)
	caption, _ := args["caption"].(string)

	vk := NewVKClient(s.vkToken)
	msgID, err := vk.SendAudioMessage(peerID, filePath, filename, caption)
	if err != nil {
		s.toolResponse(req, fmt.Sprintf("Error: %v", err), true)
		return
	}

	s.toolResponse(req, fmt.Sprintf(
		"Audio sent successfully!\n\nMessage ID: %d\nPeer ID: %d\nFile: %s",
		msgID, peerID, filePath,
	), false)
}

func (s *mcpServer) resolvePeerID(name string) int {
	switch name {
	case "default":
		return s.peerID
	case "thinking":
		return s.thinkingPeerID
	}
	var id int
	n, _ := fmt.Sscanf(name, "%d", &id)
	if n == 1 {
		return id
	}
	return 0
}

func (s *mcpServer) toolResponse(req *JSONRPCMessage, text string, isError bool) {
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
		"isError": isError,
	})

	resp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
	s.send(resp)
}


