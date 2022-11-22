package traversal_manipulator

import (
	"barbe/core"
	"context"
)

type TraversalManipulator struct {
	traversalTransforms map[string]string
	traversalMaps       map[string]core.SyntaxToken
	tokenMaps           []tokenMap
}

type tokenMap struct {
	Match     core.SyntaxToken
	ReplaceBy core.SyntaxToken
}

func NewTraversalManipulator() *TraversalManipulator {
	return &TraversalManipulator{
		traversalTransforms: map[string]string{},
		traversalMaps:       map[string]core.SyntaxToken{},
		tokenMaps:           []tokenMap{},
	}
}

func (t *TraversalManipulator) Name() string {
	return "traversal_manipulator"
}

func (t *TraversalManipulator) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	output := core.NewConfigContainer()
	err := t.transformTraversals(ctx, data, output)
	if err != nil {
		return core.ConfigContainer{}, err
	}

	err = t.mapTraversals(ctx, data, output)
	if err != nil {
		return core.ConfigContainer{}, err
	}

	err = t.mapTokens(ctx, data, output)
	if err != nil {
		return core.ConfigContainer{}, err
	}
	return *output, nil
}
