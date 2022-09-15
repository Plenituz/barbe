package zipper_fmt

import (
	"archive/zip"
	"barbe/core/chown_util"
	"barbe/core/zipper_fmt/wildcard"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type fileMapEntry struct {
	PatternRgx *regexp.Regexp
	Pattern    string
	Template   string
}

func doTheZip(ctx context.Context, outputPath string, baseDir string, includePatterns []string, excludePatterns []string, fileMap map[string]string) error {
	for i, pattern := range includePatterns {
		includePatterns[i] = cleanupPattern(pattern)
	}
	for i, pattern := range excludePatterns {
		excludePatterns[i] = cleanupPattern(pattern)
	}
	cleanedMap := make([]fileMapEntry, 0, len(fileMap))
	for k, v := range fileMap {
		pattern := cleanupPattern(k)
		patternRgx, err := buildPatternRegex(pattern)
		if err != nil {
			return errors.Wrap(err, "failed to build pattern regex for '"+pattern+"'")
		}
		cleanedMap = append(cleanedMap, fileMapEntry{
			PatternRgx: patternRgx,
			Pattern:    cleanupPattern(k),
			Template:   v,
		})
	}
	sort.Slice(cleanedMap, func(i, j int) bool {
		// sort by "preciseness", the more stars (*) the less precise,
		// then the more slashes (/) the more precise.
		// that way we can match the most specific patterns first
		c1 := strings.Count(cleanedMap[i].Pattern, "*")
		c2 := strings.Count(cleanedMap[j].Pattern, "*")
		if c1 < c2 {
			return true
		}
		if c1 > c2 {
			return false
		}
		return strings.Count(cleanedMap[i].Pattern, "/") > strings.Count(cleanedMap[j].Pattern, "/")
	})

	toZip := make([]string, 0)
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		isDir := d.IsDir()
		fPath := cleanupPath(baseDir, path, isDir)
		if fPath == "/" {
			return nil
		}

		for _, pattern := range excludePatterns {
			if wildcard.MatchSimple(pattern, fPath) {
				return nil
			}
		}

		for _, pattern := range includePatterns {
			if wildcard.MatchSimple(pattern, fPath) {
				if !isDir {
					toZip = append(toZip, fPath)
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to walk directory '"+baseDir+"'")
	}

	err = os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create zip output directory '"+filepath.Dir(outputPath)+"'")
	}

	zipFile, err := os.Create(outputPath)
	if err != nil {
		return errors.Wrap(err, "failed to create zip file at '"+outputPath+"'")
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, file := range toZip {
		nameInZip := file
		for _, item := range cleanedMap {
			fPath := cleanupPath(baseDir, file, false)
			if !wildcard.MatchSimple(item.Pattern, fPath) {
				continue
			}
			nameInZip, err = mapFileName(item, fPath)
			if err != nil {
				return errors.Wrap(err, "failed to map file name '"+item.Pattern+"' to '"+fPath+"' with value '"+item.Template+"'")
			}
			log.Ctx(ctx).Debug().Msgf("mapped file name '%s' to '%s' according to '%s'", fPath, nameInZip, item.Pattern)
			break
		}
		log.Ctx(ctx).Debug().Msgf("adding file '%s' to zip as '%s'", file, nameInZip)
		err = addToZip(file, nameInZip, zipWriter)
		if err != nil {
			return errors.Wrap(err, "failed to add file '"+file+"' to zip")
		}
	}

	chown_util.TryRectifyRootFiles(ctx, []string{
		filepath.Dir(outputPath),
		outputPath,
	})

	return nil
}

func mapFileName(item fileMapEntry, value string) (string, error) {
	if !strings.Contains(item.Pattern, "*") {
		return item.Template, nil
	}
	/*
		pattern = "public/tmp/*"
		template = "public/*"
		value = "public/tmp/image.png"
		output = "public/image.png"
	*/

	matches := item.PatternRgx.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return "", errors.New("failed to find pattern '" + item.Pattern + "' in value '" + value + "'")
	}
	group := matches[0]
	if len(group) < 2 {
		return "", errors.New("failed to find pattern group '" + item.Pattern + "' in value '" + value + "'")
	}
	output := item.Template
	for i := 1; i < len(group); i++ {
		output = strings.ReplaceAll(output, fmt.Sprintf("*[%d]", i-1), group[i])
	}
	output = strings.ReplaceAll(output, "*", group[1])
	return output, nil
}

func buildPatternRegex(expr string) (*regexp.Regexp, error) {
	regexStr := ""
	for _, char := range expr {
		if char == '*' {
			regexStr += "(.*?)"
			continue
		}
		regexStr += regexp.QuoteMeta(string(char))
	}
	regexStr += "$"
	compiled, err := regexp.Compile(regexStr)
	if err != nil {
		return nil, errors.Wrap(err, "error compiling regex '"+regexStr+"'")
	}
	return compiled, nil
}

func cleanupPath(baseDir string, path string, isDir bool) string {
	fPath := filepath.ToSlash(path)
	fPath = strings.TrimPrefix(fPath, filepath.ToSlash(baseDir))
	fPath = strings.TrimPrefix(fPath, "/")
	if isDir {
		fPath = strings.TrimSuffix(fPath, "/") + "/"
	}
	return fPath
}

func cleanupPattern(pattern string) string {
	pattern = filepath.ToSlash(pattern)
	pattern = strings.TrimPrefix(pattern, "/")
	return pattern
}

func addToZip(filePath string, nameInZip string, zipWriter *zip.Writer) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to stat file '"+filePath+"'")
	}

	fileToZip, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "error opening file to zip at '"+filePath+"'")
	}
	defer fileToZip.Close()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return errors.Wrap(err, "error creating file info header for file to zip at '"+filePath+"'")
	}
	header.Name = nameInZip
	header.Method = zip.Deflate
	header.SetMode(info.Mode())
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "error creating header for file to zip at '"+filePath+"'")
	}

	_, err = io.Copy(writer, fileToZip)
	if err != nil {
		return errors.Wrap(err, "error copying file at '"+filePath+"' into zip")
	}
	return nil
}
