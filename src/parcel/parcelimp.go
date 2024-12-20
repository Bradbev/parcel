package parcel

import (
	"cmp"
	"encoding/json"
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
	factories      map[reflect.Type]func() (any, error)
	objectFromPath map[string]any
	pathFromObject map[any]string
}

func NewParcel() *Parcel {
	return &Parcel{
		factories:      make(map[reflect.Type]func() (any, error)),
		objectFromPath: make(map[string]any),
		pathFromObject: make(map[any]string),
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
	p.factories[typ] = create
	return nil
}

func (p *Parcel) New(T any) (any, error) {
	typ := reflect.TypeOf(T)
	if fn, ok := p.factories[typ]; ok {
		obj, err := fn()
		if err == nil && obj != nil {
			if postloader, ok := obj.(PostLoader); ok {
				postloader.PostLoad()
			}
		}
		return obj, err
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
	Obj    map[string]any
}

type diskLoadFormat struct {
	Type   string
	Parent string
	Obj    json.RawMessage
}

func (p *Parcel) Load(T any, path string) (any, error) {
	path = normPath(path)
	if obj, exists := p.objectFromPath[path]; exists {
		return obj, nil
	}
	t, err := p.New(T)
	if err != nil {
		return nil, err
	}
	data, err := p.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var loaded diskLoadFormat
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		return nil, err
	}
	loadAbleType, err := makeLoadableType(reflect.TypeOf(T))
	loadAble := reflect.New(loadAbleType).Interface()
	err = json.Unmarshal(loaded.Obj, loadAble)
	if err != nil {
		return nil, err
	}
	err = p.fromLoadable(loadAble, t)
	p.objectFromPath[path] = t
	return t, nil
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
	saveFmt, err := p.toSaveFormat(T)
	if err != nil {
		return err
	}
	toSave := diskSaveFormat{
		Type: typeStr(reflect.TypeOf(T)),
		Obj:  saveFmt,
	}
	data, err := json.MarshalIndent(toSave, "", " ")
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

func (p *Parcel) toSaveFormat(T any) (map[string]any, error) {
	ptyp := reflect.TypeOf(T)
	if !isPointer(ptyp) {
		return nil, fmt.Errorf("toSaveFormat only accepts pointers")
	}
	typ := ptyp.Elem()
	if !isStruct(typ) {
		return nil, fmt.Errorf("toSaveFormat only accepts pointers to struct")
	}
	val := reflect.ValueOf(T).Elem()
	out := map[string]any{}

	for i := 0; i < typ.NumField(); i++ {
		tfield := typ.Field(i)
		if !tfield.IsExported() {
			continue
		}
		tvalue := val.Field(i)
		out[tfield.Name] = tvalue.Interface()
		if isPointer(tfield.Type) {
			// replace the pointer with the asset path
			if path, exists := p.pathFromObject[tvalue.Interface()]; exists {
				out[tfield.Name] = path
			}
		}
	}

	return out, nil
}

func (p *Parcel) fromLoadable(from any, T any) error {
	ptyp := reflect.TypeOf(T)
	if !isPointer(ptyp) {
		return fmt.Errorf("fromLoadable only accepts pointers")
	}
	typ := ptyp.Elem()
	if !isStruct(typ) {
		return fmt.Errorf("fromLoadable only accepts pointers to struct")
	}
	dest := reflect.ValueOf(T).Elem()
	fromV := reflect.ValueOf(from).Elem()

	for i := 0; i < typ.NumField(); i++ {
		tfield := typ.Field(i)
		if !tfield.IsExported() {
			continue
		}
		tvalue := dest.Field(i)
		val := fromV.FieldByName(tfield.Name).Interface()

		if isPointer(tfield.Type) {
			// when we expect a pointer but find a string, it means we need to load
			// the asset at the path
			if loadPath, ok := val.(string); ok {
				t := reflect.New(tfield.Type.Elem())
				val, _ = p.Load(t.Interface(), loadPath)
			}

		}
		if val != nil {
			tvalue.Set(reflect.ValueOf(val).Convert(tfield.Type))
		}
	}
	return nil
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
