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

func TestNilFieldsAndElementsAreOmitted(t *testing.T) {
	type S struct {
		A string
		P *int
		I any
	}
	for _, format := range []int{XMLFormat, BinaryFormat, OpenStepFormat, GNUStepFormat} {
		data, err := Marshal(S{A: "x"}, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var out map[string]any
		if _, err := Unmarshal(data, &out); err != nil {
			t.Fatalf("format %d: %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(out, map[string]any{"A": "x"}) {
			t.Errorf("format %d: got %v", format, out)
		}
		data, err = Marshal([]any{nil, "a", nil}, format)
		if err != nil {
			t.Fatalf("format %d: %v", format, err)
		}
		var arr []any
		if _, err := Unmarshal(data, &arr); err != nil {
			t.Fatalf("format %d: %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(arr, []any{"a"}) {
			t.Errorf("format %d: got %v", format, arr)
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
	in := []string{"", "hi 😀", "", "é", "中文", ""}
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
	in := []float64{math.NaN(), math.Copysign(0, -1), 0, math.Inf(1)}
	data, err := Marshal(in, BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	var out []float64
	if _, err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 || !math.IsNaN(out[0]) || !math.Signbit(out[1]) || math.Signbit(out[2]) || !math.IsInf(out[3], 1) {
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
