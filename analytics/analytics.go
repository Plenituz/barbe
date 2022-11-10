package analytics

import (
	"barbe/core/version"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// scaffolding here is from https://github.com/dagger/dagger/blob/main/analytics

type AnalyticsData struct {
	Events []AnalyticsEvent
}
type AnalyticsEvent struct {
	EventType       string
	DeviceId        string
	ExecutionId     string
	Time            int64
	CliVersion      string
	Platform        string
	OSName          string
	EventProperties map[string]interface{}
}

var (
	executionId  = uuid.NewString()
	eventQueue   = make(chan AnalyticsEvent)
	waitGroup    = &sync.WaitGroup{}
	stopConsumer = false
)

func QueueEvent(ctx context.Context, event AnalyticsEvent) {
	if analyticsDisabled() {
		return
	}

	event.Time = time.Now().Unix()
	event.ExecutionId = executionId
	event.DeviceId, _ = getDeviceID(gitRepoURL(ctx, "."))
	event.Platform = runtime.GOARCH
	event.OSName = runtime.GOOS
	event.CliVersion = version.Version
	if event.EventProperties == nil {
		event.EventProperties = make(map[string]interface{})
	}

	repo := gitRepoURL(ctx, ".")
	if repo != "" {
		event.EventProperties["GitRepository"] = repo
	}
	eventQueue <- event
}

func sendEvents(ctx context.Context, events AnalyticsData) {
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(events); err != nil {
		log.Ctx(ctx).Debug().Err(err).Msg("failed to encode analytics payload")
		return
	}

	analyticsURL := "https://barbe-analytics.maplecone.com/analytics"
	req, err := http.NewRequest("POST", analyticsURL, b)
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Msg("failed to prepare request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Ctx(ctx).Debug().Msg("failed to send analytics event")
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Ctx(ctx).Debug().Str("status", resp.Status).Msg("analytics request failed")
		return
	}
}

func StartConsumer(ctx context.Context) {
	if analyticsDisabled() {
		return
	}
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for !stopConsumer {
			event := <-eventQueue
			sendEvents(ctx, AnalyticsData{Events: []AnalyticsEvent{event}})
		}
	}()
}

func Flush() {
	if analyticsDisabled() {
		return
	}
	stopConsumer = true
	close(eventQueue)
	waitGroup.Wait()
}

func analyticsDisabled() bool {
	return os.Getenv("BARBE_DISABLE_ANALYTICS") != "" || os.Getenv("DO_NOT_TRACK") != ""
}

func isCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" || // TaskCluster, dsari
		os.Getenv("CODEBUILD_BUILD_ID") != "" // AWS CodeBuild
}

func getDeviceID(repo string) (string, error) {
	if isCI() {
		if repo == "" {
			return "", fmt.Errorf("unable to determine device ID")
		}
		return "ci-" + hash(repo), nil
	}

	idFilePath := "~/.config/barbe/cli_id"
	idFile, err := homedir.Expand(idFilePath)
	if err != nil {
		return "", err
	}
	id, err := os.ReadFile(idFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		if err := os.MkdirAll(filepath.Dir(idFile), 0755); err != nil {
			return "", err
		}

		id = []byte(uuid.NewString())
		if err := os.WriteFile(idFile, id, 0600); err != nil {
			return "", err
		}
	}
	return string(id), nil
}

// hash returns the sha256 digest of the string
func hash(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

// gitRepoURL returns the git repository remote, if any.
func gitRepoURL(ctx context.Context, path string) string {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return ""
	}

	origin, err := repo.Remote("origin")
	if err != nil {
		return ""
	}

	urls := origin.Config().URLs
	if len(urls) == 0 {
		return ""
	}

	endpoint, err := parseGitURL(urls[0])
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Str("url", urls[0]).Msg("failed to parse git URL")
		return ""
	}

	return endpoint
}
