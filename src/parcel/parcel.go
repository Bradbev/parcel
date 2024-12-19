package parcel

import "io/fs"

// PostLoader is an optional interface.  If a type implements
// PostLoader, then the PostLoad function will be called immediately
// after the object has been created.
type PostLoader interface {
	PostLoad()
}

// RegisterFS adds a new fs.FS to be used when loading files.
// Search priority is natrually ordered, ie an fs with priority 0
// will be searched before priority 1.  The first file that is found
// is read from.
func RegisterFS(fsys fs.FS, priority int) {
	d.RegisterFS(fsys, priority)
}

func RegisterWriteableFS(fsys WritableFS) {
	d.RegisterWriteableFS(fsys)
}

// AddType only accepts concrete types, not pointers.
func AddType[T any]() error {
	var t *T
	return d.AddType(t)
}

// AddFactoryForType only accepts concrete types, not pointers.
func AddFactoryForType[T any](create func() (any, error)) error {
	var t *T
	return d.AddFactoryForType(t, create)
}

func New[T any]() (*T, error) {
	var t *T
	obj, err := d.New(t)
	if err == nil && obj != nil {
		return obj.(*T), nil
	}
	return nil, err
}

func SetSavePath(T any, path string) error {
	return d.SetSavePath(T, path)
}

func Load[T any](path string) (*T, error) {
	var t *T
	loaded, err := d.Load(t, path)
	if err == nil && loaded != nil {
		return loaded.(*T), nil
	}
	return nil, err
}

func Save(T any) error {
	return d.Save(T)
}

func SetParent[T any](child *T, parent *T) error {
	return d.SetParent(child, parent)
}

func Delete(path string) error {
	return d.Delete(path)
}

var d *Parcel = NewParcel()

func GetDefault() *Parcel {
	return d
}

// Should not ever need this, except for testing
func SetDefault(p *Parcel) {
	d = p
}
