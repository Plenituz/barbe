package wasm

import (
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type RpcFunc = func(args []any) (any, error)

type RpcProtocol struct {
	logger zerolog.Logger

	RegisteredFunctions map[string]RpcFunc
}

type rpcRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
}

type rpcResponse struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (r RpcProtocol) HandleMessage(text []byte) ([]byte, error) {
	var req rpcRequest
	err := json.Unmarshal(text, &req)
	//all logs arrive here, so if we cant parse it, it's probably just a log
	if err != nil {
		return nil, nil
	}
	if req.Method == "" {
		return nil, nil
	}
	f, ok := r.RegisteredFunctions[req.Method]
	if !ok {
		return nil, nil
	}

	result, err := f(req.Params)
	if err != nil {
		r.logger.Error().Str("req", string(text)).Err(err).Msgf("error executing rpc function '%s'", req.Method)
		resp, err := json.Marshal(rpcResponse{
			Error: err.Error(),
		})
		if err != nil {
			return nil, errors.Wrap(err, "error marshaling error response to rpc function")
		}
		return resp, nil
	}

	resp, err := json.Marshal(rpcResponse{
		Result: result,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling success response to rpc function")
	}
	return resp, nil
}
