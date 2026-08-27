package controls

import "testing"

func TestCatalogIsCompleteUniqueAndRoundTrips(t *testing.T) {
	names := Names()
	if len(names) != MainButtonCount {
		t.Fatalf("catalog length=%d, want %d", len(names), MainButtonCount)
	}
	seen := make(map[string]struct{}, MainButtonCount)
	for i, name := range names {
		if name == "" {
			t.Fatalf("button %d has empty name", i)
		}
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate button name %q", name)
		}
		seen[name] = struct{}{}
		if got := Name(i); got != name {
			t.Fatalf("Name(%d)=%q, want %q", i, got, name)
		}
		if got, ok := Index(name); !ok || got != i {
			t.Fatalf("Index(%q)=(%d,%v), want (%d,true)", name, got, ok, i)
		}
	}
}

func TestIndexNormalizesHumanReferences(t *testing.T) {
	cases := map[string]int{
		"index-1":   0,
		" INDEX 6 ": 5,
		"middle-2":  7,
		"ring 5":    15,
		"pinky_6":   21,
	}
	for reference, want := range cases {
		got, ok := Index(reference)
		if !ok || got != want {
			t.Fatalf("Index(%q)=(%d,%v), want (%d,true)", reference, got, ok, want)
		}
	}
}

func TestInvalidControlsAreRejectedAndNamesAreCopied(t *testing.T) {
	if Name(-1) != "" || Name(MainButtonCount) != "" {
		t.Fatal("invalid index unexpectedly resolved")
	}
	if _, ok := Index(""); ok {
		t.Fatal("empty reference unexpectedly resolved")
	}
	if _, ok := Index("INDEX_99"); ok {
		t.Fatal("unknown reference unexpectedly resolved")
	}

	copy := Names()
	copy[0] = "MUTATED"
	if Name(0) != "INDEX_1" {
		t.Fatal("caller mutated canonical catalog through Names")
	}
}
