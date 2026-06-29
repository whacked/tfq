package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Mode is the primary operation tfq performs. Search is the default.
type Mode int

const (
	ModeSearch Mode = iota
	ModeList
	ModeShow
	ModeLinks
	ModeTags
	ModeTypes
	ModeNext
	ModeNew
	ModeSet
	ModeValidate
	ModeInspect
	ModeGraph
	ModeVersion
	ModeHelp
)

// Invocation is a fully parsed command line.
type Invocation struct {
	Mode     Mode
	Selector string
	Root     string
	JSON     bool

	Type      string
	Status    string
	Tags      []string
	In        []string
	DependsOn []string
	Limit     int

	IgnoreCase bool
	FilesOnly  bool
	Count      bool
	Heading    bool

	Raw         bool
	Frontmatter bool

	Inbound  bool
	Outbound bool

	Strict  bool
	Schema  string // explicit schema path for --validate (.cue or markdown ```cue)
	Verbose bool   // --help --verbose / --examples → extended agent help
	Color   string // auto | always | never

	Fields map[string]string
}

// usageError marks an error that should exit 2 (vs 1 for runtime errors).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }
func usageErr(msg string) error    { return usageError{msg} }

// shortName maps a single-char short flag to its long name ("" if unknown).
func shortName(s string) string {
	switch s {
	case "i":
		return "ignore-case"
	case "l":
		return "files-with-matches"
	case "c":
		return "count"
	case "e":
		return "query"
	default:
		return ""
	}
}

// parse turns raw args into an Invocation. Non-flag tokens (and -e/--query
// values) join into the selector; -- stops flag parsing; exactly one primary
// mode flag is allowed.
func parse(raw []string) (Invocation, error) {
	inv := Invocation{Mode: ModeSearch, Heading: true, Color: "auto", Fields: map[string]string{}}
	var sel []string
	modeFlag := "" // the mode flag already chosen (for the "one mode" error)

	setMode := func(m Mode, name string) error {
		if modeFlag != "" {
			return usageErr(fmt.Sprintf("only one mode allowed (got --%s and --%s)", modeFlag, name))
		}
		inv.Mode = m
		modeFlag = name
		return nil
	}

	i := 0
	for i < len(raw) {
		a := raw[i]
		i++

		if a == "--" {
			sel = append(sel, raw[i:]...)
			break
		}
		if a == "" || a == "-" || a[0] != '-' {
			sel = append(sel, a)
			continue
		}

		var name, val string
		hasVal := false
		if strings.HasPrefix(a, "--") {
			name = a[2:]
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				val, hasVal = name[eq+1:], true
				name = name[:eq]
			}
		} else {
			name = shortName(a[1:])
			if name == "" {
				return inv, usageErr("unknown flag " + a)
			}
		}

		needVal := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i >= len(raw) {
				return "", usageErr("flag --" + name + " needs a value")
			}
			v := raw[i]
			i++
			return v, nil
		}

		switch name {
		// primary modes
		case "search":
			if err := setMode(ModeSearch, name); err != nil {
				return inv, err
			}
		case "list":
			if err := setMode(ModeList, name); err != nil {
				return inv, err
			}
		case "show":
			if err := setMode(ModeShow, name); err != nil {
				return inv, err
			}
		case "links":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
		case "tags":
			if err := setMode(ModeTags, name); err != nil {
				return inv, err
			}
		case "types":
			if err := setMode(ModeTypes, name); err != nil {
				return inv, err
			}
		case "next":
			if err := setMode(ModeNext, name); err != nil {
				return inv, err
			}
		case "new":
			if err := setMode(ModeNew, name); err != nil {
				return inv, err
			}
		case "set":
			if err := setMode(ModeSet, name); err != nil {
				return inv, err
			}
		case "validate":
			if err := setMode(ModeValidate, name); err != nil {
				return inv, err
			}
		case "inspect":
			if err := setMode(ModeInspect, name); err != nil {
				return inv, err
			}
		case "graph":
			if err := setMode(ModeGraph, name); err != nil {
				return inv, err
			}
		case "version":
			if err := setMode(ModeVersion, name); err != nil {
				return inv, err
			}
		case "help":
			if err := setMode(ModeHelp, name); err != nil {
				return inv, err
			}
		case "examples":
			if err := setMode(ModeHelp, name); err != nil {
				return inv, err
			}
			inv.Verbose = true
		case "verbose":
			inv.Verbose = true
		// mode aliases
		case "done":
			if err := setMode(ModeSet, name); err != nil {
				return inv, err
			}
			inv.Status = "done"
		case "task":
			if err := setMode(ModeNew, name); err != nil {
				return inv, err
			}
			inv.Type = "task"
		case "backlinks":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
			inv.Inbound = true
		case "outlinks", "forward-links":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
			inv.Outbound = true
		// universal
		case "json":
			inv.JSON = true
		case "root":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Root = v
		case "query":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			sel = append(sel, v)
		// filters
		case "type":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Type = v
		case "status":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Status = v
		case "tag":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Tags = append(inv.Tags, v)
		case "in":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			switch v {
			case "heading", "tag", "link":
				inv.In = append(inv.In, v)
			default:
				return inv, usageErr("--in must be heading|tag|link, got " + v)
			}
		case "limit":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			n, cerr := strconv.Atoi(v)
			if cerr != nil {
				return inv, usageErr("--limit needs an integer, got " + v)
			}
			inv.Limit = n
		case "field":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			eq := strings.IndexByte(v, '=')
			if eq < 0 {
				return inv, usageErr("--field needs k=v, got " + v)
			}
			inv.Fields[v[:eq]] = v[eq+1:]
		// porcelain task fields — scalar sugar over --field k=v
		case "priority", "effort", "parent":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Fields[name] = v
		case "depends-on":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			for _, d := range strings.Split(v, ",") {
				if d = strings.TrimSpace(d); d != "" {
					inv.DependsOn = append(inv.DependsOn, d)
				}
			}
		// search output
		case "ignore-case":
			inv.IgnoreCase = true
		case "files-with-matches":
			inv.FilesOnly = true
		case "count":
			inv.Count = true
		case "heading":
			inv.Heading = true
		case "no-heading":
			inv.Heading = false
		// show
		case "raw":
			inv.Raw = true
		case "frontmatter":
			inv.Frontmatter = true
		// links
		case "inbound":
			inv.Inbound = true
		case "outbound":
			inv.Outbound = true
		// validate
		case "strict":
			inv.Strict = true
		case "schema":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Schema = v
		// color
		case "color":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			switch v {
			case "auto", "always", "never":
				inv.Color = v
			default:
				return inv, usageErr("--color must be auto|always|never, got " + v)
			}
		case "no-color":
			inv.Color = "never"
		default:
			return inv, usageErr("unknown flag --" + name)
		}
	}

	inv.Selector = strings.Join(sel, " ")
	return inv, nil
}
