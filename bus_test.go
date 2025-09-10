package requestbus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testRequest struct {
}

type testRequestWithName struct {
}

func (t testRequestWithName) RequestName() string {
	return "custom-name"
}

// -- register
func TestRegisterHandler_ShouldGiveErrorWhenHandlerAlreadyRegistered(t *testing.T) {
	bus := New()
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
	assert.NoError(t, err)

	err = bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
	assert.ErrorContains(t, err, "handler with same signature already registered")
}

func TestRegisterHandler_ShouldGiveErrorWhenHandlerIsSomethingElseThanFunc(t *testing.T) {
	bus := New()
	err := bus.RegisterHandler(int(1))

	assert.ErrorContains(t, err, "unsupported handler signature, expecting a callback function")
}

func TestRegisterHandler_ShouldGiveErrorWhenRegisteringHandlersWithWrongSignature(t *testing.T) {
	bus := New()

	errWrongCountOfArguments := bus.RegisterHandler(func() error { return nil })
	assert.ErrorContains(t, errWrongCountOfArguments, "unsupported handler signature, invalid number of arguments")

	errNoContextAsFirstParam := bus.RegisterHandler(func(int, int) error { return nil })
	assert.ErrorContains(t, errNoContextAsFirstParam, "unsupported handler signature, first argument should be a context.Context")

	errWrongCountOfReturnArguments := bus.RegisterHandler(func(context.Context, int) {})
	assert.ErrorContains(t, errWrongCountOfReturnArguments, "unsupported handler signature, invalid number of return types")

	errWrongCountOfReturnArgumentsTooMany := bus.RegisterHandler(func(context.Context, int) (int, int, error) { return 0, 0, nil })
	assert.ErrorContains(t, errWrongCountOfReturnArgumentsTooMany, "unsupported handler signature, invalid number of return types")

	errLastReturnTypeIsNotAnError := bus.RegisterHandler(func(context.Context, int) int { return 0 })
	assert.ErrorContains(t, errLastReturnTypeIsNotAnError, "unsupported handler signature, last return type should be an implementation of the error interface")
}

// --- dispatch

func TestDispatch(t *testing.T) {
	bus := New()
	called := false

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		called = true
		return nil
	})
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequest{})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestDispatch_WithCustomRequestNameImplementation(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequestWithName) error { return nil })
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequestWithName{})

	assert.NoError(t, err)
}

func TestDispatch_WithCustomRequestNameImplementation_FailsWhenNotRegisterd(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)

	err := bus.Dispatch(context.Background(), testRequestWithName{})

	busErr := BusError{}
	if assert.ErrorAs(t, err, &busErr) {
		assert.Equal(t, "custom-name", err.(BusError).RequestName)
	}
}

func TestDispatch_Fails_when_no_matching_handler_can_be_found(t *testing.T) {
	bus := New()

	err := bus.Dispatch(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
}

func TestDispatch_Fails_when_no_matching_handler_of_type_can_be_found(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, nil })
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
}

func TestDispatch_ShouldReturnErrorFromHandler(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return errors.New("foo bar") })
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "foo bar")
}

func TestDispatch_WithMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "2-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "2-after")
			return res, err
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return nil
	})
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequest{})

	assert.NoError(t, err)
	assert.Equal(t, []string{"1-before", "2-before", "handler", "2-after", "1-after"}, middlewareCallOrder)
}

func TestDispatch_WithMiddleware_StopChainOnMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (res any, err error) {
			middlewareCallOrder = append(middlewareCallOrder, "stop")
			return res, errors.New("foo")
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return nil
	})
	assert.NoError(t, err)

	err = bus.Dispatch(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "foo")
	assert.Equal(t, []string{"1-before", "stop", "1-after"}, middlewareCallOrder)
}

//--- generic dispatch

func TestGenericDispatch(t *testing.T) {
	bus := New()
	called := false

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		called = true
		return nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestGenericDispatch_WithTypeDefinedSignature(t *testing.T) {
	called := false
	type testHandler func(context.Context, testRequest) error

	h := func() testHandler {
		return func(ctx context.Context, request testRequest) error {
			called = true
			return nil
		}
	}
	bus := New()

	err := bus.RegisterHandler(h())
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestGenericDispatch_WithCustomRequestNameImplementation(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequestWithName) error { return nil })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequestWithName{})

	assert.NoError(t, err)
}

func TestGenericDispatch_WithCustomRequestNameImplementation_FailsWhenNotRegisterd(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)

	ctx := ContextWithBus(context.Background(), bus)
	err := Dispatch(ctx, testRequestWithName{})

	busErr := BusError{}
	if assert.ErrorAs(t, err, &busErr) {
		assert.Equal(t, "custom-name", err.(BusError).RequestName)
	}
}

func TestGenericDispatch_Fails_when_no_matching_handler_can_be_found(t *testing.T) {
	bus := New()

	ctx := ContextWithBus(context.Background(), bus)
	err := Dispatch(ctx, testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
}

func TestGenericDispatch_Fails_when_no_matching_handler_of_type_can_be_found(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, nil })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
}

func TestGenericDispatch_ShouldReturnErrorFromHandler(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return errors.New("foo bar") })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.ErrorContains(t, err, "foo bar")
}

func TestGenericDispatch_WithMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "2-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "2-after")
			return res, err
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.NoError(t, err)
	assert.Equal(t, []string{"1-before", "2-before", "handler", "2-after", "1-after"}, middlewareCallOrder)
}

func TestGenericDispatch_WithMiddleware_StopChainOnMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (res any, err error) {
			middlewareCallOrder = append(middlewareCallOrder, "stop")
			return res, errors.New("foo")
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	err = Dispatch(ctx, testRequest{})

	assert.ErrorContains(t, err, "foo")
	assert.Equal(t, []string{"1-before", "stop", "1-after"}, middlewareCallOrder)
}

//--- dispatch with result

func TestDispatchWithResult(t *testing.T) {
	bus := New()
	called := false

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		called = true
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 123, res)
}

func TestDispatchWithResult_WithCustomRequestNameImplementation(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequestWithName) (int, error) { return 123, nil })
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequestWithName{})

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
}

func TestDispatchWithResult_WithCustomRequestNameImplementation_FailsWhenNotRegisterd(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)

	res, err := bus.DispatchWithResult(context.Background(), testRequestWithName{})

	busErr := BusError{}
	if assert.ErrorAs(t, err, &busErr) {
		assert.Equal(t, "custom-name", err.(BusError).RequestName)
	}
	assert.Nil(t, res)
}

func TestDispatchWithResult_Fails_when_no_matching_handler_can_be_found(t *testing.T) {
	bus := New()

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Nil(t, res)
}

func TestDispatchWithResult_Fails_when_no_matching_handler_of_type_can_be_found(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Nil(t, res)
}

func TestDispatchWithResult_ShouldReturnErrorFromHandler(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, errors.New("foo bar") })
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "foo bar")
	assert.Equal(t, 123, res)
}

func TestDispatchWithResult_WithMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "2-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "2-after")
			return res, err
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
	assert.Equal(t, []string{"1-before", "2-before", "handler", "2-after", "1-after"}, middlewareCallOrder)
}

func TestDispatchWithResult_WithMiddleware_StopChainOnMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (res any, err error) {
			middlewareCallOrder = append(middlewareCallOrder, "stop")
			return int(321), errors.New("foo")
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithResult(context.Background(), testRequest{})

	assert.ErrorContains(t, err, "foo")
	assert.Equal(t, 321, res)
	assert.Equal(t, []string{"1-before", "stop", "1-after"}, middlewareCallOrder)
}

//--- dispatch with typed result

func TestDispatchWithTypedResult(t *testing.T) {
	bus := New()
	called := false

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		called = true
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0))

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 123, res)
}

func TestDispatchWithTypedResult_WithCustomRequestNameImplementation(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequestWithName) (int, error) { return 123, nil })
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequestWithName{}, int(0))

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
}

func TestDispatchWithTypeResult_WithCustomRequestNameImplementation_FailsWhenNotRegisterd(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequestWithName{}, int(0))

	busErr := BusError{}
	if assert.ErrorAs(t, err, &busErr) {
		assert.Equal(t, "custom-name", err.(BusError).RequestName)
	}
	assert.Nil(t, res)
}

func TestDispatchWithTypedResult_Fails_when_no_matching_handler_can_be_found(t *testing.T) {
	bus := New()

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(1))

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Nil(t, res)
}

func TestDispatchWithTyped_Fails_when_no_matching_handler_of_type_can_be_found(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0))

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Nil(t, res)
}

func TestDispatchWithTyped_ShouldReturnErrorFromHandler(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, errors.New("foo bar") })
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0))

	assert.ErrorContains(t, err, "foo bar")
	assert.Equal(t, 123, res)
}

func TestDispatchWithTypedResult_WithMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "2-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "2-after")
			return res, err
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0))

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
	assert.Equal(t, []string{"1-before", "2-before", "handler", "2-after", "1-after"}, middlewareCallOrder)
}

func TestDispatchWithTypedResult_WithMiddleware_StopChainOnMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (res any, err error) {
			middlewareCallOrder = append(middlewareCallOrder, "stop")
			return int(321), errors.New("foo")
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	res, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0))

	assert.ErrorContains(t, err, "foo")
	assert.Equal(t, 321, res)
	assert.Equal(t, []string{"1-before", "stop", "1-after"}, middlewareCallOrder)
}

//--- generic dispatch with result

func TestGenericDispatchWithResult(t *testing.T) {
	bus := New()
	called := false

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		called = true
		return 123, nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 123, res)
}

func TestGenericDispatchWithResult_WithTypeDefinedSignature(t *testing.T) {
	called := false

	type testHandler func(context.Context, testRequest) (int, error)

	h := func() testHandler {
		return func(ctx context.Context, request testRequest) (int, error) {
			called = true
			return 123, nil
		}
	}

	bus := New()

	err := bus.RegisterHandler(h())
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
	assert.True(t, called)
}

func TestGenericDispatchWithResult_WithCustomRequestNameImplementation(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)
	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequestWithName) (int, error) { return 123, nil })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequestWithName{})

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
}

func TestGenericDispatchWithResult_WithCustomRequestNameImplementation_FailsWhenNotRegisterd(t *testing.T) {
	bus := New(
		WithNameResolver(DefaultNameResolver),
		WithTypeResolver(DefaultTypeResolver),
	)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequestWithName{})

	busErr := BusError{}
	if assert.ErrorAs(t, err, &busErr) {
		assert.Equal(t, "custom-name", err.(BusError).RequestName)
	}
	assert.Equal(t, 0, res)
}

func TestGenericDispatchWithResult_Fails_when_no_matching_handler_can_be_found(t *testing.T) {
	bus := New()

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Equal(t, 0, res)
}

func TestGenericDispatchWithResult_Fails_when_no_matching_handler_of_type_can_be_found(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.ErrorContains(t, err, "no handler found for request")
	assert.Equal(t, 0, res)
}

func TestGenericDispatchWithResult_ShouldReturnErrorFromHandler(t *testing.T) {
	bus := New()

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, errors.New("foo bar") })
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.ErrorContains(t, err, "foo bar")
	assert.Equal(t, 123, res)
}

func TestGenericDispatchWithResult_WithMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "2-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "2-after")
			return res, err
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.NoError(t, err)
	assert.Equal(t, 123, res)
	assert.Equal(t, []string{"1-before", "2-before", "handler", "2-after", "1-after"}, middlewareCallOrder)
}

func TestGenericDispatchWithResult_WithMiddleware_StopChainOnMiddleware(t *testing.T) {
	bus := New()
	middlewareCallOrder := []string{}
	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (any, error) {
			middlewareCallOrder = append(middlewareCallOrder, "1-before")
			res, err := next(ctx, a)
			middlewareCallOrder = append(middlewareCallOrder, "1-after")
			return res, err
		}
	})

	bus.Use(func(next ResponseHandler) ResponseHandler {
		return func(ctx context.Context, a any) (res any, err error) {
			middlewareCallOrder = append(middlewareCallOrder, "stop")
			return int(321), errors.New("foo")
		}
	})

	err := bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) {
		middlewareCallOrder = append(middlewareCallOrder, "handler")
		return 123, nil
	})
	assert.NoError(t, err)

	ctx := ContextWithBus(context.Background(), bus)
	res, err := DispatchWithResult[int](ctx, testRequest{})

	assert.ErrorContains(t, err, "foo")
	assert.Equal(t, 321, res)
	assert.Equal(t, []string{"1-before", "stop", "1-after"}, middlewareCallOrder)
}

// --- generic context
func TestGenericDispatch_PanicsWhenNoBusFoundInContext(t *testing.T) {
	assert.PanicsWithValue(t, "unable to retrieve bus from context", func() {
		_ = Dispatch(context.Background(), testRequest{})
	})
}

func TestGenericDispatchWithResult_PanicsWhenNoBusFoundInContext(t *testing.T) {
	assert.PanicsWithValue(t, "unable to retrieve bus from context", func() {
		_, _ = DispatchWithResult[int](context.Background(), testRequest{})
	})
}
