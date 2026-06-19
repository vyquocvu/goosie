package form

import (
	"strings"

	"golang.org/x/net/html"
)

type FormData map[string]string

func (fd FormData) Get(key string) string {
	if val, ok := fd[key]; ok {
		return val
	}
	return ""
}

type SubmitCallback func(data FormData)
type CancelCallback func()
type SuccessCallback func(result *SubmissionResult)
type ErrorCallback func(err error)

type FormState struct {
	formNode      *html.Node
	submitEnabled bool
	cancelEnabled bool
	onSubmit      SubmitCallback
	onCancel      CancelCallback
}

func NewFormState(formNode *html.Node) *FormState {
	return &FormState{
		formNode:      formNode,
		submitEnabled: true,
		cancelEnabled: true,
	}
}

func (s *FormState) SetSubmitCallback(callback SubmitCallback) {
	s.onSubmit = callback
}

func (s *FormState) SetCancelCallback(callback CancelCallback) {
	s.onCancel = callback
}

func (s *FormState) Submit() {
	if !s.submitEnabled {
		return
	}
	s.submitEnabled = false
	defer func() { s.submitEnabled = true }()

	if s.onSubmit != nil {
		s.onSubmit(s.GetFormData())
	}
}

func (s *FormState) CancelSubmission() {
	if !s.cancelEnabled {
		return
	}
	s.cancelEnabled = false
	defer func() { s.cancelEnabled = true }()

	if s.onCancel != nil {
		s.onCancel()
	}
}

func (s *FormState) TriggerEnterKey() {
	s.Submit()
}

func (s *FormState) GetFormData() FormData {
	data := make(FormData)
	collectFormData(s.formNode, data)
	return data
}

func (s *FormState) AddField(input *html.Node) {
	s.formNode.AppendChild(input)
}

func collectFormData(node *html.Node, data FormData) {
	if node.Type != html.ElementNode {
		return
	}

	tag := strings.ToLower(node.Data)

	switch tag {
	case "input":
		name := getAttrValue(node, "name")
		if name == "" {
			return
		}
		inputType := getAttrValue(node, "type")
		if inputType == "submit" || inputType == "button" || inputType == "reset" {
			return
		}
		if hasAttr(node, "disabled") {
			return
		}

		switch inputType {
		case "checkbox":
			if hasAttr(node, "checked") {
				data[name] = "on"
			}
		case "radio":
			if hasAttr(node, "checked") {
				data[name] = getAttrValue(node, "value")
			}
		default:
			data[name] = getAttrValue(node, "value")
		}

	case "select":
		name := getAttrValue(node, "name")
		if name == "" {
			return
		}
		if hasAttr(node, "disabled") {
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "option" {
				if hasAttr(c, "selected") {
					val := getAttrValue(c, "value")
					if val == "" {
						val = c.FirstChild.Data
					}
					data[name] = val
					break
				}
			}
		}

	case "textarea":
		name := getAttrValue(node, "name")
		if name == "" {
			return
		}
		if hasAttr(node, "disabled") {
			return
		}
		data[name] = extractTextContent(node)
	}

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		collectFormData(c, data)
	}
}

func extractTextContent(node *html.Node) string {
	var sb strings.Builder
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}
