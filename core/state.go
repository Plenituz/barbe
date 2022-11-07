package core

import (
	"context"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type StatePersister interface {
	// ReadState reads the state from the storage layer.
	// the input params is a syntax token containing the configuration specific to the implementation
	// if none are found nil must be returned
	ReadState() (*StateHolder, error)
	StoreState(stateHolder StateHolder) error
}

type StateActionName = string

type StateAction struct {
	Action StateActionName

	//if Action is StateActionSet or StateActionDelete
	Key *string

	//if Action is StateActionSet
	SetValue any
}

const (
	StateStoreDatabagType           = "barbe_state_store"
	StateDatabagType                = "barbe_state"
	BarbeStateSetDatabagType        = "barbe_state_set"
	BarbeStateDeleteDatabagType     = "barbe_state_delete"
	CurrentStateHolderFormatVersion = 1

	StatePersisterLocal = "local"

	StateActionSet    = "set"
	StateActionDelete = "delete"

	ErrNoStatePersister = "no state persister found"
)

func NewStatePersister(ctx context.Context, maker *Maker, name string, config SyntaxToken) (StatePersister, error) {
	switch name {
	case StatePersisterLocal:
		return NewLocalStatePersister(ctx, maker, config), nil
	}
	return nil, errors.New("unknown state persister '" + name + "'")
}

type StateHolder struct {
	FormatVersion int64
	//State is arbitrary data from the templates
	//the values of this map must be json marshallable
	States map[string]any
}

func NewStateHolder() *StateHolder {
	return &StateHolder{
		FormatVersion: CurrentStateHolderFormatVersion,
		States:        make(map[string]any),
	}
}

type StateHandler struct {
	Maker        *Maker
	CurrentState *StateHolder

	alreadyCreatedPersisters map[string]struct{}
	peristers                []StatePersister
}

func NewStateHandler(maker *Maker) *StateHandler {
	return &StateHandler{
		Maker:                    maker,
		alreadyCreatedPersisters: make(map[string]struct{}),
	}
}

func (s *StateHandler) DatabagsChanged(ctx context.Context, container *ConfigContainer) error {
	err := s.CreatePersisters(ctx, container)
	if err != nil {
		return err
	}
	_, err = s.HandleStateActions(container)
	if err != nil {
		return err
	}
	container.DeleteDataBagsOfType(BarbeStateSetDatabagType)
	container.DeleteDataBagsOfType(BarbeStateDeleteDatabagType)
	if s.CurrentState == nil || s.CurrentState.States == nil {
		return nil
	}

	container.DeleteDataBagsOfType(StateDatabagType)
	for key, value := range s.CurrentState.States {
		token, err := DecodeValue(value)
		if err != nil {
			return errors.Wrap(err, "error decoding state value as syntax token")
		}
		err = container.Insert(DataBag{
			Type:  StateDatabagType,
			Name:  key,
			Value: token,
		})
		if err != nil {
			return errors.Wrap(err, "error inserting state databag '"+key+"'")
		}
	}
	return nil
}

func (s *StateHandler) CreatePersisters(ctx context.Context, container *ConfigContainer) error {
	group := container.GetDataBagsOfType(StateStoreDatabagType)
	if len(group) == 0 {
		return nil
	}
	for _, bag := range group {
		if _, found := s.alreadyCreatedPersisters[bag.Name]; found {
			continue
		}
		s.alreadyCreatedPersisters[bag.Name] = struct{}{}
		persister, err := NewStatePersister(ctx, s.Maker, bag.Name, bag.Value)
		if err != nil {
			return errors.Wrap(err, "error creating state persister '"+bag.Name+"' of type")
		}
		err = s.AddPersister(persister)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *StateHandler) AddPersister(persister StatePersister) error {
	s.peristers = append(s.peristers, persister)

	if s.CurrentState != nil {
		return nil
	}
	newPersister := s.peristers[len(s.peristers)-1]
	newState, err := newPersister.ReadState()
	if err != nil {
		return errors.Wrap(err, "error reading state from new persister")
	}
	if newState == nil {
		return nil
	}
	s.CurrentState = newState
	return nil
}

func (s *StateHandler) Persist() error {
	if s.CurrentState == nil {
		return nil
	}
	eg := errgroup.Group{}
	eg.SetLimit(15)
	for _, persister := range s.peristers {
		persister := persister
		eg.Go(func() error {
			return persister.StoreState(*s.CurrentState)
		})
	}
	return eg.Wait()
}

func (s *StateHandler) HandleStateActions(container *ConfigContainer) (hasChanged bool, err error) {
	stateActions := make([]StateAction, 0)

	sets := container.GetDataBagsOfType(BarbeStateSetDatabagType)
	for _, bag := range sets {
		stateActions = append(stateActions, StateAction{
			Action:   StateActionSet,
			Key:      Ptr(bag.Name),
			SetValue: bag.Value,
		})
	}

	deletes := container.GetDataBagsOfType(BarbeStateDeleteDatabagType)
	for _, bag := range deletes {
		stateActions = append(stateActions, StateAction{
			Action: StateActionDelete,
			Key:    Ptr(bag.Name),
		})
	}

	for _, action := range stateActions {
		err := s.ApplyStateAction(action)
		if err != nil {
			return false, errors.Wrap(err, "error applying state action")
		}
	}

	hasChanged = len(deletes) != 0 || len(sets) != 0

	if hasChanged {
		err = s.Persist()
		if err != nil {
			return false, errors.Wrap(err, "error persisting state")
		}
	}
	return hasChanged, nil
}

func (s *StateHandler) ApplyStateAction(action StateAction) error {
	switch action.Action {
	case StateActionSet:
		return s.applySetAction(action)
	case StateActionDelete:
		return s.applyDeleteAction(action)
	default:
		return errors.New("unknown state action '" + action.Action + "'")
	}
}

func (s *StateHandler) applySetAction(action StateAction) error {
	if action.Key == nil {
		return errors.New("key is required for set action")
	}
	if s.CurrentState == nil {
		s.CurrentState = NewStateHolder()
	}
	if s.CurrentState.States == nil {
		s.CurrentState.States = make(map[string]any)
	}
	s.CurrentState.States[*action.Key] = action.SetValue
	return nil
}

func (s *StateHandler) applyDeleteAction(action StateAction) error {
	if s.CurrentState == nil {
		return nil
	}
	if action.Key == nil {
		return errors.New("key is required for delete action")
	}
	if s.CurrentState == nil || s.CurrentState.States == nil {
		return nil
	}
	delete(s.CurrentState.States, *action.Key)
	return nil
}