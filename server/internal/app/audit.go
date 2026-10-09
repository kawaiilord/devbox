package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

func finalizeAuditEvent(event AuditEvent) AuditEvent {
	event.Action = sanitizeAuditText(event.Action, 96)
	event.TargetType = sanitizeAuditText(event.TargetType, 64)
	event.TargetID = sanitizeAuditText(event.TargetID, 256)
	event.Metadata = sanitizeAuditMetadata(event.Metadata, 0)
	metadata, _ := json.Marshal(event.Metadata)
	payload := strings.Join([]string{
		event.PreviousHash,
		event.ActorID,
		event.Action,
		event.TargetType,
		event.TargetID,
		string(metadata),
		strconv.FormatInt(event.CreatedAt, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(payload))
	event.EntryHash = hex.EncodeToString(digest[:])
	return event
}

func sanitizeAuditMetadata(metadata map[string]any, depth int) map[string]any {
	clean := make(map[string]any)
	if depth > 3 {
		return clean
	}
	count := 0
	for key, value := range metadata {
		if count >= 32 {
			break
		}
		key = sanitizeAuditText(key, 64)
		if key == "" || sensitiveAuditKey(key) {
			continue
		}
		if sanitized, ok := sanitizeAuditValue(value, depth); ok {
			clean[key] = sanitized
			count++
		}
	}
	return clean
}

func sanitizeAuditValue(value any, depth int) (any, bool) {
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, true
	case string:
		return sanitizeAuditText(typed, 256), true
	case map[string]any:
		return sanitizeAuditMetadata(typed, depth+1), depth < 3
	case []any:
		if depth >= 3 {
			return nil, false
		}
		clean := make([]any, 0, len(typed))
		for _, item := range typed {
			if len(clean) >= 32 {
				break
			}
			if sanitized, ok := sanitizeAuditValue(item, depth+1); ok {
				clean = append(clean, sanitized)
			}
		}
		return clean, true
	default:
		return nil, false
	}
}

func sensitiveAuditKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, sensitive := range []string{
		"authorization", "cookie", "credential", "password", "passphrase",
		"private_key", "api_key", "secret", "token",
	} {
		if strings.Contains(key, sensitive) {
			return true
		}
	}
	return false
}

func verifyAuditChain(events []AuditEvent) bool {
	previous := strings.Repeat("0", 64)
	for _, event := range events {
		if event.PreviousHash != previous {
			return false
		}
		expected := finalizeAuditEvent(AuditEvent{
			ActorID: event.ActorID, Action: event.Action, TargetType: event.TargetType,
			TargetID: event.TargetID, Metadata: event.Metadata,
			PreviousHash: event.PreviousHash, CreatedAt: event.CreatedAt,
		})
		if event.EntryHash != expected.EntryHash {
			return false
		}
		previous = event.EntryHash
	}
	return true
}

func sanitizeAuditText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
