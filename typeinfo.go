package plist

import (
	"reflect"
	"strings"
	"sync"
)

// TypeInfo holds details for the plist representation of a type.
type TypeInfo struct {
	Fields []FieldInfo
}

// FieldInfo holds details for the plist representation of a single field.
type FieldInfo struct {
	idx       []int
	Name      string
	OmitEmpty bool
}

var tinfoMap = &sync.Map{} //make(map[reflect.Type]*typeInfo)

// GetTypeInfo returns the typeInfo structure with details necessary
// for marshalling and unmarshalling typ.
func GetTypeInfo(typ reflect.Type) (*TypeInfo, error) {
	ltinfo, ok := tinfoMap.Load(typ)
	if ok {
		return ltinfo.(*TypeInfo), nil
	}
	tinfo := &TypeInfo{}
	if typ.Kind() == reflect.Struct {
		n := typ.NumField()
		for i := 0; i < n; i++ {
			f := typ.Field(i)
			if f.Tag.Get("plist") == "-" || (!f.Anonymous && f.PkgPath != "") {
				continue // Private field
			}

			// For embedded structs, embed its fields.
			if f.Anonymous {
				t := f.Type
				if t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
				if t.Kind() == reflect.Struct {
					inner, err := GetTypeInfo(t)
					if err != nil {
						return nil, err
					}
					for _, finfo := range inner.Fields {
						finfo.idx = append([]int{i}, finfo.idx...)
						if err := addFieldInfo(typ, tinfo, &finfo); err != nil {
							return nil, err
						}
					}
					continue
				}
			}

			finfo, err := structFieldInfo(typ, &f)
			if err != nil {
				return nil, err
			}

			// Add the field if it doesn't conflict with other fields.
			if err := addFieldInfo(typ, tinfo, finfo); err != nil {
				return nil, err
			}
		}
	}
	tinfoMap.Store(typ, tinfo)
	return tinfo, nil
}

// structFieldInfo builds and returns a fieldInfo for f.
func structFieldInfo(typ reflect.Type, f *reflect.StructField) (*FieldInfo, error) {
	finfo := &FieldInfo{idx: f.Index}
	// Split the tag from the xml namespace if necessary.
	tag := f.Tag.Get("plist")
	// Parse flags.
	tokens := strings.Split(tag, ",")
	tag = tokens[0]
	if len(tokens) > 1 {
		for _, flag := range tokens[1:] {
			switch flag {
			case "omitempty":
				finfo.OmitEmpty = true
			}
		}
	}
	if tag == "" {
		// If the name part of the tag is completely empty,
		// use the field name
		finfo.Name = f.Name
		return finfo, nil
	}
	finfo.Name = tag
	return finfo, nil
}

// addFieldInfo adds finfo to tinfo.fields if there are no
// conflicts, or if conflicts arise from previous fields that were
// obtained from deeper embedded structures than finfo. In the latter
// case, the conflicting entries are dropped.
// A conflict occurs when the path (parent + name) to a field is
// itself a prefix of another path, or when two paths match exactly.
// It is okay for field paths to share a common, shorter prefix.
func addFieldInfo(typ reflect.Type, tinfo *TypeInfo, newf *FieldInfo) error {
	var conflicts []int
	// First, figure all conflicts. Most working code will have none.
	for i := range tinfo.Fields {
		oldf := &tinfo.Fields[i]
		if newf.Name == oldf.Name {
			conflicts = append(conflicts, i)
		}
	}

	// Without conflicts, add the new field and return.
	if conflicts == nil {
		tinfo.Fields = append(tinfo.Fields, *newf)
		return nil
	}

	// If any conflict is shallower, ignore the new field.
	// This matches the Go field resolution on embedding.
	for _, i := range conflicts {
		if len(tinfo.Fields[i].idx) < len(newf.idx) {
			return nil
		}
	}

	// Otherwise, the new field is shallower, and thus takes precedence,
	// so drop the conflicting fields from tinfo and append the new one.
	for c := len(conflicts) - 1; c >= 0; c-- {
		i := conflicts[c]
		copy(tinfo.Fields[i:], tinfo.Fields[i+1:])
		tinfo.Fields = tinfo.Fields[:len(tinfo.Fields)-1]
	}
	tinfo.Fields = append(tinfo.Fields, *newf)
	return nil
}

// Value returns v's field value corresponding to finfo.
// It's equivalent to v.FieldByIndex(finfo.idx), but initializes
// and dereferences pointers as necessary.
func (finfo *FieldInfo) Value(v reflect.Value) reflect.Value {
	for i, x := range finfo.idx {
		if i > 0 {
			t := v.Type()
			if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
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
