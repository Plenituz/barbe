package gcp_token_provider

import (
	"barbe/cli/logger"
	"barbe/core"
	"barbe/core/chown_util"
	"barbe/core/gcp_token_provider/browser"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	gcloudCliDefaultClientId     = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	gcloudCliDefaultClientSecret = "d-FL95Q19q7MQmFpd7hHD0Ty"
	googleAuthEndpoint           = "https://accounts.google.com/o/oauth2/auth"
	googleTokenEndpoint          = "https://oauth2.googleapis.com/token"
)

type GcpTokenProviderTransformer struct{}

func (t GcpTokenProviderTransformer) Name() string {
	return "gcp_token_provider"
}

func (t GcpTokenProviderTransformer) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	output := core.NewConfigContainer()
	for resourceType, m := range data.DataBags {
		if resourceType != "gcp_token_request" {
			continue
		}
		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				existing := data.GetDataBagGroup("gcp_token", databag.Name)
				if len(existing) > 0 {
					continue
				}
				newBag, err := populateGcpToken(ctx, databag)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error populating gcp token")
				}
				err = output.Insert(newBag)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error inserting aws credentials")
				}
			}
		}
	}
	return *output, nil
}

func populateGcpToken(ctx context.Context, dataBag core.DataBag) (core.DataBag, error) {
	optional := false
	if dataBag.Value.Type == core.TokenTypeObjectConst {
		optionalTokens := core.GetObjectKeyValues("optional", dataBag.Value.ObjectConst)
		if len(optionalTokens) == 1 {
			tmp, err := core.ExtractAsBool(optionalTokens[0])
			if err == nil {
				optional = tmp
			}
		}
	}

	chown_util.TryAdjustRootHomeDir(ctx)
	creds, err := GetCredentials(ctx, []string{
		"https://www.googleapis.com/auth/cloud-platform",
	}, false, optional)
	if err != nil {
		if optional {
			log.Debug().Msgf("error getting optional gcp credentials: %s", err.Error())
			return core.DataBag{}, nil
		}
		return core.DataBag{}, errors.Wrap(err, "error getting gcp credentials")
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return core.DataBag{}, errors.Wrap(err, "error getting gcp token")
	}

	bag := core.DataBag{
		Name:   dataBag.Name,
		Type:   "gcp_token",
		Labels: dataBag.Labels,
		Value: core.SyntaxToken{
			Type: core.TokenTypeObjectConst,
			ObjectConst: []core.ObjectConstItem{
				{
					Key: "access_token",
					Value: core.SyntaxToken{
						Type:  core.TokenTypeLiteralValue,
						Value: token.AccessToken,
					},
				},
				{
					Key: "refresh_token",
					Value: core.SyntaxToken{
						Type:  core.TokenTypeLiteralValue,
						Value: token.RefreshToken,
					},
				},
			},
		},
	}
	return bag, nil
}

type staticTokenSource struct {
	oauth2.TokenSource
}

func GetCredentials(ctx context.Context, clientScopes []string, initialCredentialsOnly bool, optional bool) (googleoauth.Credentials, error) {
	//TODO read terraform config it may have access_token, credentials or impersonate_service_account
	credentials := multiEnvSearch([]string{
		"GOOGLE_CREDENTIALS",
		"GOOGLE_CLOUD_KEYFILE_JSON",
		"GCLOUD_KEYFILE_JSON",
	})

	accessToken := multiEnvSearch([]string{
		"GOOGLE_OAUTH_ACCESS_TOKEN",
	})
	if accessToken != "" {
		contents, _, err := pathOrContents(accessToken)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("error loading access token: %s", err)
		}

		token := &oauth2.Token{AccessToken: contents}
		//if c.ImpersonateServiceAccount != "" && !initialCredentialsOnly {
		//	opts := []option.ClientOption{option.WithTokenSource(oauth2.StaticTokenSource(token)), option.ImpersonateCredentials(c.ImpersonateServiceAccount, c.ImpersonateServiceAccountDelegates...), option.WithScopes(clientScopes...)}
		//	creds, err := transport.Creds(context.TODO(), opts...)
		//	if err != nil {
		//		return googleoauth.Credentials{}, err
		//	}
		//	return *creds, nil
		//}

		return googleoauth.Credentials{
			TokenSource: staticTokenSource{oauth2.StaticTokenSource(token)},
		}, nil
	}

	if credentials != "" {
		contents, _, err := pathOrContents(credentials)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("error loading credentials: %s", err)
		}

		//if c.ImpersonateServiceAccount != "" && !initialCredentialsOnly {
		//	opts := []option.ClientOption{option.WithCredentialsJSON([]byte(contents)), option.ImpersonateCredentials(c.ImpersonateServiceAccount, c.ImpersonateServiceAccountDelegates...), option.WithScopes(clientScopes...)}
		//	creds, err := transport.Creds(context.TODO(), opts...)
		//	if err != nil {
		//		return googleoauth.Credentials{}, err
		//	}
		//	return *creds, nil
		//}

		creds, err := googleoauth.CredentialsFromJSON(ctx, []byte(contents), clientScopes...)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("unable to parse credentials from '%s': %s", contents, err)
		}
		return *creds, nil
	}

	//if c.ImpersonateServiceAccount != "" && !initialCredentialsOnly {
	//	opts := option.ImpersonateCredentials(c.ImpersonateServiceAccount, c.ImpersonateServiceAccountDelegates...)
	//	creds, err := transport.Creds(context.TODO(), opts, option.WithScopes(clientScopes...))
	//	if err != nil {
	//		return googleoauth.Credentials{}, err
	//	}
	//
	//	return *creds, nil
	//}

	defaultTS, err := googleoauth.DefaultTokenSource(context.Background(), clientScopes...)
	if err != nil {
		if optional {
			return googleoauth.Credentials{}, err
		}
		log.Ctx(ctx).Debug().Err(err).Msg("original error getting default token source")
		startFlow, err := logger.PromptUserYesNo(ctx, "Couldn't locate Google Cloud credentials. You can run 'gcloud auth application-default login' if you have it installed, or Barbe can start the browser authentication flow directly. Would you like to start the browser authentication flow?")
		if err != nil {
			return googleoauth.Credentials{}, errors.Wrap(err, "error prompting user")
		}
		if !startFlow {
			return googleoauth.Credentials{}, errors.New("no credentials found")
		}

		err = selfHostedGCloudAuth(ctx)
		if err != nil {
			return googleoauth.Credentials{}, errors.Wrap(err, "error starting self hosted gcloud auth")
		}

		defaultTS, err = googleoauth.DefaultTokenSource(context.Background(), clientScopes...)
		if err != nil {
			return googleoauth.Credentials{}, errors.Wrap(err, "error getting default token source after self hosted gcloud auth, try running 'gcloud auth application-default login' or providing one of the following environment variables: GOOGLE_CREDENTIALS, GOOGLE_CLOUD_KEYFILE_JSON, GCLOUD_KEYFILE_JSON, GOOGLE_OAUTH_ACCESS_TOKEN")
		}
	}

	return googleoauth.Credentials{
		TokenSource: defaultTS,
	}, nil
}

func selfHostedGCloudAuth(ctx context.Context) error {
	//this flow is documented in the python gcloud sdk
	codeVerifier := randSeq(128)
	codeHash := sha256.New()
	codeHash.Write([]byte(codeVerifier))
	unencodedChallenge := codeHash.Sum(nil)
	b64Challenge := base64.URLEncoding.EncodeToString(unencodedChallenge)
	codeChallenge := strings.TrimRight(b64Challenge, "=")

	scopes := []string{
		"openid",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/sqlservice.login",
		"https://www.googleapis.com/auth/accounts.reauth",
	}

	port := findAvailablePort(8085)
	qs := url.Values{}
	qs.Add("response_type", "code")
	qs.Add("client_id", gcloudCliDefaultClientId)
	//TODO find available port
	qs.Add("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	qs.Add("scope", strings.Join(scopes, " "))
	qs.Add("code_challenge", codeChallenge)
	qs.Add("code_challenge_method", "S256")
	qs.Add("access_type", "offline")
	uri := googleAuthEndpoint + "?" + qs.Encode()

	code, err := receiveAuthCallback(ctx, uri, port, time.Minute*10)
	if err != nil {
		return errors.Wrap(err, "error receiving waiting for google authentication callback")
	}

	body := url.Values{}
	body.Add("code", code)
	body.Add("grant_type", "authorization_code")
	body.Add("client_id", gcloudCliDefaultClientId)
	body.Add("client_secret", gcloudCliDefaultClientSecret)
	body.Add("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	body.Add("code_verifier", codeVerifier)
	req, err := http.NewRequest("POST", googleTokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.Errorf("failed to get token: %s", resp.Status)
	}
	var tokenResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tokenResp)
	if err != nil {
		return errors.Wrap(err, "failed to parse response")
	}
	if tokenResp.RefreshToken == "" {
		return errors.New("no refresh token in response")
	}

	appDefaultCredentials := map[string]string{
		"client_id":     gcloudCliDefaultClientId,
		"client_secret": gcloudCliDefaultClientSecret,
		"refresh_token": tokenResp.RefreshToken,
		"type":          "authorized_user",
	}
	credentialsFile, err := os.Create(gcloudWellKnownFile())
	if err != nil {
		return errors.Wrap(err, "failed to create credentials file")
	}
	defer credentialsFile.Close()
	err = json.NewEncoder(credentialsFile).Encode(appDefaultCredentials)
	if err != nil {
		return errors.Wrap(err, "failed to write credentials file")
	}
	return nil
}

func gcloudWellKnownFile() string {
	const f = "application_default_credentials.json"
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", f)
	}
	return filepath.Join(guessUnixHomeDir(), ".config", "gcloud", f)
}

func guessUnixHomeDir() string {
	// Prefer $HOME over user.Current due to glibc bug: golang.org/issue/13470
	if v := os.Getenv("HOME"); v != "" {
		return v
	}
	//TODO handle sudo user mode see chown.go

	// Else, fall back to user.Current:
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return ""
}

func receiveAuthCallback(ctx context.Context, uri string, port int, timeout time.Duration) (code string, e error) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	var errSrv error

	s := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
	}
	s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code = r.URL.Query().Get("code")
		w.Write([]byte("Authentication successful. You can close this window now."))
		go func() {
			errSrv = s.Shutdown(ctx)
			if errSrv != nil {
				errSrv = errors.Wrap(errSrv, "error to shutdown server")
				log.Ctx(ctx).Error().Err(errSrv).Msg("")
			}
		}()
	})

	go func() {
		select {
		case <-time.After(timeout):
			errSrv = errors.New("timed out waiting for auth code")
			err := s.Shutdown(ctx)
			if err != nil {
				s.Close()
			}
		}
	}()
	go func() {
		defer wg.Done()
		errSrv = s.ListenAndServe()
		if errSrv != nil {
			if errSrv == http.ErrServerClosed {
				errSrv = nil
			} else {
				errSrv = errors.Wrap(errSrv, "error starting webserver")
				log.Ctx(ctx).Error().Err(errSrv).Msg("")
			}
		}
	}()

	log.Ctx(ctx).Info().Msgf("Opening browser to %s", uri)
	err := openBrowser(uri)
	if err != nil {
		return "", errors.Wrap(err, "failed to open browser")
	}
	wg.Wait()
	if errSrv != nil {
		return "", errSrv
	}
	if code == "" {
		return "", errors.New("no code received")
	}
	return code, nil
}

func openBrowser(url string) error {
	return browser.OpenURL(url, editCmd)
}

func findAvailablePort(startFrom int) int {
	port := startFrom
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			break
		}
		port++
	}
	return port
}

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func multiEnvSearch(ks []string) string {
	for _, k := range ks {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// If the argument is a path, pathOrContents loads it and returns the contents,
// otherwise the argument is assumed to be the desired contents and is simply
// returned.
//
// The boolean second return value can be called `wasPath` - it indicates if a
// path was detected and a file loaded.
func pathOrContents(poc string) (string, bool, error) {
	if len(poc) == 0 {
		return poc, false, nil
	}

	path := poc
	if path[0] == '~' {
		var err error
		path, err = homedir.Expand(path)
		if err != nil {
			return path, true, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return string(contents), true, err
		}
		return string(contents), true, nil
	}

	return poc, false, nil
}
