package plist

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"time"
	"unicode/utf16"
)

func bplistMinimumIntSize(n uint64) int {
	switch {
	case n <= uint64(0xff):
		return 1
	case n <= uint64(0xffff):
		return 2
	case n <= uint64(0xffffffff):
		return 4
	default:
		return 8
	}
}

func bplistValueShouldUnique(pval cfValue) bool {
	switch pval.(type) {
	case cfString, *cfNumber, *cfReal, cfDate, cfData:
		return true
	}
	return false
}

type bplistGenerator struct {
	writer   *countedWriter
	scratch  [8]byte
	objmap   map[any]uint64 // maps pValue.hash()es to object locations
	strmap   map[string]uint64
	objtable []cfValue
	trailer  bplistTrailer
}

func (p *bplistGenerator) flattenPlistValue(pval cfValue) {
	// Strings (every dictionary key and most values) get their own typed map:
	// boxing each one into an `any` key was a quarter of all allocations. A
	// string key can never equal a key of another type, so uniquing and the
	// order of the object table are unchanged.
	if str, ok := pval.(cfString); ok {
		if _, seen := p.strmap[string(str)]; seen {
			return
		}
		p.strmap[string(str)] = uint64(len(p.objtable))
		p.objtable = append(p.objtable, pval)
		return
	}

	key := pval.hash()
	if bplistValueShouldUnique(pval) {
		if _, ok := p.objmap[key]; ok {
			return
		}
	}

	p.objmap[key] = uint64(len(p.objtable))
	p.objtable = append(p.objtable, pval)

	switch pval := pval.(type) {
	case *cfDictionary:
		// nil pointers/interfaces marshal to a nil value; binary plists have no
		// way to reference one, so drop the pair instead of panicking below.
		if slices.Contains(pval.values, nil) {
			keys, values := pval.keys[:0:0], pval.values[:0:0]
			for i, v := range pval.values {
				if v != nil {
					keys, values = append(keys, pval.keys[i]), append(values, v)
				}
			}
			pval.keys, pval.values = keys, values
		}
		pval.sort()
		for _, k := range pval.keys {
			p.flattenPlistValue(cfString(k))
		}
		for _, v := range pval.values {
			p.flattenPlistValue(v)
		}
	case *cfArray:
		pval.values = slices.DeleteFunc(pval.values, func(v cfValue) bool { return v == nil })
		for _, v := range pval.values {
			p.flattenPlistValue(v)
		}
	}
}

func (p *bplistGenerator) indexForPlistValue(pval cfValue) (uint64, bool) {
	if str, isString := pval.(cfString); isString {
		v, ok := p.strmap[string(str)]
		return v, ok
	}
	v, ok := p.objmap[pval.hash()]
	return v, ok
}

func (p *bplistGenerator) generateDocument(root cfValue) error {
	p.objtable = make([]cfValue, 0, 16)
	p.objmap = make(map[any]uint64)
	p.strmap = make(map[string]uint64)
	p.flattenPlistValue(root)

	p.trailer.NumObjects = uint64(len(p.objtable))
	p.trailer.ObjectRefSize = uint8(bplistMinimumIntSize(p.trailer.NumObjects))

	if _, err := p.writer.Write([]byte("bplist00")); err != nil {
		return err
	}

	offtable := make([]uint64, p.trailer.NumObjects)
	for i, pval := range p.objtable {
		offtable[i] = uint64(p.writer.BytesWritten())
		if err := p.writePlistValue(pval); err != nil {
			return err
		}
	}

	p.trailer.OffsetIntSize = uint8(bplistMinimumIntSize(uint64(p.writer.BytesWritten())))
	p.trailer.TopObject = p.objmap[root.hash()]
	p.trailer.OffsetTableOffset = uint64(p.writer.BytesWritten())

	for _, offset := range offtable {
		if err := p.writeSizedInt(offset, int(p.trailer.OffsetIntSize)); err != nil {
			return err
		}
	}

	return binary.Write(p.writer, binary.BigEndian, p.trailer)
}

func (p *bplistGenerator) writePlistValue(pval cfValue) error {
	if pval == nil {
		return nil
	}

	switch pval := pval.(type) {
	case *cfDictionary:
		return p.writeDictionaryTag(pval)
	case *cfArray:
		return p.writeArrayTag(pval.values)
	case cfString:
		return p.writeStringTag(string(pval))
	case *cfNumber:
		return p.writeIntTag(pval.signed, pval.value)
	case *cfReal:
		if pval.wide {
			return p.writeRealTag(pval.value, 64)
		} else {
			return p.writeRealTag(pval.value, 32)
		}
	case cfBoolean:
		return p.writeBoolTag(bool(pval))
	case cfData:
		return p.writeDataTag([]byte(pval))
	case cfDate:
		return p.writeDateTag(time.Time(pval))
	case cfUID:
		return p.writeUIDTag(UID(pval))
	default:
		return fmt.Errorf("unknown plist type %t", pval)
	}
}

// put writes b with a single Write call, exactly as binary.Write did for each
// value, but without going through reflection.
func (p *bplistGenerator) put(b []byte) error {
	_, err := p.writer.Write(b)
	return err
}

func (p *bplistGenerator) putByte(b uint8) error {
	p.scratch[0] = b
	return p.put(p.scratch[:1])
}

// putUint writes the low nbytes (1, 2, 4 or 8) of n in big-endian order.
func (p *bplistGenerator) putUint(n uint64, nbytes int) error {
	switch nbytes {
	case 1:
		p.scratch[0] = uint8(n)
	case 2:
		binary.BigEndian.PutUint16(p.scratch[:], uint16(n))
	case 4:
		binary.BigEndian.PutUint32(p.scratch[:], uint32(n))
	case 8:
		binary.BigEndian.PutUint64(p.scratch[:], n)
	default:
		return errors.New("illegal integer size")
	}
	return p.put(p.scratch[:nbytes])
}

func (p *bplistGenerator) writeSizedInt(n uint64, nbytes int) error {
	return p.putUint(n, nbytes)
}

func (p *bplistGenerator) writeBoolTag(v bool) error {
	tag := uint8(bpTagBoolFalse)
	if v {
		tag = bpTagBoolTrue
	}
	return p.putByte(tag)
}

func (p *bplistGenerator) writeIntTag(signed bool, n uint64) error {
	var tag uint8
	var nbytes int
	switch {
	case n <= uint64(0xff):
		nbytes = 1
		tag = bpTagInteger // | 0x0
	case n <= uint64(0xffff):
		nbytes = 2
		tag = bpTagInteger | 0x1
	case n <= uint64(0xffffffff):
		nbytes = 4
		tag = bpTagInteger | 0x2
	case n > uint64(0x7fffffffffffffff) && !signed:
		// 64-bit values are always *signed* in format 00.
		// Any unsigned value that doesn't intersect with the signed
		// range must be sign-extended and stored as a SInt128
		nbytes = 8
		tag = bpTagInteger | 0x4
	default:
		nbytes = 8
		tag = bpTagInteger | 0x3
	}

	if err := p.putByte(tag); err != nil {
		return err
	}
	if tag&0xF == 0x4 {
		// SInt128; in the absence of true 128-bit integers in Go,
		// we'll just fake the top half. We only got here because
		// we had an unsigned 64-bit int that didn't fit,
		// so sign extend it with zeroes.
		if err := p.putUint(0, 8); err != nil {
			return err
		}
	}
	return p.putUint(n, nbytes)
}

func (p *bplistGenerator) writeUIDTag(u UID) error {
	nbytes := bplistMinimumIntSize(uint64(u))
	tag := bpTagUID | uint8((nbytes - 1))

	if err := p.putByte(tag); err != nil {
		return err
	}
	return p.writeSizedInt(uint64(u), nbytes)
}

func (p *bplistGenerator) writeRealTag(n float64, bits int) error {
	if bits == 32 {
		if err := p.putByte(bpTagReal | 0x2); err != nil {
			return err
		}
		return p.putUint(uint64(math.Float32bits(float32(n))), 4)
	}
	if err := p.putByte(bpTagReal | 0x3); err != nil {
		return err
	}
	return p.putUint(math.Float64bits(n), 8)
}

func (p *bplistGenerator) writeDateTag(t time.Time) error {
	tag := uint8(bpTagDate) | 0x3
	var val float64
	if sec := t.Unix(); sec > math.MinInt64/int64(time.Second) && sec < math.MaxInt64/int64(time.Second) {
		val = float64(t.In(time.UTC).UnixNano()) / float64(time.Second)
	} else {
		// UnixNano is undefined outside 1678-2262; build the value from seconds
		val = float64(sec) + float64(t.Nanosecond())/float64(time.Second)
	}
	val -= 978307200 // Adjust to Apple Epoch

	if err := p.putByte(tag); err != nil {
		return err
	}
	return p.putUint(math.Float64bits(val), 8)
}

func (p *bplistGenerator) writeCountedTag(tag uint8, count uint64) error {
	marker := tag
	if count >= 0xF {
		marker |= 0xF
	} else {
		marker |= uint8(count)
	}

	if err := p.putByte(marker); err != nil {
		return err
	}

	if count >= 0xF {
		return p.writeIntTag(false, count)
	}
	return nil
}

func (p *bplistGenerator) writeDataTag(data []byte) error {
	if err := p.writeCountedTag(bpTagData, uint64(len(data))); err != nil {
		return err
	}
	return p.put(data)
}

func (p *bplistGenerator) writeStringTag(str string) error {
	for _, r := range str {
		if r > 0x7F {
			utf16Runes := utf16.Encode([]rune(str))
			if err := p.writeCountedTag(bpTagUTF16String, uint64(len(utf16Runes))); err != nil {
				return err
			}
			encoded := make([]byte, 2*len(utf16Runes))
			for i, unit := range utf16Runes {
				binary.BigEndian.PutUint16(encoded[2*i:], unit)
			}
			return p.put(encoded)
		}
	}

	if err := p.writeCountedTag(bpTagASCIIString, uint64(len(str))); err != nil {
		return err
	}
	return p.put([]byte(str))
}

func (p *bplistGenerator) writeDictionaryTag(dict *cfDictionary) error {
	// assumption: sorted already; flattenPlistValue did this.
	cnt := len(dict.keys)
	if err := p.writeCountedTag(bpTagDictionary, uint64(cnt)); err != nil {
		return err
	}
	vals := make([]uint64, cnt*2)
	for i, k := range dict.keys {
		// invariant: keys have already been "uniqued" (as PStrings)
		keyIdx, ok := p.strmap[k]
		if !ok {
			return errors.New("failed to find key " + k + " in object map during serialization")
		}
		vals[i] = keyIdx
	}

	for i, v := range dict.values {
		// invariant: values have already been "uniqued"
		objIdx, ok := p.indexForPlistValue(v)
		if !ok {
			return errors.New("failed to find value in object map during serialization")
		}
		vals[i+cnt] = objIdx
	}

	for _, v := range vals {
		if err := p.writeSizedInt(v, int(p.trailer.ObjectRefSize)); err != nil {
			return err
		}
	}
	return nil
}

func (p *bplistGenerator) writeArrayTag(arr []cfValue) error {
	if err := p.writeCountedTag(bpTagArray, uint64(len(arr))); err != nil {
		return err
	}
	for _, v := range arr {
		objIdx, ok := p.indexForPlistValue(v)
		if !ok {
			return errors.New("failed to find value in object map during serialization")
		}

		if err := p.writeSizedInt(objIdx, int(p.trailer.ObjectRefSize)); err != nil {
			return err
		}
	}
	return nil
}

func (p *bplistGenerator) Indent(i string) {
	// There's nothing to indent.
}

func newBplistGenerator(w io.Writer) *bplistGenerator {
	return &bplistGenerator{
		writer: &countedWriter{Writer: w},
	}
}
