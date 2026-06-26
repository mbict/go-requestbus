package requestbus

import (
	"context"
	"errors"
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
		return errors.New("handler with same signature already registered")
		//return NewBusError("handler with same signature already registered", requestName, resultType)
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
		return errors.New("handler not found for signature")
		//return NewBusError("no handler found for request", requestName, "void")
	}

	requestHandler, ok := handler.(func(context.Context, I) error)
	if !ok {
		return errors.New("cannot dispatch request, handler signature not compatible")
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
		return result, errors.New("handler not found for signature")
		//NewBusError("no handler found for request, could not determine result type", requestName, "")
	}

	requestHandler, ok := handler.(func(context.Context, I) (O, error))
	if !ok {
		return result, errors.New("cannot dispatch request, handler signature not compatible")
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
