package main

import (
	"reflect"
	"testing"
)

func TestPartition(t *testing.T) {
	bools := map[string]bool{"strict": true}

	pos, flags, err := partition([]string{"foo", "dir", "--type", "note", "--strict"}, bools)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"foo", "dir"}) {
		t.Errorf("pos = %#v", pos)
	}
	if flags["type"] != "note" || flags["strict"] != "true" {
		t.Errorf("flags = %#v", flags)
	}

	// flags before positionals, and --k=v form
	pos, flags, err = partition([]string{"--tag=urgent", "ref", "dir"}, bools)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"ref", "dir"}) || flags["tag"] != "urgent" {
		t.Errorf("pos=%#v flags=%#v", pos, flags)
	}

	// non-bool flag missing a value -> error
	if _, _, err := partition([]string{"--type"}, bools); err == nil {
		t.Error("expected error for --type with no value")
	}
}
