package conformance

import (
	"strings"
	"testing"
)

// TestMatchesFeatureSelectsTheGroupNotThePrefix pins the filter that makes a partial run
// worth doing. A lowering slice works in an area rather than on one leaf, so -feature
// math has to bring the whole area with it; but the boundary has to be the dot, or a
// tag that merely starts with the same letters would be pulled in and the run would
// quietly cover more than it claimed.
func TestMatchesFeatureSelectsTheGroupNotThePrefix(t *testing.T) {
	cases := []struct {
		tag, want string
		selected  bool
	}{
		{"math.hypot", "math.hypot", true},
		{"math.hypot", "math", true},
		{"math.trunc", "math", true},
		{"temporal.now.instant", "temporal", true},
		{"temporal.now.instant", "temporal.now", true},
		{"mathml.render", "math", false},
		{"math", "math.hypot", false},
		{"string.raw", "math", false},
		{"", "math", false},
	}
	for _, c := range cases {
		if got := matchesFeature(c.tag, c.want); got != c.selected {
			t.Errorf("matchesFeature(%q, %q) = %v, want %v", c.tag, c.want, got, c.selected)
		}
	}
}

// TestFeatureNamesListsGroupsAlongsideLeaves pins what a mistyped -feature prints. The
// point of the message is to name the shorter thing that would have worked, so a list of
// leaves alone would be the wrong help: the group is the selectable name a developer is
// usually reaching for.
func TestFeatureNamesListsGroupsAlongsideLeaves(t *testing.T) {
	all := []Fixture{
		{Meta: Meta{Feature: "math.hypot"}},
		{Meta: Meta{Feature: "math.trunc"}},
		{Meta: Meta{Feature: "temporal.now.instant"}},
		{Meta: Meta{Feature: "json"}},
		{Meta: Meta{Feature: ""}},
	}
	got := strings.Join(featureNames(all), " ")
	want := "json math math.hypot math.trunc temporal temporal.now temporal.now.instant"
	if got != want {
		t.Errorf("featureNames = %q, want %q", got, want)
	}
}

// TestEveryCorpusFeatureIsSelectable is the guard against a tag that no -feature value
// can reach. A fixture whose group name is empty or malformed, ".math" or "math..hypot",
// would sit in the corpus and never appear in a filtered run, so the slice working in
// that area would report green without having touched it.
func TestEveryCorpusFeatureIsSelectable(t *testing.T) {
	for _, f := range mustDiscover(t) {
		tag := f.Meta.Feature
		if tag == "" {
			t.Errorf("fixture %s has no feature tag, so no filtered run can select it", f.Slug)
			continue
		}
		for _, part := range strings.Split(tag, ".") {
			if part == "" {
				t.Errorf("fixture %s has feature %q with an empty dotted part", f.Slug, tag)
			}
		}
		if !matchesFeature(tag, tag) {
			t.Errorf("fixture %s feature %q does not select itself", f.Slug, tag)
		}
	}
}
