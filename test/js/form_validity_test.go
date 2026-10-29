package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInputMaxLengthValidityStateAndFormChecks(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "field")
	input.SetAttribute("maxlength", "3")
	input.SetAttribute("value", "four")
	form.AppendChild(input)

	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return c.validity.tooLong===false&&c.validity.valid&&c.validity===c.validity&&c.validity instanceof ValidityState&&c.willValidate})()`); err != nil || got != "true" {
		t.Fatalf("script-set value validity = %q, err %v", got, err)
	}
	r.DispatchTrustedInput(input, nil, "insertText", false)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),f=document.getElementById("form"),events=0;c.addEventListener("invalid",function(){events++});var state=c.validity;var fieldInvalid=!c.checkValidity()&&events===1;var formInvalid=!f.checkValidity()&&events===2;return state.tooLong&&!state.valid&&fieldInvalid&&formInvalid&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("user-edited overlength validity = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`document.getElementById("field").value="four"`); err != nil {
		t.Fatalf("set value from script: %v", err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return !c.validity.tooLong&&c.validity.valid&&c.checkValidity()&&c.matches(":valid")})()`); err != nil || got != "true" {
		t.Fatalf("script-reset value validity = %q, err %v", got, err)
	}
}

func TestTextareaMaxLengthValidityNormalizesNewlines(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	textarea := doc.NewElement("textarea")
	textarea.SetAttribute("id", "field")
	textarea.SetAttribute("maxlength", "1")
	textarea.AppendChild(doc.NewText("\r\n"))
	textarea.ValueChangedByUser = true
	body.AppendChild(textarea)

	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return c.validity.tooLong===false&&c.validity.valid&&c.willValidate})()`); err != nil || got != "true" {
		t.Fatalf("newline-normalized textarea validity = %q, err %v", got, err)
	}
}

func TestRequiredSelectValueMissingUsesPlaceholderOptionRules(t *testing.T) {
	doc := domtest.Parse(`<form id="f">
		<select id="single" required><option value="" selected>Choose</option><option value="">Other empty</option></select>
		<select id="multiple" required multiple><option value="" selected>Empty choice</option></select>
		<select id="grouped" required><optgroup><option value="" selected>Grouped empty</option></optgroup></select>
		<select id="none" required><option value="x">X</option></select>
	</form>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var f=document.getElementById('f');
		var single=document.getElementById('single');
		var multiple=document.getElementById('multiple');
		var grouped=document.getElementById('grouped');
		var none=document.getElementById('none');
		var initial=single.validity.valueMissing&&!single.checkValidity()&&
			!multiple.validity.valueMissing&&multiple.checkValidity()&&
			!grouped.validity.valueMissing&&grouped.checkValidity();
		single.selectedIndex=1;
		none.selectedIndex=-1;
		var changed=!single.validity.valueMissing&&single.checkValidity()&&
			none.validity.valueMissing&&!none.checkValidity()&&
			!f.checkValidity()&&single.matches(':valid')&&none.matches(':invalid');
		return initial&&changed;
	})()`); err != nil || got != "true" {
		t.Fatalf("required select placeholder/multiple validity = %q, err %v", got, err)
	}
}

func TestSelectPlaceholderRestorationAndOptionConstructor(t *testing.T) {
	doc := domtest.Parse(`<body></body>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var select=document.createElement('select'); select.required=true;
		var placeholder=new Option('', '', false, true);
		var other=new Option('Choice', 'choice');
		select.appendChild(placeholder); select.appendChild(other);
		var initiallyMissing=!select.checkValidity();
		placeholder.remove();
		var validAfterRemoval=select.checkValidity();
		select.prepend(placeholder);
		var missingAfterRestore=!select.checkValidity();
		var optionDefaults=new Option('label','value',true);
		var optionExplicit=new Option('label','value',true,true);
		return [initiallyMissing,validAfterRemoval,missingAfterRestore,
			optionDefaults.defaultSelected,!optionDefaults.selected,
			optionExplicit.defaultSelected,optionExplicit.selected,
			new Option('plain').value].join('|');
	})()`); err != nil || got != "true|true|true|true|true|true|true|plain" {
		t.Fatalf("placeholder restoration / Option constructor = %q, err %v", got, err)
	}
}

func TestDisabledFieldsetDisablesControlsExceptFirstLegend(t *testing.T) {
	doc := domtest.Parse(`<form id="f">
		<fieldset disabled>
			<legend><input id="legend" name="legend" value="kept" required></legend>
			<label for="disabled">Disabled choice</label>
			<select id="disabled" name="disabled" required><option value="" selected>Choose</option></select>
			<fieldset disabled><legend><input id="nestedLegend" name="nestedLegend" value="nested"></legend><input id="nestedDisabled" name="nestedDisabled" value="skip"></fieldset>
		</fieldset>
	</form>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var f=document.getElementById('f');
		var legend=document.getElementById('legend');
		var disabled=document.getElementById('disabled');
		var nestedLegend=document.getElementById('nestedLegend');
		var nestedDisabled=document.getElementById('nestedDisabled');
		var clicks=0;
		disabled.addEventListener('click',function(){clicks++});
		document.querySelector('label').click();
		disabled.click();
		var state=legend.willValidate&&!legend.matches(':disabled')&&
			!disabled.willValidate&&disabled.matches(':disabled')&&
			nestedLegend.matches(':disabled')&&nestedDisabled.matches(':disabled');
		var data=new FormData(f);
		return state&&data.get('legend')==='kept'&&!data.has('disabled')&&
			!data.has('nestedLegend')&&!data.has('nestedDisabled')&&clicks===0;
	})()`); err != nil || got != "true" {
		t.Fatalf("disabled fieldset first-legend behavior = %q, err %v", got, err)
	}
}

func TestDatalistDescendantsAreBarredFromConstraintValidation(t *testing.T) {
	doc := domtest.Parse(`<datalist id="list"></datalist><div id="outside"></div>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var list=document.getElementById('list');
		var controls=[document.createElement('input'),document.createElement('select'),
			document.createElement('textarea'),document.createElement('button')];
		controls[3].type='submit';
		for(var i=0;i<controls.length;i++){
			if(!controls[i].willValidate) return false;
			list.appendChild(controls[i]);
			if(controls[i].willValidate) return false;
			controls[i].remove();
			if(!controls[i].willValidate) return false;
		}
		var nested=document.createElement('div');
		var foreign=document.createElementNS('urn:test','x:wrapper');
		var input=document.createElement('input');
		nested.appendChild(input);foreign.appendChild(nested);list.appendChild(foreign);
		return !input.willValidate;
	})()`); err != nil || got != "true" {
		t.Fatalf("datalist validation candidate behavior = %q, err %v", got, err)
	}
}

func TestInputMinLengthValidityStateAndFormChecks(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "field")
	input.SetAttribute("minlength", "3")
	input.SetAttribute("value", "😀")
	form.AppendChild(input)

	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return c.minLength===3&&!c.validity.tooShort})()`); err != nil || got != "true" {
		t.Fatalf("script-set short value validity = %q, err %v", got, err)
	}
	r.DispatchTrustedInput(input, nil, "insertText", false)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),f=document.getElementById("form"),events=0;c.addEventListener("invalid",function(){events++});return c.validity.tooShort&&!c.validity.valid&&!c.checkValidity()&&events===1&&!f.checkValidity()&&events===2&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("user-edited short value validity = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`document.getElementById("field").value="😀"`); err != nil {
		t.Fatalf("set value from script: %v", err)
	}
	if got, err := r.Evaluate(`!document.getElementById("field").validity.tooShort`); err != nil || got != "true" {
		t.Fatalf("script-reset short value validity = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`document.getElementById("field").value=""`); err != nil {
		t.Fatalf("set empty value from script: %v", err)
	}
	r.DispatchTrustedInput(input, nil, "deleteContentBackward", false)
	if got, err := r.Evaluate(`!document.getElementById("field").validity.tooShort`); err != nil || got != "true" {
		t.Fatalf("empty user-edited value validity = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.minLength=2;var reflected=c.getAttribute("minlength")==="2";var threw=false;try{c.minLength=-1}catch(e){threw=e.name==="IndexSizeError"}return reflected&&threw})()`); err != nil || got != "true" {
		t.Fatalf("minLength IDL reflection = %q, err %v", got, err)
	}
}

func TestTextareaMinLengthCountsNormalizedNewlines(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	textarea := doc.NewElement("textarea")
	textarea.SetAttribute("id", "field")
	textarea.SetAttribute("minlength", "3")
	textarea.AppendChild(doc.NewText("\r\n"))
	textarea.AppendChild(doc.NewText("x"))
	textarea.ValueChangedByUser = true
	body.AppendChild(textarea)

	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return c.minLength===3&&c.validity.tooShort})()`); err != nil || got != "true" {
		t.Fatalf("newline-normalized short textarea validity = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`document.getElementById("field").setAttribute("minlength","2")`); err != nil {
		t.Fatalf("lower minlength: %v", err)
	}
	if got, err := r.Evaluate(`!document.getElementById("field").validity.tooShort`); err != nil || got != "true" {
		t.Fatalf("textarea validity at normalized minlength = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.maxLength=1;return c.getAttribute("maxlength")==="1"&&c.maxLength===1})()`); err != nil || got != "true" {
		t.Fatalf("textarea maxLength reflection = %q, err %v", got, err)
	}
}

func TestNativeSetCustomValidityAffectsControlAndForm(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "field")
	input.SetAttribute("value", "ok")
	form.AppendChild(input)
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),f=document.getElementById("form"),events=0;c.addEventListener("invalid",function(){events++});c.setCustomValidity("bad\r\nvalue");var invalid=!c.checkValidity()&&events===1&&!f.checkValidity()&&events===2;return invalid&&c.validity.customError&&!c.validity.valid&&c.validationMessage==="bad\nvalue"&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("custom validity state and form checks = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.setCustomValidity("");return !c.validity.customError&&c.validity.valid&&c.validationMessage===""&&c.checkValidity()&&c.matches(":valid")})()`); err != nil || got != "true" {
		t.Fatalf("cleared custom validity = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),threw=false;try{c.setCustomValidity()}catch(e){threw=e instanceof TypeError}c.disabled=true;c.setCustomValidity("disabled custom error");return threw&&c.validity.customError&&!c.willValidate&&c.validationMessage===""&&c.checkValidity()})()`); err != nil || got != "true" {
		t.Fatalf("argument and barred-control behavior = %q, err %v", got, err)
	}
}

func TestInputPatternMismatchAffectsValidityAndPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "field")
	input.SetAttribute("pattern", "[A-Z]{2}[0-9]{3}")
	input.SetAttribute("value", "AB123")
	form.AppendChild(input)
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");return !c.validity.patternMismatch&&c.validity.valid&&c.checkValidity()})()`); err != nil || got != "true" {
		t.Fatalf("matching pattern validity = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.type="email";c.multiple=true;c.pattern="[A-Za-z0-9.@]{1,12}";c.value="a@b.com, c@d.com";var allMatch=!c.validity.patternMismatch&&c.checkValidity();c.value="a@b.com, verylong@example.com";return allMatch&&c.validity.patternMismatch&&!c.checkValidity()&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("multiple email patterns must match each trimmed member = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.type="text";c.multiple=false;c.pattern="[A-Z]{2}[0-9]{3}"})()`); err != nil {
		t.Fatalf("restore text pattern setup: %v", err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),f=document.getElementById("form"),events=0;c.addEventListener("invalid",function(){events++});c.value="AB12";var invalid=c.validity.patternMismatch&&!c.validity.valid&&!c.checkValidity()&&events===1&&!f.checkValidity()&&events===2;return invalid&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("mismatching pattern validity = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field");c.value="";var emptyOkay=!c.validity.patternMismatch&&c.validity.valid;c.value="abc";c.setAttribute("pattern","[");return emptyOkay&&!c.validity.patternMismatch&&c.checkValidity()})()`); err != nil || got != "true" {
		t.Fatalf("empty value/invalid pattern handling = %q, err %v", got, err)
	}
}

func TestInputPatternUnicodeSetsAndInvalidSyntax(t *testing.T) {
	doc := domtest.Parse(`<form>
		<input id="ascii" pattern="\p{ASCII_Hex_Digit}+" value="c0ff33">
		<input id="emoji" pattern="\p{RGI_Emoji}+" value="😘💋">
		<input id="difference" pattern="[\p{ASCII_Hex_Digit}--[Ff]]" value="0123456789abcdefABCDEF">
		<input id="strings" pattern="[_\q{a|bc|def}]" value="q">
		<input id="invalid-v" pattern="[(]" value="foo">
	</form>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var ascii=document.getElementById('ascii');
		var emoji=document.getElementById('emoji');
		var difference=document.getElementById('difference');
		var strings=document.getElementById('strings');
		var invalid=document.getElementById('invalid-v');
		return ascii.validity.valid&&!ascii.validity.patternMismatch&&
			emoji.validity.valid&&!emoji.validity.patternMismatch&&
			!difference.validity.valid&&difference.validity.patternMismatch&&
			!strings.validity.valid&&strings.validity.patternMismatch&&
			invalid.validity.valid&&!invalid.validity.patternMismatch;
	})()`); err != nil || got != "true" {
		t.Fatalf("Unicode Sets pattern validity = %q, err %v", got, err)
	}
}

func TestInputEmailAndURLTypeMismatchAffectsValidityAndPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "field")
	input.SetAttribute("type", "email")
	input.SetAttribute("value", "user@example.com")
	form.AppendChild(input)
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var c=document.getElementById("field"),f=document.getElementById("form");var initiallyValid=c.validity.valid&&!c.validity.typeMismatch;c.value="not-an-email";var emailInvalid=c.validity.typeMismatch&&!c.validity.valid&&!c.checkValidity()&&!f.checkValidity()&&c.matches(":invalid");c.value="first@example.com, second@example.org";c.multiple=true;var listValid=!c.validity.typeMismatch&&c.checkValidity();c.value="first@example.com, bad";var listInvalid=c.validity.typeMismatch&&!c.checkValidity();c.type="url";c.value="https://example.com/path";var urlValid=!c.validity.typeMismatch&&c.checkValidity();c.value="relative/path";var urlInvalid=c.validity.typeMismatch&&!c.checkValidity()&&c.matches(":invalid");c.value="";var emptyValid=!c.validity.typeMismatch&&c.checkValidity();return initiallyValid&&emailInvalid&&listValid&&listInvalid&&urlValid&&urlInvalid&&emptyValid})()`); err != nil || got != "true" {
		t.Fatalf("email/URL type mismatch behavior = %q, err %v", got, err)
	}
}

func TestInputNumberRangeAndStepValidity(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "number")
	input.SetAttribute("type", "number")
	input.SetAttribute("min", "1")
	input.SetAttribute("max", "5")
	input.SetAttribute("step", "2")
	input.SetAttribute("value", "3")
	form.AppendChild(input)
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var c=document.getElementById("number"),f=document.getElementById("form");var initial=!c.validity.rangeUnderflow&&!c.validity.rangeOverflow&&!c.validity.stepMismatch&&c.validity.valid&&c.checkValidity()&&c.matches(":in-range");c.value="0";var underflow=c.validity.rangeUnderflow&&!c.validity.rangeOverflow&&!c.checkValidity()&&!f.checkValidity()&&c.matches(":invalid")&&c.matches(":out-of-range")&&c.validationMessage!=="";c.value="6";var overflow=c.validity.rangeOverflow&&!c.validity.rangeUnderflow&&!c.checkValidity()&&c.matches(":out-of-range");c.value="2";var stepMismatch=c.validity.stepMismatch&&!c.checkValidity()&&c.matches(":invalid")&&c.matches(":in-range")&&!c.matches(":out-of-range");c.step="any";var anyStep=!c.validity.stepMismatch&&c.checkValidity()&&c.matches(":valid");c.min="0.1";c.max="0.5";c.step="0.1";c.value="0.3";var decimalStep=!c.validity.stepMismatch&&c.checkValidity();c.value="";var emptyInRange=c.matches(":in-range")&&!c.matches(":out-of-range");return initial&&underflow&&overflow&&stepMismatch&&anyStep&&decimalStep&&emptyInRange})()`); err != nil || got != "true" {
		t.Fatalf("number range and step validity = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("number");c.min="1";c.max="5";c.step="0";c.value="2";var invalidStepUsesDefault=!c.validity.stepMismatch&&c.checkValidity();c.value="2.5";return invalidStepUsesDefault&&c.validity.stepMismatch&&!c.checkValidity()})()`); err != nil || got != "true" {
		t.Fatalf("invalid step falls back to the default step = %q, err %v", got, err)
	}
}

func TestInputNumberStepMismatchPreservesSmallDecimalPrecision(t *testing.T) {
	doc := domtest.Parse(`<form><input id="tiny-step" type="number" step="3e-15"></form>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var input=document.getElementById('tiny-step');
		input.value='17';
		return input.validity.stepMismatch && !input.validity.valid && !input.checkValidity();
	})()`); err != nil || got != "true" {
		t.Fatalf("small-step mismatch validity = %q, err %v", got, err)
	}
}

func TestValueMissingForDisabledControlsFollowsControlTypeRules(t *testing.T) {
	doc := domtest.Parse(`<form>
		<input id="text" type="text" required disabled>
		<input id="pattern" type="text" value="abc" pattern="[A-Z]" disabled>
		<input id="checkbox" type="checkbox" required disabled>
		<textarea id="textarea" required disabled></textarea>
		<textarea id="enabled-textarea" required></textarea>
		<select id="select" required disabled><option value="" selected>Choose</option></select>
	</form>`)
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){
		var text=document.getElementById('text');
		var pattern=document.getElementById('pattern');
		var checkbox=document.getElementById('checkbox');
		var textarea=document.getElementById('textarea');
		var enabledTextarea=document.getElementById('enabled-textarea');
		var select=document.getElementById('select');
		var source=document.createElement('textarea');
		document.body.appendChild(source);
		var clonedTextarea=source.cloneNode(true);
		clonedTextarea.required=true;
		clonedTextarea.value='';
		var unnamedRadio=document.createElement('input');
		unnamedRadio.type='radio';
		unnamedRadio.required=true;
		var disabledClone=source.cloneNode(true);
		disabledClone.required=true;
		disabledClone.value='';
		disabledClone.disabled=true;
		var readOnlyClone=source.cloneNode(true);
		readOnlyClone.required=true;
		readOnlyClone.value='';
		readOnlyClone.readOnly=true;
		return !text.validity.valueMissing && text.validity.valid &&
			pattern.validity.patternMismatch && !pattern.validity.valid &&
			checkbox.validity.valueMissing && !checkbox.validity.valid &&
			!textarea.validity.valueMissing && enabledTextarea.validity.valueMissing &&
			clonedTextarea.willValidate && clonedTextarea.required &&
			clonedTextarea.validity.valueMissing &&
			!unnamedRadio.validity.valueMissing && unnamedRadio.validity.valid &&
			!disabledClone.willValidate && disabledClone.disabled &&
			!disabledClone.validity.valueMissing && disabledClone.validity.valid &&
			!readOnlyClone.willValidate && readOnlyClone.readOnly &&
			!readOnlyClone.validity.valueMissing && readOnlyClone.validity.valid && select.validity.valueMissing &&
			text.checkValidity() && checkbox.checkValidity();
	})()`); err != nil || got != "true" {
		t.Fatalf("disabled-control valueMissing states = %q, err %v", got, err)
	}
}

func TestInputNumberBadInputIsUserEditState(t *testing.T) {
	doc := domtest.Parse(`<input id="number" type="number" required value="2">`)
	var node *dom.Node
	doc.Node.Walk(func(candidate *dom.Node) bool {
		if candidate.Data == "input" && candidate.GetAttribute("id") == "number" {
			node = candidate
			return false
		}
		return true
	})
	if node == nil {
		t.Fatal("number input not parsed")
	}
	r := newDOMRuntime(t, doc)
	node.InputBadInput = true
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("number");return c.value==="2"&&c.validity.badInput&&!c.validity.valid})()`); err != nil || got != "true" {
		t.Fatalf("user-entered unrepresentable number state = %q, err %v", got, err)
	}
	if _, err := r.Evaluate(`document.getElementById("number").value="1.e"`); err != nil {
		t.Fatalf("set number value from script: %v", err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("number");return c.value===""&&!c.validity.badInput&&c.validity.valueMissing})()`); err != nil || got != "true" {
		t.Fatalf("script-set invalid number is sanitized without badInput = %q, err %v", got, err)
	}
}

func TestEmailInputSelectionExposesDisplayedUserText(t *testing.T) {
	doc := domtest.Parse(`<input id="email" type="email">`)
	var input *dom.Node
	doc.Node.Walk(func(node *dom.Node) bool {
		if node.Data == "input" && node.GetAttribute("id") == "email" {
			input = node
			return false
		}
		return true
	})
	if input == nil {
		t.Fatal("email input not parsed")
	}
	raw := " foo@bar   "
	input.SetInputControlValue("foo@bar", true)
	input.InputUserEditValue = raw
	input.InputUserEditValueActive = true
	r := newDOMRuntime(t, doc)
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("email");c.select();return c.value==="foo@bar"&&window.getSelection().toString()===" foo@bar   ";})()`); err != nil || got != "true" {
		t.Fatalf("email input select exposes user edit text = %q, err %v", got, err)
	}
	if got, err := r.Evaluate(`(function(){var c=document.getElementById("email");c.value="foo@bar";c.select();return window.getSelection().toString()==="foo@bar";})()`); err != nil || got != "true" {
		t.Fatalf("script-set email selection text = %q, err %v", got, err)
	}
}

func TestInputDateRangeAndStepValidity(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	form.SetAttribute("id", "form")
	body.AppendChild(form)
	input := doc.NewElement("input")
	input.SetAttribute("id", "date")
	input.SetAttribute("type", "date")
	input.SetAttribute("min", "2024-02-28")
	input.SetAttribute("max", "2024-03-02")
	input.SetAttribute("step", "2")
	input.SetAttribute("value", "2024-03-01")
	form.AppendChild(input)
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var c=document.getElementById("date"),f=document.getElementById("form");var initial=c.validity.valid&&!c.validity.rangeUnderflow&&!c.validity.rangeOverflow&&!c.validity.stepMismatch&&c.matches(":in-range");c.value="2024-02-29";var leapDayMismatch=c.validity.stepMismatch&&!c.checkValidity()&&c.matches(":invalid")&&c.matches(":in-range");c.value="2024-02-27";var underflow=c.validity.rangeUnderflow&&!c.validity.rangeOverflow&&!f.checkValidity()&&c.matches(":out-of-range");c.value="2024-03-03";var overflow=c.validity.rangeOverflow&&!c.validity.rangeUnderflow&&!c.checkValidity();c.step="any";c.value="2024-02-29";var anyStep=!c.validity.stepMismatch&&c.checkValidity();c.value="";var emptyInRange=!c.validity.rangeUnderflow&&!c.validity.rangeOverflow&&!c.validity.stepMismatch&&c.matches(":in-range");return initial&&leapDayMismatch&&underflow&&overflow&&anyStep&&emptyInRange})()`); err != nil || got != "true" {
		t.Fatalf("date range and step validity = %q, err %v", got, err)
	}
}

func TestInputMonthWeekTimeAndLocalDateTimeRangeValidity(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html
	form := doc.NewElement("form")
	body.AppendChild(form)
	for _, attrs := range []map[string]string{
		{"id": "month", "type": "month", "min": "2024-01", "max": "2024-06", "step": "2", "value": "2024-03"},
		{"id": "week", "type": "week", "min": "2020-W01", "max": "2020-W05", "step": "2", "value": "2020-W03"},
		{"id": "time", "type": "time", "min": "21:00", "max": "06:00", "step": "60", "value": "00:00"},
		{"id": "local", "type": "datetime-local", "min": "2024-03-01T00:00", "max": "2024-03-02T00:00", "step": "60", "value": "2024-03-01T00:01"},
	} {
		input := doc.NewElement("input")
		for name, value := range attrs {
			input.SetAttribute(name, value)
		}
		form.AppendChild(input)
	}
	r := newDOMRuntime(t, doc)

	if got, err := r.Evaluate(`(function(){var m=document.getElementById("month"),w=document.getElementById("week"),t=document.getElementById("time"),l=document.getElementById("local");var initial=m.validity.valid&&w.validity.valid&&t.validity.valid&&l.validity.valid&&m.matches(":in-range")&&w.matches(":in-range")&&t.matches(":in-range");m.value="2024-02";var monthStep=m.validity.stepMismatch&&!m.checkValidity();m.value="2023-12";var monthLow=m.validity.rangeUnderflow&&m.matches(":out-of-range");w.value="2020-W02";var weekStep=w.validity.stepMismatch&&!w.checkValidity();t.value="12:00";var wrappedOut=t.validity.rangeUnderflow&&t.validity.rangeOverflow&&!t.checkValidity()&&t.matches(":out-of-range");t.value="22:00";var wrappedIn=!t.validity.rangeUnderflow&&!t.validity.rangeOverflow&&t.checkValidity()&&t.matches(":in-range");l.value="2024-03-01T00:00:30";var localStep=l.validity.stepMismatch&&!l.checkValidity();l.step="any";var localAny=!l.validity.stepMismatch&&l.checkValidity();return initial&&monthStep&&monthLow&&weekStep&&wrappedOut&&wrappedIn&&localStep&&localAny})()`); err != nil || got != "true" {
		t.Fatalf("month/week/time/local date-time validity = %q, err %v", got, err)
	}
}
