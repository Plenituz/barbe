package fetcher

import (
	"barbe/core/version"
	"encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
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

func FetchFile(fileUrl string) ([]byte, error) {
	if strings.HasPrefix(fileUrl, "file://") {
		return fetchLocalFile(strings.TrimPrefix(fileUrl, "file://"))
	} else if strings.HasPrefix(fileUrl, "http://") || strings.HasPrefix(fileUrl, "https://") {
		return fetchRemoteFile(fileUrl)
	} else if strings.HasPrefix(fileUrl, "base64://") {
		return decodeBase64File(strings.TrimPrefix(fileUrl, "base64://"))
	} else {
		return fetchLocalFile(fileUrl)
	}
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
