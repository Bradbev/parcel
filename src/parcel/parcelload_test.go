package parcel

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type loadTypeTest struct {
	Int int
	Ptr *int
}

func TestMakeLoadableMetadataForType(t *testing.T) {
	typ, err := makeLoadableMetadataForType(reflect.TypeFor[*loadTypeTest]())
	assert.NoError(t, err)

	expected := []struct {
		Name string
		Type string
	}{
		{"Type", "string"},
		{"Parent", "string"},
		{"Obj", "struct { Int int; Ptr interface {} }"},
	}

	for i, field := range reflect.VisibleFields(typ) {
		assert.Equal(t, expected[i].Name, field.Name)
		assert.Equal(t, expected[i].Type, field.Type.String())
	}
}
