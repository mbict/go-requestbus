package requestbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultNameResolver(t *testing.T) {
	assert.Equal(t, "requestbus.testRequest", DefaultNameResolver(testRequest{}))
}

func TestDefaultNameResolver_WithCustomNameImplementation(t *testing.T) {
	assert.Equal(t, "custom-name", DefaultNameResolver(testRequestWithName{}))
}

func TestDefaultTypeResolver(t *testing.T) {
	assert.Equal(t, "requestbus.testRequest", DefaultTypeResolver(testRequest{}))
	assert.Equal(t, "requestbus.testRequestWithName", DefaultTypeResolver(testRequestWithName{}))
	assert.Equal(t, "int", DefaultTypeResolver(int(0)))
}

func TestReflectionNameResolver(t *testing.T) {
	assert.Equal(t, "github.com/mbict/go-requestbus/v2/testRequest", ReflectionNameResolver(testRequest{}))
	assert.Equal(t, "github.com/mbict/go-requestbus/v2/testRequestWithName", ReflectionNameResolver(testRequestWithName{}))
	assert.Equal(t, "/int", ReflectionNameResolver(int(0)))
	assert.Equal(t, "/*int", ReflectionNameResolver((*int)(nil)))
}
