package aws_session_provider

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type AwsSessionProviderTransformer struct{}

func (t AwsSessionProviderTransformer) Name() string {
	return "aws_session_provider"
}

func (t AwsSessionProviderTransformer) Transform(ctx context.Context, data *core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "aws_credentials_request" {
			continue
		}
		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				err := populateAwsSession(ctx, data, *databag)
				if err != nil {
					return errors.Wrap(err, "error populating aws session")
				}
			}
		}
	}

	return nil
}

func populateAwsSession(ctx context.Context, data *core.ConfigContainer, dataBag core.DataBag) error {
	existing := data.GetDataBagGroup("aws_credentials", dataBag.Name)
	if len(existing) > 0 {
		return nil
	}

	var profile *string
	var region *string

	objConst := dataBag.Value.ObjectConst
	profileToken := core.GetObjectKeyValues("profile", objConst)
	if len(profileToken) > 0 {
		if len(profileToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple profile found on aws_session_provider, using the first one")
		}
		tmp, err := core.ExtractAsStringValue(profileToken[0])
		if err != nil {
			return errors.Wrap(err, "error extracting profile value as string on aws_session_provider")
		}
		profile = &tmp
	}

	regionToken := core.GetObjectKeyValues("region", objConst)
	if len(regionToken) > 0 {
		if len(regionToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple region found on aws_session_provider, using the first one")
		}
		tmp, err := core.ExtractAsStringValue(regionToken[0])
		if err != nil {
			return errors.Wrap(err, "error extracting region value as string on aws_session_provider")
		}
		region = &tmp
	}

	opts := session.Options{}
	config := aws.Config{}
	if profile != nil {
		opts.Profile = *profile
	}
	if region != nil {
		config.Region = region
	}
	opts.Config.MergeIn(&config)

	chown_util.TryAdjustRootHomeDir(ctx)
	sess, err := session.NewSessionWithOptions(opts)
	if err != nil {
		return errors.Wrap(err, "error creating aws session")
	}
	creds, err := sess.Config.Credentials.Get()
	if err != nil {
		return errors.Wrap(err, "error getting aws credentials")
	}

	bag := core.DataBag{
		Name:   dataBag.Name,
		Type:   "aws_credentials",
		Labels: dataBag.Labels,
		Value: core.SyntaxToken{
			Type: core.TokenTypeObjectConst,
			ObjectConst: []core.ObjectConstItem{
				{
					Key: "access_key_id",
					Value: core.SyntaxToken{
						Type:  core.TokenTypeLiteralValue,
						Value: creds.AccessKeyID,
					},
				},
				{
					Key: "secret_access_key",
					Value: core.SyntaxToken{
						Type:  core.TokenTypeLiteralValue,
						Value: creds.SecretAccessKey,
					},
				},
				{
					Key: "session_token",
					Value: core.SyntaxToken{
						Type:  core.TokenTypeLiteralValue,
						Value: creds.SessionToken,
					},
				},
			},
		},
	}

	err = data.Insert(bag)
	if err != nil {
		return errors.Wrap(err, "error inserting aws credentials")
	}
	return nil
}
