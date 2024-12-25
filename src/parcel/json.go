package parcel

import (
	"encoding/base64"
	"reflect"

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
	case reflect.Pointer:
		p.jsonSaveWriter(w, v.Elem())
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
			propWriter := obj.Name(field.Name)
			err := p.jsonSaveWriter(propWriter, v.FieldByIndex(field.Index))
			if err != nil {
				return err
			}
		}
		obj.End()

	case reflect.Map:
		obj := w.Object()
		itr := v.MapRange()
		for itr.Next() {
			k := itr.Key().String()
			v := itr.Value()
			propWriter := obj.Name(k)
			err := p.jsonSaveWriter(propWriter, v)
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

func (p *Parcel) jsonLoad(T any, data []byte) error {
	r := jreader.NewReader(data)
	return p.jsonLoadReader(&r, reflect.ValueOf(T))
}

func (p *Parcel) jsonLoadReader(r *jreader.Reader, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Pointer:
		return p.jsonLoadReader(r, v.Elem())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(r.Int()))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(r.Int()))

	case reflect.Float32, reflect.Float64:
		v.SetFloat(r.Float64())

	case reflect.String:
		v.SetString(r.String())

	case reflect.Struct:
		for obj := r.Object(); obj.Next(); {
			name := string(obj.Name())
			fieldV := v.FieldByName(name)
			if fieldV != valueZero {
				err := p.jsonLoadReader(r, fieldV)
				if err != nil {
					return err
				}
			}
		}

	case reflect.Map:
		if v.IsNil() {
			// TODO - handle maps with ints & MarshalText keys
			m := reflect.MakeMap(v.Type())
			valType := v.Type().Elem()
			for obj := r.Object(); obj.Next(); {
				key := reflect.ValueOf(string(obj.Name()))
				v := reflect.New(valType)
				err := p.jsonLoadReader(r, v.Elem())
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
			err := p.jsonLoadReader(r, v)
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
