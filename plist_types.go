package plist

import (
	"math"
	"sort"
	"strconv"
	"time"
)

// magic value used in the non-binary encoding of UIDs
// (stored as a dictionary mapping CF$UID->integer)
const cfUIDMagic = "CF$UID"

type cfValue interface {
	typeName() string
	hash() any
}

type cfDictionary struct {
	keys   sort.StringSlice
	values []cfValue
}

func (*cfDictionary) typeName() string {
	return "dictionary"
}

func (p *cfDictionary) hash() any {
	return p
}

func (p *cfDictionary) Len() int {
	return len(p.keys)
}

func (p *cfDictionary) Less(i, j int) bool {
	return p.keys.Less(i, j)
}

func (p *cfDictionary) Swap(i, j int) {
	p.keys.Swap(i, j)
	p.values[i], p.values[j] = p.values[j], p.values[i]
}

func (p *cfDictionary) sort() {
	sort.Sort(p)
}

func (p *cfDictionary) maybeUID(lax bool) cfValue {
	if len(p.keys) == 1 && p.keys[0] == "CF$UID" && len(p.values) == 1 {
		pval := p.values[0]
		if integer, ok := pval.(*cfNumber); ok {
			return cfUID(integer.value)
		}
		// Openstep only has cfString. Act like the unmarshaller a bit.
		if lax {
			if str, ok := pval.(cfString); ok {
				if i, err := strconv.ParseUint(string(str), 10, 64); err == nil {
					return cfUID(i)
				}
			}
		}
	}
	return p
}

type cfArray struct {
	values []cfValue
}

func (*cfArray) typeName() string {
	return "array"
}

func (p *cfArray) hash() any {
	return p
}

type cfString string

func (cfString) typeName() string {
	return "string"
}

func (p cfString) hash() any {
	return string(p)
}

type cfNumber struct {
	signed bool
	value  uint64
}

func (*cfNumber) typeName() string {
	return "integer"
}

func (p *cfNumber) hash() any {
	if p.signed {
		return int64(p.value)
	}
	return p.value
}

type cfReal struct {
	wide  bool
	value float64
}

func (cfReal) typeName() string {
	return "real"
}

// cfNaNKey lets NaN be found again in the object map (NaN != NaN, so a plain
// float key can never match). All other reals keep their historical keys so
// that uniquing, and therefore the emitted bytes, are unchanged.
type cfNaNKey struct {
	wide bool
	bits uint64
}

func (p *cfReal) hash() any {
	if math.IsNaN(p.value) {
		return cfNaNKey{p.wide, math.Float64bits(p.value)}
	}
	if p.wide {
		return p.value
	}
	return float32(p.value)
}

type cfBoolean bool

func (cfBoolean) typeName() string {
	return "boolean"
}

func (p cfBoolean) hash() any {
	return bool(p)
}

type cfUID UID

func (cfUID) typeName() string {
	return "UID"
}

func (p cfUID) hash() any {
	return p
}

func (p cfUID) toDict() *cfDictionary {
	return &cfDictionary{
		keys: []string{cfUIDMagic},
		values: []cfValue{&cfNumber{
			signed: false,
			value:  uint64(p),
		}},
	}
}

type cfData []byte

func (cfData) typeName() string {
	return "data"
}

// cfDataKey keeps data keys distinct from cfString keys in the object map.
type cfDataKey string

func (p cfData) hash() any {
	// Data are uniqued by content; a checksum alone would silently merge
	// distinct values that happen to collide.
	return cfDataKey(p)
}

type cfDate time.Time

func (cfDate) typeName() string {
	return "date"
}

func (p cfDate) hash() any {
	return time.Time(p)
}
