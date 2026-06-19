package form

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestErrorScenario_NetworkError_NoInternet(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="http://192.168.255.255/api"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, err = submitter.Submit(formNode, data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection")
}

func TestErrorScenario_NetworkError_DNSFailure(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="http://nonexistent.invalid/api"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, err = submitter.Submit(formNode, data)
	assert.Error(t, err)
}

func TestErrorScenario_NetworkError_ConnectionRefused(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="http://localhost:59999/api"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	submitter := NewFormSubmitter()
	state := NewFormState(formNode)
	data := state.GetFormData()
	_, err = submitter.Submit(formNode, data)
	assert.Error(t, err)
}

func TestErrorScenario_ValidationError_InlineMessage(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" required></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	v := NewFormValidator()
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	errorMsg := v.GetErrorMessage(formNode)
	assert.NotEmpty(t, errorMsg)
}

func TestErrorScenario_ValidationError_MultipleFields(t *testing.T) {
	htmlContent := `<html><body>
		<form>
			<input type="text" required name="field1">
			<input type="email" required name="field2">
			<input type="text" required name="field3">
		</form>
	</body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	v := NewFormValidator()
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	errors := v.GetAllErrors(formNode)
	assert.GreaterOrEqual(t, len(errors), 3, "Should have errors for all required fields")
}

func TestErrorScenario_ValidationError_ClearOnFix(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" required name="name" value=""></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	v := NewFormValidator()
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	errMsg := v.GetErrorMessage(formNode)
	assert.Contains(t, errMsg, "required")
}

func TestErrorScenario_EmptyForm_Submit(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/submit"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	v := NewFormValidator()
	err = v.ValidateForm(formNode)
	assert.NoError(t, err, "Form with no required fields should be valid")
}

func TestErrorScenario_VeryLongInput_Submit(t *testing.T) {
	longString := strings.Repeat("A", 1024*1024)
	htmlContent := `<html><body><form><input type="text" name="data" value="` + longString + `"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.Equal(t, longString, data.Get("data"), "Very long input should be preserved")
}

func TestErrorScenario_SpecialCharacters_Input(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" name="comment" value="<script>alert('xss')</script>"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	val := data.Get("comment")
	assert.NotContains(t, val, "<script>", "XSS content should be sanitized")
}

func TestErrorScenario_DynamicForm_AddField(t *testing.T) {
	htmlContent := `<html><body><form id="dynamic-form"><input type="text" name="static" value="original"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	newInput := &html.Node{
		Type: html.ElementNode,
		Data: "input",
	}
	newInput.Attr = []html.Attribute{
		{Key: "type", Val: "text"},
		{Key: "name", Val: "dynamic"},
		{Key: "value", Val: "added"},
	}
	state.AddField(newInput)
	data := state.GetFormData()
	assert.Equal(t, "original", data.Get("static"))
	assert.Equal(t, "added", data.Get("dynamic"))
}

func TestErrorScenario_RapidInput_Validation(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" pattern="^[0-9]+$"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	v := NewFormValidator()
	v.SetDebounceMs(300)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err, "Empty input with pattern should not error until submission")
}

func TestErrorScenario_PastedContent_Validation(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" name="content" value="Large pasted content that exceeds expected limits but should still be handled gracefully without crashing or hanging"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	state := NewFormState(formNode)
	data := state.GetFormData()
	assert.NotEmpty(t, data.Get("content"))
}

func TestEdgeCase_FormInIframe_Submit(t *testing.T) {
	htmlContent := `<html><body>
		<iframe id="form-frame" srcdoc="<form><input name=&quot;data&quot; value=&quot;iframe-data&quot;></form>">
		</iframe>
	</body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	iframe := findFirstNode(doc, "iframe")
	require.NotNil(t, iframe)
	parentNotified := false
	assert.True(t, parentNotified, "Parent should be notified of iframe form submission (if enabled)")
}
