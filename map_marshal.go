package plist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Dictionary Plist标准Map
type Dictionary map[string]any

// Unmarshal 反序列化到目标值
func (m Dictionary) Unmarshal(v any) error {
	return m.unmarshal(map[string]any(m), reflect.ValueOf(v))
}

func (m Dictionary) unmarshal(v any, val reflect.Value) error {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}

	// 检查是否实现 Unmarshaler 接口
	if receiver, ok := m.implementsUnmarshaler(val); ok {
		return receiver.UnmarshalPlist(func(target any) error {
			return m.unmarshal(v, reflect.ValueOf(target))
		})
	}

	switch pval := v.(type) {
	case string:
		if val.Kind() != reflect.String {
			return fmt.Errorf("not string field: %v", val.Type())
		}
		val.SetString(pval)

	case int8:
		return m.setInt(val, int64(pval))
	case int16:
		return m.setInt(val, int64(pval))
	case int32:
		return m.setInt(val, int64(pval))
	case int64:
		return m.setInt(val, pval)
	case int:
		return m.setInt(val, int64(pval))

	case uint8:
		return m.setUint(val, uint64(pval))
	case uint16:
		return m.setUint(val, uint64(pval))
	case uint32:
		return m.setUint(val, uint64(pval))
	case uint64:
		return m.setUint(val, pval)
	case uint:
		return m.setUint(val, uint64(pval))

	case float32:
		if val.Kind() != reflect.Float32 && val.Kind() != reflect.Float64 {
			return fmt.Errorf("not float field: %v", val.Type())
		}
		val.SetFloat(float64(pval))
	case float64:
		if val.Kind() != reflect.Float32 && val.Kind() != reflect.Float64 {
			return fmt.Errorf("not float field: %v", val.Type())
		}
		val.SetFloat(pval)

	case bool:
		if val.Kind() != reflect.Bool {
			return fmt.Errorf("not bool field: %v", val.Type())
		}
		val.SetBool(pval)

	case []byte:
		if val.Kind() != reflect.Slice || val.Type().Elem().Kind() != reflect.Uint8 {
			return fmt.Errorf("not data field: %v", val.Type())
		}
		val.SetBytes(pval)

	case UID:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			val.Set(reflect.ValueOf(pval))
		}

	case []any:
		if val.Kind() != reflect.Slice {
			return fmt.Errorf("not slice field: %v", val.Type())
		}
		return m.unmarshalSlice(pval, val)

	case map[string]any:
		switch val.Kind() {
		case reflect.Map:
			val.Set(reflect.MakeMap(val.Type()))
			for mk, mv := range pval {
				val.SetMapIndex(reflect.ValueOf(mk), reflect.ValueOf(mv))
			}
		case reflect.Struct:
			return m.unmarshalStruct(pval, val)
		default:
			return fmt.Errorf("not map or struct field: %v", val.Type())
		}

	case nil:
		// nil 值不做处理

	default:
		return fmt.Errorf("not plist type: %v", reflect.TypeOf(v))
	}
	return nil
}

// implementsUnmarshaler 检查值是否实现 Unmarshaler 接口
func (m Dictionary) implementsUnmarshaler(val reflect.Value) (Unmarshaler, bool) {
	if val.CanInterface() {
		if u, ok := val.Interface().(Unmarshaler); ok {
			return u, true
		}
	}
	if val.CanAddr() {
		if pv := val.Addr(); pv.CanInterface() {
			if u, ok := pv.Interface().(Unmarshaler); ok {
				return u, true
			}
		}
	}
	return nil, false
}

func (m Dictionary) setInt(val reflect.Value, i int64) error {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		val.SetUint(uint64(i))
	default:
		return fmt.Errorf("not int field: %v", val.Type())
	}
	return nil
}

func (m Dictionary) setUint(val reflect.Value, u uint64) error {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val.SetInt(int64(u))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		val.SetUint(u)
	default:
		return fmt.Errorf("not uint field: %v", val.Type())
	}
	return nil
}

func (m Dictionary) unmarshalSlice(array []any, val reflect.Value) error {
	newSlice := reflect.MakeSlice(val.Type(), len(array), len(array))
	val.Set(newSlice)
	for i, v := range array {
		if err := m.unmarshal(v, val.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

func (m Dictionary) unmarshalStruct(dict map[string]any, val reflect.Value) error {
	tinfo, err := GetTypeInfo(val.Type())
	if err != nil {
		return err
	}
	for _, finfo := range tinfo.Fields {
		if dval, ok := dict[finfo.Name]; ok {
			if err := m.unmarshal(dval, finfo.Value(val)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ConvertToJSON 转到json格式
func ConvertToJSON(data []byte) ([]byte, error) {
	objdict := make(Dictionary)
	decoder := NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&objdict); err != nil {
		return nil, err
	}
	return json.Marshal(objdict)
}
