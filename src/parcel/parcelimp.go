package parcel

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
)

type Parcel struct {
	fsys           []fsPriority
	writefs        WritableFS
	objectNewFunc  map[reflect.Type]func() (any, error)
	objectFromPath map[string]any
	pathFromObject map[any]string
	loadableTypes  map[reflect.Type]reflect.Type
}

func NewParcel() *Parcel {
	return &Parcel{
		objectNewFunc:  make(map[reflect.Type]func() (any, error)),
		objectFromPath: make(map[string]any),
		pathFromObject: make(map[any]string),
		loadableTypes:  make(map[reflect.Type]reflect.Type),
	}
}

func (p *Parcel) RegisterFS(fsys fs.FS, priority int) {
	p.fsys = append(p.fsys, fsPriority{fsys, priority})
	slices.SortStableFunc(p.fsys, func(a fsPriority, b fsPriority) int {
		return cmp.Compare(a.priority, b.priority)
	})
}

func (p *Parcel) RegisterWriteableFS(fsys WritableFS) {
	p.writefs = fsys
}

func (p *Parcel) AddType(T any) error {
	typ := reflect.TypeOf(T)
	return p.AddFactoryForType(T, func() (any, error) {
		val := reflect.New(typ.Elem())
		return val.Interface(), nil
	})
}

func (p *Parcel) AddFactoryForType(T any, create func() (any, error)) error {
	typ := reflect.TypeOf(T)
	if !isPointer(typ) {
		return fmt.Errorf("Type being registered must be a pointer")
	}
	if isPointer(typ.Elem()) {
		return fmt.Errorf("Type being registered must be a pointer that dereferences to a concrete type")
	}
	p.objectNewFunc[typ] = create
	return nil
}

func (p *Parcel) New(T any) (any, error) {
	typ := reflect.TypeOf(T)
	return p.newFromType(typ)
}

func (p *Parcel) newFromType(typ reflect.Type) (any, error) {
	if fn, ok := p.objectNewFunc[typ]; ok {
		newObj, err := fn()
		if err == nil {
			if postcreator, ok := newObj.(PostCreator); ok {
				postcreator.PostCreate()
			}
		}
		return newObj, err
	}
	return nil, fmt.Errorf("unknown asset type %s, make sure this type is added", typ.String())
}

func (p *Parcel) SetSavePath(T any, path string) error {
	if _, exists := p.objectFromPath[path]; exists {
		return fmt.Errorf("cannot SetSavePath at path '%s' because it already exists", path)
	}
	path = normPath(path)
	p.pathFromObject[T] = path
	return p.Save(T)
}

type diskSaveFormat struct {
	Type   string
	Parent string
	Obj    any
}

// Load takes a pointer to a type and a path.  A new object of type will be created,
// the on-disk meta format (a variation on diskSaveFormat) for T will be loaded and
// finally the newly created T will be returned.
func (p *Parcel) Load(T any, path string) (any, error) {
	path = normPath(path)
	if obj, exists := p.objectFromPath[path]; exists {
		return obj, nil
	}
	data, e1 := p.ReadFile(path)
	loadableType, e2 := p.getLoadableSaveFormatType(reflect.TypeOf(T))

	if err := errors.Join(e1, e2); err != nil {
		return nil, err
	}

	loadableV := reflect.New(loadableType)
	err := p.jsonLoad(loadableV.Interface(), data)
	if err != nil {
		return nil, err
	}

	newObj := loadableV.Elem().FieldByName("Obj").Interface()
	if postloader, ok := newObj.(PostLoader); ok {
		postloader.PostLoad()
	}
	return newObj, nil
}

func (p *Parcel) Save(T any) error {
	if p.writefs == nil {
		return fmt.Errorf("No WritableFS has been registered yet")
	}
	path, exists := p.pathFromObject[T]
	if !exists {
		return fmt.Errorf("object has no save path.  Call SetSavePath first")
	}
	p.objectFromPath[path] = T
	toSave := diskSaveFormat{
		Type: typeStr(reflect.TypeOf(T)),
		Obj:  T,
	}
	data, err := p.jsonSave(toSave)
	if err != nil {
		return err
	}
	return p.writefs.WriteFile(path, data)
}

func (p *Parcel) SetParent(child any, parent any) error {
	return nil
}

func (p *Parcel) Delete(path string) error {
	return nil
}

func (p *Parcel) ReadFile(path string) ([]byte, error) {
	for _, f := range p.fsys {
		s, err := f.fsys.Open(path)
		if err == nil && s != nil {
			return io.ReadAll(s)
		}
	}
	return nil, fmt.Errorf("unable to find filepath '%s' in any registered filesystem", path)
}

func (p *Parcel) getLoadableSaveFormatType(ptyp reflect.Type) (reflect.Type, error) {
	ret, ok := p.loadableTypes[ptyp]
	if !ok {
		var err error
		ret, err = makeLoadableSaveFormatForType(ptyp)
		if err != nil {
			return nil, err
		}
		p.loadableTypes[ptyp] = ret
	}
	return ret, nil
}

type fsPriority struct {
	fsys     fs.FS
	priority int
}

func isPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer
}

func isStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct
}

func typeStr(t reflect.Type) string {
	return t.String()
}

const fileExt = ".parcel"

func normPath(path string) string {
	if filepath.Ext(path) != fileExt {
		return path + fileExt
	}
	return path
}
