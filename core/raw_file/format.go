package raw_file

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type RawFileFormatter struct{}

func (t RawFileFormatter) Name() string {
	return "raw_file"
}

func (t RawFileFormatter) Format(ctx context.Context, data core.ConfigContainer) error {
	return crawl(ctx, data)
}

func (t RawFileFormatter) Transform(ctx context.Context, container core.ConfigContainer) (core.ConfigContainer, error) {
	return *core.NewConfigContainer(), crawl(ctx, container)
}

func crawl(ctx context.Context, data core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "raw_file" {
			continue
		}

		for name, group := range m {
			for i, databag := range group {
				err := applyRawFile(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying raw_file to '%s[%d]'", name, i)
				}
			}
		}
	}
	for resourceType, m := range data.DataBags {
		if resourceType != "raw_folder" {
			continue
		}

		for name, group := range m {
			for i, databag := range group {
				err := applyRawFolder(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying raw_folder to '%s[%d]'", name, i)
				}
			}
		}
	}
	return nil
}

func applyRawFile(ctx context.Context, databag core.DataBag) error {
	if databag.Value.Type != core.TokenTypeObjectConst {
		return errors.New("raw_file databag's syntax token must be of type object")
	}

	outputDir := ctx.Value("maker").(*core.Maker).OutputDir
	outputPath := ""
	content := ""
	for _, pair := range databag.Value.ObjectConst {
		switch pair.Key {
		case "path":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_file."+pair.Key+" as string")
			}
			outputPath = path.Join(outputDir, o)
		case "content":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_file."+pair.Key+" as string")
			}
			content = o
		case "content_from_path":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_file."+pair.Key+" as string")
			}
			contentBytes, err := os.ReadFile(o)
			if err != nil {
				return errors.Wrap(err, "error reading file at '"+o+"'")
			}
			content = string(contentBytes)
		}
	}
	if outputPath == "" {
		return errors.New("raw_file.path must be defined")
	}
	defer chown_util.TryRectifyRootFiles(ctx, []string{
		path.Dir(outputPath),
		outputPath,
	})

	err := os.MkdirAll(path.Dir(outputPath), 0755)
	if err != nil {
		return errors.Wrap(err, "error creating raw_file directory '"+path.Dir(outputPath)+"'")
	}

	err = os.WriteFile(outputPath, []byte(content), 0644)
	if err != nil {
		return errors.Wrap(err, "error writing file at '"+outputPath+"'")
	}
	return nil
}

func applyRawFolder(ctx context.Context, databag core.DataBag) error {
	if databag.Value.Type != core.TokenTypeObjectConst {
		return errors.New("raw_folder databag's syntax token must be of type object")
	}

	outputDir := ctx.Value("maker").(*core.Maker).OutputDir
	outputPath := ""
	folderToCopy := ""
	for _, pair := range databag.Value.ObjectConst {
		switch pair.Key {
		case "path":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_folder."+pair.Key+" as string")
			}
			outputPath = path.Join(outputDir, o)
		case "folder_to_copy":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_folder."+pair.Key+" as string")
			}
			folderToCopy = o
		}
	}
	if outputPath == "" {
		return errors.New("raw_folder.path must be defined")
	}
	if folderToCopy == "" {
		return errors.New("raw_folder.folder_to_copy must be defined")
	}
	defer chown_util.TryRectifyRootFiles(ctx, []string{
		path.Dir(outputPath),
		outputPath,
	})

	err := os.MkdirAll(outputPath, 0755)
	if err != nil {
		return errors.Wrap(err, "error creating raw_file directory '"+path.Dir(outputPath)+"'")
	}

	err = CopyFolder(folderToCopy, outputPath)
	if err != nil {
		return errors.Wrap(err, "error copying folder '"+folderToCopy+"' to '"+outputPath+"'")
	}
	return nil
}

func CopyFolder(src string, dest string) error {
	// Get properties of the source folder
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("unable to access source folder: %w", err)
	}

	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory")
	}

	// Create the destination folder
	err = os.MkdirAll(dest, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("unable to create destination folder: %w", err)
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return fmt.Errorf("unable to read source folder: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			// Recursively copy sub-folders
			if err := CopyFolder(srcPath, destPath); err != nil {
				return fmt.Errorf("error copying folder %s: %w", entry.Name(), err)
			}
		} else {
			// Copy files
			if err := copyFile(srcPath, destPath); err != nil {
				return fmt.Errorf("error copying file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

func copyFile(src string, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("unable to open source file: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("unable to create destination file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("error copying file contents: %w", err)
	}

	// Copy file permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("unable to retrieve file info: %w", err)
	}

	return os.Chmod(dest, srcInfo.Mode())
}
