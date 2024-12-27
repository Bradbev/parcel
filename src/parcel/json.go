package parcel

/*
This file implements a custom json reader/writer.
Exported fields are saved as normal.
Pointer fields are saved as either
1. A string to an object path if the pointer is to a known object OR
2. The normal save structure of the object

*/

import (
	"encoding"
	"encoding/base64"
	"reflect"
	"strconv"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-jsonstream/v3/jwriter"
)

var valueZero = reflect.Value{}

func (p *Parcel) jsonSave(T any) ([]byte, error) {
	w := jwriter.NewWriter()
	defer w.Flush()
	err := p.jsonSaveWriter(&w, reflect.ValueOf(T))
	return w.Bytes(), err
}

func (p *Parcel) jsonSaveWriter(w *jwriter.Writer, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Interface:
		p.jsonSaveWriter(w, v.Elem())

	case reflect.Pointer:
		p.jsonSaveWriter(w, v.Elem())

	case reflect.Bool:
		w.Bool(v.Bool())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.Int(int(v.Int()))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		w.Int(int(v.Uint()))

	case reflect.Float32, reflect.Float64:
		w.Float64(v.Float())

	case reflect.String:
		w.String(v.String())

	case reflect.Struct:
		obj := w.Object()
		for _, field := range reflect.VisibleFields(v.Type()) {
			if !field.IsExported() {
				continue
			}
			fv := v.FieldByIndex(field.Index)
			if fv.Kind() == reflect.Pointer {
				if fv.IsNil() { // don't bother to write nil ptrs
					continue
				}
				// if it's a pointer to a known object, write the path instead
				if path, ok := p.pathFromObject[fv.Interface()]; ok {
					fv = reflect.ValueOf(path)
				}
			}

			propWriter := obj.Name(field.Name)
			err := p.jsonSaveWriter(propWriter, fv)
			if err != nil {
				return err
			}
		}
		obj.End()

	case reflect.Map:
		obj := w.Object()
		itr := v.MapRange()
		for itr.Next() {
			k, err := resolveKeyName(itr.Key())
			if err != nil {
				return err
			}
			v := itr.Value()
			propWriter := obj.Name(k)
			err = p.jsonSaveWriter(propWriter, v)
			if err != nil {
				return err
			}
		}
		obj.End()

	case reflect.Slice, reflect.Array:
		// special case byte arrays
		if v.Type().Elem() == reflect.TypeFor[byte]() {
			w.String(base64.RawStdEncoding.EncodeToString(v.Bytes()))
			return nil
		}

		obj := w.Array()
		for i := 0; i < v.Len(); i++ {
			elemW := jwriter.NewWriter()
			err := p.jsonSaveWriter(&elemW, v.Index(i))
			if err != nil {
				return err
			}
			obj.Raw(elemW.Bytes())
		}
		obj.End()
	}
	return nil
}

type preader struct {
	r            *jreader.Reader
	lastAny      jreader.AnyValue
	anyWasCalled bool
}

func (p *Parcel) jsonLoad(T any, data []byte) error {
	r := jreader.NewReader(data)
	pr := &preader{
		r: &r,
	}
	return p.jsonLoadReader(pr, reflect.ValueOf(T))
}

func (p *Parcel) jsonLoadReader(pr *preader, v reflect.Value) error {
	r := pr.r
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			_, knownType := p.objectNewFunc[v.Type()]
			if knownType {
				pr.lastAny = r.Any()
				pr.anyWasCalled = true

				if pr.lastAny.Kind == jreader.StringValue {
					t := reflect.New(v.Type())
					loaded, err := p.Load(t.Elem().Interface(), pr.lastAny.String)
					v.Set(reflect.ValueOf(loaded))
					return err
				}
			}
			if o, err := p.newFromType(v.Type()); err == nil {
				v.Set(reflect.ValueOf(o))
			} else {
				v.Set(reflect.New(v.Type().Elem()))
			}
		}
		return p.jsonLoadReader(pr, v.Elem())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(r.Int()))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(r.Int()))

	case reflect.Float32, reflect.Float64:
		v.SetFloat(r.Float64())

	case reflect.String:
		v.SetString(r.String())

	case reflect.Struct:
		var obj jreader.ObjectState
		if pr.anyWasCalled {
			obj = pr.lastAny.Object
		} else {
			obj = r.Object()
		}
		pr.anyWasCalled = false
		for obj.Next() {
			name := string(obj.Name())
			fieldV := v.FieldByName(name)
			if fieldV != valueZero {
				err := p.jsonLoadReader(pr, fieldV)
				if err != nil {
					return err
				}
			}
		}

	case reflect.Map:
		if v.IsNil() {
			// TODO - handle maps with ints & MarshalText keys
			m := reflect.MakeMap(v.Type())
			keyLoader := makeKeyLoader(v.Type().Key())
			valType := v.Type().Elem()
			for obj := r.Object(); obj.Next(); {
				//key := reflect.ValueOf(string(obj.Name()))
				key, err := keyLoader(string(obj.Name()))
				if err != nil {
					return err
				}
				v := reflect.New(valType)
				err = p.jsonLoadReader(pr, v.Elem())
				if err != nil {
					return err
				}
				m.SetMapIndex(key, v.Elem())
			}
			v.Set(m)
		}

	case reflect.Slice, reflect.Array:
		elemTyp := v.Type().Elem()
		if elemTyp == reflect.TypeFor[byte]() {
			// special case byte strings
			bytes, err := base64.RawStdEncoding.DecodeString(r.String())
			if err != nil {
				return err
			}
			if v.Kind() == reflect.Array {
				// copy into the array
				for i := 0; i < min(len(bytes), v.Len()); i++ {
					v.Index(i).SetUint(uint64(bytes[i]))
				}
			} else {
				v.Set(reflect.ValueOf(bytes))
			}
			return nil
		}
		s := reflect.New(reflect.SliceOf(elemTyp)).Elem()
		for a := r.Array(); a.Next(); {
			v := reflect.New(elemTyp).Elem()
			err := p.jsonLoadReader(pr, v)
			if err != nil {
				return err
			}
			s = reflect.Append(s, v)
		}
		if v.Kind() == reflect.Array {
			// copy into the array
			for i := 0; i < v.Len(); i++ {
				v.Index(i).Set(s.Index(i))
			}
		} else {
			v.Set(s)
		}

	}
	return nil
}

func resolveKeyName(k reflect.Value) (string, error) {
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if tm, ok := k.Interface().(encoding.TextMarshaler); ok {
		if k.Kind() == reflect.Pointer && k.IsNil() {
			return "", nil
		}
		buf, err := tm.MarshalText()
		return string(buf), err
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	}
	panic("unexpected map key type")
}

func makeKeyLoader(typ reflect.Type) func(key string) (reflect.Value, error) {
	if typ.Kind() == reflect.String {
		return func(key string) (reflect.Value, error) {
			return reflect.ValueOf(key).Convert(typ), nil
		}
	}
	if reflect.PointerTo(typ).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		return func(key string) (reflect.Value, error) {
			v := reflect.New(typ)
			err := v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(key))
			return v.Elem(), err
		}
	}
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(key string) (reflect.Value, error) {
			n, err := strconv.ParseInt(key, 10, 64)
			return reflect.ValueOf(n).Convert(typ), err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(key string) (reflect.Value, error) {
			n, err := strconv.ParseUint(key, 10, 64)
			return reflect.ValueOf(n).Convert(typ), err
		}
	}
	panic("unexpected map key type to load")
}
