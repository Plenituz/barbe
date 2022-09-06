package s3_bucket_creator

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"barbe/core"
	"strings"
)

type S3BucketCreator struct{}

func (t S3BucketCreator) Name() string {
	return "s3_bucket_creator"
}

func (t S3BucketCreator) Format(ctx context.Context, data *core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "s3_bucket_creator" {
			continue
		}
		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				err := handleS3Block(databag.Value)
				if err != nil {
					return errors.Wrap(err, "error handling s3_state_store_creator.s3 block")
				}
			}
		}
	}
	return nil
}

var awsSession *session.Session

func getAwsSession(region string) (*session.Session, error) {
	if awsSession != nil {
		return awsSession, nil
	}
	sess, err := session.NewSession(&aws.Config{
		Region: &region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create aws session")
	}
	awsSession = sess
	return awsSession, nil
}

func handleS3Block(token core.SyntaxToken) error {
	bucketNameArr := core.GetObjectKeyValues("name", token.ObjectConst)
	regionArr := core.GetObjectKeyValues("region", token.ObjectConst)
	if len(bucketNameArr) == 0 {
		return nil
	}
	if len(regionArr) == 0 {
		return nil
	}

	bucketName, err := core.ExtractAsStringValue(bucketNameArr[0])
	if err != nil {
		return errors.Wrap(err, "couldn't extract 'name' attribute as string")
	}
	region, err := core.ExtractAsStringValue(regionArr[0])
	if err != nil {
		return errors.Wrap(err, "couldn't extract 'region' attribute as string")
	}

	sess, err := getAwsSession(region)
	if err != nil {
		return errors.Wrap(err, "couldn't get aws session")
	}
	s3Client := s3.New(sess)
	_, err = s3Client.HeadBucket(&s3.HeadBucketInput{
		Bucket: &bucketName,
	})
	notFound := err != nil && strings.Contains(err.Error(), "NotFound")
	if err != nil && !notFound {
		return errors.Wrap(err, "couldn't check if bucket exists")
	}
	if !notFound {
		return nil
	}

	fmt.Println(fmt.Sprintf("This template wants to create S3 bucket '%s' in region '%s', this bucket will never be deleted automatically if created. Allow? [yes/no]", bucketName, region))
	var resp string
	_, err = fmt.Scanln(&resp)
	if err != nil {
		return errors.Wrap(err, "couldn't read input")
	}
	if resp != "yes" {
		return nil
	}

	_, err = s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		return errors.Wrap(err, "couldn't create bucket")
	}
	return nil
}
