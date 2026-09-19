package plist

import (
	"reflect"
	"testing"
)

type archivedPoint struct {
	Name string
	X    int64
	Tags []string
}

func TestArchiverRoundTripStillWorks(t *testing.T) {
	// X is negative: negative integers used to fail to decode.
	in := archivedPoint{Name: "p", X: -7, Tags: []string{"a", "b"}}
	var writer Archiver
	data, err := writer.Marshal(&in)
	if err != nil {
		t.Fatal(err)
	}
	var reader Archiver
	if err := reader.ReadFromData(data); err != nil {
		t.Fatal(err)
	}
	var out archivedPoint
	if err := reader.Unmarshal(&out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %+v want %+v", out, in)
	}
	if text, err := reader.Print(); err != nil || text == "" {
		t.Fatalf("Print: %q %v", text, err)
	}
}

// Archives from untrusted sources used to crash the process.
func TestArchiverMalformedInputReturnsError(t *testing.T) {
	cyclic := map[string]any{"$class": UID(2), "NS.objects": []any{UID(1)}}
	for name, archive := range map[string]*Archiver{
		"missing top":       {Objects: []any{"$null"}},
		"root out of range": {Objects: []any{"$null"}, Top: &archiverTop{Root: 99}},
		"no class":          {Objects: []any{"$null", map[string]any{"x": int64(1)}}, Top: &archiverTop{Root: 1}},
		"class not a dict":  {Objects: []any{"$null", map[string]any{"$class": UID(0)}}, Top: &archiverTop{Root: 1}},
		"reference cycle": {Objects: []any{"$null", cyclic,
			map[string]any{"$classname": "NSArray", "$classes": []any{"NSArray", "NSObject"}}}, Top: &archiverTop{Root: 1}},
	} {
		var out any
		if err := archive.Unmarshal(&out); err == nil {
			t.Errorf("%s: Unmarshal returned no error", name)
		}
		if _, err := archive.Print(); err == nil {
			t.Errorf("%s: Print returned no error", name)
		}
	}
}
