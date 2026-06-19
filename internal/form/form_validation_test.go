package form

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestFormValidation_RequiredTextField_EmptySubmit(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" required name="test"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	formNode := findFirstNode(doc, "form")
	if formNode == nil {
		t.Fatal("Form node not found")
	}
	valErr := v.ValidateForm(formNode)
	if valErr == nil {
		t.Fatal("Expected validation error for empty required field")
	}
	if !strings.Contains(valErr.Error(), "required") {
		t.Fatalf("Expected error message to contain 'required', got: %v", valErr.Error())
	}
}

func TestFormValidation_RequiredTextField_WithValue(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" required value="valid input"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_RequiredCheckbox_Unchecked(t *testing.T) {
	htmlContent := `<html><body><form><input type="checkbox" required></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestFormValidation_RequiredCheckbox_Checked(t *testing.T) {
	htmlContent := `<html><body><form><input type="checkbox" required checked></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_RequiredSelect_NoSelection(t *testing.T) {
	htmlContent := `<html><body><form><select required><option value="">Select...</option><option value="a">A</option></select></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestFormValidation_RequiredSelect_WithSelection(t *testing.T) {
	htmlContent := `<html><body><form><select required><option value="">Select...</option><option value="a" selected>A</option></select></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_EmailPattern_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="email" value="user@example.com"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_EmailPattern_Invalid(t *testing.T) {
	htmlContent := `<html><body><form><input type="email" value="notanemail"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email")
}

func TestFormValidation_PhonePattern_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="tel" pattern="^\([0-9]{3}\) [0-9]{3}-[0-9]{4}$" value="(555) 123-4567"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_PhonePattern_Invalid(t *testing.T) {
	htmlContent := `<html><body><form><input type="tel" pattern="^\([0-9]{3}\) [0-9]{3}-[0-9]{4}$" value="abc123"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
}

func TestFormValidation_CustomPattern_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" pattern="^[A-Z]{2}[0-9]{4}$" value="AB1234"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_CustomPattern_Invalid(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" pattern="^[A-Z]{2}[0-9]{4}$" value="ab1234"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
}

func TestFormValidation_MinLength_TooShort(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" minlength="5" value="abc"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minlength")
}

func TestFormValidation_MinLength_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" minlength="5" value="abcdef"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MaxLength_TooLong(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" maxlength="10" value="12345678901"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maxlength")
}

func TestFormValidation_MaxLength_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" maxlength="10" value="1234567890"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MinMaxLength_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" minlength="3" maxlength="5" value="abcd"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MinNumber_TooLow(t *testing.T) {
	htmlContent := `<html><body><form><input type="number" min="18" value="16"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
}

func TestFormValidation_MinNumber_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="number" min="18" value="25"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MaxNumber_TooHigh(t *testing.T) {
	htmlContent := `<html><body><form><input type="number" max="100" value="150"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum")
}

func TestFormValidation_MaxNumber_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="number" max="100" value="75"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MinDate_BeforeRange(t *testing.T) {
	htmlContent := `<html><body><form><input type="date" min="2024-01-01" value="2023-06-15"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
}

func TestFormValidation_MinDate_Valid(t *testing.T) {
	htmlContent := `<html><body><form><input type="date" min="2024-01-01" value="2024-05-01"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.NoError(t, err)
}

func TestFormValidation_MultipleFields_AllInvalid(t *testing.T) {
	htmlContent := `<html><body>
		<form>
			<input type="text" required value="">
			<input type="email" required value="invalid">
			<input type="text" minlength="5" value="ab">
		</form>
	</body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	errMsg := err.Error()
	assert.Contains(t, errMsg, "required")
	assert.Contains(t, errMsg, "email")
	assert.Contains(t, errMsg, "minlength")
}

func TestFormValidation_PasswordStrength(t *testing.T) {
	htmlContent := `<html><body><form><input type="password" minlength="8" value="short"></form></body></html>`
	v := NewFormValidator()
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)
	err = v.ValidateForm(formNode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minlength")
}

func findFirstNode(n *html.Node, tagName string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tagName {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findFirstNode(c, tagName); result != nil {
			return result
		}
	}
	return nil
}
