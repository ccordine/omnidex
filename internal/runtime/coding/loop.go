package coding

import "context"

type noopTestWriter struct{}

func (noopTestWriter) WriteTest(context.Context, ChangeStep) error {
	return nil
}

type noopCoder struct{}

func (noopCoder) ApplyChange(context.Context, ChangeStep) error {
	return nil
}
