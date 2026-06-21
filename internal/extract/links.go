package extract

import (
	"regexp"
	"sort"

	"tfq/internal/model"
)

type linkPattern struct {
	re       *regexp.Regexp
	priority int // lower = higher priority on overlap
	build    func(content string, m []int) model.Link
}

func strptr(s string) *string { return &s }

var linkPatterns = []linkPattern{
	{ // embed ![[target]] / ![[target|alias]]
		re:       regexp.MustCompile(`!\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`),
		priority: 0,
		build: func(c string, m []int) model.Link {
			var label *string
			if m[4] >= 0 {
				label = strptr(c[m[4]:m[5]])
			}
			return model.Link{Kind: model.LinkEmbed, Target: c[m[2]:m[3]], Label: label}
		},
	},
	{ // org [[link][desc]]
		re:       regexp.MustCompile(`\[\[([^\]]+)\]\[([^\]]+)\]\]`),
		priority: 1,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkOrg, Target: c[m[2]:m[3]], Label: strptr(c[m[4]:m[5]])}
		},
	},
	{ // wiki [[target]] / [[target|alias]]
		re:       regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`),
		priority: 2,
		build: func(c string, m []int) model.Link {
			var label *string
			if m[4] >= 0 {
				label = strptr(c[m[4]:m[5]])
			}
			return model.Link{Kind: model.LinkWiki, Target: c[m[2]:m[3]], Label: label}
		},
	},
	{ // markdown [label](target)
		re:       regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`),
		priority: 3,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkMarkdown, Target: c[m[4]:m[5]], Label: strptr(c[m[2]:m[3]])}
		},
	},
	{ // autolink <scheme://...> or <mailto:...>
		re:       regexp.MustCompile(`<((?:[a-zA-Z][a-zA-Z0-9+.-]*://|mailto:)[^>\s]+)>`),
		priority: 4,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkAutolink, Target: c[m[2]:m[3]]}
		},
	},
	{ // bare url
		re:       regexp.MustCompile(`(?:[a-zA-Z][a-zA-Z0-9+.-]*://)[^\s)>\]]+`),
		priority: 5,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkBareURL, Target: c[m[0]:m[1]]}
		},
	},
}

type candidate struct {
	start, end, priority int
	link                 model.Link
}

// Links extracts all recognized link forms with overlap resolution: when two
// matches overlap, the higher-priority (more specific) one wins. Never fails.
func Links(content string) ([]model.Link, []model.Warning) {
	cands := []candidate{}
	for _, p := range linkPatterns {
		for _, m := range p.re.FindAllStringSubmatchIndex(content, -1) {
			l := p.build(content, m)
			line, col := lineCol(content, m[0])
			l.Line, l.Col = line, col
			cands = append(cands, candidate{start: m[0], end: m[1], priority: p.priority, link: l})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].start != cands[j].start {
			return cands[i].start < cands[j].start
		}
		return cands[i].priority < cands[j].priority
	})
	out := []model.Link{}
	occupied := []candidate{}
	overlaps := func(a, b candidate) bool { return a.start < b.end && b.start < a.end }
	for _, c := range cands {
		clash := false
		for _, o := range occupied {
			if overlaps(c, o) {
				clash = true
				break
			}
		}
		if clash {
			continue
		}
		occupied = append(occupied, c)
		out = append(out, c.link)
	}
	// stable output ordering by position
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out, nil
}
