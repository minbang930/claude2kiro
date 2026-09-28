package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sgeraldes/claude2kiro/internal/debug"
)

// DebugInfo holds debug information for a single binary frame
type DebugInfo struct {
	FrameIndex     int    `json:"frame_index"`
	TotalLen       uint32 `json:"total_len"`
	HeaderLen      uint32 `json:"header_len"`
	PayloadLen     int    `json:"payload_len"`
	RawPayloadHex  string `json:"raw_payload_hex"`
	RawPayloadStr  string `json:"raw_payload_str"`
	AfterTrimStr   string `json:"after_trim_str"`
	ParsedEvent    any    `json:"parsed_event,omitempty"`
	ParseError     string `json:"parse_error,omitempty"`
	HasToolInput   bool   `json:"has_tool_input"`
	ToolInputValue string `json:"tool_input_value,omitempty"`
}

// ParseDebugInfo holds all debug info for a parse operation
type ParseDebugInfo struct {
	Timestamp   string      `json:"timestamp"`
	TotalBytes  int         `json:"total_bytes"`
	FrameCount  int         `json:"frame_count"`
	EventCount  int         `json:"event_count"`
	Frames      []DebugInfo `json:"frames"`
	FinalEvents []SSEEvent  `json:"final_events"`
}

// Global debug flag - can be set externally
var DebugMode = false

// writeDebugFile writes debug info to the secure debug directory
func writeDebugFile(info *ParseDebugInfo) {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return
	}
	// Use secure debug directory (~/.claude2kiro/debug/) with random filename
	debug.WriteDebugFile("parser-debug", data)
}

type assistantResponseEvent struct {
	Content   string  `json:"content"`
	Input     *string `json:"input,omitempty"`
	Name      string  `json:"name"`
	ToolUseId string  `json:"toolUseId"`
	Stop      bool    `json:"stop"`
	ModelID   string  `json:"modelId,omitempty"`
}

type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type MeteringEvent struct {
	Unit       string  `json:"unit"`
	UnitPlural string  `json:"unitPlural,omitempty"`
	Usage      float64 `json:"usage"`
}

// ParseResponseModelIDs returns distinct model IDs reported by assistantResponseEvent
// frames in first-seen order. Kiro's "auto" routing can report the concrete
// backend model here, which lets experiments distinguish routing metadata from
// the model Claude Desktop requested.
// SummarizeResponseFrameFields returns the top-level JSON field names for each
// valid response frame without logging field values. This is experiment
// observability only: it lets us compare response shapes without exposing
// generated content or changing request behavior.
// SummarizeResponseFrameHeaders returns selected AWS event-stream header
// metadata for each frame. It logs header names and string values only; response
// payload values are not inspected here.
func SummarizeResponseFrameHeaders(resp []byte) []string {
	var summaries []string
	r := bytes.NewReader(resp)
	frameIndex := 0
	for {
		if r.Len() < 12 {
			break
		}
		var totalLen, headerLen uint32
		if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &headerLen); err != nil {
			break
		}
		if totalLen < headerLen+12 || int(totalLen) > r.Len()+8 {
			break
		}
		header := make([]byte, headerLen)
		if _, err := io.ReadFull(r, header); err != nil {
			break
		}
		payloadLen := int(totalLen) - int(headerLen) - 12
		if _, err := r.Seek(int64(payloadLen+4), io.SeekCurrent); err != nil {
			break
		}

		if parts := summarizeEventStreamHeaderBlock(header); len(parts) > 0 {
			summaries = append(summaries, "#"+strconv.Itoa(frameIndex)+"{"+strings.Join(parts, ",")+"}")
		}
		frameIndex++
	}
	return summaries
}

func summarizeEventStreamHeaderBlock(header []byte) []string {
	var parts []string
	r := bytes.NewReader(header)
	for r.Len() > 0 {
		nameLen, err := r.ReadByte()
		if err != nil || int(nameLen) > r.Len() {
			break
		}
		name := make([]byte, int(nameLen))
		if _, err := io.ReadFull(r, name); err != nil {
			break
		}
		typeCode, err := r.ReadByte()
		if err != nil {
			break
		}

		var value string
		switch typeCode {
		case 0:
			value = "true"
		case 1:
			value = "false"
		case 2:
			if r.Len() < 1 { return parts }
			r.Seek(1, io.SeekCurrent)
			value = "<byte>"
		case 3:
			if r.Len() < 2 { return parts }
			r.Seek(2, io.SeekCurrent)
			value = "<int16>"
		case 4:
			if r.Len() < 4 { return parts }
			r.Seek(4, io.SeekCurrent)
			value = "<int32>"
		case 5, 8:
			if r.Len() < 8 { return parts }
			r.Seek(8, io.SeekCurrent)
			if typeCode == 5 { value = "<int64>" } else { value = "<timestamp>" }
		case 6, 7:
			if r.Len() < 2 { return parts }
			var n uint16
			if err := binary.Read(r, binary.BigEndian, &n); err != nil || int(n) > r.Len() {
				return parts
			}
			b := make([]byte, int(n))
			if _, err := io.ReadFull(r, b); err != nil {
				return parts
			}
			if typeCode == 7 {
				value = string(b)
			} else {
				value = "<bytes:" + strconv.Itoa(int(n)) + ">"
			}
		case 9:
			if r.Len() < 16 { return parts }
			r.Seek(16, io.SeekCurrent)
			value = "<uuid>"
		default:
			return parts
		}
		parts = append(parts, string(name)+"="+value)
	}
	sort.Strings(parts)
	return parts
}

func SummarizeResponseFrameFields(resp []byte) []string {
	var summaries []string
	r := bytes.NewReader(resp)
	frameIndex := 0
	for {
		if r.Len() < 12 {
			break
		}
		var totalLen, headerLen uint32
		if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &headerLen); err != nil {
			break
		}
		if totalLen < headerLen+12 || int(totalLen) > r.Len()+8 {
			break
		}
		header := make([]byte, headerLen)
		if _, err := io.ReadFull(r, header); err != nil {
			break
		}
		payloadLen := int(totalLen) - int(headerLen) - 12
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			break
		}
		if _, err := r.Seek(4, io.SeekCurrent); err != nil {
			break
		}

		payloadStr := strings.TrimPrefix(string(payload), "vent")
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payloadStr), &raw); err == nil {
			keys := make([]string, 0, len(raw))
			for key := range raw {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			summaries = append(summaries, "#"+strconv.Itoa(frameIndex)+"{"+strings.Join(keys, ",")+"}")
		}
		frameIndex++
	}
	return summaries
}

func ParseResponseModelIDs(resp []byte) []string {
	var ids []string
	seen := map[string]bool{}
	r := bytes.NewReader(resp)
	for {
		if r.Len() < 12 {
			break
		}
		var totalLen, headerLen uint32
		if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &headerLen); err != nil {
			break
		}
		if totalLen < headerLen+12 || int(totalLen) > r.Len()+8 {
			break
		}
		header := make([]byte, headerLen)
		if _, err := io.ReadFull(r, header); err != nil {
			break
		}
		payloadLen := int(totalLen) - int(headerLen) - 12
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			break
		}
		if _, err := r.Seek(4, io.SeekCurrent); err != nil {
			break
		}
		payloadStr := strings.TrimPrefix(string(payload), "vent")
		var evt assistantResponseEvent
		if err := json.Unmarshal([]byte(payloadStr), &evt); err != nil {
			continue
		}
		id := strings.TrimSpace(evt.ModelID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func ParseMeteringEvents(resp []byte) []MeteringEvent {
	var events []MeteringEvent

	r := bytes.NewReader(resp)
	for {
		if r.Len() < 12 {
			break
		}

		var totalLen, headerLen uint32
		if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &headerLen); err != nil {
			break
		}
		if totalLen < headerLen+12 || int(totalLen) > r.Len()+8 {
			break
		}

		header := make([]byte, headerLen)
		if _, err := io.ReadFull(r, header); err != nil {
			break
		}

		payloadLen := int(totalLen) - int(headerLen) - 12
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			break
		}

		if _, err := r.Seek(4, io.SeekCurrent); err != nil {
			break
		}

		payloadStr := strings.TrimPrefix(string(payload), "vent")
		var raw struct {
			Unit       string   `json:"unit"`
			UnitPlural string   `json:"unitPlural"`
			Usage      *float64 `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payloadStr), &raw); err != nil {
			continue
		}
		if raw.Unit != "credit" || raw.Usage == nil {
			continue
		}
		events = append(events, MeteringEvent{
			Unit:       raw.Unit,
			UnitPlural: raw.UnitPlural,
			Usage:      *raw.Usage,
		})
	}

	return events
}

func ParseEvents(resp []byte) []SSEEvent {
	events := []SSEEvent{}

	// Debug info collection
	var debugInfo *ParseDebugInfo
	if DebugMode {
		debugInfo = &ParseDebugInfo{
			Timestamp:  time.Now().Format(time.RFC3339Nano),
			TotalBytes: len(resp),
			Frames:     []DebugInfo{},
		}
	}
	frameIndex := 0

	// Track tool indices by tool ID for multi-tool responses
	toolIndices := make(map[string]int)
	nextToolIndex := 0 // Will be adjusted to 1 if text content is present
	hasToolUse := false
	hasTextContent := false

	r := bytes.NewReader(resp)
	for {
		if r.Len() < 12 {
			break
		}

		var totalLen, headerLen uint32
		if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &headerLen); err != nil {
			break
		}

		if int(totalLen) > r.Len()+8 {
			// Frame length invalid - silently break
			break
		}

		// Skip header
		header := make([]byte, headerLen)
		if _, err := io.ReadFull(r, header); err != nil {
			break
		}

		payloadLen := int(totalLen) - int(headerLen) - 12
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			break
		}

		// Skip CRC32
		if _, err := r.Seek(4, io.SeekCurrent); err != nil {
			break
		}

		payloadStr := strings.TrimPrefix(string(payload), "vent")

		// Collect debug info for this frame
		var frameDebug *DebugInfo
		if DebugMode {
			frameDebug = &DebugInfo{
				FrameIndex:    frameIndex,
				TotalLen:      totalLen,
				HeaderLen:     headerLen,
				PayloadLen:    payloadLen,
				RawPayloadHex: hex.EncodeToString(payload),
				RawPayloadStr: string(payload),
				AfterTrimStr:  payloadStr,
			}
			frameIndex++
		}

		var evt assistantResponseEvent
		if err := json.Unmarshal([]byte(payloadStr), &evt); err == nil {
			// Debug: capture parsed event
			if frameDebug != nil {
				frameDebug.ParsedEvent = evt
				if evt.Input != nil {
					frameDebug.HasToolInput = true
					frameDebug.ToolInputValue = *evt.Input
				}
			}

			// Track if we have text content - this affects tool indexing
			if evt.Content != "" {
				hasTextContent = true
				// If we haven't assigned any tool indices yet, bump to 1
				// so tools don't conflict with text at index 0
				if len(toolIndices) == 0 && nextToolIndex == 0 {
					nextToolIndex = 1
				}
			}

			sseEvent := convertAssistantEventToSSEWithIndex(evt, toolIndices, &nextToolIndex)
			if sseEvent.Event != "" {
				events = append(events, sseEvent)
			}

			if evt.ToolUseId != "" && evt.Name != "" {
				hasToolUse = true
			}
		} else {
			// Debug: capture parse error
			if frameDebug != nil {
				frameDebug.ParseError = err.Error()
			}
		}

		// Add frame debug info
		if frameDebug != nil {
			debugInfo.Frames = append(debugInfo.Frames, *frameDebug)
		}
	}

	// Add a single message_delta at the end for ALL responses
	// - Tool responses get stop_reason: "tool_use"
	// - Text-only responses get stop_reason: "end_turn"
	// Note: hasTextContent is used above to adjust tool indices, ensuring tools start at index 1
	// when there's text content at index 0
	if len(events) > 0 {
		_ = hasTextContent // Used for index adjustment above
		stopReason := "end_turn"
		if hasToolUse {
			stopReason = "tool_use"
		}
		events = append(events, SSEEvent{
			Event: "message_delta",
			Data: map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   stopReason,
					"stop_sequence": nil,
				},
				"usage": map[string]any{"output_tokens": 0},
			},
		})
	}

	// Write debug info if enabled
	if debugInfo != nil {
		debugInfo.FrameCount = frameIndex
		debugInfo.EventCount = len(events)
		debugInfo.FinalEvents = events
		writeDebugFile(debugInfo)
	}

	return events
}

func convertAssistantEventToSSEWithIndex(evt assistantResponseEvent, toolIndices map[string]int, nextToolIndex *int) SSEEvent {
	if evt.Content != "" {
		return SSEEvent{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": evt.Content,
				},
			},
		}
	} else if evt.ToolUseId != "" && evt.Name != "" && !evt.Stop {
		// Get or assign index for this tool
		toolIndex, exists := toolIndices[evt.ToolUseId]
		if !exists {
			toolIndex = *nextToolIndex
			toolIndices[evt.ToolUseId] = toolIndex
			*nextToolIndex++
		}

		if evt.Input == nil {
			// First event for this tool - content_block_start
			return SSEEvent{
				Event: "content_block_start",
				Data: map[string]any{
					"type":  "content_block_start",
					"index": toolIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    evt.ToolUseId,
						"name":  evt.Name,
						"input": map[string]any{},
					},
				},
			}
		} else {
			// Subsequent events - input_json_delta
			return SSEEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": toolIndex,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": *evt.Input,
					},
				},
			}
		}
	} else if evt.Stop && evt.ToolUseId != "" {
		// Tool stop event
		toolIndex, exists := toolIndices[evt.ToolUseId]
		if !exists {
			toolIndex = 0 // Fallback, shouldn't happen
		}
		return SSEEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": toolIndex,
			},
		}
	}

	return SSEEvent{}
}
