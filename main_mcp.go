package main

import (
	"context"
	"fmt"
	stdlib_log "log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var mcpContext = context.Background()

type SendFileArgs struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the file on disk"`
}

func sendVKFile(peerName, filePath, filename, caption string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("config error: %v", err)
	}

	token := cfg.VkToken
	if token == "" {
		token = os.Getenv("VK_TOKEN")
	}
	if token == "" {
		return "", fmt.Errorf("VK_TOKEN not set")
	}

	peerID := resolvePeerID(cfg, peerName)
	if peerID == 0 {
		return "", fmt.Errorf("unknown peer: %s", peerName)
	}

	vk := NewVKClient(token)

	msgID, err := vk.SendFile(peerID, filePath, filename, caption)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("File sent successfully!\n\nMessage ID: %d\nPeer ID: %d", msgID, peerID), nil
}

func sendVKAudio(filePath, filename, caption string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("config error: %v", err)
	}

	token := cfg.VkToken
	if token == "" {
		token = os.Getenv("VK_TOKEN")
	}
	if token == "" {
		return "", fmt.Errorf("VK_TOKEN not set")
	}

	peerID := cfg.PeerID
	if peerID == 0 {
		return "", fmt.Errorf("peer_id not configured")
	}

	vk := NewVKClient(token)

	msgID, err := vk.SendAudioMessage(peerID, filePath, filename, caption)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Audio sent successfully!\n\nMessage ID: %d\nPeer ID: %d", msgID, peerID), nil
}

func resolvePeerID(cfg *Config, name string) int {
	if name == "" {
		return cfg.PeerID
	}
	switch name {
	case "default":
		return cfg.PeerID
	case "thinking":
		return cfg.ThinkingPeerID
	}
	var id int
	n, _ := fmt.Sscanf(name, "%d", &id)
	if n == 1 {
		return id
	}
	return 0
}

func RunMCP() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "vk-files",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_file",
		Description: "Send a file to a VK peer (user or chat) as a document attachment",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SendFileArgs) (*mcp.CallToolResult, any, error) {
		result, err := sendVKFile("", args.FilePath, "", "")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_audio",
		Description: "Send an audio file to a VK peer (user or chat) as an audio message",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SendFileArgs) (*mcp.CallToolResult, any, error) {
		result, err := sendVKAudio(args.FilePath, "", "")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})

	if err := server.Run(mcpContext, &mcp.StdioTransport{}); err != nil {
		stdlib_log.Fatalf("Server failed: %v", err)
	}
}
