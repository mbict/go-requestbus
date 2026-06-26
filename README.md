# RequestBus for Go v2

A request bus for Go with generic handlers, designed to provide a simple way to dispatch requests to handlers — similar to CQRS command/query dispatch.

> **Requires Go 1.27+** — v2 uses generic methods on struct types, which requires Go 1.27.

## Installation

```go
import "github.com/mbict/go-requestbus/v2"
```

## Handler registration

```go
type myCommand struct{}
type myQuery  struct{}
type myResult struct{ Value string }

bus := requestbus.New()

// Command handler — only returns an error
bus.RegisterHandler(func(ctx context.Context, cmd myCommand) error {
    return nil
})

// Query handler — returns a value and an error
bus.RegisterResultHandler(func(ctx context.Context, q myQuery) (myResult, error) {
    return myResult{Value: "hello"}, nil
})
```

Multiple handlers for the same request type can coexist as long as their return types differ.

## Dispatching

```go
// Dispatch a command (void handler)
err := bus.Dispatch(ctx, myCommand{})

// Dispatch a query — query type is inferred from type parameters, return type is inferred from handler signature and is something you need to provide as a type parameter.
result, err := bus.DispatchResult[myResult](ctx, myQuery{})
```

## Middleware

### Global middleware

Applied to every `Dispatch` and `DispatchResult` call.

```go
bus.Use(func(next requestbus.ResponseHandler) requestbus.ResponseHandler {
    return func(ctx context.Context, r any) (any, error) {
        log.Println("before")
        res, err := next(ctx, r)
        log.Println("after")
        return res, err
    }
})
```

Multiple calls to `Use` chain middlewares in order (first registered = outermost).

### Handler-specific middleware

Scoped to a single handler signature, with full type safety.

```go
// For a void (command) handler
bus.UseForHandler[myCommand](func(next func(context.Context, myCommand) error) func(context.Context, myCommand) error {
    return func(ctx context.Context, cmd myCommand) error {
        log.Println("before command")
        return next(ctx, cmd)
    }
})

// For a result (query) handler
bus.UseForResultHandler[myQuery, myResult](func(next func(context.Context, myQuery) (myResult, error)) func(context.Context, myQuery) (myResult, error) {
    return func(ctx context.Context, q myQuery) (myResult, error) {
        log.Println("before query")
        return next(ctx, q)
    }
})
```

### Middleware execution order

When both global and handler-specific middleware are registered:

```
global-1 → global-2 → specific-1 → specific-2 → handler
```

## Custom name / type resolvers

By default the bus uses the Go type name to identify handlers. You can override this with options:

```go
bus := requestbus.New(
    requestbus.WithNameResolver(requestbus.ReflectionNameResolver),
    requestbus.WithTypeResolver(func(req any) string {
        return fmt.Sprintf("%T", req)
    }),
)
```

Requests can also self-identify by implementing the `RequestName` interface:

```go
func (r myCommand) RequestName() string { return "my-custom-command" }
```

## More examples

See [`bus_test.go`](bus_test.go) for comprehensive examples covering all features.
