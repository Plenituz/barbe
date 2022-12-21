package core

import (
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"sync"
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

	//if Action is one of StateActionSet, StateActionDelete, StateActionPutInObject, StateActionDeleteFromObject
	Key *string

	//if Action is StateActionSet
	SetValue any

	//if Action is PutInObject
	PutInObject map[string]any

	//if Action is StateActionDeleteFromObject
	DeleteFromObject *string
}

const (
	StateStoreDatabagType = "barbe_state_store"
	//overrides the value at the given key
	BarbeStateSetDatabagType = "barbe_state(set_value)"
	//assuming the given key is an object, adds the given key/value pairs to the object
	BarbeStatePutDatabagType = "barbe_state(put_in_object)"
	//assuming the given key is an object, removes the given key/value pairs from the object
	BarbeStateDeleteFromObjectDatabagType = "barbe_state(delete_from_object)"
	//delete the given key from the state completely
	BarbeStateDeleteDatabagType     = "barbe_state(delete_key)"
	CurrentStateHolderFormatVersion = 1

	StatePersisterLocal = "local"
	StatePersisterS3    = "s3"

	StateActionSet              = "set"
	StateActionDelete           = "delete"
	StateActionPutInObject      = "put_in_object"
	StateActionDeleteFromObject = "delete_from_object"
)

var (
	//a list of all the barbe_state related databags types
	BarbeStateTypes = []string{
		BarbeStateSetDatabagType,
		BarbeStatePutDatabagType,
		BarbeStateDeleteDatabagType,
		BarbeStateDeleteFromObjectDatabagType,
	}
)

func NewStatePersister(ctx context.Context, maker *Maker, name string, config SyntaxToken) (StatePersister, error) {
	switch name {
	case StatePersisterLocal:
		return NewLocalStatePersister(ctx, maker, config), nil
	case StatePersisterS3:
		return NewS3StatePersister(ctx, config)
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
	Maker *Maker

	//currentState is private because of the mutex
	currentState             *StateHolder
	stateMutex               sync.RWMutex
	alreadyCreatedPersisters map[string]struct{}
	persisters               []StatePersister
}

func NewStateHandler(maker *Maker) *StateHandler {
	return &StateHandler{
		Maker:                    maker,
		stateMutex:               sync.RWMutex{},
		alreadyCreatedPersisters: make(map[string]struct{}),
	}
}

func (s *StateHandler) GetState() map[string]any {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	if s.currentState == nil {
		return make(map[string]any)
	}
	if s.currentState.States == nil {
		return make(map[string]any)
	}
	return s.currentState.States
}

func (s *StateHandler) HandleStateDatabags(ctx context.Context, container *ConfigContainer) error {
	err := s.CreatePersisters(ctx, container)
	if err != nil {
		return err
	}
	_, err = s.HandleStateActions(ctx, container)
	if err != nil {
		return err
	}
	for _, t := range BarbeStateTypes {
		container.DeleteDataBagsOfType(t)
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
	s.persisters = append(s.persisters, persister)

	s.stateMutex.RLock()
	if s.currentState != nil {
		s.stateMutex.RUnlock()
		return nil
	}
	s.stateMutex.RUnlock()

	newPersister := s.persisters[len(s.persisters)-1]
	newState, err := newPersister.ReadState()
	if err != nil {
		return errors.Wrap(err, "error reading state from new persister")
	}
	//TODO merge states and write to persisters if needed
	if newState == nil {
		return nil
	}
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	s.currentState = newState
	return nil
}

func (s *StateHandler) Persist() error {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	if s.currentState == nil {
		return nil
	}
	eg := errgroup.Group{}
	eg.SetLimit(15)
	for i := range s.persisters {
		persister := s.persisters[i]
		eg.Go(func() error {
			return persister.StoreState(*s.currentState)
		})
	}
	return eg.Wait()
}

func (s *StateHandler) HandleStateActions(ctx context.Context, container *ConfigContainer) (hasChanged bool, err error) {
	stateActions := make([]StateAction, 0)

	sets := container.GetDataBagsOfType(BarbeStateSetDatabagType)
	for _, bag := range sets {
		objI, _ := TokenToGoValue(bag.Value)
		if InterfaceIsNil(objI) {
			log.Ctx(ctx).Warn().Msgf("barbe_state(set_value) '%s' has a value that is interpreted as nil, ignoring it", bag.Name)
			continue
		}
		stateActions = append(stateActions, StateAction{
			Action:   StateActionSet,
			Key:      Ptr(bag.Name),
			SetValue: objI,
		})
	}

	putInObjects := container.GetDataBagsOfType(BarbeStatePutDatabagType)
	for _, bag := range putInObjects {
		if bag.Value.Type != TokenTypeObjectConst {
			log.Ctx(ctx).Warn().Msgf("barbe_state(put_in_object) '%s' has a value that is not an object, ignoring it", bag.Name)
			continue
		}
		objI, _ := TokenToGoValue(bag.Value)
		if InterfaceIsNil(objI) {
			log.Ctx(ctx).Warn().Msgf("barbe_state(put_in_object) '%s' has a value that is interpreted as nil, ignoring it", bag.Name)
			continue
		}
		obj, ok := objI.(map[string]any)
		if !ok {
			log.Ctx(ctx).Warn().Msgf("barbe_state(put_in_object) '%s' has a value that parsed as not an object: '%T'", bag.Name, objI)
			continue
		}
		stateActions = append(stateActions, StateAction{
			Action:      StateActionPutInObject,
			Key:         Ptr(bag.Name),
			PutInObject: obj,
		})
	}

	deleteFromObjects := container.GetDataBagsOfType(BarbeStateDeleteFromObjectDatabagType)
	for _, bag := range deleteFromObjects {
		valueStr, err := ExtractAsStringValue(bag.Value)
		if err != nil {
			log.Ctx(ctx).Warn().Msgf("barbe_state(delete_from_object) '%s' has a value that is not a string, ignoring it", bag.Name)
			continue
		}
		stateActions = append(stateActions, StateAction{
			Action:           StateActionDeleteFromObject,
			Key:              Ptr(bag.Name),
			DeleteFromObject: Ptr(valueStr),
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
	if len(stateActions) != 0 {
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
	case StateActionPutInObject:
		return s.applyPutInObjectAction(action)
	case StateActionDeleteFromObject:
		return s.applyDeleteFromObjectAction(action)
	default:
		return errors.New("unknown state action '" + action.Action + "'")
	}
}

func (s *StateHandler) applySetAction(action StateAction) error {
	if action.Key == nil {
		return errors.New("key is required for set action")
	}
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	if s.currentState == nil {
		s.currentState = NewStateHolder()
	}
	if s.currentState.States == nil {
		s.currentState.States = make(map[string]any)
	}
	s.currentState.States[*action.Key] = action.SetValue
	return nil
}

func (s *StateHandler) applyDeleteAction(action StateAction) error {
	if action.Key == nil {
		return errors.New("key is required for delete action")
	}
	s.stateMutex.RLock()
	if s.currentState == nil || s.currentState.States == nil {
		s.stateMutex.RUnlock()
		return nil
	}
	s.stateMutex.RUnlock()

	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	delete(s.currentState.States, *action.Key)
	return nil
}

func (s *StateHandler) applyPutInObjectAction(action StateAction) error {
	if action.Key == nil {
		return errors.New("key is required for put_in_object action")
	}
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	if s.currentState == nil {
		s.currentState = NewStateHolder()
	}
	if s.currentState.States == nil {
		s.currentState.States = make(map[string]any)
	}
	if v, ok := s.currentState.States[*action.Key]; ok {
		if _, ok := v.(map[string]any); !ok {
			return errors.New("tried to use put_in_object but the state already has a non-object at key '" + *action.Key + "'")
		}
	} else {
		s.currentState.States[*action.Key] = make(map[string]any)
	}
	for k, v := range action.PutInObject {
		s.currentState.States[*action.Key].(map[string]any)[k] = v
	}
	return nil
}

func (s *StateHandler) applyDeleteFromObjectAction(action StateAction) error {
	if action.Key == nil {
		return errors.New("key is required for delete_from_object action")
	}
	if action.DeleteFromObject == nil {
		return errors.New("delete_from_object is required for delete_from_object action")
	}
	s.stateMutex.RLock()
	if s.currentState == nil || s.currentState.States == nil {
		s.stateMutex.RUnlock()
		return nil
	}
	s.stateMutex.RUnlock()

	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	if v, ok := s.currentState.States[*action.Key]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("tried to use delete_from_object but the state already has a non-object at key '" + *action.Key + "'")
		}
		delete(m, *action.DeleteFromObject)
		s.currentState.States[*action.Key] = m
	}
	return nil
}
