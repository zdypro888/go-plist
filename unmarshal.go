package plist

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

type incompatibleDecodeTypeError struct {
	dest reflect.Type
	src  string // type name (from cfValue)
}

func (u *incompatibleDecodeTypeError) Error() string {
	return fmt.Sprintf("plist: type mismatch: tried to decode plist type `%v' into value of type `%v'", u.src, u.dest)
}

var (
	// Go 1.22+ 使用 reflect.TypeFor 简化类型获取
	plistUnmarshalerType = reflect.TypeFor[Unmarshaler]()
	textUnmarshalerType  = reflect.TypeFor[encoding.TextUnmarshaler]()
	uidType              = reflect.TypeFor[UID]()
)

func isEmptyInterface(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.NumMethod() == 0
}

func (p *Decoder) unmarshalPlistInterface(pval cfValue, unmarshalable Unmarshaler) error {
	return unmarshalable.UnmarshalPlist(func(i any) error {
		return p.unmarshal(pval, reflect.ValueOf(i))
	})
}

func (p *Decoder) unmarshalTextInterface(pval cfString, unmarshalable encoding.TextUnmarshaler) error {
	return unmarshalable.UnmarshalText([]byte(pval))
}

func (p *Decoder) unmarshalTime(pval cfDate, val reflect.Value) {
	val.Set(reflect.ValueOf(time.Time(pval)))
}

func (p *Decoder) unmarshalLaxString(s string, val reflect.Value) error {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		val.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		val.SetUint(i)
		return nil
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		val.SetFloat(f)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		val.SetBool(b)
		return nil
	case reflect.Struct:
		if val.Type() == timeType {
			t, err := time.Parse(textPlistTimeLayout, s)
			if err != nil {
				return err
			}
			val.Set(reflect.ValueOf(t.In(time.UTC)))
			return nil
		}
		fallthrough
	default:
		return &incompatibleDecodeTypeError{val.Type(), "string"}
	}
}

func (p *Decoder) unmarshal(pval cfValue, val reflect.Value) error {
	if pval == nil {
		return nil
	}
	// Handle nil or invalid value - just parse without storing
	if !val.IsValid() {
		return nil
	}
	// 解引用所有指针层级
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}
	if isEmptyInterface(val) {
		v := p.valueInterface(pval)
		val.Set(reflect.ValueOf(v))
		return nil
	}
	incompatibleTypeError := &incompatibleDecodeTypeError{val.Type(), pval.typeName()}
	// time.Time implements TextMarshaler, but we need to parse it as RFC3339
	if date, ok := pval.(cfDate); ok {
		if val.Type() == timeType {
			p.unmarshalTime(date, val)
			return nil
		}
		return incompatibleTypeError
	}
	if receiver, can := implementsInterface(val, plistUnmarshalerType); can {
		return p.unmarshalPlistInterface(pval, receiver.(Unmarshaler))
	}
	if val.Type() != timeType {
		if receiver, can := implementsInterface(val, textUnmarshalerType); can {
			if str, ok := pval.(cfString); ok {
				return p.unmarshalTextInterface(str, receiver.(encoding.TextUnmarshaler))
			}
			return incompatibleTypeError
		}
	}
	typ := val.Type()
	switch pval := pval.(type) {
	case cfString:
		switch val.Kind() {
		case reflect.String:
			val.SetString(string(pval))
			return nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i, err := strconv.ParseInt(string(pval), 10, 64)
			if err != nil {
				return err
			}
			val.SetInt(i)
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			i, err := strconv.ParseUint(string(pval), 10, 64)
			if err != nil {
				return err
			}
			val.SetUint(i)
			return nil
		case reflect.Float32, reflect.Float64:
			f, err := strconv.ParseFloat(string(pval), 64)
			if err != nil {
				return err
			}
			val.SetFloat(f)
			return nil
		}
		if p.lax {
			return p.unmarshalLaxString(string(pval), val)
		}
		return incompatibleTypeError
	case *cfNumber:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval.value))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(pval.value)
		case reflect.Float32, reflect.Float64:
			val.SetFloat(float64(pval.value))
		case reflect.String:
			val.SetString(strconv.FormatUint(pval.value, 10))
		case reflect.Bool:
			val.SetBool(pval.value != 0)
		default:
			return incompatibleTypeError
		}
	case *cfReal:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval.value))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval.value))
		case reflect.Float32, reflect.Float64:
			val.SetFloat(pval.value)
		case reflect.String:
			val.SetString(strconv.FormatFloat(pval.value, 'g', -1, 64))
		case reflect.Bool:
			val.SetBool(pval.value != 0)
		default:
			return incompatibleTypeError
		}
	case cfBoolean:
		if val.Kind() == reflect.Bool {
			val.SetBool(bool(pval))
		} else {
			return incompatibleTypeError
		}
	case cfData:
		if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
			return incompatibleTypeError
		}

		if typ.Elem().Kind() != reflect.Uint8 {
			return incompatibleTypeError
		}

		b := []byte(pval)
		switch val.Kind() {
		case reflect.Slice:
			val.SetBytes(b)
		case reflect.Array:
			if val.Len() < len(b) {
				return fmt.Errorf("plist: attempted to unmarshal %d bytes into a byte array of size %d", len(b), val.Len())
			}
			sval := reflect.ValueOf(b)
			reflect.Copy(val, sval)
		}
	case cfUID:
		if val.Type() == uidType {
			val.SetUint(uint64(pval))
		} else {
			switch val.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				val.SetInt(int64(pval))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				val.SetUint(uint64(pval))
			default:
				return incompatibleTypeError
			}
		}
	case *cfArray:
		return p.unmarshalArray(pval, val)
	case *cfDictionary:
		return p.unmarshalDictionary(pval, val)
	}
	return nil
}

func (p *Decoder) unmarshalArray(a *cfArray, val reflect.Value) error {
	var n int
	switch val.Kind() {
	case reflect.Slice:
		// 切片元素值，增长切片
		cnt := len(a.values) + val.Len()
		if cnt >= val.Cap() {
			// Go 1.21+ 使用 max 函数
			ncap := max(2*cnt, 4)
			newSlice := reflect.MakeSlice(val.Type(), val.Len(), ncap)
			reflect.Copy(newSlice, val)
			val.Set(newSlice)
		}
		n = val.Len()
		val.SetLen(cnt)
	case reflect.Array:
		if len(a.values) > val.Cap() {
			return fmt.Errorf("plist: attempted to unmarshal %d values into an array of size %d", len(a.values), val.Cap())
		}
	default:
		return &incompatibleDecodeTypeError{val.Type(), a.typeName()}
	}

	// 递归读取元素到切片
	for _, sval := range a.values {
		if err := p.unmarshal(sval, val.Index(n)); err != nil {
			return err
		}
		n++
	}
	return nil
}

func (p *Decoder) unmarshalDictionary(dict *cfDictionary, val reflect.Value) error {
	typ := val.Type()
	switch val.Kind() {
	case reflect.Struct:
		tinfo, err := GetTypeInfo(typ)
		if err != nil {
			return err
		}

		entries := make(map[string]cfValue, len(dict.keys))
		for i, k := range dict.keys {
			sval := dict.values[i]
			entries[k] = sval
		}

		for _, finfo := range tinfo.Fields {
			if err := p.unmarshal(entries[finfo.Name], finfo.Value(val)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if val.IsNil() {
			val.Set(reflect.MakeMap(typ))
		}

		for i, k := range dict.keys {
			sval := dict.values[i]

			keyv := reflect.ValueOf(k).Convert(typ.Key())
			mapElem := reflect.New(typ.Elem()).Elem()

			if err := p.unmarshal(sval, mapElem); err != nil {
				return err
			}
			val.SetMapIndex(keyv, mapElem)
		}
	default:
		return &incompatibleDecodeTypeError{typ, dict.typeName()}
	}
	return nil
}

/* *Interface is modelled after encoding/json */
func (p *Decoder) valueInterface(pval cfValue) any {
	switch pval := pval.(type) {
	case cfString:
		return string(pval)
	case *cfNumber:
		if pval.signed {
			return int64(pval.value)
		}
		return pval.value
	case *cfReal:
		if pval.wide {
			return pval.value
		} else {
			return float32(pval.value)
		}
	case cfBoolean:
		return bool(pval)
	case *cfArray:
		return p.arrayInterface(pval)
	case *cfDictionary:
		return p.dictionaryInterface(pval)
	case cfData:
		return []byte(pval)
	case cfDate:
		return time.Time(pval)
	case cfUID:
		return UID(pval)
	}
	return nil
}

func (p *Decoder) arrayInterface(a *cfArray) []any {
	out := make([]any, len(a.values))
	for i, subv := range a.values {
		out[i] = p.valueInterface(subv)
	}
	return out
}

func (p *Decoder) dictionaryInterface(dict *cfDictionary) map[string]any {
	out := make(map[string]any)
	for i, k := range dict.keys {
		subv := dict.values[i]
		out[k] = p.valueInterface(subv)
	}
	return out
}
