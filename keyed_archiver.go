package plist

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	uuid "github.com/satori/go.uuid"
)

const (
	secondsPerMinute       = 60
	secondsPerHour         = 60 * secondsPerMinute
	secondsPerDay          = 24 * secondsPerHour
	unixToCocoa      int64 = (31*365 + 31/4 + 1) * secondsPerDay
)

type archiverDate struct {
	Time  float64 `plist:"NS.time"`
	Class UID     `plist:"$class"`
}
type archiverData struct {
	Data  []byte `plist:"NS.data"`
	Class UID    `plist:"$class"`
}
type archiverUUID struct {
	Bytes []byte `plist:"NS.uuidbytes"`
	Class UID    `plist:"$class"`
}
type archiverArray struct {
	Objects []interface{} `plist:"NS.objects"`
	Class   UID           `plist:"$class"`
}
type archiverTable struct {
	Keys    []UID         `plist:"NS.keys"`
	Objects []interface{} `plist:"NS.objects"`
	Class   UID           `plist:"$class"`
}

var (
	archiverMutableDictionaryClass = &archiverClass{ClassName: "NSMutableDictionary", Classes: []string{"NSMutableDictionary", "NSDictionary", "NSObject"}}
	archiverDictionaryClass        = &archiverClass{ClassName: "NSDictionary", Classes: []string{"NSDictionary", "NSObject"}}
	archiverMutableArrayClass      = &archiverClass{ClassName: "NSMutableArray", Classes: []string{"NSMutableArray", "NSArray", "NSObject"}}
	archiverArrayClass             = &archiverClass{ClassName: "NSArray", Classes: []string{"NSArray", "NSObject"}}
	archiverMutableDataClass       = &archiverClass{ClassName: "NSMutableData", Classes: []string{"NSMutableData", "NSData", "NSObject"}}

	archiverDateType  = reflect.TypeOf((*time.Time)(nil)).Elem()
	archiverDateClass = &archiverClass{ClassName: "NSDate", Classes: []string{"NSDate", "NSObject"}}

	archiverUUIDType  = reflect.TypeOf((*uuid.UUID)(nil)).Elem()
	archiverUUIDClass = &archiverClass{ClassName: "NSUUID", Classes: []string{"NSUUID", "NSObject"}}

	archiverClasses = make(map[reflect.Type]*archiverClass)

	errArchiverNilElem = errors.New("nil item")
)

//ArchiverAddFoundation add archiver types class
func ArchiverAddFoundation(typ reflect.Type, name string, classes ...string) {
	archiverClasses[typ] = &archiverClass{ClassName: name, Classes: classes}
}

type archiverClass struct {
	ClassName string   `plist:"$classname"`
	Classes   []string `plist:"$classes"`
}

func (mcac *archiverClass) isDictionary() bool {
	return mcac.ClassName == "NSMutableDictionary" || mcac.ClassName == "NSDictionary"
}
func (mcac *archiverClass) isArray() bool {
	return mcac.ClassName == "NSMutableArray" || mcac.ClassName == "NSArray"
}
func (mcac *archiverClass) isData() bool {
	return mcac.ClassName == "NSMutableData" || mcac.ClassName == "NSData"
}
func (mcac *archiverClass) isUUID() bool {
	return mcac.ClassName == "NSUUID"
}
func (mcac *archiverClass) isDate() bool {
	return mcac.ClassName == "NSDate"
}

type archiverTop struct {
	Root UID `plist:"root"` //或着$0
}

//Archiver 序列化
type Archiver struct {
	Version  int           `plist:"$version"`
	Objects  []interface{} `plist:"$objects"`
	Archiver string        `plist:"$archiver"`
	Top      *archiverTop  `plist:"$top"`
}

//ReadFromZipData 从压缩数据读取
func (a *Archiver) ReadFromZipData(data []byte) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	mcaData, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}
	return a.ReadFromData(mcaData)
}

//ReadFromData 从数据读取
func (a *Archiver) ReadFromData(data []byte) error {
	return a.ReadFromReader(bytes.NewReader(data))
}

//ReadFromReader 从数据读取
func (a *Archiver) ReadFromReader(reader io.ReadSeeker) error {
	decoder := NewDecoder(reader)
	return decoder.Decode(a)
}
func (a *Archiver) getClass(dict map[string]interface{}) (*archiverClass, error) {
	class := &archiverClass{}
	dval := Dictionary(a.Objects[dict["$class"].(UID)].(map[string]interface{}))
	if err := dval.Unmarshal(class); err != nil {
		return nil, err
	}
	return class, nil
}

func (a *Archiver) addObject(obj interface{}) UID {
	for i, o := range a.Objects {
		if cmp.Equal(o, obj) {
			return UID(i)
		}
	}
	a.Objects = append(a.Objects, obj)
	return UID(len(a.Objects) - 1)
}

//Unmarshal 序列化
func (a *Archiver) Unmarshal(v interface{}) error {
	return a.unmarshal(a.Objects[a.Top.Root], reflect.ValueOf(v))
}
func (a *Archiver) unmarshal(v interface{}, val reflect.Value) error {
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
			return errors.New("not string field")
		}
	case uint64:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return errors.New("not int field")
		}
	case float64:
		if val.Kind() == reflect.Float32 || val.Kind() == reflect.Float64 {
			val.SetFloat(pval)
		} else {
			return errors.New("not float field")
		}
	case bool:
		if val.Kind() == reflect.Bool {
			val.SetBool(pval)
		} else {
			return errors.New("not bool field")
		}
	case []byte:
		if val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
			val.SetBytes(pval)
		} else {
			return errors.New("not data field")
		}
	case UID:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(int64(pval))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			val.SetUint(uint64(pval))
		default:
			return errors.New("not UID/int field")
		}
	case []interface{}:
		if val.Kind() == reflect.Slice {
			return a.unmarshalSlice(pval, val)
		}
		return errors.New("not slice field")
	case map[string]interface{}:
		class, err := a.getClass(pval)
		if err != nil {
			return err
		}
		switch val.Kind() {
		case reflect.Map:
			if !class.isDictionary() {
				return errors.New("not map field")
			}
			a.unmarshalMap(pval, val)
		case reflect.Array:
			if class.isUUID() && val.Type() == archiverUUIDType {
				uid, err := uuid.FromBytes(pval["NS.uuidbytes"].([]byte))
				if err != nil {
					return err
				}
				val.Set(reflect.ValueOf(uid))
				return nil
			}
			return fmt.Errorf("not uid type: %s", class.ClassName)
		case reflect.Slice:
			if class.isData() && val.Type().Elem().Kind() == reflect.Uint8 {
				return a.unmarshalData(pval, val)
			}
			if class.isArray() {
				return a.unmarshalArray(pval, val)
			}
			return fmt.Errorf("not data type: %s", class.ClassName)
		case reflect.Ptr:
			if val.IsNil() {
				val.Set(reflect.New(val.Type().Elem()))
			}
			val = val.Elem()
			fallthrough
		case reflect.Struct:
			if class.isDictionary() {
				return a.unmarshalStruct(pval, val)
			}
			if class.isDate() && val.Type() == archiverDateType {
				return a.unmarshalDate(pval, val)
			}
			if class.isUUID() && val.Type() == archiverUUIDType {
				return a.unmarshalUUID(pval, val)
			}
			return a.unmarshalNSType(pval, val)
		default:
			return fmt.Errorf("unknow dict type: %s(%v)", class.ClassName, val.Type())
		}
	case nil:
	default:
		return errors.New("type not assay")
	}
	return nil
}
func (a *Archiver) unmarshalMap(pval map[string]interface{}, val reflect.Value) {
	for k, v := range pval {
		val.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
	}
}
func (a *Archiver) unmarshalDate(pval map[string]interface{}, val reflect.Value) error {
	date := &archiverDate{}
	if err := Dictionary(pval).Unmarshal(date); err != nil {
		return err
	}
	vt := time.Unix(int64(date.Time)+unixToCocoa, 0)
	val.Set(reflect.ValueOf(vt))
	return nil
}
func (a *Archiver) unmarshalData(pval map[string]interface{}, val reflect.Value) error {
	data := &archiverData{}
	if err := Dictionary(pval).Unmarshal(data); err != nil {
		return err
	}
	val.SetBytes(data.Data)
	return nil
}
func (a *Archiver) unmarshalUUID(pval map[string]interface{}, val reflect.Value) error {
	uid := &archiverUUID{}
	if err := Dictionary(pval).Unmarshal(uid); err != nil {
		return err
	}
	val.SetBytes(uid.Bytes)
	return nil
}
func (a *Archiver) unmarshalSlice(array []interface{}, val reflect.Value) error {
	new := reflect.MakeSlice(val.Type(), len(array), len(array))
	val.Set(new)
	for i, v := range array {
		a.unmarshal(v, val.Index(i))
	}
	return nil
}
func (a *Archiver) unmarshalArray(dict map[string]interface{}, val reflect.Value) error {
	arr := &archiverArray{}
	if err := Dictionary(dict).Unmarshal(arr); err != nil {
		return err
	}
	for _, v := range arr.Objects {
		if vuid, ok := v.(UID); ok {
			item := reflect.New(val.Type().Elem())
			if err := a.unmarshal(a.Objects[vuid], item); err != nil {
				return err
			}
			val.Set(reflect.Append(val, item.Elem()))
		} else {
			val.Set(reflect.Append(val, reflect.ValueOf(v)))
		}
	}
	return nil
}
func (a *Archiver) unmarshalNSType(pval map[string]interface{}, val reflect.Value) error {
	tinfo, err := GetTypeInfo(val.Type())
	if err != nil {
		return err
	}
	for _, finfo := range tinfo.Fields {
		value := pval[finfo.Name]
		if uindex, ok := value.(UID); ok {
			value = a.Objects[uindex]
		}
		if err = a.unmarshal(value, finfo.Value(val)); err != nil {
			return err
		}
	}
	return nil
}
func (a *Archiver) unmarshalStruct(dict map[string]interface{}, val reflect.Value) error {
	typ := val.Type()
	tinfo, err := GetTypeInfo(typ)
	if err != nil {
		return err
	}
	tab := &archiverTable{}
	if err := Dictionary(dict).Unmarshal(tab); err != nil {
		return err
	}
	kvs := make(map[string]interface{})
	for i, keyI := range tab.Keys {
		key := a.Objects[keyI].(string)
		value := tab.Objects[i]
		if uid, ok := value.(UID); ok {
			kvs[key] = a.Objects[uid]
		} else {
			kvs[key] = value
		}
	}
	for _, finfo := range tinfo.Fields {
		if dval, ok := kvs[finfo.Name]; ok {
			if err := a.unmarshal(dval, finfo.Value(val)); err != nil {
				return err
			}
		} else if !finfo.OmitEmpty {
			//return fmt.Errorf("field[%s] can not empty", finfo.name)
		}
	}
	return nil
}

//Marshal 序列化
func (a *Archiver) Marshal(v interface{}) ([]byte, error) {
	a.Version = 100000
	a.Archiver = "NSKeyedArchiver"
	a.Objects = make([]interface{}, 0)
	a.addObject("$null")
	index, err := a.marshal(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	a.Top = &archiverTop{Root: index}
	buf := &bytes.Buffer{}
	encoder := NewEncoderForFormat(buf, BinaryFormat)
	if err = encoder.Encode(a); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func (a *Archiver) marshal(val reflect.Value) (UID, error) {
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return UID(0), errArchiverNilElem
		}
		val = val.Elem()
	}
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return a.addObject(val.Int()), nil
	case reflect.Int64:
		return a.addObject(val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return a.addObject(val.Uint()), nil
	case reflect.Uint64, reflect.Uintptr:
		return a.addObject(val.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return a.addObject(val.Float()), nil
	case reflect.Bool:
		return a.addObject(val.Bool()), nil
	case reflect.String:
		str := val.String()
		if str == "$null" {
			return UID(0), nil
		}
		return a.addObject(str), nil
	case reflect.Slice:
		return a.marshalSlice(val)
	case reflect.Array:
		if val.Type() == archiverUUIDType {
			uid := &archiverUUID{}
			uid.Bytes = val.Interface().(uuid.UUID).Bytes()
			uid.Class = a.addObject(archiverUUIDClass)
			return a.addObject(uid), nil
		}
		return 0, fmt.Errorf("unknow array: %v", val.Type())
	case reflect.Struct:
		if val.Type() == archiverDateType {
			date := &archiverDate{}
			date.Time = float64(val.Interface().(time.Time).Unix() - unixToCocoa)
			date.Class = a.addObject(archiverDateClass)
			return a.addObject(date), nil
		}
		return a.marshalStruct(val)
	}
	return UID(0), fmt.Errorf("unknow type: %v", val.Type())
}
func (a *Archiver) marshalSlice(val reflect.Value) (UID, error) {
	if val.Type().Elem().Kind() == reflect.Uint8 {
		data := &archiverData{}
		data.Data = val.Bytes()
		data.Class = a.addObject(archiverMutableDataClass)
		return a.addObject(data), nil
	}
	arr := &archiverArray{}
	for i := 0; i < val.Len(); i++ {
		valueIndex, err := a.marshal(val.Index(i))
		if err != nil {
			return 0, err
		}
		arr.Objects = append(arr.Objects, valueIndex)
	}
	arr.Class = a.addObject(archiverMutableArrayClass)
	return a.addObject(arr), nil
}
func (a *Archiver) marshalStruct(val reflect.Value) (UID, error) {
	typ := val.Type()
	tinfo, err := GetTypeInfo(typ)
	if err != nil {
		return 0, err
	}
	if class, ok := archiverClasses[typ]; ok {
		nsobj := make(map[string]interface{})
		for _, ti := range tinfo.Fields {
			valueIndex, err := a.marshal(ti.Value(val))
			if err != nil {
				return 0, err
			}
			nsobj[ti.Name] = valueIndex
		}
		nsobj["$class"] = a.addObject(class)
		return a.addObject(nsobj), nil
	}
	table := &archiverTable{}
	for _, ti := range tinfo.Fields {
		valueIndex, err := a.marshal(ti.Value(val))
		if err != nil {
			if err == errArchiverNilElem && ti.OmitEmpty {
				continue
			}
			return 0, err
		}
		table.Keys = append(table.Keys, a.addObject(ti.Name))
		table.Objects = append(table.Objects, valueIndex)
	}
	table.Class = a.addObject(archiverMutableDictionaryClass)
	return a.addObject(table), nil
}

//Unmarshal 序列化
func (a *Archiver) Print() string {
	return a.printObject(a.Objects[a.Top.Root])
}

func (a *Archiver) printObject(v interface{}) string {
	switch pval := v.(type) {
	case string:
		return fmt.Sprintf("string(%v)", pval)
	case int64:
		return fmt.Sprintf("int64(%v)", pval)
	case uint64:
		return fmt.Sprintf("uint64(%v)", pval)
	case float32:
		return fmt.Sprintf("float32(%v)", pval)
	case float64:
		return fmt.Sprintf("float64(%v)", pval)
	case bool:
		return fmt.Sprintf("bool(%v)", pval)
	case []byte:
		return fmt.Sprintf("[]byte(%x)", pval)
	case UID:
		return a.printObject(a.Objects[pval])
		// return fmt.Sprintf("UID(%v)", pval)
	case []interface{}:
		return a.printSlice(pval)
	case map[string]interface{}:
		class, err := a.getClass(pval)
		if err != nil {
			panic(err)
		}
		if class.isDate() {
			return a.printDate(pval)
		}
		if class.isData() {
			return a.printData(pval)
		}
		if class.isArray() {
			return a.printArray(pval)
		}
		if class.isUUID() {
			return a.printUUID(pval)
		}
		if class.isDictionary() {
			return a.printStruct(pval)
		}
		return a.printNSType(pval)
	default:
		return fmt.Sprintf("unknow : %v", pval)
	}
}
func (a *Archiver) printDate(pval map[string]interface{}) string {
	date := &archiverDate{}
	if err := Dictionary(pval).Unmarshal(date); err != nil {
		panic(err)
	}
	vt := time.Unix(int64(date.Time)+unixToCocoa, 0)
	return fmt.Sprintf("time(%v)", vt)
}
func (a *Archiver) printData(pval map[string]interface{}) string {
	data := &archiverData{}
	if err := Dictionary(pval).Unmarshal(data); err != nil {
		panic(err)
	}
	return fmt.Sprintf("[]byte(%x)", data.Data)
}
func (a *Archiver) printUUID(pval map[string]interface{}) string {
	uid := &archiverUUID{}
	if err := Dictionary(pval).Unmarshal(uid); err != nil {
		panic(err)
	}
	return fmt.Sprintf("UID(%x)", uid.Bytes)
}
func (a *Archiver) printSlice(array []interface{}) string {
	builder := &strings.Builder{}
	builder.WriteString("[]interface{\n")
	for i, v := range array {
		builder.WriteString(fmt.Sprintf("\t[%d]: %s\n", i, a.printObject(v)))
	}
	builder.WriteString("}")
	return builder.String()
}
func (a *Archiver) printArray(dict map[string]interface{}) string {
	arr := &archiverArray{}
	if err := Dictionary(dict).Unmarshal(arr); err != nil {
		panic(err)
	}
	builder := &strings.Builder{}
	builder.WriteString("[]array{\n")
	for i, v := range arr.Objects {
		builder.WriteString(fmt.Sprintf("\t[%d]: %s\n", i, a.printObject(v)))
	}
	builder.WriteString("}")
	return builder.String()
}
func (a *Archiver) printNSType(pval map[string]interface{}) string {
	builder := &strings.Builder{}
	builder.WriteString("NS{\n")
	for k, v := range pval {
		builder.WriteString(fmt.Sprintf("\t[%s]: %s\n", k, a.printObject(v)))
	}
	builder.WriteString("}")
	return builder.String()
}
func (a *Archiver) printStruct(dict map[string]interface{}) string {
	tab := &archiverTable{}
	if err := Dictionary(dict).Unmarshal(tab); err != nil {
		panic(err)
	}
	kvs := make(map[string]interface{})
	for i, keyI := range tab.Keys {
		key := a.Objects[keyI].(string)
		value := a.printObject(tab.Objects[i])
		kvs[key] = value
	}
	builder := &strings.Builder{}
	builder.WriteString("struct{\n")
	for k, v := range kvs {
		builder.WriteString(fmt.Sprintf("\t[%s]: %s\n", k, v))
	}
	builder.WriteString("}")
	return builder.String()
}
