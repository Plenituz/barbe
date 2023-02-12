package fetcher

import (
	"barbe/core/version"
	"encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type FileDescription struct {
	Name    string
	Content []byte
}

//Fetches urls and cache their contents, will eventually also handle auth
type Fetcher struct {
	mutex          *sync.RWMutex
	fileCache      map[string]FileDescription
	UrlTransformer func(string) string
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		mutex:     &sync.RWMutex{},
		fileCache: map[string]FileDescription{},
	}
}

func (fetcher *Fetcher) Fetch(url string) (FileDescription, error) {
	if fetcher.UrlTransformer != nil {
		url = fetcher.UrlTransformer(url)
	}
	fetcher.mutex.RLock()
	if cached, ok := fetcher.fileCache[url]; ok {
		fetcher.mutex.RUnlock()
		return cached, nil
	}
	fetcher.mutex.RUnlock()
	content, err := FetchFile(url)
	if err != nil {
		return FileDescription{}, errors.Wrap(err, "error fetching file at '"+url+"'")
	}
	file := FileDescription{
		Name:    url,
		Content: content,
	}
	fetcher.mutex.Lock()
	defer fetcher.mutex.Unlock()
	fetcher.fileCache[url] = file
	return file, nil
}

// anyfront/manifest.json:v0.2.1
var BarbeHubRegex = regexp.MustCompile(`^(?P<owner>[a-zA-Z0-9-_]+)/(?P<comp>[a-zA-Z0-9-_]+)\.(?P<ext>[a-zA-Z0-9.]+):?(?P<tag>[a-zA-Z0-9.]+)?$`)

const BarbeHubDomain = "hub.barbe.app"

func ExtractExtension(fileUrl string) string {
	_, _, ext1, _, errHudId := ParseBarbeHubIdentifier(fileUrl)
	_, _, ext2, _, errHudUrl := ParseBarbeHubUrl(fileUrl)
	switch {
	case errHudId == nil:
		return ext1
	case errHudUrl == nil:
		return ext2
	case strings.HasPrefix(fileUrl, "file://"),
		strings.HasPrefix(fileUrl, "http://"),
		strings.HasPrefix(fileUrl, "https://"):
		return path.Ext(fileUrl)
	case strings.HasPrefix(fileUrl, "base64://"):
		return ""
	}
	if _, err := os.Stat(fileUrl); !os.IsNotExist(err) {
		return path.Ext(fileUrl)
	}
	return ""
}

func FetchFile(fileUrl string) ([]byte, error) {
	if strings.HasPrefix(fileUrl, "file://") {
		return fetchLocalFile(strings.TrimPrefix(fileUrl, "file://"))
	}
	if strings.HasPrefix(fileUrl, "http://") || strings.HasPrefix(fileUrl, "https://") {
		return fetchRemoteFile(fileUrl)
	}
	if strings.HasPrefix(fileUrl, "base64://") {
		return decodeBase64File(strings.TrimPrefix(fileUrl, "base64://"))
	}
	if _, err := os.Stat(fileUrl); !os.IsNotExist(err) {
		return fetchLocalFile(fileUrl)
	}
	//try barbe hub
	owner, component, ext, tag, err := ParseBarbeHubIdentifier(fileUrl)
	if err != nil {
		return nil, errors.New("unknown file type")
	}
	barbeHubUrl := MakeBarbeHubUrl(owner, component, ext, tag)
	return fetchRemoteFile(barbeHubUrl)
}

func MakeBarbeHubUrl(owner string, component string, ext string, tag string) string {
	//https://hub.barbe.app/anyfront/anyfront.js:v0.2.1
	return fmt.Sprintf("https://%s/%s/%s%s:%s", BarbeHubDomain, owner, component, ext, tag)
}

func ParseBarbeHubUrl(fileUrl string) (owner string, component string, ext string, tag string, e error) {
	if !strings.HasPrefix(fileUrl, "https://"+BarbeHubDomain) {
		return "", "", "", "", errors.New("not a barbe hub url")
	}
	fileUrl = strings.TrimPrefix(fileUrl, "https://"+BarbeHubDomain+"/")
	return ParseBarbeHubIdentifier(fileUrl)
}

func ParseBarbeHubIdentifier(fileUrl string) (owner string, component string, ext string, tag string, e error) {
	matches := BarbeHubRegex.FindAllStringSubmatch(fileUrl, 1)
	if len(matches) == 0 {
		return "", "", "", "", errors.New("not a barbe hub identifier")
	}
	match := matches[0]
	owner = match[1]
	component = match[2]
	ext = "." + strings.ToLower(match[3])
	tag = match[4]
	if tag == "" {
		tag = "latest"
	}
	return
}

func fetchLocalFile(filePath string) ([]byte, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading local file at '"+filePath+"'")
	}
	return file, nil
}

func fetchRemoteFile(fileUrl string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, fileUrl, nil)
	if err != nil {
		return nil, err
	}

	agent := fmt.Sprintf("barbe/"+version.Version+" (%s; %s)", runtime.GOOS, runtime.GOARCH)
	req.Header.Set("User-Agent", agent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error fetching remote file at '%s', status '%s'", fileUrl, resp.Status)
	}
	return content, nil
}

func decodeBase64File(fileB64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(fileB64)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding base64 file")
	}
	return data, nil
}
