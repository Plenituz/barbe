package wasm

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"testing/fstest"
	"time"
)

//this is from fstest/mapfs.go

// A mapFileInfo implements fs.FileInfo and fs.DirEntry for a given map file.
type mapFileInfo struct {
	name string
	f    *fstest.MapFile
}

func (i *mapFileInfo) Name() string               { return i.name }
func (i *mapFileInfo) Size() int64                { return int64(len(i.f.Data)) }
func (i *mapFileInfo) Mode() fs.FileMode          { return i.f.Mode }
func (i *mapFileInfo) Type() fs.FileMode          { return i.f.Mode.Type() }
func (i *mapFileInfo) ModTime() time.Time         { return i.f.ModTime }
func (i *mapFileInfo) IsDir() bool                { return i.f.Mode&fs.ModeDir != 0 }
func (i *mapFileInfo) Sys() any                   { return i.f.Sys }
func (i *mapFileInfo) Info() (fs.FileInfo, error) { return i, nil }

// An openMapFile is a regular (non-directory) fs.File open for reading.
type fakeFile struct {
	path string
	mapFileInfo
	offset int64
}

func (f *fakeFile) Stat() (fs.FileInfo, error) { return &f.mapFileInfo, nil }

func (f *fakeFile) Close() error { return nil }

func (f *fakeFile) Read(b []byte) (int, error) {
	if f.offset >= int64(len(f.f.Data)) {
		return 0, io.EOF
	}
	if f.offset < 0 {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *fakeFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		// offset += 0
	case 1:
		offset += f.offset
	case 2:
		offset += int64(len(f.f.Data))
	}
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "seek", Path: f.path, Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

func (f *fakeFile) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 || offset > int64(len(f.f.Data)) {
		return 0, &fs.PathError{Op: "read", Path: f.path, Err: fs.ErrInvalid}
	}
	n := copy(b, f.f.Data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// fs that returns all the files in the current directory + the files in the map that are in memory
type semiRealFs map[string]*fstest.MapFile

func (fakeFs semiRealFs) Open(name string) (fs.File, error) {
	if file, ok := fakeFs[name]; ok {
		return &fakeFile{name, mapFileInfo{path.Base(name), file}, 0}, nil
	}
	//http.Dir handles requests for "../.." more gracefully than os.DirFS
	return http.Dir(".").Open(name)
}
