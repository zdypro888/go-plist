package plist

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/google/go-cmp/cmp"
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

// addObject must hand out exactly the UIDs the original linear cmp.Equal scan did.
func TestAddObjectMatchesLinearScan(t *testing.T) {
	legacy := func(objects *[]any, obj any) UID {
		for i, o := range *objects {
			if cmp.Equal(o, obj) {
				return UID(i)
			}
		}
		*objects = append(*objects, obj)
		return UID(len(*objects) - 1)
	}
	r := rand.New(rand.NewSource(7))
	pick := func() any {
		switch r.Intn(9) {
		case 0:
			return fmt.Sprintf("s%d", r.Intn(40))
		case 1:
			return int64(r.Intn(40) - 20)
		case 2:
			return uint64(r.Intn(40))
		case 3:
			return []float64{0, math.Copysign(0, -1), 1.5, math.NaN(), math.Inf(1)}[r.Intn(5)]
		case 4:
			return r.Intn(2) == 0
		case 5:
			return archiverMutableArrayClass
		case 6:
			return map[string]any{"$class": UID(r.Intn(3)), "k": fmt.Sprintf("v%d", r.Intn(5))}
		case 7:
			return &archiverClass{ClassName: fmt.Sprintf("C%d", r.Intn(4)), Classes: []string{"NSObject"}}
		default:
			return UID(r.Intn(10))
		}
	}
	var archive Archiver
	var reference []any
	for i := 0; i < 20000; i++ {
		obj := pick()
		if got, want := archive.addObject(obj), legacy(&reference, obj); got != want {
			t.Fatalf("step %d (%#v): uid %d, want %d", i, obj, got, want)
		}
		if i == 9000 { // the exported slice may be replaced from outside
			archive.Objects = append([]any(nil), archive.Objects[:len(archive.Objects)/2]...)
			reference = append([]any(nil), reference[:len(reference)/2]...)
		}
	}
	if len(archive.Objects) != len(reference) {
		t.Fatalf("objects: %d, want %d", len(archive.Objects), len(reference))
	}
}

func BenchmarkArchiverAddObject(b *testing.B) {
	for b.Loop() {
		var archive Archiver
		for i := 0; i < 4000; i++ {
			archive.addObject(int64(i))
		}
	}
}
