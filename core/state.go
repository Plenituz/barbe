package core

type StatePersister interface {
	// ReadState reads the state from the storage layer.
	// the input params is a syntax token containing the configuration specific to the implementation
	// if none are found nil must be returned
	ReadState(params SyntaxToken) (*StateHolder, error)
	StoreState(params SyntaxToken, stateHolder StateHolder) error
}

type StateHolder struct {
	Version string
	States  map[string]State
}

//State is arbitrary data from the templates
//the values of this map must be a primitive type: bool, any numerical type, or string
type State = map[string]any
