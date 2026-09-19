package plist

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// bplistWithObjects builds a minimal binary plist whose objects are given raw.
func bplistWithObjects(objs ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("bplist00")
	offs := make([]byte, 0, len(objs))
	for _, o := range objs {
		offs = append(offs, byte(b.Len()))
		b.Write(o)
	}
	tableOff := uint64(b.Len())
	b.Write(offs)
	trailer := make([]byte, 32)
	trailer[6] = 1 // offset int size
	trailer[7] = 1 // object ref size
	binary.BigEndian.PutUint64(trailer[8:], uint64(len(objs)))
	binary.BigEndian.PutUint64(trailer[16:], 0)
	binary.BigEndian.PutUint64(trailer[24:], tableOff)
	b.Write(trailer)
	return b.Bytes()
}

func TestBplistHugeCountsDoNotPanic(t *testing.T) {
	huge := func(tag byte, n uint64) []byte {
		o := []byte{tag | 0x0F, 0x13}
		return binary.BigEndian.AppendUint64(o, n)
	}
	for _, tag := range []byte{bpTagData, bpTagASCIIString, bpTagUTF16String, bpTagArray, bpTagDictionary} {
		for _, n := range []uint64{math.MaxUint64, 1 << 63, 1<<63 + 1, 1 << 40} {
			var v any
			if _, err := Unmarshal(bplistWithObjects(huge(tag, n)), &v); err == nil {
				t.Errorf("tag %#x count %#x: expected error", tag, n)
			}
		}
	}
}

func TestNestingDepthIsBounded(t *testing.T) {
	n := maxNestingDepth * 4
	docs := map[string]string{
		"text": strings.Repeat("(", n),
		"xml":  "<plist>" + strings.Repeat("<array>", n),
	}
	for name, doc := range docs {
		var v any
		if _, err := Unmarshal([]byte(doc), &v); err == nil {
			t.Errorf("%s: expected nesting error", name)
		}
	}
	// nesting below the limit still parses
	ok := strings.Repeat("(", 100) + strings.Repeat(")", 100)
	var v any
	if _, err := Unmarshal([]byte(ok), &v); err != nil {
		t.Fatalf("moderate nesting: %v", err)
	}
}

func TestXMLKeysAreEscaped(t *testing.T) {
	in := map[string]any{"a<b&c": "v", "x</key><string>1</string><key>admin": "w"}
	for _, format := range []int{XMLFormat, BinaryFormat, OpenStepFormat, GNUStepFormat} {
		data, err := Marshal(in, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var out map[string]any
		if _, err := Unmarshal(data, &out); err != nil {
			t.Fatalf("format %d: %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("format %d: got %v want %v", format, out, in)
		}
	}
}

// Binary plists cannot reference a nil value, so nil pointer/interface fields
// and elements are dropped there instead of panicking. The XML and text
// generators are intentionally left byte-for-byte unchanged.
func TestBplistOmitsNilValues(t *testing.T) {
	type S struct {
		A string
		P *int
		I any
		Z string
	}
	data, err := Marshal(S{A: "x", Z: "z"}, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if _, err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, map[string]any{"A": "x", "Z": "z"}) {
		t.Errorf("got %v", out)
	}
	data, err = Marshal([]any{nil, "a", nil}, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var arr []any
	if _, err := Unmarshal(data, &arr); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(arr, []any{"a"}) {
		t.Errorf("got %v", arr)
	}
}

// A nil pointer or interface field is omitted in every format. Writing the key
// without a value made Apple's parser take the next key as its value.
func TestNilFieldsAreOmittedInEveryFormat(t *testing.T) {
	type S struct {
		A string
		P *int
		I any
		Z string
	}
	for _, format := range []int{XMLFormat, BinaryFormat, OpenStepFormat, GNUStepFormat} {
		data, err := Marshal(S{A: "x", Z: "z"}, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var out map[string]any
		if _, err := Unmarshal(data, &out); err != nil {
			t.Fatalf("format %d: %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(out, map[string]any{"A": "x", "Z": "z"}) {
			t.Errorf("format %d: got %v (%s)", format, out, data)
		}
		data, err = Marshal([]any{nil, "a", nil}, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var arr []any
		if _, err := Unmarshal(data, &arr); err != nil || !reflect.DeepEqual(arr, []any{"a"}) {
			t.Errorf("format %d: got %v err %v (%s)", format, arr, err, data)
		}
	}
	// a field that is set is written exactly as before
	n := 5
	data, _ := Marshal(S{A: "x", P: &n, I: "i", Z: "z"}, XMLFormat)
	want := "<dict><key>A</key><string>x</string><key>I</key><string>i</string><key>P</key><integer>5</integer><key>Z</key><string>z</string></dict>"
	if !strings.Contains(string(data), want) {
		t.Errorf("got %s", data)
	}
}

// Keys that were already well-formed XML must be written exactly as before.
func TestXMLWellFormedKeysUnchanged(t *testing.T) {
	data, err := Marshal(map[string]string{`q"uote's > x`: "v"}, XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `<key>q"uote's > x</key>`) {
		t.Errorf("got %s", data)
	}
}

// In-range dates keep the historical UnixNano-based encoding bit for bit.
func TestBplistDateEncodingUnchanged(t *testing.T) {
	for i := 0; i < 5000; i++ {
		in := time.Unix(int64(1500000000+i*7919), int64((i*1000003)%1000000000)).UTC()
		data, err := Marshal(in, BinaryFormat)
		if err != nil {
			t.Fatal(err)
		}
		want := float64(in.UnixNano())/float64(time.Second) - 978307200
		if got := math.Float64frombits(binary.BigEndian.Uint64(data[9:17])); got != want {
			t.Fatalf("%v: got %v want %v", in, got, want)
		}
	}
}

func TestBplistDataWithCollidingCRC(t *testing.T) {
	a := []byte{0x68, 0x63, 0xda, 0x74, 0xf8, 0xe7, 0xc0, 0x2c}
	b := []byte{0x56, 0x55, 0xd0, 0x29, 0x8b, 0x34, 0xe7, 0xf3}
	data, err := Marshal([][]byte{a, b}, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	if _, err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, [][]byte{a, b}) {
		t.Errorf("got %x", out)
	}
}

func TestTextStringsRoundTrip(t *testing.T) {
	in := []string{"hi 😀", "é", "中文", "tab\t", "q\"uote"}
	for _, format := range []int{OpenStepFormat, GNUStepFormat} {
		data, err := Marshal(in, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var out []string
		if _, err := Unmarshal(data, &out); err != nil {
			t.Fatalf("format %d: %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("format %d: got %q want %q (%s)", format, out, in, data)
		}
	}
}

func TestGNUStepEmptyExtendedValues(t *testing.T) {
	for _, doc := range []string{`<*I"">`, `<*B"">`, `<*R"">`, `<*D"">`, `<*Z1>`, `<*D100%bad>`} {
		var v any
		_, err := Unmarshal([]byte(doc), &v)
		if err == nil {
			t.Errorf("%s: expected error", doc)
		} else if strings.Contains(err.Error(), "%!") {
			t.Errorf("%s: garbled error %q", doc, err)
		}
	}
}

func TestBplistDatesOutsideUnixNanoRange(t *testing.T) {
	for _, year := range []int{1, 1600, 2024, 3000} {
		in := time.Date(year, 3, 4, 5, 6, 7, 0, time.UTC)
		data, err := Marshal(in, BinaryFormat)
		if err != nil {
			t.Fatal(err)
		}
		var out time.Time
		if _, err := Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		if !out.Equal(in) {
			t.Errorf("year %d: got %v", year, out)
		}
	}
}

func TestBplistSpecialFloats(t *testing.T) {
	in := []float64{math.NaN(), 1.5, 0, math.Inf(1)}
	data, err := Marshal(in, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var out []float64
	if _, err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 || !math.IsNaN(out[0]) || out[1] != 1.5 || out[2] != 0 || !math.IsInf(out[3], 1) {
		t.Errorf("got %v", out)
	}
}

func TestDecodeTargetValidation(t *testing.T) {
	data, _ := Marshal("x", XMLFormat)
	var s string
	if _, err := Unmarshal(data, s); err == nil {
		t.Error("non-pointer: expected error")
	}
	var np *string
	if _, err := Unmarshal(data, np); err == nil {
		t.Error("nil pointer: expected error")
	}
	if _, err := Unmarshal(data, &s); err != nil || s != "x" {
		t.Errorf("pointer: %q %v", s, err)
	}
	dict, _ := Marshal(map[string]string{"k": "v"}, XMLFormat)
	m := map[string]string{}
	if _, err := Unmarshal(dict, m); err != nil || m["k"] != "v" {
		t.Errorf("map: %v %v", m, err)
	}
}

// nestedSharedBplist builds a binary plist of `levels` arrays, each holding
// `fanout` references to the next one: a few bytes that expand to fanout^levels.
func nestedSharedBplist(levels, fanout int) []byte {
	objs := make([][]byte, 0, levels+1)
	for level := 0; level < levels; level++ {
		o := []byte{bpTagArray | byte(fanout)}
		for i := 0; i < fanout; i++ {
			o = append(o, byte(level+1))
		}
		objs = append(objs, o)
	}
	objs = append(objs, []byte{bpTagASCIIString | 1, 'x'})
	return bplistWithObjects(objs...)
}

func TestSharedCollectionsCannotExpandWithoutBound(t *testing.T) {
	// 10^8 leaves from about 130 bytes
	bomb := nestedSharedBplist(8, 10)
	var v any
	if _, err := Unmarshal(bomb, &v); err == nil {
		t.Fatal("expected an expansion error")
	}
	var typed [][][][][][][][]string
	if _, err := Unmarshal(bomb, &typed); err == nil {
		t.Fatal("typed target: expected an expansion error")
	}
	// moderate sharing still decodes, every reference being its own copy
	small := nestedSharedBplist(3, 10)
	if _, err := Unmarshal(small, &v); err != nil {
		t.Fatal(err)
	}
	outer := v.([]any)
	if len(outer) != 10 || len(outer[0].([]any)) != 10 {
		t.Fatalf("unexpected shape: %d", len(outer))
	}
	outer[0].([]any)[0] = "changed"
	if _, same := outer[1].([]any)[0].(string); same {
		t.Fatal("references to a shared array must stay independent copies")
	}
}

// A large document without sharing is not affected by the expansion budget.
func TestLargeUnsharedDocumentDecodes(t *testing.T) {
	in := make([]int, 1<<21)
	for i := range in {
		in[i] = i
	}
	data, err := Marshal(in, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	if _, err := Unmarshal(data, &out); err != nil || len(out) != len(in) || out[len(out)-1] != len(in)-1 {
		t.Fatalf("len=%d err=%v", len(out), err)
	}
}

type EmbeddedInner struct {
	X int
	Y string
}

type embeddedOuter struct {
	*EmbeddedInner
	Z int
}

// Marshalling by value with a nil embedded pointer used to panic; it now gives
// the same bytes as marshalling a pointer to the same struct.
func TestNilEmbeddedPointerByValue(t *testing.T) {
	for _, format := range []int{XMLFormat, BinaryFormat, OpenStepFormat} {
		byPointer, err := Marshal(&embeddedOuter{Z: 3}, format)
		if err != nil {
			t.Fatal(err)
		}
		byValue, err := Marshal(embeddedOuter{Z: 3}, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		if !bytes.Equal(byPointer, byValue) {
			t.Errorf("format %d: by value %q, by pointer %q", format, byValue, byPointer)
		}
	}
	filled, _ := Marshal(embeddedOuter{EmbeddedInner: &EmbeddedInner{X: 1, Y: "y"}, Z: 3}, XMLFormat)
	var out embeddedOuter
	if _, err := Unmarshal(filled, &out); err != nil || out.EmbeddedInner == nil || out.X != 1 || out.Y != "y" || out.Z != 3 {
		t.Fatalf("round trip: %+v %v", out, err)
	}
}
