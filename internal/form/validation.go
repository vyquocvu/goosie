package form

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type MultiValidationError struct {
	Errors []ValidationError
}

func (e *MultiValidationError) Error() string {
	if len(e.Errors) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

type FormValidator struct {
	debounceMs int
}

func NewFormValidator() *FormValidator {
	return &FormValidator{
		debounceMs: 300,
	}
}

func (v *FormValidator) SetDebounceMs(ms int) {
	v.debounceMs = ms
}

func (v *FormValidator) ValidateForm(formNode *html.Node) error {
	if formNode == nil {
		return &ValidationError{Field: "form", Message: "form node is nil"}
	}
	var errors []ValidationError
	inputs := findFormInputs(formNode)
	for _, input := range inputs {
		errs := v.validateNode(input)
		errors = append(errors, errs...)
	}
	if len(errors) > 0 {
		return &MultiValidationError{Errors: errors}
	}
	return nil
}

func (v *FormValidator) validateNode(input *html.Node) []ValidationError {
	tag := strings.ToLower(input.Data)
	name := getAttrValue(input, "name")
	if name == "" {
		name = "field"
	}

	var errors []ValidationError

	switch tag {
	case "input":
		inputType := strings.ToLower(getAttrValue(input, "type"))
		if inputType == "" {
			inputType = "text"
		}
		switch inputType {
		case "checkbox", "radio":
			if hasAttr(input, "required") && !hasAttr(input, "checked") {
				errors = append(errors, ValidationError{Field: name, Message: "required"})
			}
		default:
			value := getAttrValue(input, "value")
			if hasAttr(input, "required") && value == "" {
				errors = append(errors, ValidationError{Field: name, Message: "required"})
			}
			errors = append(errors, v.validateValue(input, inputType, name, value)...)
		}

	case "select":
		if hasAttr(input, "required") {
			selected := findSelectedOption(input)
			if selected == "" {
				errors = append(errors, ValidationError{Field: name, Message: "required"})
			}
		}

	case "textarea":
		value := getTextareaValue(input)
		if hasAttr(input, "required") && strings.TrimSpace(value) == "" {
			errors = append(errors, ValidationError{Field: name, Message: "required"})
		}
		if minLen := getIntAttrValue(input, "minlength"); minLen > 0 && len(value) < minLen {
			errors = append(errors, ValidationError{Field: name, Message: "minlength"})
		}
		if maxLen := getIntAttrValue(input, "maxlength"); maxLen > 0 && len(value) > maxLen {
			errors = append(errors, ValidationError{Field: name, Message: "maxlength"})
		}
	}

	return errors
}

func (v *FormValidator) validateValue(input *html.Node, inputType, name, value string) []ValidationError {
	var errors []ValidationError

	switch inputType {
	case "email":
		if value != "" && !isValidEmail(value) {
			errors = append(errors, ValidationError{Field: name, Message: "invalid email format"})
		}
	case "url":
		if value != "" && !isValidURL(value) {
			errors = append(errors, ValidationError{Field: name, Message: "invalid URL format"})
		}
	}

	if pattern := getAttrValue(input, "pattern"); pattern != "" && value != "" {
		if !matchPattern("^(?:"+pattern+")$", value) {
			errors = append(errors, ValidationError{Field: name, Message: "pattern mismatch"})
		}
	}

	if minLen := getIntAttrValue(input, "minlength"); minLen > 0 && len(value) < minLen {
		errors = append(errors, ValidationError{Field: name, Message: "minlength"})
	}
	if maxLen := getIntAttrValue(input, "maxlength"); maxLen > 0 && len(value) > maxLen {
		errors = append(errors, ValidationError{Field: name, Message: "maxlength"})
	}

	if inputType == "number" || inputType == "range" {
		numVal := parseFloat(value)
		if min := getAttrValue(input, "min"); min != "" {
			if minF := parseFloat(min); value != "" && numVal < minF {
				errors = append(errors, ValidationError{Field: name, Message: "minimum value"})
			}
		}
		if max := getAttrValue(input, "max"); max != "" {
			if maxF := parseFloat(max); value != "" && numVal > maxF {
				errors = append(errors, ValidationError{Field: name, Message: "maximum value"})
			}
		}
	}

	if inputType == "date" {
		if minDate := getAttrValue(input, "min"); minDate != "" && value != "" && value < minDate {
			errors = append(errors, ValidationError{Field: name, Message: "minimum date"})
		}
		if maxDate := getAttrValue(input, "max"); maxDate != "" && value != "" && value > maxDate {
			errors = append(errors, ValidationError{Field: name, Message: "maximum date"})
		}
	}

	return errors
}

func (v *FormValidator) ValidateInput(input *html.Node) error {
	errs := v.validateNode(input)
	if len(errs) > 0 {
		return &MultiValidationError{Errors: errs}
	}
	return nil
}

func (v *FormValidator) GetErrorMessage(formNode *html.Node) string {
	err := v.ValidateForm(formNode)
	if err == nil {
		return ""
	}
	return err.Error()
}

func (v *FormValidator) GetAllErrors(formNode *html.Node) []ValidationError {
	err := v.ValidateForm(formNode)
	if multiErr, ok := err.(*MultiValidationError); ok {
		return multiErr.Errors
	}
	if valErr, ok := err.(*ValidationError); ok {
		return []ValidationError{*valErr}
	}
	return nil
}

func findFormInputs(node *html.Node) []*html.Node {
	var inputs []*html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "input" || tag == "select" || tag == "textarea" {
				inputs = append(inputs, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(node)
	return inputs
}

func findSelect(node *html.Node) *html.Node {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "select" {
			return c
		}
	}
	return nil
}

func findSelectedOption(selectNode *html.Node) string {
	// Return the value of the explicitly selected option, or "" if none selected
	// (or if the first option has empty value and no explicit selection).
	var firstValue string
	firstSeen := false
	for c := selectNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "option" {
			val := getAttrValue(c, "value")
			if !firstSeen {
				firstValue = val
				firstSeen = true
			}
			if hasAttr(c, "selected") {
				return val
			}
		}
	}
	// No explicit selection: browser auto-selects first option
	return firstValue
}

func getAttrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func hasAttr(node *html.Node, key string) bool {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func getTextareaValue(node *html.Node) string {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			return c.Data
		}
	}
	return ""
}

func getIntAttrValue(node *html.Node, key string) int {
	val := getAttrValue(node, key)
	if val == "" {
		return 0
	}
	var result int
	fmt.Sscanf(val, "%d", &result)
	return result
}

func getFloatAttrValue(node *html.Node, key string) float64 {
	val := getAttrValue(node, key)
	if val == "" {
		return 0
	}
	var result float64
	fmt.Sscanf(val, "%f", &result)
	return result
}

func isValidEmail(email string) bool {
	emailPattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(emailPattern, email)
	return matched
}

func isValidURL(url string) bool {
	urlPattern := `^https?://[^\s/$.?#].[^\s]*$`
	matched, _ := regexp.MatchString(urlPattern, url)
	return matched
}

func matchPattern(pattern, value string) bool {
	matched, err := regexp.MatchString(pattern, value)
	return err == nil && matched
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func isNaN(f float64) bool {
	return f == 0
}
