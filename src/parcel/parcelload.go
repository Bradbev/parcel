package parcel

import (
	"fmt"
	"reflect"
)

func makeLoadableMetadataForType(ptyp reflect.Type) (reflect.Type, error) {
	innerTyp, err := makeLoadableType(ptyp)
	if err != nil {
		return nil, err
	}
	fields := reflect.VisibleFields(reflect.TypeFor[diskSaveFormat]())
	fields[len(fields)-1].Type = innerTyp
	return reflect.StructOf(fields), nil
}

// makeLoadableType accepts a reflect.TypeFor[*struct] and returns a created struct that
// is the same as ptyp, but with pointer members replaced with any.  This allows loading
// the new type from JSON without error, and then later replacing the saved asset path
// with a pointer to the loaded asset
func makeLoadableType(ptyp reflect.Type) (reflect.Type, error) {
	if !isPointer(ptyp) {
		return nil, fmt.Errorf("ptyp must be a pointer to a struct.  Is not a pointer")
	}
	typ := ptyp.Elem()
	if !isStruct(typ) {
		return nil, fmt.Errorf("ptyp must be a pointer to a struct.  Is not a struct")
	}
	fields := []reflect.StructField{}
	for _, field := range reflect.VisibleFields(typ) {
		if isPointer(field.Type) {
			f := field
			f.Type = reflect.TypeFor[any]()
			fields = append(fields, f)
		} else {
			fields = append(fields, field)
		}
	}
	return reflect.StructOf(fields), nil
}
