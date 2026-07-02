# mcp-vk-files

MCP server for sending files and audio messages to VK Messenger.

## Configuration

Copy `config.json.example` to `config.json` and fill in your credentials:

```json
{
  "vk_token": "your_vk_bot_token_here",
  "peer_id": 2000000001
}
```

Or set via environment variable:

```bash
export VK_TOKEN="your_vk_bot_token_here"
```

### Config Fields

| Field             | Description                              |
|-------------------|------------------------------------------|
| `vk_token`        | VK Bot Long-Lived Access Token           |
| `peer_id`         | Default recipient chat/user ID           |
| `thinking_peer_id`| Alternate recipient (for "thinking" peer)|

## Running

```bash
go build -o mcp-vk-files .
./mcp-vk-files mcp
```

Or with the legacy transport:

```bash
./mcp-vk-files
```

## Tools

### send_file

Sends a file as a document attachment to a VK peer.

**Parameters:**

| Parameter   | Type   | Required | Description                          |
|-------------|--------|----------|--------------------------------------|
| `peer_name` | string | yes      | Peer key (`default`, `thinking`) or numeric ID |
| `file_path` | string | yes      | Absolute path to the file on disk    |
| `filename`  | string | no       | Custom filename in the chat          |
| `caption`   | string | no       | Optional text message                |

### send_audio

Sends an audio file as an audio message (playable audio clip) to a VK peer.

**Parameters:**

| Parameter   | Type   | Required | Description                          |
|-------------|--------|----------|--------------------------------------|
| `peer_name` | string | yes      | Peer key (`default`, `thinking`) or numeric ID |
| `file_path` | string | yes      | Absolute path to the audio file (.mp3, .ogg, .m4a) |
| `filename`  | string | no       | Custom filename in the chat          |
| `caption`   | string | no       | Optional text message                |

The audio message is uploaded via VK's `audio_message` document type, so it appears as a playable audio clip in VK Messenger rather than a generic file attachment.

## Building

```bash
go build -o mcp-vk-files .
```

## License

MIT
