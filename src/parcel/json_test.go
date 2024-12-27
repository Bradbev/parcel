package parcel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type basicTypes struct {
	/*
		Int32      int32
		Int64      int64
		Float32    float32
		Float64    float64
		Array      [5]byte
		Slice      []int
		Map        map[string]int
		MapInt     map[int]int
		Bytes      []byte
		Rune       rune
	*/
	InvalidMap map[invalidMapKey]int
}

type invalidMapKey struct {
	Name string
}

func (i invalidMapKey) MarshalText() ([]byte, error) {
	return []byte(i.Name), nil
}

func (i *invalidMapKey) UnmarshalText(text []byte) error {
	i.Name = string(text)
	return nil
}

var basic = basicTypes{
	/*
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
		MapInt: map[int]int{
			1: 1, 2: 2,
		},
		Bytes: []byte("this is a test"),
		Rune:  'a',
	*/
	InvalidMap: map[invalidMapKey]int{
		{"A"}: 0,
		{"B"}: 1,
	},
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

type basicWithPointer struct {
	Name    string
	Other   *basicWithPointer
	Unknown *unknownType
}

type unknownType struct {
	Unknown string
}

func TestJsonReplacePointerWithString(t *testing.T) {
	AddType[basicWithPointer]()
	main, _ := New[basicWithPointer]()
	linked, _ := New[basicWithPointer]()
	main.Name = "main"
	main.Other = linked
	linked.Name = "linked"
	linked.Other = &basicWithPointer{Name: "Not known", Unknown: &unknownType{Unknown: "unk"}}

	SetSavePath(main, "main")
	SetSavePath(linked, "linked")

	p := GetDefault()
	p.objectFromPath["linked.parcel"] = linked

	bmain, err := p.jsonSave(main)
	assert.NoError(t, err)
	os.WriteFile("./testdata/replacePointerWithStringMain.json", bmain, 0666)

	mainBack := basicWithPointer{}
	err = p.jsonLoad(&mainBack, bmain)
	assert.NoError(t, err)
	assert.Equal(t, main, &mainBack)

	blinked, err := p.jsonSave(linked)
	assert.NoError(t, err)
	os.WriteFile("./testdata/replacePointerWithStringLinked.json", blinked, 0666)

	otherBack := basicWithPointer{}
	err = p.jsonLoad(&otherBack, blinked)
	assert.NoError(t, err)
}
