package core

import (
	"bytes"
	"cloud.google.com/go/storage"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type GCSStatePersister struct {
	storageClient *storage.Client
	Bucket        string
	Key           string
}

func NewGCSStatePersister(ctx context.Context, params SyntaxToken) (GCSStatePersister, error) {
	objI, err := TokenToGoValue(params, false)
	if InterfaceIsNil(objI) {
		return GCSStatePersister{}, fmt.Errorf("error extracting GCSStatePersister params, params is nil: %w", err)
	}
	var parsed struct {
		Bucket string `mapstructure:"bucket"`
		Key    string `mapstructure:"key"`
	}
	err = mapstructure.Decode(objI, &parsed)
	if err != nil {
		return GCSStatePersister{}, errors.Wrap(err, "error parsing GCSStatePersister params")
	}
	if parsed.Bucket == "" {
		return GCSStatePersister{}, errors.New("bucket is empty")
	}
	if parsed.Key == "" {
		return GCSStatePersister{}, errors.New("key is empty")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return GCSStatePersister{}, err
	}
	return GCSStatePersister{
		storageClient: client,
		Bucket:        parsed.Bucket,
		Key:           parsed.Key,
	}, nil
}

func (l GCSStatePersister) ReadState() (*StateHolder, error) {
	rc, err := l.storageClient.Bucket(l.Bucket).Object(l.Key).NewReader(context.Background())
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, nil
		}
		return nil, errors.Wrap(err, "error getting state from gcs")
	}
	defer rc.Close()

	var stateHolder StateHolder
	err = json.NewDecoder(rc).Decode(&stateHolder)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding barbe state file as json")
	}
	return &stateHolder, nil
}

func (l GCSStatePersister) StoreState(stateHolder StateHolder) error {
	buffer := &bytes.Buffer{}
	err := json.NewEncoder(buffer).Encode(stateHolder)
	if err != nil {
		return errors.Wrap(err, "error encoding barbe state as json")
	}

	writer := l.storageClient.Bucket(l.Bucket).Object(l.Key).NewWriter(context.Background())
	defer writer.Close()
	_, err = writer.Write(buffer.Bytes())
	if err != nil {
		return errors.Wrap(err, "error putting object on gcs")
	}
	return nil
}
