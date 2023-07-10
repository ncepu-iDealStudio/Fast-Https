package files

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

type File struct {
	file   multipart.File
	header *multipart.FileHeader
	path   string
}

// Save function saves file to filesystem. dir is the dictionary that saves file.
// For example Save("resource", "test.md")
func (f *File) Save(dir string, path string) error {
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		dir := filepath.Join(filepath.Dir(filepath.Dir(wd)), dir)
		path = filepath.Join(dir, path)
	}

	b, _ := io.ReadAll(f.file)
	err := os.WriteFile(path, b, 0777)
	if err != nil {
		return err
	}
	return nil
}
