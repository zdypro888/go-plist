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
