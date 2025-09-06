# RequestBus for go

This package contains a request bus for go with some Generic handlers.
Its main purpose is to provide a simple way to call handlers based on a request structure like in CQRS.

### Example usage

Register handlers
```go
    type testRequest struct {}
    
    bus := requestbus.New()
    
    //register handler that only returns an error (usually used for commands in CQRS)
    bus.RegisterHandler(func(ctx context.Context, cmd testRequest) error { return nil })
    
    //register handler that returns a value and an error (usually used for queries in CQRS)
    bus.RegisterHandler(func(ctx context.Context, cmd testRequest) (int, error) { return 123, nil })
```

Call the bus
```go
    //call handler as a command (only returns an error)
    err := bus.Dispatch(context.Background(), testRequest{})
    
    //call handler as a query (returns a value and an error)
    result, err := bus.DispatchWithResult(context.Background(), testRequest{})
    
    //call handler with return type hint, required if multiple handlers with different return types are registered for the same request structure
    result, err := bus.DispatchWithTypedResult(context.Background(), testRequest{}, int(0)
```

Call the bus with generic helper.
The bus needs to be passed as a context value.
```go
    //add the bus to the context
    ctx := ContextWithBus(context.Background(), bus)
    
    //call handler as a command (only returns an error)
    err := requestbus.Dispatch[int](ctx, testRequest{})
    
	//call handler as a query (returns a value and an error)
	//Provide the expected return type as a generic type parameter.
	//There is now no need to cast the result.
	res, err := requestbus.DispatchWithResult[int](ctx, testRequest{})
```

### For more information
Please check the bus_test.go file for more examples.