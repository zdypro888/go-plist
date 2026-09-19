package plist

import (
	"fmt"
	"testing"
	"time"
)

func benchDoc() any {
	items := make([]any, 0, 2000)
	for i := 0; i < 2000; i++ {
		items = append(items, map[string]any{
			"id": int64(i), "name": fmt.Sprintf("item-%d", i), "ok": i%2 == 0, "ratio": float64(i) / 3,
			"blob": []byte{byte(i), 1, 2, 3}, "when": time.Unix(int64(1700000000+i), 0).UTC(), "tags": []any{"a", "b", "中文"},
		})
	}
	return items
}

func benchFormat(b *testing.B, format int) {
	doc := benchDoc()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(doc, format); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkMarshalBinary(b *testing.B)   { benchFormat(b, BinaryFormat) }
func BenchmarkMarshalXML(b *testing.B)      { benchFormat(b, XMLFormat) }
func BenchmarkMarshalOpenStep(b *testing.B) { benchFormat(b, OpenStepFormat) }

type benchItem struct {
	ID    int64     `plist:"id"`
	Name  string    `plist:"name"`
	OK    bool      `plist:"ok"`
	Ratio float64   `plist:"ratio"`
	Blob  []byte    `plist:"blob"`
	When  time.Time `plist:"when"`
	Tags  []string  `plist:"tags"`
}

func benchUnmarshal(b *testing.B, format int, target func() any) {
	data, err := Marshal(benchDoc(), format)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Unmarshal(data, target()); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkUnmarshalBinaryStruct(b *testing.B) {
	benchUnmarshal(b, BinaryFormat, func() any { return new([]benchItem) })
}
func BenchmarkUnmarshalBinaryAny(b *testing.B) {
	benchUnmarshal(b, BinaryFormat, func() any { return new(any) })
}
func BenchmarkUnmarshalXMLStruct(b *testing.B) {
	benchUnmarshal(b, XMLFormat, func() any { return new([]benchItem) })
}
