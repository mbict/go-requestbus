package requestbus

import (
	"fmt"
	"reflect"
)

type RequestName interface {
	RequestName() string
}

// DefaultNameResolver is a helper for extracting the request name
var DefaultNameResolver = func(req any) string {
	switch m := req.(type) {
	case RequestName:
		return m.RequestName()
	}
	return DefaultTypeResolver(req)
}

// DefaultTypeResolver is a helper for extracting the type name from a return type
var DefaultTypeResolver = func(req any) string {
	return fmt.Sprintf("%T", req)
}

// ReflectionNameResolver is a helper for extracting the request name using reflection
func ReflectionNameResolver(req any) string {
	t := reflect.TypeOf(req)
	if t.Kind() == reflect.Ptr {
		return t.Elem().PkgPath() + "/*" + t.Elem().Name()
	}
	return t.PkgPath() + "/" + t.Name()
}
