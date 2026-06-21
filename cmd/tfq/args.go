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

// partitionMulti is like partition but also collects repeated --field k=v into
// multi["field"] (a map) and repeated --add-tag values into the returned tags
// slice. Single-valued flags land in flags.
func partitionMulti(raw []string, bools map[string]bool) (pos []string, flags map[string]string, fields map[string]string, tags []string, err error) {
	flags = map[string]string{}
	fields = map[string]string{}
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name := a[2:]
		val := ""
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, val = name[:eq], name[eq+1:]
		} else if bools != nil && bools[name] {
			flags[name] = "true"
			continue
		} else {
			if i+1 >= len(raw) {
				return nil, nil, nil, nil, fmt.Errorf("flag --%s needs a value", name)
			}
			i++
			val = raw[i]
		}
		switch name {
		case "field":
			eq := strings.IndexByte(val, '=')
			if eq < 0 {
				return nil, nil, nil, nil, fmt.Errorf("--field needs k=v, got %q", val)
			}
			fields[val[:eq]] = val[eq+1:]
		case "add-tag":
			tags = append(tags, val)
		default:
			flags[name] = val
		}
	}
	return pos, flags, fields, tags, nil
}
