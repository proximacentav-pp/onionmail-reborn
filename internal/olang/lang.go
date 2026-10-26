package olang

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Bundle struct {
	Languages     []string                     `json:"languages"`
	LanguageNames map[string]string            `json:"language_names"`
	Om            map[string]map[string]string `json:"om"`
	Front         map[string]map[string]string `json:"front"`
	Vendor        map[string]map[string]string `json:"vendor"`
	VendorDynamic map[string][]string          `json:"vendor_dynamic"`
}

func Load(path string) (*Bundle, error) {
	if path == "" {
		path = findLangFile()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	if len(b.Languages) == 0 {
		b.Languages = []string{"en", "de", "fr", "ru", "es"}
	}
	return &b, nil
}

func findLangFile() string {
	candidates := []string{}
	if v := os.Getenv("ONIONMAIL_LANG_FILE"); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates,
		"/usr/local/share/onionmail/languages.json",
		"languages.json",
	)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "languages.json"),
			filepath.Join(dir, "..", "share", "onionmail", "languages.json"),
		)
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "languages.json"
}

func (b *Bundle) HasLang(lang string) bool {
	for _, l := range b.Languages {
		if l == lang {
			return true
		}
	}
	return false
}

func (b *Bundle) OmText(lang, key string) string {
	if m, ok := b.Om[key]; ok {
		if v, ok := m[lang]; ok && v != "" {
			return v
		}
	}
	return key
}

func (b *Bundle) FrontText(lang, key string) string {
	if m, ok := b.Front[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

func (b *Bundle) FrontFormat(lang, key string, args map[string]string) string {
	text := b.FrontText(lang, key)
	return Format(text, args)
}

func (b *Bundle) VendorText(lang, key string) string {
	if m, ok := b.Vendor[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

func Format(text string, args map[string]string) string {
	if len(args) == 0 {
		return text
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		text = strings.ReplaceAll(text, "{"+k+"}", args[k])
	}
	return text
}
