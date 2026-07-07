package requestbus

import (
	"context"
	"errors"
	"fmt"
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

// Register registers a handler for a given request type, this is an genric
// The handler should be a function with one of the following signatures:
// func(ctx context.Context, request <type as request>) error
// func(ctx context.Context, request <type as request>) (<type for result>, error)
func (b *RequestBus) Register(h any) error {
	t := reflect.TypeOf(h)
	if !(t.Kind() == reflect.Func) {
		return fmt.Errorf("unsupported handler signature, expecting a callback function: handlerType: `%s`\n%s", reflect.TypeOf(h).String(), stackTrace(1))
	}

	if t.NumIn() != 2 {
		return fmt.Errorf("unsupported handler signature, invalid number of arguments: handlerType: `%s`\n%s", reflect.TypeOf(h).String(), stackTrace(1))
	}

	if !t.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return fmt.Errorf("unsupported handler signature, first argument should be a context.Context: handlerType: `%s`\n%s", reflect.TypeOf(h).String(), stackTrace(1))
	}
	reqestType := b.nameResolver(reflect.New(t.In(1)).Elem().Interface())

	if t.NumOut() < 1 || t.NumOut() > 2 {
		return fmt.Errorf("unsupported handler signature, invalid number of return types: handlerType: `%s`\n%s", reflect.TypeOf(h).String(), stackTrace(1))
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	lastResultType := t.Out(t.NumOut() - 1)
	if !lastResultType.Implements(errorType) {
		return fmt.Errorf("unsupported handler signature, last return type should be an implementation of the error interface: handlerType: `%s`\n%s", reflect.TypeOf(h).String(), stackTrace(1))
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

func wrapResultHandler(handler any) any {
	handlerType := reflect.TypeOf(handler)
	ctxType := reflect.TypeFor[context.Context]()
	errType := reflect.TypeFor[error]()
	inputType := handlerType.In(1)
	outputType := handlerType.Out(0)

	fnType := reflect.FuncOf(
		[]reflect.Type{
			ctxType,
			inputType,
		},
		[]reflect.Type{
			outputType,
			errType,
		},
		false,
	)

	handlerFunc := reflect.ValueOf(handler)
	return reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		return handlerFunc.Call(args)
	}).Interface()
}

func wrapHandler(handler any) any {
	handlerType := reflect.TypeOf(handler)
	ctxType := reflect.TypeFor[context.Context]()
	errType := reflect.TypeFor[error]()
	inputType := handlerType.In(1)

	fnType := reflect.FuncOf(
		[]reflect.Type{
			ctxType,
			inputType,
		},
		[]reflect.Type{
			errType,
		},
		false,
	)

	handlerFunc := reflect.ValueOf(handler)
	return reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		return handlerFunc.Call(args)
	}).Interface()
}

func (b *RequestBus) RegisterHandler[I any](h func(context.Context, I) error) error {
	var in I
	requestType := b.nameResolver(in)
	resultType := "void"

	return b.addHandler(requestType, resultType, h)
}

func (b *RequestBus) RegisterResultHandler[I, O any](h func(context.Context, I) (O, error)) error {
	var in I
	requestType := b.nameResolver(in)

	var out O
	resultType := b.typeResolver(out)

	return b.addHandler(requestType, resultType, h)
}

func (b *RequestBus) addHandler(requestName string, resultType string, handler any) error {
	handlers, ok := b.handlers[requestName]
	if !ok {
		handlers = make(map[string]any)
		b.handlers[requestName] = handlers
	}

	if _, exists := handlers[resultType]; exists {
		return fmt.Errorf("handler with same signature already registered: requestName: `%s`, resultType: `%s`, handlerType: `%s`\n%s", requestName, resultType, reflect.TypeOf(handler).String(), stackTrace(1))
	}
	handlers[resultType] = handler

	return nil
}

// Use adds a generic middleware to the request bus for all the handlers
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

func (b *RequestBus) UseForHandler[I any](middlewares ...Middleware[I]) error {
	var request I
	requestName := b.nameResolver(request)
	handler, found := b.getHandler(requestName, "void")

	if !found {
		return errors.New("handler not found for signature")
		//return NewBusError("no handler found for request", requestName, "void")
	}

	requestHandler, ok := handler.(func(context.Context, I) error)
	if !ok {
		return errors.New("cannot dispatch request, handler signature not compatible")
	}

	//chain handlers
	h := requestHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}

	b.replaceHandler(requestName, "void", h)
	return nil
}

func (b *RequestBus) UseForResultHandler[I, O any](middlewares ...MiddlewareWithResult[I, O]) error {
	var request I
	requestName := b.nameResolver(request)
	var result O
	resultType := b.typeResolver(result)

	handler, found := b.getHandler(requestName, resultType)

	if !found {
		return errors.New("handler not found for signature")
		//return NewBusError("no handler found for request", requestName, "void")
	}

	requestHandler, ok := handler.(func(context.Context, I) (O, error))
	if !ok {
		return errors.New("cannot dispatch request, handler signature not compatible")
	}

	//chain handlers
	h := requestHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}

	b.replaceHandler(requestName, resultType, h)
	return nil
}

func (b *RequestBus) replaceHandler(requestName string, resultType string, handler any) {
	handlers, ok := b.handlers[requestName]
	if !ok {
		handlers = make(map[string]any)
		b.handlers[requestName] = handlers
	}

	handlers[resultType] = handler
}

// Dispatch sends a request to the appropriate handler and executes it. Returns an error if no compatible handler is found.
func (b *RequestBus) Dispatch[I any](ctx context.Context, request I) error {
	requestName := b.nameResolver(request)
	handler, found := b.getHandler(requestName, "void")
	if !found {
		return fmt.Errorf("handler not found for signature: requestName: `%s`\n%s", requestName, stackTrace(1))
	}

	requestHandler, ok := handler.(func(context.Context, I) error)
	if !ok {
		return fmt.Errorf("cannot dispatch request, handler signature not compatible: requestName: `%s`, handlerType: `%s`\n%s", requestName, reflect.TypeOf(handler).String(), stackTrace(1))
	}

	//apply global middleware
	if b.middleware != nil {
		h := requestHandler
		requestHandler = func(ctx context.Context, request I) error {
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

// DispatchResult sends a typed request to the appropriate handler and retrieves a response. Returns an error if no handler is found.
func (b *RequestBus) DispatchResult[O, I any](ctx context.Context, request I) (O, error) {
	var result O
	requestName := b.nameResolver(request)
	handler, found := b.getFirstResponseHandler(requestName)
	if !found {
		return result, fmt.Errorf("handler not found for signature: requestName: `%s`\n%s", requestName, stackTrace(1))
	}

	requestHandler, ok := handler.(func(context.Context, I) (O, error))
	if !ok {
		return result, fmt.Errorf("cannot dispatch request, handler signature not compatible: requestName: `%s`, handlerType: `%s`\n%s", requestName, reflect.TypeOf(handler).String(), stackTrace(1))
	}

	//apply global middleware
	if b.middleware != nil {
		h := requestHandler
		requestHandler = func(ctx context.Context, request I) (O, error) {
			res, err := b.middleware(
				func(ctx context.Context, r any) (any, error) {
					return h(ctx, request)
				},
			)(ctx, request)
			return res.(O), err
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
