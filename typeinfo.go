package plist

import (
	"reflect"
	"strings"
	"sync"
)

// TypeInfo 保存类型的 plist 表示细节
type TypeInfo struct {
	Fields []FieldInfo
}

// FieldInfo 保存单个字段的 plist 表示细节
type FieldInfo struct {
	idx       []int
	Name      string
	OmitEmpty bool
}

var tinfoMap = &sync.Map{}

// GetTypeInfo 返回用于序列化和反序列化的类型信息
func GetTypeInfo(typ reflect.Type) (*TypeInfo, error) {
	if ltinfo, ok := tinfoMap.Load(typ); ok {
		return ltinfo.(*TypeInfo), nil
	}

	tinfo := &TypeInfo{}
	if typ.Kind() == reflect.Struct {
		n := typ.NumField()
		for i := range n {
			f := typ.Field(i)
			if f.Tag.Get("plist") == "-" || (!f.Anonymous && f.PkgPath != "") {
				continue // 私有字段
			}
			// 处理嵌入结构体
			if f.Anonymous || f.Tag.Get("plist") == ",inline" {
				t := f.Type
				if t.Kind() == reflect.Pointer {
					t = t.Elem()
				}
				if t.Kind() == reflect.Struct {
					inner, err := GetTypeInfo(t)
					if err != nil {
						return nil, err
					}
					for _, finfo := range inner.Fields {
						finfo.idx = append([]int{i}, finfo.idx...)
						if err := addFieldInfo(tinfo, &finfo); err != nil {
							return nil, err
						}
					}
					continue
				}
			}

			finfo, err := structFieldInfo(&f)
			if err != nil {
				return nil, err
			}

			if err := addFieldInfo(tinfo, finfo); err != nil {
				return nil, err
			}
		}
	}
	tinfoMap.Store(typ, tinfo)
	return tinfo, nil
}

// structFieldInfo 构建并返回字段信息
func structFieldInfo(f *reflect.StructField) (*FieldInfo, error) {
	finfo := &FieldInfo{idx: f.Index}
	tag := f.Tag.Get("plist")

	// 解析标签
	tokens := strings.Split(tag, ",")
	tag = tokens[0]
	for _, flag := range tokens[1:] {
		if flag == "omitempty" {
			finfo.OmitEmpty = true
		}
	}

	if tag == "" {
		finfo.Name = f.Name
	} else {
		finfo.Name = tag
	}
	return finfo, nil
}

// addFieldInfo 添加字段信息，处理冲突
func addFieldInfo(tinfo *TypeInfo, newf *FieldInfo) error {
	var conflicts []int
	for i := range tinfo.Fields {
		if newf.Name == tinfo.Fields[i].Name {
			conflicts = append(conflicts, i)
		}
	}

	if len(conflicts) == 0 {
		tinfo.Fields = append(tinfo.Fields, *newf)
		return nil
	}

	// 如果存在更浅层的冲突，忽略新字段
	for _, i := range conflicts {
		if len(tinfo.Fields[i].idx) < len(newf.idx) {
			return nil
		}
	}

	// 新字段更浅层，移除冲突字段
	for c := len(conflicts) - 1; c >= 0; c-- {
		i := conflicts[c]
		tinfo.Fields = append(tinfo.Fields[:i], tinfo.Fields[i+1:]...)
	}
	tinfo.Fields = append(tinfo.Fields, *newf)
	return nil
}

// Value 返回字段对应的反射值
func (finfo *FieldInfo) Value(v reflect.Value) reflect.Value {
	for i, x := range finfo.idx {
		if i > 0 {
			t := v.Type()
			if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
		}
		v = v.Field(x)
	}
	return v
}
