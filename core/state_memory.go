package core

type MemoryStatePersister struct {
	stateHolder *StateHolder
}

func NewMemoryStatePersister() *MemoryStatePersister {
	return &MemoryStatePersister{}
}

func (m *MemoryStatePersister) ReadState() (*StateHolder, error) {
	return m.stateHolder, nil
}

func (m *MemoryStatePersister) StoreState(stateHolder StateHolder) error {
	m.stateHolder = &stateHolder
	return nil
}
