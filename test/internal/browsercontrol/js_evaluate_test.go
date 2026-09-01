package browsercontrol_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// TestJS_Evaluate_Basic tests basic JavaScript evaluation
func TestJS_Evaluate_Basic(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Test string return
	result, err := ec.Evaluate(ctx, `"hello world"`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "string", result.Type)
	assert.Equal(t, "hello world", result.Value)
	assert.False(t, result.IsError)
}

// TestJS_Evaluate_Number tests number returns
func TestJS_Evaluate_Number(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `42 + 8`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "number", result.Type)
	// Result should be 50 (either int or float depending on implementation)
	if intVal, ok := result.Value.(int64); ok {
		assert.Equal(t, int64(50), intVal)
	} else if floatVal, ok := result.Value.(float64); ok {
		assert.InDelta(t, 50.0, floatVal, 0.001)
	}
}

// TestJS_Evaluate_Boolean tests boolean returns
func TestJS_Evaluate_Boolean(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `true && false`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "boolean", result.Type)
	assert.Equal(t, false, result.Value)
}

// TestJS_Evaluate_Object tests object returns
func TestJS_Evaluate_Object(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `({name: "test", value: 42})`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "object", result.Type)
	// Export() returns a map for objects
	if obj, ok := result.Value.(map[string]interface{}); ok {
		assert.Equal(t, "test", obj["name"])
	}
}

// TestJS_Evaluate_Array tests array returns
func TestJS_Evaluate_Array(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `[1, 2, 3]`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "object", result.Type) // Arrays are objects in JS
}

// TestJS_Evaluate_Null tests null return
func TestJS_Evaluate_Null(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `null`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "null", result.Type)
	assert.Nil(t, result.Value)
}

// TestJS_Evaluate_Undefined tests undefined return
func TestJS_Evaluate_Undefined(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `(function() {})()`, browsercontrol.EvaluateOptions{}) // returns undefined
	require.NoError(t, err)
	assert.Equal(t, "undefined", result.Type)
}

// TestJS_Evaluate_Error tests JavaScript error handling
func TestJS_Evaluate_Error(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `throw new Error("test error")`, browsercontrol.EvaluateOptions{})
	// Error is returned in result, not as Go error
	require.NoError(t, err) // The Evaluate call itself succeeds
	assert.True(t, result.IsError)
	assert.NotEmpty(t, result.ErrorText)
}

// TestJS_Evaluate_SyntaxError tests syntax error handling
func TestJS_Evaluate_SyntaxError(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `{invalid syntax`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestJS_Evaluate_SourceLengthLimit tests source length limit
func TestJS_Evaluate_SourceLengthLimit(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Create source larger than MaxSourceBytes
	longSource := make([]byte, browsercontrol.MaxSourceBytes+100)
	for i := range longSource {
		longSource[i] = 'x'
	}

	result, err := ec.Evaluate(ctx, string(longSource), browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.ErrorText, "exceeds maximum length")
}

// TestJS_Evaluate_ContextID tests that context ID is in result
func TestJS_Evaluate_ContextID(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `"test"`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, info.ID, result.ContextID)
}

// TestJS_Evaluate_PageRevision tests that page revision is in result
func TestJS_Evaluate_PageRevision(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `"test"`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.PageRevision) // No navigation yet
}

// TestJS_Evaluate_Cancellation tests that context cancellation works
func TestJS_Evaluate_Cancellation(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Create cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = ec.Evaluate(cancelledCtx, `"test"`, browsercontrol.EvaluateOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestJS_Evaluate_Function tests function definition and call
func TestJS_Evaluate_Function(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Define and call a function
	result, err := ec.Evaluate(ctx, `(function(x, y) { return x + y; })(5, 3)`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "number", result.Type)
	if intVal, ok := result.Value.(int64); ok {
		assert.Equal(t, int64(8), intVal)
	}
}

// TestJS_Evaluate_StringLength tests string manipulation
func TestJS_Evaluate_StringLength(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Evaluate(ctx, `"hello".toUpperCase()`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "string", result.Type)
	assert.Equal(t, "HELLO", result.Value)
}

// TestJS_Evaluate_ConsoleLog tests that console.log doesn't break
func TestJS_Evaluate_ConsoleLog(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// console.log should not cause an error
	result, err := ec.Evaluate(ctx, `console.log("hello"); 42`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, int64(42), result.Value)
}

// TestJS_Evaluate_NavigateDOM tests that Navigate loads DOM and origin into jsRuntime
func TestJS_Evaluate_NavigateDOM(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><head><title>Goosie Page</title></head><body><h1 id=\"headline\">Welcome to Goosie</h1><div class=\"content\"><p>Paragraph text</p></div></body></html>"))
	})
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitInteractive, 1000)
	require.NoError(t, err)

	// Verify Evaluate interacts with the loaded DOM
	resTitle, err := ec.Evaluate(ctx, `document.title`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Goosie Page", resTitle.Value)

	resHeadline, err := ec.Evaluate(ctx, `document.getElementById("headline").textContent`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Welcome to Goosie", resHeadline.Value)

	resContent, err := ec.Evaluate(ctx, `document.querySelector(".content p").textContent`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Paragraph text", resContent.Value)

	resOrigin, err := ec.Evaluate(ctx, `document.location.href`, browsercontrol.EvaluateOptions{})
	require.NoError(t, err)
	assert.Equal(t, srv.URL, resOrigin.Value)
}
