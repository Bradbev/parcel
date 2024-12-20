package parcel_test

import (
	"os"
	"testing"

	"github.com/Bradbev/parcel/src/parcel"
	"github.com/stretchr/testify/assert"
)

type testType struct {
	String     string
	Uint64     uint64
	Float      float32
	OtherObj   *testType
	postloaded bool
}

func (t *testType) PostLoad() {
	t.postloaded = true
}

type setupOpts struct {
	NoEraseStore bool
}

func newDefault() *parcel.Parcel {
	p := parcel.NewParcel()
	parcel.SetDefault(p)
	return p
}

func setupBasic(p *parcel.Parcel, opts setupOpts) {
	if opts.NoEraseStore == false {
		os.RemoveAll("./testdata")
		os.Mkdir("./testdata", 0750)
	}
	p.RegisterFS(os.DirFS("./testdata"), 0)
	p.RegisterWriteableFS(parcel.SimpleWritableFS("./testdata"))
	p.AddType(&testType{})
}

func TestNew(t *testing.T) {
	createCount := 0
	e := parcel.AddFactoryForType[testType](func() (any, error) {
		createCount++
		return &testType{String: "Correct"}, nil
	})
	assert.NoError(t, e)

	obj, err := parcel.New[testType]()
	assert.NotNil(t, obj)
	assert.NoError(t, err)
	assert.Equal(t, 1, createCount)
	assert.Equal(t, "Correct", obj.String)
	assert.True(t, obj.postloaded)

	obj2, err := parcel.New[testType]()
	assert.False(t, obj == obj2, "Unique pointers are returned")
	assert.Equal(t, 2, createCount)
}

func TestSaveLoad(t *testing.T) {
	setupBasic(newDefault(), setupOpts{})

	path := "testsaveload"
	obj, _ := parcel.New[testType]()
	obj.String = "TestSaveLoad"
	parcel.SetSavePath(obj, path)

	err := parcel.Save(obj)
	assert.NoError(t, err)

	obj2, err := parcel.Load[testType](path)
	assert.NoError(t, err)

	assert.True(t, obj == obj2, "Pointer from load is same as saved")
}

func TestSaveLoadPersist(t *testing.T) {
	setupBasic(newDefault(), setupOpts{})

	path := "testsaveload"
	obj, _ := parcel.New[testType]()
	obj.String = "TestSaveLoad"
	obj.Float = 1.5
	obj.Uint64 = 0xFFFFFFFFFFFFFFFF
	parcel.SetSavePath(obj, path)

	err := parcel.Save(obj)
	assert.NoError(t, err)

	p := parcel.NewParcel()
	setupBasic(p, setupOpts{NoEraseStore: true})

	obj2, err := p.Load(&testType{}, path)
	assert.NoError(t, err)

	assert.True(t, obj != obj2, "Pointer from load is different from default parcel")
	assert.Equal(t, obj, obj2)
}

func TestLoadError(t *testing.T) {
	setupBasic(newDefault(), setupOpts{})
	obj, err := parcel.Load[testType]("")
	assert.Nil(t, obj)
	assert.Error(t, err)
}

func TestSaveLoadWithIndirectObject(t *testing.T) {
	setupBasic(newDefault(), setupOpts{})

	// Link 2 objects together and save them
	path := "testsaveloadmain"
	obj, _ := parcel.New[testType]()
	parcel.SetSavePath(obj, path)

	linkedPath := "testsaveloadlinked"
	linked, _ := parcel.New[testType]()
	parcel.SetSavePath(linked, linkedPath)
	obj.OtherObj = linked
	linked.String = "Linked Object"
	parcel.Save(linked)
	parcel.Save(obj)

	/////// Load the objects

	p := parcel.NewParcel()
	setupBasic(p, setupOpts{NoEraseStore: true})

	anyObj2, err := p.Load(&testType{}, path)
	assert.NoError(t, err)
	obj2 := anyObj2.(*testType)

	anyLinked2, err := p.Load(&testType{}, linkedPath)
	assert.NoError(t, err)
	linked2 := anyLinked2.(*testType)

	assert.Equal(t, obj2.OtherObj, linked2)
}

type basicTypes struct {
	Int32   int32
	Int64   int64
	Float32 float32
	Float64 float64
	Slice   []int
	Map     map[string]int
}

var basic = basicTypes{
	Int32:   0,
	Int64:   1,
	Float32: 2,
	Float64: 3,
	Slice:   []int{0, 1, 2},
	Map: map[string]int{
		"a": 0,
		"b": 1,
	},
}

func TestBasicTypes(t *testing.T) {
	setupBasic(newDefault(), setupOpts{})
	parcel.AddType[basicTypes]()

	path := "testbasictypes"
	obj, _ := parcel.New[basicTypes]()
	*obj = basic
	parcel.SetSavePath(obj, path)

	p := parcel.NewParcel()
	setupBasic(p, setupOpts{NoEraseStore: true})
	p.AddType(&basicTypes{})

	loadedA, err := p.Load(&basicTypes{}, path)
	assert.NoError(t, err)
	loaded := loadedA.(*basicTypes)
	assert.Equal(t, basic, *loaded)
}
