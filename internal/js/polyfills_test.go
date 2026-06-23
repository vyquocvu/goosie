package js

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromiseResolve(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.RunScript(`
		var result = "";
		Promise.resolve(42).then(function(v) { result = "got:" + v; });
	`)
	assert.NoError(t, err)
	val, err := rt.RunScript(`result`)
	assert.NoError(t, err)
	assert.Equal(t, "got:42", val.String())
}

func TestPromiseAll(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.RunScript(`
		var out = "";
		Promise.all([Promise.resolve(1), Promise.resolve(2)]).then(function(vs) {
			out = vs.join(",");
		});
	`)
	assert.NoError(t, err)
	// Read the variable after microtasks have been flushed
	val, err := rt.RunScript(`out`)
	assert.NoError(t, err)
	assert.Equal(t, "1,2", val.String())
}

func TestMapBasic(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`
		var m = new Map();
		m.set("key", "value");
		m.get("key");
	`)
	assert.NoError(t, err)
	assert.Equal(t, "value", val.String())
}

func TestSetBasic(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`
		var s = new Set([1, 2, 3, 2, 1]);
		s.size;
	`)
	assert.NoError(t, err)
	assert.Equal(t, "3", val.String())
}

func TestArrayFrom(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`Array.from([1,2,3]).join(",")`)
	assert.NoError(t, err)
	assert.Equal(t, "1,2,3", val.String())
}

func TestObjectEntries(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`Object.entries({a:1,b:2}).length`)
	assert.NoError(t, err)
	assert.Equal(t, "2", val.String())
}

func TestStringIncludes(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`"hello world".includes("world")`)
	assert.NoError(t, err)
	assert.Equal(t, "true", val.String())
}
