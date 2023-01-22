package gcp_token_provider

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
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
	chown_util.TryAdjustRootHomeDir(ctx)
	creds, err := GetCredentials(ctx, []string{
		"https://www.googleapis.com/auth/cloud-platform",
	}, false)
	if err != nil {
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

func GetCredentials(ctx context.Context, clientScopes []string, initialCredentialsOnly bool) (googleoauth.Credentials, error) {
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
		err := selfHostedGCloudAuth(ctx)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("Attempted to load application default credentials since neither `credentials` nor `access_token` was set in the provider block.  No credentials loaded. To use your gcloud credentials, run 'gcloud auth application-default login'.  Original error: %w", err)
		}
		return googleoauth.Credentials{}, fmt.Errorf("Attempted to load application default credentials since neither `credentials` nor `access_token` was set in the provider block.  No credentials loaded. To use your gcloud credentials, run 'gcloud auth application-default login'.  Original error: %w", err)
	}

	return googleoauth.Credentials{
		TokenSource: defaultTS,
	}, err
}

type serverHandler struct {
	f func(token string)
}

func (s *serverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.f(r.URL.Query().Get("code"))
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
	port := 8085

	qs := url.Values{}
	qs.Add("response_type", "code")
	qs.Add("client_id", "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com")
	//TODO find available port
	qs.Add("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	qs.Add("scope", strings.Join(scopes, " "))
	qs.Add("code_challenge", codeChallenge)
	qs.Add("code_challenge_method", "S256")
	qs.Add("access_type", "offline")

	wg := sync.WaitGroup{}
	wg.Add(1)
	var errSrv error
	var code string
	go func() {
		defer wg.Done()
		s := &http.Server{
			Addr: fmt.Sprintf(":%d", port),
		}
		s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code = r.URL.Query().Get("code")
			errSrv = s.Shutdown(ctx)
			if errSrv != nil {
				fmt.Println("Error shutting down server", errSrv)
			}
		})
		//http://localhost:8085/?state=state&code=4/0AWgavdcR-eimvZFlOip1bzrvYK8jo1bvMnMtCyKfbvdUfqWbPrvO9tNjAwxAk4PcD-0I0w&scope=email%20openid%20https://www.googleapis.com/auth/userinfo.email%20https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/sqlservice.login%20https://www.googleapis.com/auth/accounts.reauth&authuser=0&prompt=consent
		//s.Shutdown(ctx)

		fmt.Println("Starting server", s.Addr)
		//TODO add timeout
		errSrv = s.ListenAndServe()
		if errSrv != nil {
			if errSrv == http.ErrServerClosed {
				errSrv = nil
			} else {
				log.Ctx(ctx).Error().Err(errSrv).Msg("Failed to start webserver")
			}
		}
	}()

	uri := "https://accounts.google.com/o/oauth2/auth?" + qs.Encode()
	log.Ctx(ctx).Info().Msgf("Opening browser to %s", uri)
	err := browser.OpenURL(uri)
	if err != nil {
		return errors.Wrap(err, "failed to open browser")
	}
	wg.Wait()
	if errSrv != nil {
		return errors.Wrap(errSrv, "failed to start server")
	}
	if code == "" {
		return errors.New("no code received")
	}

	body := url.Values{}
	body.Add("code", code)
	body.Add("grant_type", "authorization_code")
	body.Add("client_id", "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com")
	body.Add("client_secret", "d-FL95Q19q7MQmFpd7hHD0Ty")
	body.Add("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	body.Add("code_verifier", codeVerifier)
	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(body.Encode()))
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
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response")
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	err = json.Unmarshal(b, &tokenResp)
	if err != nil {
		return errors.Wrap(err, "failed to parse response")

	}
	fmt.Println(tokenResp.AccessToken)

	return nil
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
