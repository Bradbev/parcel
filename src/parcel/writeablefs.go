package parcel

import (
	"os"
	"path/filepath"
)

type WritableFS interface {
	WriteFile(path string, data []byte) error
	//DeleteFile(path string) error
}

func SimpleWritableFS(path string) WritableFS {
	return &writeableFS{base: path}
}

type writeableFS struct {
	base string
}

func (w *writeableFS) WriteFile(path string, data []byte) error {
	return os.WriteFile(filepath.Join(w.base, path), data, 0666)
}
