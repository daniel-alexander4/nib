package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

const (
	maxPlaceholderFile  = 1 << 20  // bytes of --set-file
	maxPlaceholderKeys  = 10000    // distinct keys
	maxPlaceholderValue = 64 << 10 // bytes in one value
)

var placeholderKey = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// placeholderValues is the value table for a template's {{key}} placeholders: --set-file's JSON object of strings,
// then every --set KEY=VALUE over it. Refusals name the key and never a value (ADR-183).
func placeholderValues(setFile string, sets []string) (map[string]string, error) {
	vars := map[string]string{}
	if setFile != "" {
		raw, err := readInput(setFile)
		if err != nil {
			return nil, err
		}
		if len(raw) > maxPlaceholderFile {
			return nil, fmt.Errorf("--set-file is larger than %d bytes", maxPlaceholderFile)
		}
		var obj map[string]string
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("--set-file is not a JSON object of string values: %v", err)
		}
		for k, v := range obj {
			vars[k] = v
		}
	}
	for _, kv := range sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--set takes KEY=VALUE, and one of them has no =")
		}
		vars[k] = v
	}
	if len(vars) > maxPlaceholderKeys {
		return nil, fmt.Errorf("more than %d placeholder keys", maxPlaceholderKeys)
	}
	for k, v := range vars {
		if !placeholderKey.MatchString(k) {
			return nil, fmt.Errorf("placeholder key %q is not letters, digits, _ . or -", k)
		}
		if len(v) > maxPlaceholderValue {
			return nil, fmt.Errorf("the value for %q is longer than %d bytes", k, maxPlaceholderValue)
		}
	}
	return vars, nil
}
