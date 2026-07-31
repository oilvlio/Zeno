package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func readSecret(secretValue, secretFile string) (string, error) {
	if secretFile != "" {
		content, err := os.ReadFile(secretFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	return strings.TrimSpace(secretValue), nil
}

func readAgentToken(tokenValue, tokenFile string) (string, error) {
	return readSecret(tokenValue, tokenFile)
}

func readNotificationCredentialKeyFile(keyFile string) ([]byte, error) {
	keyFile = strings.TrimSpace(keyFile)
	if keyFile == "" {
		return nil, nil
	}
	info, err := os.Lstat(keyFile)
	if err != nil {
		return nil, fmt.Errorf("notification key file unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("notification key file must be a regular file")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("notification key file must be a regular file")
	}
	// Root-owned Docker secrets may grant read-only access to the fixed runtime
	// group (0640). Group write/execute and every "other" bit remain forbidden.
	if info.Mode().Perm()&0o037 != 0 {
		return nil, fmt.Errorf("notification key file permissions are too open")
	}
	file, err := os.Open(keyFile)
	if err != nil {
		return nil, fmt.Errorf("notification key file unavailable")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("notification key file changed while opening")
	}
	if openedInfo.Mode().Perm()&0o037 != 0 {
		return nil, fmt.Errorf("notification key file permissions are too open")
	}
	content, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil {
		return nil, fmt.Errorf("notification key file unavailable")
	}
	if len(content) > 1024 {
		return nil, fmt.Errorf("notification key file is too large")
	}
	key, err := parseNotificationCredentialKey(content)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func readNotificationAuthorityKeyFile(keyFile string) (string, error) {
	keyFile = strings.TrimSpace(keyFile)
	if keyFile == "" {
		return "", nil
	}
	content, err := readRestrictedNotificationKeyringFile(keyFile)
	if err != nil {
		return "", fmt.Errorf("notification authority key file unavailable")
	}
	if len(content) > 1024 {
		return "", fmt.Errorf("notification authority key file is too large")
	}
	key := strings.TrimSpace(string(content))
	if key == "" {
		return "", fmt.Errorf("notification authority key file is empty")
	}
	return key, nil
}

func parseNotificationCredentialKey(content []byte) ([]byte, error) {
	if len(content) == notificationCredentialKeySize {
		key := make([]byte, notificationCredentialKeySize)
		copy(key, content)
		return key, nil
	}
	raw := bytes.TrimRight(content, "\r\n")
	if len(raw) == notificationCredentialKeySize {
		key := make([]byte, notificationCredentialKeySize)
		copy(key, raw)
		return key, nil
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return nil, fmt.Errorf("notification key file is empty")
	}
	if decoded, err := hex.DecodeString(text); err == nil && len(decoded) == notificationCredentialKeySize {
		return decoded, nil
	}
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		decoded, err := encoding.DecodeString(text)
		if err == nil && len(decoded) == notificationCredentialKeySize {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("notification key file must contain a 32-byte key")
}

func readNotificationCredentialKeyringFile(path string) (string, map[string][]byte, error) {
	content, err := readRestrictedNotificationKeyringFile(path)
	if err != nil || len(content) == 0 {
		return "", nil, err
	}
	activeKeyID, encodedKeys, err := decodeNotificationKeyringDocument(content)
	if err != nil {
		return "", nil, fmt.Errorf("invalid notification credential key ring")
	}
	keys := make(map[string][]byte, len(encodedKeys))
	for keyID, encodedKey := range encodedKeys {
		key, err := parseNotificationCredentialKey([]byte(encodedKey))
		if err != nil {
			return "", nil, fmt.Errorf("invalid notification credential key ring")
		}
		keys[keyID] = key
	}
	if _, ok := keys[activeKeyID]; !ok {
		return "", nil, fmt.Errorf("invalid notification credential key ring")
	}
	return activeKeyID, keys, nil
}

func readNotificationAuthorityKeyringFile(path string) (string, map[string]string, error) {
	content, err := readRestrictedNotificationKeyringFile(path)
	if err != nil || len(content) == 0 {
		return "", nil, err
	}
	activeKeyID, encodedKeys, err := decodeNotificationKeyringDocument(content)
	if err != nil {
		return "", nil, fmt.Errorf("invalid notification authority key ring")
	}
	keys := make(map[string]string, len(encodedKeys))
	for keyID, key := range encodedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", nil, fmt.Errorf("invalid notification authority key ring")
		}
		keys[keyID] = key
	}
	if _, ok := keys[activeKeyID]; !ok {
		return "", nil, fmt.Errorf("invalid notification authority key ring")
	}
	return activeKeyID, keys, nil
}

// decodeNotificationKeyringDocument deliberately avoids unmarshalling JSON
// objects straight into maps: encoding/json otherwise accepts duplicate keys
// with last-value-wins semantics. Key material and active-key selection must be
// unambiguous, so duplicate fields, duplicate key ids, surrounding key-id
// whitespace, unknown fields and trailing JSON are all rejected.
func decodeNotificationKeyringDocument(content []byte) (string, map[string]string, error) {
	document, err := decodeStrictJSONObject(content)
	if err != nil || len(document) != 2 {
		return "", nil, fmt.Errorf("invalid key ring document")
	}
	activeRaw, activeOK := document["active_key_id"]
	keysRaw, keysOK := document["keys"]
	if !activeOK || !keysOK {
		return "", nil, fmt.Errorf("invalid key ring document")
	}
	var activeKeyID string
	if err := json.Unmarshal(activeRaw, &activeKeyID); err != nil {
		return "", nil, err
	}
	normalizedActiveKeyID := strings.TrimSpace(activeKeyID)
	if normalizedActiveKeyID != activeKeyID || !validNotificationKeyID(normalizedActiveKeyID) {
		return "", nil, fmt.Errorf("invalid active key id")
	}
	keys, err := decodeStrictJSONStringMap(keysRaw)
	if err != nil || len(keys) == 0 {
		return "", nil, fmt.Errorf("invalid key map")
	}
	for keyID := range keys {
		normalizedKeyID := strings.TrimSpace(keyID)
		if normalizedKeyID != keyID || !validNotificationKeyID(normalizedKeyID) {
			return "", nil, fmt.Errorf("invalid key id")
		}
	}
	if _, ok := keys[normalizedActiveKeyID]; !ok {
		return "", nil, fmt.Errorf("active key id is not in key map")
	}
	return normalizedActiveKeyID, keys, nil
}

func validNotificationKeyID(keyID string) bool {
	if len(keyID) == 0 || len(keyID) > 64 {
		return false
	}
	for _, character := range keyID {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func decodeStrictJSONObject(content []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("JSON object required")
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON object key")
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate JSON object key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, fmt.Errorf("invalid JSON object")
	}
	if err := requireJSONDecoderEOF(decoder); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeStrictJSONStringMap(content []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("JSON object required")
	}
	values := make(map[string]string)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON object key")
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate JSON object key")
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, fmt.Errorf("invalid JSON object")
	}
	if err := requireJSONDecoderEOF(decoder); err != nil {
		return nil, err
	}
	return values, nil
}

func requireJSONDecoderEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func readRestrictedNotificationKeyringFile(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("notification key ring file unavailable")
	}
	if info.Mode().Perm()&0o037 != 0 {
		return nil, fmt.Errorf("notification key ring file permissions are too open")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("notification key ring file unavailable")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Mode().Perm()&0o037 != 0 {
		return nil, fmt.Errorf("notification key ring file changed while opening")
	}
	const maximumKeyringBytes = 16 << 10
	content, err := io.ReadAll(io.LimitReader(file, maximumKeyringBytes+1))
	if err != nil {
		return nil, fmt.Errorf("notification key ring file unavailable")
	}
	if len(content) > maximumKeyringBytes {
		return nil, fmt.Errorf("notification key ring file is too large")
	}
	return content, nil
}

const notificationCredentialKeySize = 32
