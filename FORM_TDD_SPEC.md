# Form Feature TDD Test Specifications

**Project:** Goosie Browser  
**Date:** 2026-04-15  
**Status:** Draft - Pre-Implementation

## 1. Overview

This document defines the comprehensive Test-Driven Development (TDD) strategy for implementing the HTML form feature in the Goosie browser. All tests must be written BEFORE implementing any production code following the red-green-refactor cycle.

## 2. Current State Analysis

### 2.1 Existing Form Infrastructure

| Component | Location | Status |
|-----------|----------|--------|
| Basic form element rendering | `canvas.go:1223-1296` | Basic implementation exists |
| Form test suite | `form_and_table_test.go` | Limited coverage |
| Test data files | `testdata/*_forms.html` | 10 test files exist |
| JS Runtime | `runtime.go` | Form-related APIs needed |

### 2.2 Missing Components (Require Implementation)

1. **Form Validation Engine** - Client-side validation for required fields, patterns, min/max
2. **Form Submission Handler** - Submit event handling, form data collection, HTTP submission
3. **Form State Management** - Track input values, dirty state, validation state per field
4. **Error Display System** - Inline validation messages, error styling
5. **Accessibility Support** - ARIA attributes, keyboard navigation, screen reader support
6. **Advanced Input Types** - date, time, email, url, tel, number, range, color, file

## 3. TDD Test Structure

### 3.1 Test File Organization

```
internal/
├── renderer/
│   └── form_test.go              # Core form rendering tests
│   └── form_validation_test.go  # Form validation logic tests
│   └── form_submission_test.go   # Form submission tests
│   └── form_a11y_test.go         # Accessibility tests
│   └── form_edge_cases_test.go   # Edge case and error tests
├── js/
│   └── form_api_test.go          # JavaScript Form API tests
│   └── form_events_test.go       # Form event handling tests
├── test_suite/
│   ├── form/
│   │   ├── validation_test.go
│   │   ├── submission_test.go
│   │   ├── cross_browser_test.go
│   │   └── integration_test.go
│   └── e2e/
│       └── form_workflow_test.go  # End-to-end form workflows
└── testutil/
    └── form_helpers.go            # Test utilities for form testing
```

## 4. Test Specifications by Category

### 4.1 Form Validation Tests

#### 4.1.1 Required Field Validation

| Test ID | Test Name | Input | Expected Result | Priority |
|---------|-----------|-------|-----------------|----------|
| VAL-001 | RequiredTextField_EmptySubmit | empty string | Validation error shown, submit prevented | P0 |
| VAL-002 | RequiredTextField_WithValue | "valid input" | No error, submit proceeds | P0 |
| VAL-003 | RequiredCheckbox_Unchecked | unchecked | Validation error shown | P0 |
| VAL-004 | RequiredCheckbox_Checked | checked | No error | P0 |
| VAL-005 | RequiredSelect_NoSelection | no selection | Validation error shown | P0 |
| VAL-006 | RequiredSelect_WithSelection | valid option selected | No error | P0 |

#### 4.1.2 Pattern Validation (Regex)

| Test ID | Test Name | Pattern | Input | Expected Result |
|---------|-----------|---------|-------|-----------------|
| VAL-007 | EmailPattern_Valid | email regex | "user@example.com" | No error |
| VAL-008 | EmailPattern_Invalid | email regex | "notanemail" | Validation error shown |
| VAL-009 | PhonePattern_Valid | phone regex | "(555) 123-4567" | No error |
| VAL-010 | PhonePattern_Invalid | phone regex | "abc123" | Validation error shown |
| VAL-011 | CustomPattern_Valid | `^[A-Z]{2}[0-9]{4}$` | "AB1234" | No error |
| VAL-012 | CustomPattern_Invalid | `^[A-Z]{2}[0-9]{4}$` | "ab1234" | Validation error shown |

#### 4.1.3 Min/Max Length Validation

| Test ID | Test Name | Constraint | Input | Expected Result |
|---------|-----------|------------|-------|-----------------|
| VAL-013 | MinLength_TooShort | minlength=5 | "abc" | Validation error shown |
| VAL-014 | MinLength_Valid | minlength=5 | "abcdef" | No error |
| VAL-015 | MaxLength_TooLong | maxlength=10 | "12345678901" | Validation error shown |
| VAL-016 | MaxLength_Valid | maxlength=10 | "1234567890" | No error |
| VAL-017 | MinMaxLength_Valid | minlength=3, maxlength=5 | "abcd" | No error |

#### 4.1.4 Min/Max Value Validation (Number/Date)

| Test ID | Test Name | Constraint | Input | Expected Result |
|---------|-----------|------------|-------|-----------------|
| VAL-018 | MinNumber_TooLow | min=18 | "16" | Validation error shown |
| VAL-019 | MinNumber_Valid | min=18 | "25" | No error |
| VAL-020 | MaxNumber_TooHigh | max=100 | "150" | Validation error shown |
| VAL-021 | MaxNumber_Valid | max=100 | "75" | No error |
| VAL-022 | MinDate_BeforeRange | min=2024-01-01 | "2023-06-15" | Validation error shown |
| VAL-023 | MinDate_Valid | min=2024-01-01 | "2024-05-01" | No error |

### 4.2 Form Submission Tests

#### 4.2.1 Submit Event Handling

| Test ID | Test Name | Scenario | Expected Result |
|---------|-----------|----------|----------------|
| SUB-001 | SubmitButton_Click | Button click | Submit event fired |
| SUB-002 | EnterKey_Submit | Enter on input | Submit event fired |
| SUB-003 | JavaScript_Submit | `form.submit()` called | Submit event fired (if novalidate=false) |
| SUB-004 | FormSubmission_Cancel | `event.preventDefault()` called | No HTTP request |
| SUB-005 | MultiSubmit_Prevention | Double click | Only one submission |

#### 4.2.2 Form Data Collection

| Test ID | Test Name | Form Fields | Expected Result |
|---------|-----------|-------------|----------------|
| SUB-006 | FormData_TextInput | input[type=text]=John | {text: "John"} in form data |
| SUB-007 | FormData_PasswordInput | input[type=password]=secret | {password: "secret"} in form data |
| SUB-008 | FormData_Checkbox | input[type=checkbox]=checked | {checkbox: "on"} in form data |
| SUB-009 | FormData_RadioGroup | input[radio]=option2 | {radio: "option2"} in form data |
| SUB-010 | FormData_Select | select option selected | {select: "value"} in form data |
| SUB-011 | FormData_Textarea | textarea content | {textarea: "content"} in form data |
| SUB-012 | FormData_MultipleInputs | multiple inputs | All values in form data |
| SUB-013 | FormData_DisabledField | disabled input | Field NOT in form data |
| SUB-014 | FormData_UncheckedCheckbox | unchecked checkbox | Field NOT in form data |

#### 4.2.3 HTTP Submission

| Test ID | Test Name | Method | Expected Behavior |
|---------|-----------|--------|-------------------|
| SUB-015 | HTTPGet_Submission | GET method | URL-encoded form data in query string |
| SUB-016 | HTTPPost_Submission | POST method | Form data in request body |
| SUB-017 | HTTPResponse_Success | 200 OK | Success handler called |
| SUB-018 | HTTPResponse_Error | 400 Bad Request | Error handler called |
| SUB-019 | HTTPResponse_ServerError | 500 Internal Server Error | Error handler called |
| SUB-020 | HTTPTimeout | 30s timeout | Timeout error shown |

### 4.3 Error Scenario Tests

#### 4.3.1 Network Errors

| Test ID | Test Name | Scenario | Expected Result |
|---------|-----------|----------|-----------------|
| ERR-001 | NetworkError_NoInternet | Connection failed | Offline error message displayed |
| ERR-002 | NetworkError_DNSFailure | DNS resolution failed | DNS error message displayed |
| ERR-003 | NetworkError_Timeout | Request timeout | Timeout error message displayed |
| ERR-004 | NetworkError_ConnectionRefused | Server not running | Connection refused message |

#### 4.3.2 Validation Errors

| Test ID | Test Name | Scenario | Expected Result |
|---------|-----------|----------|-----------------|
| ERR-005 | ValidationError_InlineMessage | Invalid input | Error message near field |
| ERR-006 | ValidationError_MultipleFields | Multiple invalid | All errors shown |
| ERR-007 | ValidationError_ClearOnFix | User fixes error | Error message removed |
| ERR-008 | ValidationError_Summary | Form-level errors | Summary shown at top |

#### 4.3.3 Edge Cases

| Test ID | Test Name | Scenario | Expected Result |
|---------|-----------|----------|-----------------|
| ERR-009 | EmptyForm_Submit | Form with no fields | Depends on required fields |
| ERR-010 | VeryLongInput_Submit | 1MB text input | Handled gracefully |
| ERR-011 | SpecialCharacters_Input | XSS attempt `<script>` | Sanitized or error |
| ERR-012 | RapidInput_Validation | Fast typing | Debounced validation |
| ERR-013 | PastedContent_Validation | Paste large text | Validated correctly |
| ERR-014 | FormInIframe_Submit | Form in iframe | Parent notified (if enabled) |
| ERR-015 | DynamicForm_AddField | JS adds field | New field works correctly |

### 4.4 Accessibility (A11y) Tests

#### 4.4.1 Keyboard Navigation

| Test ID | Test Name | Key Press | Expected Result |
|---------|-----------|-----------|-----------------|
| A11Y-001 | TabNavigation_Forward | Tab | Focus moves to next field |
| A11Y-002 | TabNavigation_Backward | Shift+Tab | Focus moves to previous field |
| A11Y-003 | EnterActivates | Enter | Submit or activate button |
| A11Y-004 | EscapeCancels | Escape | Clear/reset if appropriate |
| A11Y-005 | FocusIndicator_Visible | Focus on field | Clear focus indicator shown |

#### 4.4.2 Screen Reader Support

| Test ID | Test Name | Scenario | Expected ARIA |
|---------|-----------|----------|---------------|
| A11Y-006 | ARIA_Label | Labeled input | aria-labelledby linked |
| A11Y-007 | ARIA_DescribedBy | Input with description | aria-describedby linked |
| A11Y-008 | ARIA_Invalid | Invalid field | aria-invalid="true" |
| A11Y-009 | ARIA_Errormessage | Error message | aria-errormessage linked |
| A11Y-010 | ARIA_Required | Required field | aria-required="true" |
| A11Y-011 | ARIA_Disabled | Disabled field | aria-disabled="true" |

#### 4.4.3 Color Contrast

| Test ID | Test Name | Scenario | Expected Ratio |
|---------|-----------|----------|----------------|
| A11Y-012 | Contrast_NormalText | Normal text | ≥ 4.5:1 |
| A11Y-013 | Contrast_LargeText | Large text (≥18pt) | ≥ 3:1 |
| A11Y-014 | Contrast_ErrorText | Error message text | ≥ 4.5:1 |

### 4.5 Cross-Browser Compatibility Tests

#### 4.5.1 Input Type Support

| Test ID | Test Name | Input Type | Expected Result |
|---------|-----------|------------|-----------------|
| XBROWS-001 | Type_Text | text | Text input rendered |
| XBROWS-002 | Type_Password | password | Masked input rendered |
| XBROWS-003 | Type_Email | email | Email input with validation |
| XBROWS-004 | Type_URL | url | URL input with validation |
| XBROWS-005 | Type_Number | number | Number input with spinners |
| XBROWS-006 | Type_Range | range | Slider control rendered |
| XBROWS-007 | Type_Date | date | Date picker rendered |
| XBROWS-008 | Type_Time | time | Time picker rendered |
| XBROWS-009 | Type_Color | color | Color picker rendered |
| XBROWS-010 | Type_File | file | File picker rendered |
| XBROWS-011 | Type_Checkbox | checkbox | Checkbox rendered |
| XBROWS-012 | Type_Radio | radio | Radio buttons rendered |

#### 4.5.2 HTML5 Validation Attributes

| Test ID | Test Name | Attribute | Expected Behavior |
|---------|-----------|-----------|-------------------|
| XBROWS-013 | Attr_Required | required | Native validation |
| XBROWS-014 | Attr_Pattern | pattern | Regex validation |
| XBROWS-015 | Attr_MinLength | minlength | Length validation |
| XBROWS-016 | Attr_MaxLength | maxlength | Length validation |
| XBROWS-017 | Attr_Min | min | Value validation |
| XBROWS-018 | Attr_Max | max | Value validation |
| XBROWS-019 | Attr_Step | step | Step value validation |
| XBROWS-020 | Attr_Autocomplete | autocomplete | Auto-fill hints |

## 5. Red-Green-Refactor Implementation Plan

### Phase 1: Form Validation Engine (Week 1)

#### Step 1.1: Core Validation Infrastructure
```
Tests to write:
- VAL-001 through VAL-006 (Required validation)
- VAL-013 through VAL-017 (Min/Max length)

Implementation tasks:
1. Create FormValidator struct
2. Implement required field validation
3. Implement min/max length validation
4. Add validation error types and messages
```

#### Step 1.2: Pattern Validation
```
Tests to write:
- VAL-007 through VAL-012 (Pattern validation)

Implementation tasks:
1. Implement regex pattern matching
2. Add built-in patterns (email, url, etc.)
3. Support custom patterns
```

### Phase 2: Form State Management (Week 2)

#### Step 2.1: Form State Tracking
```
Tests to write:
- SUB-006 through SUB-014 (Form data collection)
- ERR-009 through ERR-012 (Edge cases)

Implementation tasks:
1. Create FormState struct
2. Implement field value tracking
3. Implement dirty state tracking
4. Implement form reset functionality
```

#### Step 2.2: Validation State Updates
```
Tests to write:
- ERR-005 through ERR-008 (Validation errors)

Implementation tasks:
1. Create error message display system
2. Implement inline error styling
3. Add error summary support
```

### Phase 3: Form Submission (Week 3)

#### Step 3.1: Submit Event Handling
```
Tests to write:
- SUB-001 through SUB-005 (Submit event handling)
- ERR-009 (Empty form handling)

Implementation tasks:
1. Implement submit event firing
2. Add form validation on submit
3. Implement submit prevention
```

#### Step 3.2: HTTP Submission
```
Tests to write:
- SUB-015 through SUB-020 (HTTP submission)

Implementation tasks:
1. Implement form data encoding
2. Add GET/POST method support
3. Implement response handling
```

### Phase 4: Accessibility & Polish (Week 4)

#### Step 4.1: Accessibility Features
```
Tests to write:
- A11Y-001 through A11Y-014 (All accessibility tests)

Implementation tasks:
1. Implement keyboard navigation
2. Add ARIA attributes
3. Ensure color contrast compliance
```

#### Step 4.2: Advanced Input Types & Edge Cases
```
Tests to write:
- XBROWS-001 through XBROWS-020
- ERR-013 through ERR-015

Implementation tasks:
1. Implement advanced input types
2. Add file upload support
3. Implement iframe handling
```

## 6. Success Criteria

### 6.1 Test Coverage

| Metric | Target | Measurement Method |
|--------|--------|-------------------|
| Test coverage for critical form paths | 100% | `go test -coverprofile` |
| Form validation code coverage | 100% | `go test -coverprofile internal/form/` |
| Edge case coverage | ≥90% | Manual audit of test cases |

### 6.2 Performance Metrics

| Metric | Target | Measurement Method |
|--------|--------|-------------------|
| Validation response time | < 100ms | Benchmark tests |
| Form data collection | < 50ms | Benchmark tests |
| Submit processing | < 100ms | Benchmark tests |
| Validation debounce | 300ms delay | UI timing tests |

### 6.3 Quality Metrics

| Metric | Target | Measurement Method |
|--------|--------|-------------------|
| Critical bugs in production | 0 | Bug tracking system |
| High priority bugs | < 3 | Bug tracking system |
| User acceptance test pass rate | 100% | UAT sign-off |
| Accessibility compliance | WCAG 2.1 AA | A11y audit |

### 6.4 Verification Checkpoints

| Checkpoint | Description | Criteria |
|------------|------------|----------|
| CP-1 | All unit tests pass | `go test ./...` 100% pass |
| CP-2 | Integration tests pass | `go test ./test_suite/...` 100% pass |
| CP-3 | E2E tests pass | Playwright tests 100% pass |
| CP-4 | Performance benchmarks meet targets | All benchmarks < threshold |
| CP-5 | A11y audit passes | Zero critical/high issues |
| CP-6 | UAT with real form data | 100% scenario success |

## 7. Test Execution Plan

### 7.1 Local Development
```bash
# Run all form-related tests
go test ./internal/renderer/... -run "Form" -v

# Run validation tests only
go test ./internal/renderer/... -run "Validation" -v

# Run with coverage
go test ./internal/renderer/... -run "Form" -coverprofile=form_coverage.out
go tool cover -html=form_coverage.out -o form_coverage.html
```

### 7.2 Continuous Integration
- All tests run on every PR
- Coverage must not decrease
- Performance benchmarks on main branch

### 7.3 Pre-Release Checklist
- [ ] All tests pass (unit, integration, E2E)
- [ ] Coverage ≥ 100% for critical paths
- [ ] Performance benchmarks within targets
- [ ] A11y audit complete with no critical issues
- [ ] Security review complete
- [ ] UAT sign-off received

## 8. Appendix

### 8.1 Test Data Files

```
testdata/
├── form_basic.html              # Basic form with all input types
├── form_validation.html         # Form with validation attributes
├── form_complex.html            # Multi-field complex form
├── form_dynamic.html            # Dynamically generated form
├── form_upload.html             # File upload form
└── form_calculator.html         # Interactive form example
```

### 8.2 Mock Data for Testing

```go
// Example test data
var (
    ValidEmail    = "user@example.com"
    InvalidEmail = "notanemail"
    ValidPhone   = "(555) 123-4567"
    ValidAge     = "25"
    InvalidAge   = "16"
)
```

### 8.3 Known Limitations

| Limitation | Description | Workaround |
|-----------|-------------|------------|
| No native date picker | Fyne doesn't have native date picker | Use custom picker widget |
| Limited file upload | File picker integration incomplete | Basic file selection only |
| No drag-drop forms | Drag-drop form builder not in scope | Manual form creation |

## 9. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-04-15 | Claude | Initial draft |
