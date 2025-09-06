package requestbus

import "context"

const busCtxKey = "bus"

func DispatchWithResult[O any, I any](ctx context.Context, request I) (O, error) {
	bus := fromCTX(ctx)

	var t O
	res, err := bus.DispatchWithTypedResult(ctx, request, t)

	if res == nil {
		return t, err
	}
	return res.(O), err
}

func Dispatch[I any](ctx context.Context, request I) error {
	bus := fromCTX(ctx)

	return bus.Dispatch(ctx, request)
}

func ContextWithBus(ctx context.Context, bus *RequestBus) context.Context {
	return context.WithValue(ctx, busCtxKey, bus)
}

func fromCTX(ctx context.Context) *RequestBus {
	bus, ok := ctx.Value(busCtxKey).(*RequestBus)
	if !ok {
		panic("unable to retrieve bus from context")
	}
	return bus
}
