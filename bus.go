package requestbus

import (
	"context"
	"errors"
	"reflect"
)

type Middleware[I any] func(next func(ctx context.Context, q I) error) func(ctx context.Context, q I) error
type MiddlewareWithResult[I any, O any] func(next func(ctx context.Context, q I) (O, error)) func(ctx context.Context, q I) (O, error)

type Handler func(context.Context, any) error
type ResponseHandler func(context.Context, any) (any, error)

type MiddlewareFunc func(next ResponseHandler) ResponseHandler

type RequestBus struct {
	handlers     map[string]map[string]any
	nameResolver func(req any) string
	typeResolver func(req any) string

	middleware MiddlewareFunc
}

// New creates a new request bus instance
func New(options ...Option) *RequestBus {
	bus := &RequestBus{
		//middleware:   nil,
		handlers:     make(map[string]map[string]any),
		nameResolver: DefaultNameResolver,
		typeResolver: DefaultTypeResolver,
	}

	for _, option := range options {
		option(bus)
	}

	return bus
}

// RegisterHandler registers a handler for a given request type
// The handler should be a function with one of the following signatures:
// func(ctx context.Context, request <type as request>) error
// func(ctx context.Context, request <type as request>) (<type for result>, error)
func (b *RequestBus) RegisterHandler(h any) error {
	//extract arguments
	t := reflect.TypeOf(h)
	if !(t.Kind() == reflect.Func) {
		return errors.New("unsupported handler signature, expecting a callback function")
	}

	if t.NumIn() != 2 {
		return errors.New("unsupported handler signature, invalid number of arguments")
	}

	if !t.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return errors.New("unsupported handler signature, first argument should be a context.Context")
	}
	reqestType := b.nameResolver(reflect.New(t.In(1)).Elem().Interface())

	if t.NumOut() < 1 || t.NumOut() > 2 {
		return errors.New("unsupported handler signature, invalid number of return types")
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	lastResultType := t.Out(t.NumOut() - 1)
	if !lastResultType.Implements(errorType) {
		return errors.New("unsupported handler signature, last return type should be an implementation of the error interface")
	}

	resultType := "void"
	if t.NumOut() == 2 {
		resultType = b.typeResolver(reflect.New(t.Out(0)).Elem().Interface())
	}

	if t.NumOut() >= 2 {
		return b.addHandler(reqestType, resultType, wrapResultHandler(h))
	}
	return b.addHandler(reqestType, resultType, wrapHandler(h))
}

func (b *RequestBus) addHandler(requestName string, resultType string, handler any) error {
	handlers, ok := b.handlers[requestName]
	if !ok {
		handlers = make(map[string]any)
		b.handlers[requestName] = handlers
	}

	if _, exists := handlers[resultType]; exists {
		return BusError{
			error:       "handler with same signature already registered",
			RequestName: requestName,
			ResultType:  resultType,
		}
	}
	handlers[resultType] = handler

	return nil
}

func wrapResultHandler(handler any) ResponseHandler {
	handlerFunc := reflect.ValueOf(handler)
	return func(ctx context.Context, request any) (any, error) {
		results := handlerFunc.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(request),
		})

		if results[1].IsNil() {
			return results[0].Interface(), nil
		}
		return results[0].Interface(), results[1].Interface().(error)
	}
}

func wrapHandler(handler any) Handler {
	handlerFunc := reflect.ValueOf(handler)
	return func(ctx context.Context, request any) error {
		results := handlerFunc.Call([]reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(request),
		})

		if results[0].IsNil() {
			return nil
		}
		return results[0].Interface().(error)
	}
}

// Use adds middleware to the request bus for all the handlers
func (b *RequestBus) Use(middleware ...MiddlewareFunc) {
	if b.middleware != nil {
		b.middleware = chainMiddleware(append([]MiddlewareFunc{b.middleware}, middleware...)...)
		return
	}
	b.middleware = chainMiddleware(middleware...)
}

func chainMiddleware(middlewares ...MiddlewareFunc) MiddlewareFunc {
	return func(final ResponseHandler) ResponseHandler {
		h := final
		for i := len(middlewares) - 1; i >= 0; i-- {
			h = middlewares[i](h)
		}
		return h
	}
}

func (b *RequestBus) getHandler(requestName, resultType string) (any, bool) {
	handlers, ok := b.handlers[requestName]
	if !ok {
		return nil, false
	}

	h, ok := handlers[resultType]
	if !ok {
		return nil, false
	}
	return h, true
}

func (b *RequestBus) Dispatch(ctx context.Context, request any) error {
	requestName := b.nameResolver(request)
	handler, found := b.getHandler(requestName, "void")
	if !found {
		return BusError{
			error:       "no handler found for request",
			RequestName: requestName,
			ResultType:  "void",
		}
	}

	requestHandler := handler.(Handler)
	if b.middleware != nil {
		h := requestHandler
		requestHandler = func(ctx context.Context, request any) error {
			_, err := b.middleware(
				func(ctx context.Context, r any) (any, error) {
					return nil, h(ctx, request)
				},
			)(ctx, request)
			return err
		}
	}
	return requestHandler(ctx, request)
}

// DispatchWithResult dispatches a request and returns the result
// It will dispatch to the first handler that returns a result,
// if there are multiple handlers registered with different return types this method wil fail
// in that case use DispatchWithTypedResult or the generic DispatchWithResult
func (b *RequestBus) DispatchWithResult(ctx context.Context, request any) (any, error) {
	requestName := b.nameResolver(request)
	handler, found := b.getFirstResponseHandler(requestName)
	if !found {
		return nil, BusError{
			error:       "no handler found for request, could not determine result type",
			RequestName: requestName,
			ResultType:  "",
		}
	}

	requestHandler := handler.(ResponseHandler)
	if b.middleware != nil {
		h := requestHandler
		requestHandler = func(ctx context.Context, request any) (any, error) {
			return b.middleware(
				func(ctx context.Context, r any) (any, error) {
					return h(ctx, request)
				},
			)(ctx, request)
		}
	}
	return requestHandler(ctx, request)
}

func (b *RequestBus) getFirstResponseHandler(requestName string) (any, bool) {
	handlers, ok := b.handlers[requestName]
	if !ok {
		return nil, false
	}

	_, hasVoid := handlers["void"]
	l := len(handlers)
	if hasVoid && l == 2 || l == 1 {
		for k, h := range handlers {
			if k != "void" {
				return h, true
			}
		}
	}
	return nil, false
}

// DispatchWithTypedResult dispatches a request and returns the result
// It will dispatch to the handler that returns a result of the given type
func (b *RequestBus) DispatchWithTypedResult(ctx context.Context, request any, responseType any) (any, error) {
	requestName := b.nameResolver(request)
	resultType := b.typeResolver(responseType)
	handler, found := b.getHandler(requestName, resultType)
	if !found {
		return nil, BusError{
			error:       "no handler found for request",
			RequestName: requestName,
			ResultType:  resultType,
		}
	}

	requestHandler := handler.(ResponseHandler)
	if b.middleware != nil {
		h := requestHandler
		requestHandler = func(ctx context.Context, request any) (any, error) {
			return b.middleware(
				func(ctx context.Context, r any) (any, error) {
					return h(ctx, request)
				},
			)(ctx, request)
		}
	}
	return requestHandler(ctx, request)
}
