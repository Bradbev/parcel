package parcel

import (
	"cmp"
	"encoding/json"
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
	factories      map[reflect.Type]func() (any, error)
	objectFromPath map[string]any
	pathFromObject map[any]string
	loadableTypes  map[reflect.Type]reflect.Type
}

func NewParcel() *Parcel {
	return &Parcel{
		factories:      make(map[reflect.Type]func() (any, error)),
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
	Obj    any
}

type inlinedOrPath struct {
	Path string
	Obj  json.RawMessage
}

// Load takes a pointer to a type and a path.  A new object of type will be created,
// the on-disk meta format (a variation on diskSaveFormat) for T will be loaded and
// finally the newly created T will be returned.
func (p *Parcel) Load(T any, path string) (any, error) {
	path = normPath(path)
	if obj, exists := p.objectFromPath[path]; exists {
		return obj, nil
	}
	data, e2 := p.ReadFile(path)
	loadableType, e3 := p.getLoadableSaveFormatType(reflect.TypeOf(T))

	newObj, e1 := p.New(T)

	if err := errors.Join(e1, e2, e3); err != nil {
		return nil, err
	}

	loadableV := reflect.New(loadableType)
	err := json.Unmarshal(data, loadableV.Interface())
	if err != nil {
		return nil, err
	}

	err = p.fromLoadableType(loadableV.Elem().FieldByName("Obj").Interface(), newObj)
	return newObj, err
}

// loadFromBytes takes a pointer to T and bytes that represent the on-disk format for T.
// NOTE: the bytes must not be the full diskSaveFormat variant, they must be a type that
// makeLoadableType has returned
func (p *Parcel) loadFromBytes(T any, data []byte) (any, error) {
	newObj, e1 := p.New(T)
	loadableType, e2 := makeLoadableType(reflect.TypeOf(T))
	if err := errors.Join(e1, e2); err != nil {
		return nil, err
	}
	loadableV := reflect.New(loadableType)
	err := json.Unmarshal(data, loadableV.Interface())
	if err != nil {
		return nil, err
	}
	err = p.fromLoadableType(loadableV.Interface(), newObj)
	return newObj, err
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
			pathOrFull := map[string]any{}
			// replace the pointer with the asset path
			if path, exists := p.pathFromObject[tvalue.Interface()]; exists {
				pathOrFull["Path"] = path
			} else if !tvalue.IsNil() {
				inlinedSave, err := p.toSaveFormat(tvalue.Interface())
				if err != nil {
					return nil, err
				}
				pathOrFull["Obj"] = inlinedSave
			}
			out[tfield.Name] = pathOrFull
		}
	}

	return out, nil
}

func (p *Parcel) fromLoadableType(from any, T any) error {
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
			// pointers are replaced with the inlinedOrPath object
			iop, ok := val.(inlinedOrPath)
			if !ok {
				return fmt.Errorf("unable to convert %v into inlinedOrPath", val)
			}
			t := reflect.New(tfield.Type.Elem())
			if iop.Path != "" {
				val, _ = p.Load(t.Interface(), iop.Path)
			} else {
				var err error
				val, err = p.loadFromBytes(t.Interface(), iop.Obj)
				if err != nil {
					val = nil
				}
			}

		}
		if val != nil {
			tvalue.Set(reflect.ValueOf(val).Convert(tfield.Type))
		}
	}
	return nil
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
