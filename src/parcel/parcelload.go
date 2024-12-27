package parcel

import (
	"reflect"
)

func makeLoadableSaveFormatForType(ptyp reflect.Type) (reflect.Type, error) {
	fields := reflect.VisibleFields(reflect.TypeFor[diskSaveFormat]())
	fields[len(fields)-1].Type = ptyp
	return reflect.StructOf(fields), nil
}
