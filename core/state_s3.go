package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"os"
)

const (
	s3StateDefaultRegion = "us-east-1"
)

type S3StatePersister struct {
	s3Client *s3.S3
	uploader *s3manager.Uploader
	Bucket   string
	Key      string
}

func NewS3StatePersister(ctx context.Context, params SyntaxToken) (S3StatePersister, error) {
	objI, err := TokenToGoValue(params)
	if InterfaceIsNil(objI) {
		return S3StatePersister{}, fmt.Errorf("error extracting S3StatePersister params, params is nil: %w", err)
	}
	var parsed struct {
		Bucket  string `mapstructure:"bucket"`
		Key     string `mapstructure:"key"`
		Region  string `mapstructure:"region"`
		Profile string `mapstructure:"profile"`
	}
	err = mapstructure.Decode(objI, &parsed)
	if err != nil {
		return S3StatePersister{}, errors.Wrap(err, "error parsing S3StatePersister params")
	}
	if parsed.Bucket == "" {
		return S3StatePersister{}, errors.New("bucket is empty")
	}
	if parsed.Key == "" {
		return S3StatePersister{}, errors.New("key is empty")
	}
	if parsed.Region == "" {
		parsed.Region = os.Getenv("AWS_REGION")
	}
	if parsed.Region == "" {
		parsed.Region = s3StateDefaultRegion
	}

	opts := session.Options{
		Profile: parsed.Profile,
	}
	opts.Config.MergeIn(&aws.Config{
		Region: aws.String(parsed.Region),
	})
	sess, err := session.NewSessionWithOptions(opts)
	if err != nil {
		return S3StatePersister{}, errors.Wrap(err, "error creating aws session")
	}

	return S3StatePersister{
		s3Client: s3.New(sess),
		uploader: s3manager.NewUploader(sess),
		Bucket:   parsed.Bucket,
		Key:      parsed.Key,
	}, nil
}

func (l S3StatePersister) ReadState() (*StateHolder, error) {
	obj, err := l.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(l.Bucket),
		Key:    aws.String(l.Key),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchKey" {
			return nil, nil
		}
		return nil, errors.Wrap(err, "error getting state from s3")
	}
	defer obj.Body.Close()

	var stateHolder StateHolder
	err = json.NewDecoder(obj.Body).Decode(&stateHolder)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding barbe state file as json")
	}
	return &stateHolder, nil
}

func (l S3StatePersister) StoreState(stateHolder StateHolder) error {
	buffer := &bytes.Buffer{}
	err := json.NewEncoder(buffer).Encode(stateHolder)
	if err != nil {
		return errors.Wrap(err, "error encoding barbe state as json")
	}

	_, err = l.uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(l.Bucket),
		Key:    aws.String(l.Key),
		Body:   buffer,
	})
	if err != nil {
		return errors.Wrap(err, "error putting object on s3")
	}
	return nil
}
