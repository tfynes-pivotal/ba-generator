// Unit tests for ba-deleter selection logic (no Hub connectivity required).

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

var sample = []BusinessApp{
	{EntityID: "id-3", EntityName: "ad12345678"},
	{EntityID: "id-1", EntityName: "Payments Platform"},
	{EntityID: "id-2", EntityName: "Identity Service"},
	{EntityID: "id-4", EntityName: "ad87654321"},
	{EntityID: "id-5", EntityName: "Payments Platform"}, // duplicate name
	{EntityID: "id-6", EntityName: "xad12345678x"},
}

func ids(bas []BusinessApp) []string {
	out := make([]string, len(bas))
	for i, b := range bas {
		out[i] = b.EntityID
	}
	return out
}

func TestSelectByNames(t *testing.T) {
	targets, missing := selectTargets(sample, []string{"Payments Platform", "Nope", "Identity Service"}, nil)
	if got, want := ids(targets), []string{"id-2", "id-1", "id-5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
	if want := []string{"Nope"}; !reflect.DeepEqual(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

func TestSelectByNamesIsCaseSensitive(t *testing.T) {
	targets, missing := selectTargets(sample, []string{"payments platform"}, nil)
	if len(targets) != 0 || len(missing) != 1 {
		t.Errorf("expected no match for different case, got targets=%v missing=%v", ids(targets), missing)
	}
}

func TestSelectByRegexAnchored(t *testing.T) {
	targets, _ := selectTargets(sample, nil, regexp.MustCompile(`^ad\d{8}$`))
	if got, want := ids(targets), []string{"id-3", "id-4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

func TestSelectByRegexUnanchored(t *testing.T) {
	targets, _ := selectTargets(sample, nil, regexp.MustCompile(`ad\d{8}`))
	if got, want := ids(targets), []string{"id-3", "id-4", "id-6"}; !reflect.DeepEqual(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

func TestParseNames(t *testing.T) {
	got := parseNames(" A , B,,A", []string{"C", "B", " "})
	if want := []string{"A", "B", "C"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseNames = %v, want %v", got, want)
	}
}

func TestLoadNamesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "names.txt")
	if err := os.WriteFile(p, []byte("# comment\nAlpha\n\n  Beta Two  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadNamesFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Alpha", "Beta Two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("loadNamesFile = %v, want %v", got, want)
	}
}
