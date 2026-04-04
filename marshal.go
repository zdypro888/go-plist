package plist

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
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
	// 直接检查类型是否实现接口，避免创建接口值
	if val.Type().Implements(interfaceType) {
		if val.CanInterface() {
			return val.Interface(), true
		}
	}
	// 检查指针类型是否实现接口
	if val.CanAddr() {
		pv := val.Addr()
		if pv.Type().Implements(interfaceType) && pv.CanInterface() {
			return pv.Interface(), true
		}
	}
	return nil, false
}

func (p *Encoder) marshalPlistInterface(marshalable Marshaler) (cfValue, error) {
	value, err := marshalable.MarshalPlist()
	if err != nil {
		return nil, err
	}
	return p.marshal(reflect.ValueOf(value))
}

// marshalTextInterface marshals a TextMarshaler to a plist string.
func (p *Encoder) marshalTextInterface(marshalable encoding.TextMarshaler) (cfValue, error) {
	s, err := marshalable.MarshalText()
	if err != nil {
		return nil, err
	}
	return cfString(s), nil
}

// marshalStruct 将结构体序列化为 plist dictionary
func (p *Encoder) marshalStruct(val reflect.Value) (cfValue, error) {
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
		cfv, err := p.marshal(value)
		if err != nil {
			return nil, err
		}
		dict.keys = append(dict.keys, finfo.Name)
		dict.values = append(dict.values, cfv)
	}
	return dict, nil
}

func (p *Encoder) marshal(val reflect.Value) (cfValue, error) {
	if !val.IsValid() {
		return nil, nil
	}
	// interface, map, pointer, or slice
	// Descend into pointers or interfaces
	if val.Kind() == reflect.Pointer || (val.Kind() == reflect.Interface && val.NumMethod() == 0) {
		valelem := val.Elem()
		if !valelem.IsValid() {
			// For nil interface{}, just return nil
			if val.Kind() == reflect.Interface {
				return nil, nil
			}
			// For nil pointer to struct, return empty dict
			typelem := val.Type().Elem()
			if typelem.Kind() == reflect.Struct {
				return &cfDictionary{}, nil
			}
			return nil, nil
		}
		return p.marshal(valelem)
	}
	typ := val.Type()
	// time.Time implements TextMarshaler, but we need to store it in RFC3339
	if typ == timeType {
		time := val.Interface().(time.Time)
		return cfDate(time), nil
	}
	if receiver, can := implementsInterface(val, plistMarshalerType); can {
		return p.marshalPlistInterface(receiver.(Marshaler))
	}
	// Check for text marshaler.
	if receiver, can := implementsInterface(val, textMarshalerType); can {
		return p.marshalTextInterface(receiver.(encoding.TextMarshaler))
	}
	if typ == uidType {
		return cfUID(val.Uint()), nil
	}
	if val.Kind() == reflect.Struct {
		return p.marshalStruct(val)
	}

	switch val.Kind() {
	case reflect.String:
		return cfString(val.String()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &cfNumber{signed: true, value: uint64(val.Int())}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &cfNumber{signed: false, value: val.Uint()}, nil
	case reflect.Float32:
		return &cfReal{wide: false, value: val.Float()}, nil
	case reflect.Float64:
		return &cfReal{wide: true, value: val.Float()}, nil
	case reflect.Bool:
		return cfBoolean(val.Bool()), nil
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
			return cfData(bytes), nil
		} else {
			values := make([]cfValue, val.Len())
			for i, length := 0, val.Len(); i < length; i++ {
				subpval, err := p.marshal(val.Index(i))
				if err != nil {
					return nil, err
				}
				if subpval != nil {
					values[i] = subpval
				}
			}
			return &cfArray{values}, nil
		}
	case reflect.Map:
		keyStr, err := marshalMapKeyFunc(typ.Key())
		if err != nil {
			return nil, err
		}
		l := val.Len()
		dict := &cfDictionary{
			keys:   make([]string, 0, l),
			values: make([]cfValue, 0, l),
		}
		iter := val.MapRange()
		for iter.Next() {
			k, err := keyStr(iter.Key())
			if err != nil {
				return nil, err
			}
			subpval, err := p.marshal(iter.Value())
			if err != nil {
				return nil, err
			}
			if subpval != nil {
				dict.keys = append(dict.keys, k)
				dict.values = append(dict.values, subpval)
			}
		}
		return dict, nil
	default:
		return nil, &unknownTypeError{typ}
	}
}

// marshalMapKeyFunc returns a function that converts a map key reflect.Value to a string.
func marshalMapKeyFunc(keyType reflect.Type) (func(reflect.Value) (string, error), error) {
	if keyType.Implements(textMarshalerType) {
		return func(v reflect.Value) (string, error) {
			b, err := v.Interface().(encoding.TextMarshaler).MarshalText()
			return string(b), err
		}, nil
	}
	if reflect.PointerTo(keyType).Implements(textMarshalerType) {
		return func(v reflect.Value) (string, error) {
			if v.CanAddr() {
				b, err := v.Addr().Interface().(encoding.TextMarshaler).MarshalText()
				return string(b), err
			}
			// 不可寻址时创建临时指针
			tmp := reflect.New(keyType)
			tmp.Elem().Set(v)
			b, err := tmp.Interface().(encoding.TextMarshaler).MarshalText()
			return string(b), err
		}, nil
	}
	switch keyType.Kind() {
	case reflect.String:
		return func(v reflect.Value) (string, error) { return v.String(), nil }, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(v reflect.Value) (string, error) { return strconv.FormatInt(v.Int(), 10), nil }, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(v reflect.Value) (string, error) { return strconv.FormatUint(v.Uint(), 10), nil }, nil
	case reflect.Float32:
		return func(v reflect.Value) (string, error) { return strconv.FormatFloat(v.Float(), 'g', -1, 32), nil }, nil
	case reflect.Float64:
		return func(v reflect.Value) (string, error) { return strconv.FormatFloat(v.Float(), 'g', -1, 64), nil }, nil
	default:
		return nil, fmt.Errorf("plist: unsupported map key type %v", keyType)
	}
}
