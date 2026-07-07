package requestbus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── shared test types ────────────────────────────────────────────────────────

type testRequest struct{}

type testRequestWithName struct{}

func (t testRequestWithName) RequestName() string {
	return "custom-name"
}

type testResponse struct {
	Value string
}

// ── sentinel errors ──────────────────────────────────────────────────────────

var (
	errHandlerFailed         = errors.New("handler failed")
	errMiddlewareFailed      = errors.New("middleware failed")
	errMiddlewareIntercepted = errors.New("middleware intercepted")
)

// ── middleware execution tracking helpers ─────────────────────────────────────

type executionTracker struct {
	calls []string
}

func (e *executionTracker) record(name string) {
	e.calls = append(e.calls, name)
}

func (e *executionTracker) getOrder() []string {
	return e.calls
}

// ── RegisterHandler ──────────────────────────────────────────────────────────

func TestRegisterHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		bus := New()
		err := bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("duplicate returns error", func(t *testing.T) {
		bus := New()
		h := func(_ context.Context, _ testRequest) error { return nil }

		require.NoError(t, bus.RegisterHandler(h))
		err := bus.RegisterHandler(h)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler with same signature already registered")
	})

	t.Run("coexists with result handler for same request type", func(t *testing.T) {
		bus := New()
		err := bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		})
		require.NoError(t, err)

		err = bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		})
		assert.NoError(t, err)
	})
}

// ── RegisterResultHandler ────────────────────────────────────────────────────

func TestRegisterResultHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		bus := New()
		err := bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		})
		assert.NoError(t, err)
	})

	t.Run("duplicate returns error", func(t *testing.T) {
		bus := New()
		h := func(_ context.Context, _ testRequest) (testResponse, error) { return testResponse{}, nil }

		require.NoError(t, bus.RegisterResultHandler(h))
		err := bus.RegisterResultHandler(h)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler with same signature already registered")
	})

	t.Run("coexists with void handler for same request type", func(t *testing.T) {
		bus := New()
		err := bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		})
		require.NoError(t, err)

		err = bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		})
		assert.NoError(t, err)
	})
}

// ── Register (wrapper) ───────────────────────────────────────────────────────

func TestRegister(t *testing.T) {
	t.Run("void handler success delegates to RegisterHandler behavior", func(t *testing.T) {
		bus := New()

		err := bus.Register(func(_ context.Context, _ testRequest) error {
			return nil
		})
		assert.NoError(t, err)

		handler, found := bus.getHandler(bus.nameResolver(testRequest{}), "void")
		require.True(t, found)
		_, ok := handler.(func(context.Context, testRequest) error)
		assert.True(t, ok)
	})

	t.Run("result handler success delegates to RegisterResultHandler behavior", func(t *testing.T) {
		bus := New()

		err := bus.Register(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{Value: "ok"}, nil
		})
		assert.NoError(t, err)

		handler, found := bus.getHandler(bus.nameResolver(testRequest{}), bus.typeResolver(testResponse{}))
		require.True(t, found)
		_, ok := handler.(func(context.Context, testRequest) (testResponse, error))
		assert.True(t, ok)
	})

	t.Run("duplicate void handler returns wrapped handler signature error", func(t *testing.T) {
		bus := New()
		h := func(_ context.Context, _ testRequest) error { return nil }

		require.NoError(t, bus.Register(h))
		err := bus.Register(h)
		require.Error(t, err)

		assert.ErrorContains(t, err, "handler with same signature already registered")
		assert.ErrorContains(t, err, "handlerType: `func(context.Context, requestbus.testRequest) error`")
	})

	t.Run("duplicate result handler returns wrapped handler signature error", func(t *testing.T) {
		bus := New()
		h := func(_ context.Context, _ testRequest) (testResponse, error) { return testResponse{}, nil }

		require.NoError(t, bus.Register(h))
		err := bus.Register(h)
		require.Error(t, err)

		assert.ErrorContains(t, err, "handler with same signature already registered")
		assert.ErrorContains(t, err, "handlerType: `func(context.Context, requestbus.testRequest) (requestbus.testResponse, error)`")
	})

	t.Run("void and result handlers can coexist for same request type", func(t *testing.T) {
		bus := New()

		err := bus.Register(func(_ context.Context, _ testRequest) error { return nil })
		require.NoError(t, err)

		err = bus.Register(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{Value: "coexists"}, nil
		})
		assert.NoError(t, err)

		voidHandler, hasVoid := bus.getHandler(bus.nameResolver(testRequest{}), "void")
		resultHandler, hasResult := bus.getHandler(bus.nameResolver(testRequest{}), bus.typeResolver(testResponse{}))
		require.True(t, hasVoid)
		require.True(t, hasResult)
		_, isVoidWrapped := voidHandler.(func(context.Context, testRequest) error)
		_, isResultWrapped := resultHandler.(func(context.Context, testRequest) (testResponse, error))
		assert.True(t, isVoidWrapped)
		assert.True(t, isResultWrapped)
	})

	t.Run("invalid signatures return descriptive errors", func(t *testing.T) {
		tests := []struct {
			name              string
			handler           any
			exactFirstLine    string
			containsFragments []string
		}{
			{
				name:           "non function",
				handler:        42,
				exactFirstLine: "unsupported handler signature, expecting a callback function: handlerType: `int`",
			},
			{
				name:              "invalid number of arguments",
				handler:           func(_ context.Context) error { return nil },
				containsFragments: []string{"unsupported handler signature, invalid number of arguments", "handlerType: `func(context.Context) error`"},
			},
			{
				name:              "first argument is not context.Context",
				handler:           func(_ testRequest, _ testRequest) error { return nil },
				containsFragments: []string{"unsupported handler signature, first argument should be a context.Context", "handlerType: `func(requestbus.testRequest, requestbus.testRequest) error`"},
			},
			{
				name:              "invalid number of return types",
				handler:           func(_ context.Context, _ testRequest) {},
				containsFragments: []string{"unsupported handler signature, invalid number of return types", "handlerType: `func(context.Context, requestbus.testRequest)`"},
			},
			{
				name:              "last return type must implement error",
				handler:           func(_ context.Context, _ testRequest) (testResponse, int) { return testResponse{}, 0 },
				containsFragments: []string{"unsupported handler signature, last return type should be an implementation of the error interface", "handlerType: `func(context.Context, requestbus.testRequest) (requestbus.testResponse, int)`"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bus := New()
				err := bus.Register(tt.handler)
				require.Error(t, err)

				if tt.exactFirstLine != "" {
					firstLine := strings.SplitN(err.Error(), "\n", 2)[0]
					assert.Equal(t, tt.exactFirstLine, firstLine)
				}

				for _, fragment := range tt.containsFragments {
					assert.ErrorContains(t, err, fragment)
				}
			})
		}
	})
}

// ── Dispatch ─────────────────────────────────────────────────────────────────

func TestDispatch(t *testing.T) {
	t.Run("success - handler called with correct input", func(t *testing.T) {
		bus := New()
		req := testRequest{}
		var capturedInput testRequest

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, r testRequest) error {
			capturedInput = r
			return nil
		}))

		err := bus.Dispatch(context.Background(), req)
		assert.NoError(t, err)
		assert.Equal(t, req, capturedInput)
	})

	t.Run("handler error is propagated", func(t *testing.T) {
		bus := New()
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return errHandlerFailed
		}))

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, err, errHandlerFailed)
	})

	t.Run("no handler returns error", func(t *testing.T) {
		bus := New()
		err := bus.Dispatch(context.Background(), testRequest{})
		assert.Error(t, err)
	})
}

// ── DispatchResult ───────────────────────────────────────────────────────────

func TestDispatchResult(t *testing.T) {
	t.Run("success - handler called with correct input and returns value", func(t *testing.T) {
		bus := New()
		req := testRequest{}
		expected := testResponse{Value: "ok"}
		var capturedInput testRequest

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, r testRequest) (testResponse, error) {
			capturedInput = r
			return expected, nil
		}))

		result, err := bus.DispatchResult[testResponse](context.Background(), req)
		assert.NoError(t, err)
		assert.Equal(t, req, capturedInput)
		assert.Equal(t, expected, result)
	})

	t.Run("handler error is propagated", func(t *testing.T) {
		bus := New()
		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, errHandlerFailed
		}))

		_, err := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.ErrorIs(t, err, errHandlerFailed)
	})

	t.Run("no handler returns error", func(t *testing.T) {
		bus := New()
		result, err := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.Error(t, err)
		assert.Equal(t, testResponse{}, result)
	})
}

// ── Use (Global Middleware) ──────────────────────────────────────────────────

func TestUse(t *testing.T) {
	t.Run("single middleware applied to Dispatch", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("middleware-enter")
				defer tracker.record("middleware-exit")
				return next(ctx, r)
			}
		})

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, []string{"middleware-enter", "handler", "middleware-exit"}, tracker.getOrder())
	})

	t.Run("single middleware applied to DispatchResult", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			tracker.record("handler")
			return testResponse{Value: "result"}, nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("middleware-enter")
				defer tracker.record("middleware-exit")
				return next(ctx, r)
			}
		})

		result, err := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, testResponse{Value: "result"}, result)
		assert.Equal(t, []string{"middleware-enter", "handler", "middleware-exit"}, tracker.getOrder())
	})

	t.Run("multiple middlewares execute in correct order", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		// Register first middleware
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("middleware-1-enter")
				defer tracker.record("middleware-1-exit")
				return next(ctx, r)
			}
		})

		// Register second middleware (should be wrapped by the first)
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("middleware-2-enter")
				defer tracker.record("middleware-2-exit")
				return next(ctx, r)
			}
		})

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err)
		// Execution order: middleware-1 enters (outer), then middleware-2 enters (inner), then handler, then middleware-2 exits, then middleware-1 exits
		assert.Equal(t, []string{
			"middleware-1-enter",
			"middleware-2-enter",
			"handler",
			"middleware-2-exit",
			"middleware-1-exit",
		}, tracker.getOrder())
	})

	t.Run("middleware can intercept and modify behavior", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("middleware-before")
				result, err := next(ctx, r)
				tracker.record("middleware-after")
				return result, err
			}
		})

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, []string{"middleware-before", "handler", "middleware-after"}, tracker.getOrder())
	})

	t.Run("middleware error is propagated from Dispatch", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				return nil, errMiddlewareFailed
			}
		})

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, err, errMiddlewareFailed)
	})

	t.Run("middleware error is propagated from DispatchResult", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				return testResponse{}, errMiddlewareFailed
			}
		})

		_, err := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.ErrorIs(t, err, errMiddlewareFailed)
	})

	t.Run("middleware receives correct context", func(t *testing.T) {
		bus := New()
		var capturedContext context.Context

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				capturedContext = ctx
				return next(ctx, r)
			}
		})

		ctx := context.WithValue(context.Background(), "test-key", "test-value")
		err := bus.Dispatch(ctx, testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, "test-value", capturedContext.Value("test-key"))
	})

	t.Run("middleware can prevent handler execution", func(t *testing.T) {
		bus := New()
		handlerCalled := false

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			handlerCalled = true
			return nil
		}))

		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				return nil, errMiddlewareIntercepted
			}
		})

		err := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, err, errMiddlewareIntercepted)
		assert.False(t, handlerCalled)
	})
}

// ── UseForHandler (Dispatch-Specific Middleware) ──────────────────────────────

func TestUseForHandler(t *testing.T) {
	t.Run("middleware wraps specific handler for Dispatch", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("middleware-enter")
				defer tracker.record("middleware-exit")
				return next(ctx, req)
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, []string{"middleware-enter", "handler", "middleware-exit"}, tracker.getOrder())
	})

	t.Run("multiple middlewares chain in reverse order", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		middleware1 := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("middleware-1-enter")
				defer tracker.record("middleware-1-exit")
				return next(ctx, req)
			}
		}

		middleware2 := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("middleware-2-enter")
				defer tracker.record("middleware-2-exit")
				return next(ctx, req)
			}
		}

		// Register both middlewares at once
		err := bus.UseForHandler[testRequest](middleware1, middleware2)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		// Execution order: middleware-1 enters (outer), then middleware-2 enters (inner), then handler, then middleware-2 exits, then middleware-1 exits
		assert.Equal(t, []string{
			"middleware-1-enter",
			"middleware-2-enter",
			"handler",
			"middleware-2-exit",
			"middleware-1-exit",
		}, tracker.getOrder())
	})

	t.Run("middleware can prevent handler execution", func(t *testing.T) {
		bus := New()
		handlerCalled := false

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			handlerCalled = true
			return nil
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				return errMiddlewareIntercepted
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, errMiddlewareIntercepted)
		assert.False(t, handlerCalled)
	})

	t.Run("middleware can transform errors", func(t *testing.T) {
		bus := New()
		transformedErr := errors.New("transformed error")

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return errHandlerFailed
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				err := next(ctx, req)
				if err != nil {
					return transformedErr
				}
				return nil
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, transformedErr)
	})

	t.Run("middleware error is propagated", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				return errMiddlewareFailed
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, errMiddlewareFailed)
	})

	t.Run("error when handler not found", func(t *testing.T) {
		bus := New()

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				return next(ctx, req)
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler not found")
	})

	t.Run("error when handler signature incompatible", func(t *testing.T) {
		bus := New()

		// Register a result handler, not a void handler
		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				return next(ctx, req)
			}
		}

		// UseForHandler looks for a void handler, but we only have a result handler
		err := bus.UseForHandler[testRequest](middleware)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler not found")
	})

	t.Run("middleware receives correct context", func(t *testing.T) {
		bus := New()
		var capturedContext context.Context

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		}))

		middleware := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				capturedContext = ctx
				return next(ctx, req)
			}
		}

		err := bus.UseForHandler[testRequest](middleware)
		require.NoError(t, err)

		ctx := context.WithValue(context.Background(), "test-key", "middleware-value")
		dispatchErr := bus.Dispatch(ctx, testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, "middleware-value", capturedContext.Value("test-key"))
	})
}

// ── UseForResultHandler (DispatchResult-Specific Middleware) ────────────────

func TestUseForResultHandler(t *testing.T) {
	t.Run("middleware wraps specific handler for DispatchResult", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			tracker.record("handler")
			return testResponse{Value: "result"}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				tracker.record("middleware-enter")
				defer tracker.record("middleware-exit")
				return next(ctx, req)
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, testResponse{Value: "result"}, result)
		assert.Equal(t, []string{"middleware-enter", "handler", "middleware-exit"}, tracker.getOrder())
	})

	t.Run("multiple middlewares chain in reverse order", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			tracker.record("handler")
			return testResponse{Value: "result"}, nil
		}))

		middleware1 := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				tracker.record("middleware-1-enter")
				defer tracker.record("middleware-1-exit")
				return next(ctx, req)
			}
		}

		middleware2 := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				tracker.record("middleware-2-enter")
				defer tracker.record("middleware-2-exit")
				return next(ctx, req)
			}
		}

		// Register both middlewares at once
		err := bus.UseForResultHandler[testRequest, testResponse](middleware1, middleware2)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, testResponse{Value: "result"}, result)
		// Execution order: middleware-1 enters (outer), then middleware-2 enters (inner), then handler, then middleware-2 exits, then middleware-1 exits
		assert.Equal(t, []string{
			"middleware-1-enter",
			"middleware-2-enter",
			"handler",
			"middleware-2-exit",
			"middleware-1-exit",
		}, tracker.getOrder())
	})

	t.Run("middleware can intercept and modify result", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{Value: "original"}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				result, err := next(ctx, req)
				if err == nil {
					// Modify the result
					result.Value = "modified-" + result.Value
				}
				return result, err
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, testResponse{Value: "modified-original"}, result)
	})

	t.Run("middleware can prevent handler execution", func(t *testing.T) {
		bus := New()
		handlerCalled := false

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			handlerCalled = true
			return testResponse{}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				return testResponse{Value: "intercepted"}, errMiddlewareIntercepted
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, errMiddlewareIntercepted)
		assert.Equal(t, testResponse{Value: "intercepted"}, result)
		assert.False(t, handlerCalled)
	})

	t.Run("middleware can transform errors", func(t *testing.T) {
		bus := New()
		transformedErr := errors.New("transformed error")

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, errHandlerFailed
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				result, err := next(ctx, req)
				if err != nil {
					return result, transformedErr
				}
				return result, nil
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		_, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, transformedErr)
	})

	t.Run("middleware error is propagated", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				return testResponse{}, errMiddlewareFailed
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		_, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.ErrorIs(t, dispatchErr, errMiddlewareFailed)
	})

	t.Run("error when handler not found", func(t *testing.T) {
		bus := New()

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				return next(ctx, req)
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler not found")
	})

	t.Run("error when handler signature incompatible", func(t *testing.T) {
		bus := New()

		// Register a void handler, not a result handler
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			return nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				return next(ctx, req)
			}
		}

		// UseForResultHandler looks for a result handler with testResponse type, but we only have a void handler
		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "handler not found")
	})

	t.Run("middleware receives correct context", func(t *testing.T) {
		bus := New()
		var capturedContext context.Context

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				capturedContext = ctx
				return next(ctx, req)
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		ctx := context.WithValue(context.Background(), "test-key", "result-middleware-value")
		_, dispatchErr := bus.DispatchResult[testResponse](ctx, testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, "result-middleware-value", capturedContext.Value("test-key"))
	})

	t.Run("middleware can return different result", func(t *testing.T) {
		bus := New()

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			return testResponse{Value: "original"}, nil
		}))

		middleware := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				// Skip calling the handler and return a different result
				return testResponse{Value: "override"}, nil
			}
		}

		err := bus.UseForResultHandler[testRequest, testResponse](middleware)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, testResponse{Value: "override"}, result)
	})
}

// ── Middleware Coexistence & Integration ─────────────────────────────────────

func TestMiddlewareCoexistence(t *testing.T) {
	t.Run("global and handler-specific middleware both execute", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		// Add global middleware
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("global-enter")
				defer tracker.record("global-exit")
				return next(ctx, r)
			}
		})

		// Add handler-specific middleware
		handler := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("specific-enter")
				defer tracker.record("specific-exit")
				return next(ctx, req)
			}
		}
		err := bus.UseForHandler[testRequest](handler)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		// Execution order: global enters, handler-specific enters, handler, handler-specific exits, global exits
		assert.Equal(t, []string{
			"global-enter",
			"specific-enter",
			"handler",
			"specific-exit",
			"global-exit",
		}, tracker.getOrder())
	})

	t.Run("handler-specific middleware does not affect other handlers", func(t *testing.T) {
		bus := New()
		tracker1 := &executionTracker{}
		tracker2 := &executionTracker{}

		// Register two different handler types
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker1.record("handler1")
			return nil
		}))

		// Different request type for second handler
		type secondRequest struct{}
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ secondRequest) error {
			tracker2.record("handler2")
			return nil
		}))

		// Add middleware only to the first handler type
		handler := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker1.record("specific-enter")
				defer tracker1.record("specific-exit")
				return next(ctx, req)
			}
		}
		err := bus.UseForHandler[testRequest](handler)
		require.NoError(t, err)

		// Dispatch first handler
		err1 := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err1)
		assert.Equal(t, []string{"specific-enter", "handler1", "specific-exit"}, tracker1.getOrder())

		// Dispatch second handler (middleware should NOT be applied)
		err2 := bus.Dispatch(context.Background(), secondRequest{})
		assert.NoError(t, err2)
		assert.Equal(t, []string{"handler2"}, tracker2.getOrder())
	})

	t.Run("global middleware applies to all handlers", func(t *testing.T) {
		bus := New()
		tracker1 := &executionTracker{}
		tracker2 := &executionTracker{}

		// Register two different handler types
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker1.record("handler1")
			return nil
		}))

		type secondRequest struct{}
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ secondRequest) error {
			tracker2.record("handler2")
			return nil
		}))

		// Add global middleware that records execution
		middlewareCallCount := 0
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				middlewareCallCount++
				if middlewareCallCount == 1 {
					tracker1.record("global")
				} else {
					tracker2.record("global")
				}
				return next(ctx, r)
			}
		})

		// Dispatch first handler
		err1 := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err1)
		assert.Equal(t, []string{"global", "handler1"}, tracker1.getOrder())

		// Dispatch second handler (global middleware should also be applied)
		err2 := bus.Dispatch(context.Background(), secondRequest{})
		assert.NoError(t, err2)
		assert.Equal(t, []string{"global", "handler2"}, tracker2.getOrder())
	})

	t.Run("global middleware for DispatchResult works with handler-specific middleware", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			tracker.record("handler")
			return testResponse{Value: "result"}, nil
		}))

		// Add global middleware
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("global-enter")
				defer tracker.record("global-exit")
				return next(ctx, r)
			}
		})

		// Add handler-specific middleware
		handler := func(next func(context.Context, testRequest) (testResponse, error)) func(context.Context, testRequest) (testResponse, error) {
			return func(ctx context.Context, req testRequest) (testResponse, error) {
				tracker.record("specific-enter")
				defer tracker.record("specific-exit")
				return next(ctx, req)
			}
		}
		err := bus.UseForResultHandler[testRequest, testResponse](handler)
		require.NoError(t, err)

		result, dispatchErr := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		assert.Equal(t, testResponse{Value: "result"}, result)
		// Execution order: global enters, handler-specific enters, handler, handler-specific exits, global exits
		assert.Equal(t, []string{
			"global-enter",
			"specific-enter",
			"handler",
			"specific-exit",
			"global-exit",
		}, tracker.getOrder())
	})

	t.Run("middleware chain works with Dispatch and DispatchResult coexisting", func(t *testing.T) {
		bus := New()
		dispatchTracker := &executionTracker{}
		resultTracker := &executionTracker{}

		// Register both a void handler and a result handler
		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			dispatchTracker.record("dispatch-handler")
			return nil
		}))

		require.NoError(t, bus.RegisterResultHandler(func(_ context.Context, _ testRequest) (testResponse, error) {
			resultTracker.record("result-handler")
			return testResponse{Value: "result"}, nil
		}))

		// Global middleware captures both with separate tracking
		middlewareCallCount := 0
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				middlewareCallCount++
				if middlewareCallCount == 1 {
					dispatchTracker.record("global")
				} else {
					resultTracker.record("global")
				}
				return next(ctx, r)
			}
		})

		// Dispatch the void handler
		err := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, []string{"global", "dispatch-handler"}, dispatchTracker.getOrder())

		// DispatchResult the result handler
		result, err := bus.DispatchResult[testResponse](context.Background(), testRequest{})
		assert.NoError(t, err)
		assert.Equal(t, testResponse{Value: "result"}, result)
		assert.Equal(t, []string{"global", "result-handler"}, resultTracker.getOrder())
	})

	t.Run("multiple global middlewares with multiple specific middlewares", func(t *testing.T) {
		bus := New()
		tracker := &executionTracker{}

		require.NoError(t, bus.RegisterHandler(func(_ context.Context, _ testRequest) error {
			tracker.record("handler")
			return nil
		}))

		// Add first global middleware
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("global-1-enter")
				defer tracker.record("global-1-exit")
				return next(ctx, r)
			}
		})

		// Add second global middleware
		bus.Use(func(next ResponseHandler) ResponseHandler {
			return func(ctx context.Context, r any) (any, error) {
				tracker.record("global-2-enter")
				defer tracker.record("global-2-exit")
				return next(ctx, r)
			}
		})

		// Add handler-specific middlewares
		specific1 := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("specific-1-enter")
				defer tracker.record("specific-1-exit")
				return next(ctx, req)
			}
		}
		specific2 := func(next func(context.Context, testRequest) error) func(context.Context, testRequest) error {
			return func(ctx context.Context, req testRequest) error {
				tracker.record("specific-2-enter")
				defer tracker.record("specific-2-exit")
				return next(ctx, req)
			}
		}

		err := bus.UseForHandler[testRequest](specific1, specific2)
		require.NoError(t, err)

		dispatchErr := bus.Dispatch(context.Background(), testRequest{})
		assert.NoError(t, dispatchErr)
		// Order: global-1 (outermost global), global-2, specific-1 (outermost specific), specific-2, handler
		assert.Equal(t, []string{
			"global-1-enter",
			"global-2-enter",
			"specific-1-enter",
			"specific-2-enter",
			"handler",
			"specific-2-exit",
			"specific-1-exit",
			"global-2-exit",
			"global-1-exit",
		}, tracker.getOrder())
	})
}

func Test_DispatchResultCanInferTypeRequest(t *testing.T) {

	type testRequest struct {
		ID string
	}
	testResponseHandler := func(ctx context.Context, req testRequest) (string, error) {
		return req.ID, nil
	}

	bus := New()
	assert.NoError(t, bus.RegisterResultHandler(testResponseHandler))

	res, err := bus.DispatchResult[string](context.Background(), testRequest{ID: "result"})
	assert.NoError(t, err)
	assert.Equal(t, "result", res)
}
