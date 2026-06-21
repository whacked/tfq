package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"tfq/internal/graph"
	"tfq/internal/scan"
)

// Set mutates the frontmatter of the record ref resolves to, preserving body
// and existing key order.
func Set(root, ref string, changes map[string]string, addTags []string) (WriteResult, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return WriteResult{}, err
	}
	g := graph.Build(recs, graph.DefaultOptions())
	rel, ok := g.Resolve(ref)
	if !ok {
		return WriteResult{}, fmt.Errorf("no record matches %q", ref)
	}
	full := filepath.Join(root, rel)
	b, err := os.ReadFile(full)
	if err != nil {
		return WriteResult{}, err
	}
	updated, err := rewriteFrontmatter(string(b), changes, addTags)
	if err != nil {
		return WriteResult{}, err
	}
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: rel, Action: "updated"}, nil
}

// rewriteFrontmatter applies changes/addTags to the leading --- block.
func rewriteFrontmatter(content string, changes map[string]string, addTags []string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("no frontmatter block to modify")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", fmt.Errorf("unterminated frontmatter block")
	}
	fmSrc := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmSrc), &doc); err != nil {
		return "", err
	}
	var mapping *yaml.Node
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		mapping = doc.Content[0]
	} else {
		mapping = &yaml.Node{Kind: yaml.MappingNode}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	}

	for k, v := range changes {
		setScalar(mapping, k, v)
	}
	for _, tag := range addTags {
		appendTag(mapping, tag)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return "---\n" + string(out) + "---\n" + body, nil
}

func findValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setScalar(mapping *yaml.Node, key, value string) {
	if v := findValue(mapping, key); v != nil {
		v.Kind = yaml.ScalarNode
		v.Tag = ""
		v.Value = value
		v.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}

func appendTag(mapping *yaml.Node, tag string) {
	v := findValue(mapping, "tags")
	if v == nil {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: tag})
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "tags"}, seq)
		return
	}
	if v.Kind != yaml.SequenceNode {
		return
	}
	for _, e := range v.Content {
		if e.Value == tag {
			return
		}
	}
	v.Content = append(v.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: tag})
}
