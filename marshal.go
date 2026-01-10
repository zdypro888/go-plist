package plist

import (
	"encoding"
	"reflect"
	"time"
)

// IsEmptyValue 检查值是否为空（用于 omitempty 标签）
// Go 1.20+ 使用 reflect.Value.IsZero() 优化
func IsEmptyValue(v reflect.Value) bool {
	return v.IsZero()
}

var (
	// Go 1.22+ 使用 reflect.TypeFor 简化类型获取
	plistMarshalerType = reflect.TypeFor[Marshaler]()
	textMarshalerType  = reflect.TypeFor[encoding.TextMarshaler]()
	timeType           = reflect.TypeFor[time.Time]()
)

func implementsInterface(val reflect.Value, interfaceType reflect.Type) (any, bool) {
	if val.CanInterface() {
		itf := val.Interface()
		if itf != nil && reflect.TypeOf(itf).Implements(interfaceType) {
			return itf, true
		}
	}

	if val.CanAddr() {
		if pv := val.Addr(); pv.CanInterface() {
			itf := pv.Interface()
			if itf != nil && reflect.TypeOf(itf).Implements(interfaceType) {
				return itf, true
			}
		}
	}
	return nil, false
}

func (p *Encoder) marshalPlistInterface(marshalable Marshaler) cfValue {
	value, err := marshalable.MarshalPlist()
	if err != nil {
		panic(err)
	}
	return p.marshal(reflect.ValueOf(value))
}

// marshalTextInterface marshals a TextMarshaler to a plist string.
func (p *Encoder) marshalTextInterface(marshalable encoding.TextMarshaler) cfValue {
	s, err := marshalable.MarshalText()
	if err != nil {
		panic(err)
	}
	return cfString(s)
}

// marshalStruct 将结构体序列化为 plist dictionary
func (p *Encoder) marshalStruct(val reflect.Value) cfValue {
	tinfo, _ := GetTypeInfo(val.Type())
	dict := &cfDictionary{
		keys:   make([]string, 0, len(tinfo.Fields)),
		values: make([]cfValue, 0, len(tinfo.Fields)),
	}
	for _, finfo := range tinfo.Fields {
		value := finfo.Value(val)
		if !value.IsValid() || (finfo.OmitEmpty && IsEmptyValue(value)) {
			continue
		}
		dict.keys = append(dict.keys, finfo.Name)
		dict.values = append(dict.values, p.marshal(value))
	}
	return dict
}

func (p *Encoder) marshal(val reflect.Value) cfValue {
	if !val.IsValid() {
		return nil
	}
	// interface, map, pointer, or slice
	// Descend into pointers or interfaces
	if val.Kind() == reflect.Pointer || (val.Kind() == reflect.Interface && val.NumMethod() == 0) {
		valelem := val.Elem()
		if !valelem.IsValid() {
			typelem := val.Type().Elem()
			if typelem.Kind() == reflect.Struct {
				return &cfDictionary{}
			}
		}
		return p.marshal(valelem)
	}
	typ := val.Type()
	// time.Time implements TextMarshaler, but we need to store it in RFC3339
	if typ == timeType {
		time := val.Interface().(time.Time)
		return cfDate(time)
	}
	if receiver, can := implementsInterface(val, plistMarshalerType); can {
		return p.marshalPlistInterface(receiver.(Marshaler))
	}
	// Check for text marshaler.
	if receiver, can := implementsInterface(val, textMarshalerType); can {
		return p.marshalTextInterface(receiver.(encoding.TextMarshaler))
	}
	if typ == uidType {
		return cfUID(val.Uint())
	}
	if val.Kind() == reflect.Struct {
		return p.marshalStruct(val)
	}

	switch val.Kind() {
	case reflect.String:
		return cfString(val.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &cfNumber{signed: true, value: uint64(val.Int())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &cfNumber{signed: false, value: val.Uint()}
	case reflect.Float32:
		return &cfReal{wide: false, value: val.Float()}
	case reflect.Float64:
		return &cfReal{wide: true, value: val.Float()}
	case reflect.Bool:
		return cfBoolean(val.Bool())
	case reflect.Slice, reflect.Array:
		if typ.Elem().Kind() == reflect.Uint8 {
			bytes := []byte(nil)
			if val.CanAddr() && val.Kind() == reflect.Slice {
				// arrays are may be addressable but do not support .Bytes
				bytes = val.Bytes()
			} else {
				bytes = make([]byte, val.Len())
				reflect.Copy(reflect.ValueOf(bytes), val)
			}
			return cfData(bytes)
		} else {
			values := make([]cfValue, val.Len())
			for i, length := 0, val.Len(); i < length; i++ {
				if subpval := p.marshal(val.Index(i)); subpval != nil {
					values[i] = subpval
				}
			}
			return &cfArray{values}
		}
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			panic(&unknownTypeError{typ})
		}
		l := val.Len()
		dict := &cfDictionary{
			keys:   make([]string, 0, l),
			values: make([]cfValue, 0, l),
		}
		// 使用 MapRange 替代 MapKeys + MapIndex，减少内存分配
		iter := val.MapRange()
		for iter.Next() {
			if subpval := p.marshal(iter.Value()); subpval != nil {
				dict.keys = append(dict.keys, iter.Key().String())
				dict.values = append(dict.values, subpval)
			}
		}
		return dict
	default:
		panic(&unknownTypeError{typ})
	}
}
