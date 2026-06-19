package form

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestFormSubmission_SubmitButton_Click(t *testing.T) {
	htmlContent := `<html><body><form id="test-form"><input name="username" value="testuser"><button type="submit">Submit</button></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	submitted := false
	state.SetSubmitCallback(func(data FormData) { submitted = true })
	state.Submit()
	assert.True(t, submitted, "Submit callback should be called on button click")
}

func TestFormSubmission_EnterKey_Submit(t *testing.T) {
	htmlContent := `<html><body><form id="test-form"><input name="search" value="query"><input type="text"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	submitted := false
	state.SetSubmitCallback(func(data FormData) { submitted = true })
	state.TriggerEnterKey()
	assert.True(t, submitted, "Submit should be triggered on Enter key")
}

func TestFormSubmission_CancelViaPreventDefault(t *testing.T) {
	htmlContent := `<html><body><form id="test-form"><input name="name" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	submitted := false
	state.SetSubmitCallback(func(data FormData) { submitted = true })
	state.SetCancelCallback(func() { state.CancelSubmission() })
	state.CancelSubmission()
	assert.False(t, submitted, "Submit should be cancelled")
}

func TestFormSubmission_MultiSubmit_Prevention(t *testing.T) {
	htmlContent := `<html><body><form id="test-form"><input name="email" value="test@example.com"><button type="submit">Submit</button></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	submissionCount := 0
	state.SetSubmitCallback(func(data FormData) { submissionCount++ })
	state.Submit()
	state.Submit()
	assert.Equal(t, 1, submissionCount, "Only one submission should occur (double-click prevention)")
}

func TestFormData_TextInput(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" name="username" value="John"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "John", data.Get("username"))
}

func TestFormData_PasswordInput(t *testing.T) {
	htmlContent := `<html><body><form><input type="password" name="password" value="secret123"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "secret123", data.Get("password"))
}

func TestFormData_Checkbox(t *testing.T) {
	htmlContent := `<html><body><form><input type="checkbox" name="agree" checked></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "on", data.Get("agree"), "Checked checkbox should have 'on' value")
}

func TestFormData_RadioGroup(t *testing.T) {
	htmlContent := `<html><body><form>
		<input type="radio" name="plan" value="basic">
		<input type="radio" name="plan" value="premium" checked>
		<input type="radio" name="plan" value="enterprise">
	</form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "premium", data.Get("plan"), "Selected radio should be included")
}

func TestFormData_Select(t *testing.T) {
	htmlContent := `<html><body><form><select name="country"><option value="">Select...</option><option value="us" selected>United States</option></select></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "us", data.Get("country"))
}

func TestFormData_Textarea(t *testing.T) {
	htmlContent := `<html><body><form><textarea name="message">Hello World</textarea></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "Hello World", data.Get("message"))
}

func TestFormData_MultipleInputs(t *testing.T) {
	htmlContent := `<html><body><form>
		<input type="text" name="firstname" value="John">
		<input type="text" name="lastname" value="Doe">
		<input type="email" name="email" value="john@example.com">
	</form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, "John", data.Get("firstname"))
	assert.Equal(t, "Doe", data.Get("lastname"))
	assert.Equal(t, "john@example.com", data.Get("email"))
}

func TestFormData_DisabledField(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" name="disabled" value="cant submit" disabled><input type="text" name="enabled" value="can submit"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, exists := data["disabled"]
	assert.False(t, exists, "Disabled field should not be in form data")
	assert.Equal(t, "can submit", data.Get("enabled"))
}

func TestFormData_UncheckedCheckbox(t *testing.T) {
	htmlContent := `<html><body><form><input type="checkbox" name="optin"><input type="text" name="name" value="John"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, exists := data["optin"]
	assert.False(t, exists, "Unchecked checkbox should not be in form data")
	assert.Equal(t, "John", data.Get("name"))
}

func TestHTTPGet_Submission(t *testing.T) {
	htmlContent := `<html><body><form method="GET" action="/api/search"><input name="q" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	result, err := submitter.Submit(formNode, data)
	require.NoError(t, err)
	assert.Equal(t, "GET", result.Method)
	assert.Contains(t, result.URL, "/api/search?q=test")
}

func TestHTTPPost_Submission(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/user"><input name="name" value="John"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	result, err := submitter.Submit(formNode, data)
	require.NoError(t, err)
	assert.Equal(t, "POST", result.Method)
	assert.Equal(t, "John", result.Body.Get("name"))
}

func TestHTTPResponse_Success(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/submit"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	onSuccess := false
	submitter.SetSuccessCallback(func(r *SubmissionResult) { onSuccess = true })
	_, err = submitter.Submit(formNode, data)
	require.NoError(t, err)
	assert.True(t, onSuccess, "Success callback should be called on 200 OK")
}

func TestHTTPResponse_Error(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/invalid"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	onError := false
	submitter.SetErrorCallback(func(err error) { onError = true })
	_, err = submitter.Submit(formNode, data)
	assert.Error(t, err)
	assert.True(t, onError, "Error callback should be called on 400")
}

func TestHTTPTimeout(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="http://slow-server.example.com/api"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, err = submitter.Submit(formNode, data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}
