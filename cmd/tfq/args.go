package main

import (
	"fmt"
	"strings"
)

// partition splits raw CLI args into positionals and flags. bools lists flags
// that take no value. Supports --name=value, --name value, and --bool in any
// position relative to positionals.
func partition(raw []string, bools map[string]bool) ([]string, map[string]string, error) {
	pos := []string{}
	flags := map[string]string{}
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name := a[2:]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			flags[name[:eq]] = name[eq+1:]
			continue
		}
		if bools[name] {
			flags[name] = "true"
			continue
		}
		if i+1 >= len(raw) {
			return nil, nil, fmt.Errorf("flag --%s needs a value", name)
		}
		flags[name] = raw[i+1]
		i++
	}
	return pos, flags, nil
}
