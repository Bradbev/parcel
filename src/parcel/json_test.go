package parcel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type basicTypes struct {
	Int32   int32
	Int64   int64
	Float32 float32
	Float64 float64
	Array   [5]byte
	Slice   []int
	Map     map[string]int
	//MapInt  map[int]int
	Bytes []byte
	Rune  rune
}

var basic = basicTypes{
	Int32:   0,
	Int64:   1,
	Float32: 2,
	Float64: 3,
	Array:   [5]byte{1, 2, 3, 4, 5},
	Slice:   []int{0, 1, 2},
	Map: map[string]int{
		"a": 0,
		"b": 1,
	},
	/*
		MapInt: map[int]int{
			1: 1, 2: 2,
		},
	*/
	Bytes: []byte("this is a test"),
	Rune:  'a',
}

func TestJsonWriteBasic(t *testing.T) {
	p := GetDefault()

	b, err := p.jsonSave(&basic)
	assert.NoError(t, err)

	os.WriteFile("./testdata/jsonwritebasic.json", b, 0666)

	back := basicTypes{}
	err = p.jsonLoad(&back, b)
	assert.NoError(t, err)

	assert.Equal(t, basic, back)
}
