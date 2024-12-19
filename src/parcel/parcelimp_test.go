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
	setupBasic(parcel.GetDefault(), setupOpts{})

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
	setupBasic(parcel.GetDefault(), setupOpts{})

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

func TestSaveLoadWithIndirectObject(t *testing.T) {
	setupBasic(parcel.GetDefault(), setupOpts{})

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

	p := parcel.NewParcel()
	setupBasic(p, setupOpts{NoEraseStore: true})

	anyObj2, err := p.Load(&testType{}, path)
	obj2 := anyObj2.(*testType)
	assert.NoError(t, err)

	l2, err := p.Load(&testType{}, linkedPath)
	linked2 := l2.(*testType)
	assert.NoError(t, err)

	assert.True(t, obj2.OtherObj == linked2)
}
