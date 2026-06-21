package extract

import (
	"testing"

	"tfq/internal/model"
)

func hasMarker(ms []model.Marker, kind, val string) bool {
	for _, m := range ms {
		if m.Kind == kind && m.Value == val {
			return true
		}
	}
	return false
}

func TestMarkersHashtagsAndAngles(t *testing.T) {
	c := "intro #alpha and <single phrase> then <<double phrase>>\n"
	ms, _ := Markers(c, "markdown")
	if !hasMarker(ms, model.MarkerHashtag, "alpha") {
		t.Errorf("missing hashtag alpha: %#v", ms)
	}
	if !hasMarker(ms, model.MarkerDoubleAngle, "double phrase") {
		t.Errorf("missing double-angle: %#v", ms)
	}
	if !hasMarker(ms, model.MarkerAngle, "single phrase") {
		t.Errorf("missing single angle: %#v", ms)
	}
	// the inside of a << >> must NOT also be reported as a single angle
	if hasMarker(ms, model.MarkerAngle, "double phrase") {
		t.Errorf("double-angle double-counted as angle: %#v", ms)
	}
}

func TestMarkersOrgTagsOnlyInOrg(t *testing.T) {
	c := "* Heading :work:urgent:\n"
	org, _ := Markers(c, "org")
	if !hasMarker(org, model.MarkerOrgTag, "work") || !hasMarker(org, model.MarkerOrgTag, "urgent") {
		t.Errorf("missing org tags: %#v", org)
	}
	md, _ := Markers(c, "markdown")
	for _, m := range md {
		if m.Kind == model.MarkerOrgTag {
			t.Errorf("org tags should not fire in markdown: %#v", m)
		}
	}
}

func TestMarkersSkipURLAngles(t *testing.T) {
	c := "see <https://example.com> for more\n"
	ms, _ := Markers(c, "markdown")
	for _, m := range ms {
		if m.Kind == model.MarkerAngle {
			t.Errorf("url autolink should not be an angle marker: %#v", m)
		}
	}
}
