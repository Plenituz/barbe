package core

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"path"
)

const localStateDefaultPath = "barbe_state.json"

type FileStatePersister struct {
	BaseDir       string
	StateFilePath string
}

func NewLocalStatePersister(ctx context.Context, maker *Maker, params SyntaxToken) FileStatePersister {
	o := FileStatePersister{
		BaseDir: maker.OutputDir,
	}
	if params.Type == TokenTypeObjectConst {
		stateFilePathToken := GetObjectKeyValues("state_file_path", params.ObjectConst)
		if len(stateFilePathToken) > 0 {
			tmp, err := ExtractAsStringValue(stateFilePathToken[0])
			if err == nil {
				o.StateFilePath = tmp
			} else {
				log.Ctx(ctx).Debug().Err(err).Msg("error extracting state_file_path value, using default")
			}
		}
	}
	if o.StateFilePath == "" {
		o.StateFilePath = localStateDefaultPath
	}
	return o
}

func (l FileStatePersister) ReadState() (*StateHolder, error) {
	p := path.Join(l.BaseDir, l.StateFilePath)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, errors.Wrap(err, "error opening barbe state file")
	}
	defer f.Close()

	var stateHolder StateHolder
	err = json.NewDecoder(f).Decode(&stateHolder)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding barbe state file as json")
	}
	return &stateHolder, nil
}

func (l FileStatePersister) StoreState(stateHolder StateHolder) error {
	p := path.Join(l.BaseDir, l.StateFilePath)
	f, err := os.Create(p)
	if err != nil {
		return errors.Wrap(err, "error creating barbe state file")
	}
	defer f.Close()

	err = json.NewEncoder(f).Encode(stateHolder)
	if err != nil {
		return errors.Wrap(err, "error encoding barbe state as json")
	}
	return nil
}
