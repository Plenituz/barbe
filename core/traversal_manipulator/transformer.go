package traversal_manipulator

import (
	"context"
	"barbe/core"
)

type TraversalManipulator struct{}

func (t TraversalManipulator) Name() string {
	return "traversal_manipulator"
}

func (t TraversalManipulator) Transform(ctx context.Context, data *core.ConfigContainer) error {
	err := transformTraversals(ctx, data)
	if err != nil {
		return err
	}

	err = mapTraversals(ctx, data)
	if err != nil {
		return err
	}
	return nil
}
