package requestbus

type Option func(*RequestBus)

func WithNameResolver(nameResolver func(req any) string) func(bus *RequestBus) {
	return func(bus *RequestBus) {
		bus.nameResolver = nameResolver
	}
}

func WithTypeResolver(typeResolver func(req any) string) func(bus *RequestBus) {
	return func(bus *RequestBus) {
		bus.typeResolver = typeResolver
	}
}
