package graph

import (
	"sort"

	"tfq/internal/model"
)

// NextOptions configures the dependency-aware ready-set computation.
type NextOptions struct {
	DepField     string
	StatusField  string
	DoneStatuses []string
}

// DefaultNextOptions returns the conventional taskmd-compatible settings.
func DefaultNextOptions() NextOptions {
	return NextOptions{
		DepField:     "dependencies",
		StatusField:  "status",
		DoneStatuses: []string{"completed", "done", "cancelled"},
	}
}

// Next returns the records that are ready to work on: tasks (records with the
// status field) that are not done and whose dependencies are all satisfied.
func (g *Graph) Next(o NextOptions) ([]model.FileVitals, []model.Warning) {
	done := map[string]bool{}
	for _, s := range o.DoneStatuses {
		done[s] = true
	}
	status := func(r model.FileVitals) (string, bool) {
		return fmString(r.Frontmatter, o.StatusField)
	}

	ready := []model.FileVitals{}
	var warns []model.Warning
	for _, r := range g.records {
		st, isTask := status(r)
		if !isTask || done[st] {
			continue
		}
		blocked := false
		for _, raw := range edgeValues(r.Frontmatter[o.DepField]) {
			to, ok := g.Resolve(raw)
			if !ok {
				warns = append(warns, model.Warning{Module: "next", Message: r.Path + ": unresolved dependency " + raw})
				blocked = true
				continue
			}
			depStatus := ""
			for _, dr := range g.records {
				if dr.Path == to {
					depStatus, _ = fmString(dr.Frontmatter, o.StatusField)
					break
				}
			}
			if !done[depStatus] {
				blocked = true
			}
		}
		if !blocked {
			ready = append(ready, r)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Path < ready[j].Path })
	return ready, warns
}
