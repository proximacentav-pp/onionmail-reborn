package front

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	defaultRegistryKeyPath = "/etc/onionmail/registry.key"
	registryEncPrefix      = "v1:"
)

type registryKeyState struct {
	once sync.Once
	key  []byte
	err  error
}

var registryKeys sync.Map

func (s *Server) registryKeyPath() string {
	if p := strings.TrimSpace(s.RegistryKey); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("ONIONMAIL_REGISTRY_KEY_FILE")); p != "" {
		return p
	}
	return defaultRegistryKeyPath
}

func (s *Server) secretKey() ([]byte, error) {
	path := s.registryKeyPath()
	v, _ := registryKeys.LoadOrStore(path, &registryKeyState{})
	st := v.(*registryKeyState)
	st.once.Do(func() {
		b, err := os.ReadFile(path)
		if err != nil {
			st.err = err
			return
		}
		switch {
		case len(b) == 32:
			st.key = append([]byte(nil), b...)
		case len(b) > 0:
			sum := sha256.Sum256(b)
			st.key = sum[:]
		default:
			st.err = fmt.Errorf("registry key is empty")
		}
	})
	return st.key, st.err
}

func (s *Server) encryptSecret(plain string) (string, error) {
	key, err := s.secretKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return registryEncPrefix + base64.StdEncoding.EncodeToString(out), nil
}

func (s *Server) decryptSecret(enc string) (string, error) {
	if !strings.HasPrefix(enc, registryEncPrefix) {
		return "", fmt.Errorf("unsupported registry secret format")
	}
	key, err := s.secretKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, registryEncPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("short registry secret")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Server) entryCredential(entry map[string]any, field string) string {
	if entry == nil {
		return ""
	}
	if enc, ok := entry[field+"_enc"].(string); ok && enc != "" {
		if v, err := s.decryptSecret(enc); err == nil {
			return v
		}
		return ""
	}
	if v, ok := entry[field].(string); ok {
		return v
	}
	return ""
}

func (s *Server) storeCredential(entry map[string]any, field, value string) bool {
	if value == "" {
		delete(entry, field)
		delete(entry, field+"_enc")
		return true
	}
	enc, err := s.encryptSecret(value)
	if err != nil {
		s.logf("cannot store %s credential: %v", field, err)
		return false
	}
	entry[field+"_enc"] = enc
	delete(entry, field)
	return true
}

func (s *Server) migrateRegistrySecrets() error {
	if _, err := s.secretKey(); err != nil {
		s.logf("registry migration skipped: %v", err)
		return nil
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	changed := false
	for _, entry := range data.Users {
		for _, field := range []string{"smtp", "pop3"} {
			v, ok := entry[field].(string)
			if !ok || v == "" {
				continue
			}
			if s.storeCredential(entry, field, v) {
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return s.saveRegistry(data)
}
