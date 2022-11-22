package gcp_token_provider

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	"io/ioutil"
	"os"
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
					return core.ConfigContainer{}, errors.Wrap(err, "error populating aws session")
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
		return googleoauth.Credentials{}, fmt.Errorf("Attempted to load application default credentials since neither `credentials` nor `access_token` was set in the provider block.  No credentials loaded. To use your gcloud credentials, run 'gcloud auth application-default login'.  Original error: %w", err)
	}

	return googleoauth.Credentials{
		TokenSource: defaultTS,
	}, err
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
