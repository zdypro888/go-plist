package plist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

//Dictionary Plist标准Map
type Dictionary map[string]interface{}

//Unmarshal 序列化
func (m Dictionary) Unmarshal(v interface{}) error {
	return m.unmarshal(map[string]interface{}(m), reflect.ValueOf(v))
}

func (m Dictionary) unmarshal(v interface{}, val reflect.Value) error {
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}
	switch pval := v.(type) {
	case string:
		if val.Kind() == reflect.String {
			val.SetString(pval)
		} else {
			return fmt.Errorf("not string field: %v", val.Type())
		}
	case int8:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not char field: %v", val.Type())
		}
	case int16:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not int16 field: %v", val.Type())
		}
	case int32:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not int32 field: %v", val.Type())
		}
	case int64:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not int64 field: %v", val.Type())
		}
	case uint8:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not byte field: %v", val.Type())
		}
	case uint16:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not uint16 field: %v", val.Type())
		}
	case uint32:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not uint32 field: %v", val.Type())
		}
	case uint64:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return fmt.Errorf("not uint64 field: %v", val.Type())
		}
	case float64:
		if val.Kind() == reflect.Float32 || val.Kind() == reflect.Float64 {
			val.SetFloat(pval)
		} else {
			return fmt.Errorf("not float field: %v", val.Type())
		}
	case bool:
		if val.Kind() == reflect.Bool {
			val.SetBool(pval)
		} else {
			return fmt.Errorf("not bool field: %v", val.Type())
		}
	case []byte:
		if val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
			val.SetBytes(pval)
		} else {
			return fmt.Errorf("not data field: %v", val.Type())
		}
	case UID:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			val.Set(reflect.ValueOf(pval))
		}
	case []interface{}:
		if val.Kind() == reflect.Slice {
			return m.unmarshalSlice(pval, val)
		}
		return fmt.Errorf("not slice field: %v", val.Type())
	case map[string]interface{}:
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
	default:
		return fmt.Errorf("not plist type: %v", reflect.TypeOf(v))
	}
	return nil
}
func (m Dictionary) unmarshalSlice(array []interface{}, val reflect.Value) error {
	new := reflect.MakeSlice(val.Type(), len(array), len(array))
	val.Set(new)
	for i, v := range array {
		if err := m.unmarshal(v, val.Index(i)); err != nil {
			return err
		}
	}
	return nil
}
func (m Dictionary) unmarshalStruct(dict map[string]interface{}, val reflect.Value) error {
	typ := val.Type()
	tinfo, err := GetTypeInfo(typ)
	if err != nil {
		return err
	}
	for _, finfo := range tinfo.Fields {
		if dval, ok := dict[finfo.Name]; ok {
			if err := m.unmarshal(dval, finfo.Value(val)); err != nil {
				return err
			}
		} else if !finfo.OmitEmpty {
			//return fmt.Errorf("field[%s] can not empty", finfo.name)
		}
	}
	return nil
}

//ConvertToJSON 转到json格式
func ConvertToJSON(data []byte) ([]byte, error) {
	objdict := make(Dictionary)
	decoder := NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(objdict); err != nil {
		return nil, err
	}
	return json.Marshal(objdict)
}
