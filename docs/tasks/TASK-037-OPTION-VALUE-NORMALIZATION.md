# TASK-037 — giá trị `<option>` phải normalize đúng như Chromium (BR-101, BR-104, BR-106, BR-139)

**Priority:** P1. **Method:** INT. **Owner:** chưa gán.
**Blocked cases:** BR-101 (chọn option ⇒ value đúng), BR-104/BR-106 (submit gửi đúng
name/value của control, và encoding của chúng) và BR-139 (AX tree phản ánh value của DOM).
**Defect:** `GOOSIE-FORMVALUE-001`.

## 1. Bằng chứng đã đo, không suy diễn

Probe chạy trên `main` sau unify `optionValue` (file probe đã xoá ngay sau khi đo, không commit):

```
GOOSIE  SUBMITTED name="c" value="\n  Charcoal\n"
GOOSIE  ENCODED "c=%0A++Charcoal%0A"
```

markup tương ứng — chính là kiểu HTML người ta viết tay mọi ngày, option một dòng/thụt lề:

```html
<select id=s name=c>
<option selected>
  Charcoal
</option>
<option value=b>B</option>
</select>
```

Cùng markup, đo Chromium qua Playwright trên cùng máy:

```
option.textContent = '  Hi\n  there\n'  ->  option.value === "Hi there"
                                         ->  option.label === "Hi there"
```

| Sự thật | Chỗ trong code |
| --- | --- |
| Giá trị option = `TextContent()` nguyên bản, không strip không collapse | `internal/engine/focus.go` `optionValue` (một định nghĩa duy nhất, dùng bởi `forms.go:controlValue`, `Session.controlValue`, `accessibility.go:162-193`) |
| `Label` của option trong AX **đã** `strings.TrimSpace` trong khi `Value` thì không ⇒ cùng một node, hai câu trả lời lệch nhau | `internal/engine/accessibility.go:171-177` |
| `<select>` có **hai** option cùng `selected`: Chromium chọn cái **cuối** (`selectedIndex 1, value "b", FormData "b"`); `SelectedOption` trả về ngay khi thấy `selected` đầu tiên ⇒ Goosie gửi `"a"` | `internal/engine/focus.go:634` `SelectedOption` |
| Hành vi first-wins này **không** có test nào khoá: `grep selected test/engine/` không có case hai option cùng `selected` | `test/engine/` |

Vì sao đáng làm: form `<GET>` nhận `?c=%0A++Charcoal%0A` thì server không khớp được giá trị nào;
đây là lỗi "browser dùng được thật" chứ không phải lỗi conformance trừu tượng.

## 2. Thiết kế tối thiểu

0. **Đã xong, không làm lại:** một helper `optionValue` duy nhất cho cả submit, bàn phím và AX,
   cùng oracle chéo `TestAccessibilitySelectValueMatchesSubmittedFormValue` (`41446b2`). Phần còn
   lại của task là *normalize* giá trị và *tie-break* `selected`. Oracle chéo phải xanh trong suốt
   quá trình sửa — nó là thứ phát hiện ngay nếu hai đường đi lệch nhau.
1. **Đo trước khi sửa.** Viết một script Playwright nhỏ (mẫu: `scripts/verify-phase*.js`, hoặc
   `python3 -c` như trên) trả về, cho từng case: option text có whitespace đầu/cuối, có chuỗi
   whitespace bên trong, và `value="  x  "` (attribute có space). **Chấp nhận kết quả đo được,
   không chấp nhận suy luận từ spec.** Dán output vào §Status.
2. Sửa `optionValue` theo đúng measurements ở (1): nhiều khả năng là "strip ASCII whitespace hai
   đầu + collapse chuỗi whitespace bên trong thành một U+0020" cho *text content*, còn *value
   attribute* thì xử lý theo measurement chứ không theo trí nhớ.
3. `SelectedOption`: đổi tie-break thành **option `selected` cuối cùng** thắng, vẫn fallback
   option đầu tiên. Phải kiểm tra bàn phím (TASK-030, `MoveSelect` ở
   `internal/engine/select.go`) không đổi hành vi trên markup một option selected — đó là ca thật.
4. `accessibility.go`: label và value của cùng một option phải dùng **chung** một helper normalize,
   để không còn tình trạng `Label` đã sạch còn `Value` thì chưa.
5. Không đụng paint, không đụng layout, không đụng parser. `internal/dom` giữ nguyên text node —
   normalize là chuyện của *giá trị*, không phải của nội dung hiển thị.

## 3. Test phải có (tất cả ở `test/engine/`, không cần display)

1. Option text `\n  Charcoal\n` selected ⇒ `engine.CollectFormData` trả `c` = `Charcoal`, và
   `EncodeFormURLEncoded` **không** chứa `%0A`.
2. Option text `"  Hi   there "` ⇒ giá trị `"Hi there"` (collapse, không chỉ strip).
3. `value` attribute có whitespace bao quanh ⇒ đúng bằng kết quả đo ở §2.1, và test phải ghi rõ
   nó đo từ Chromium chỗ nào.
4. Hai option cùng `selected` ⇒ giá trị là option **cuối** (theo Chromium), có comment dẫn chứng
   đo được ở §1.
5. Oracle chéo đã tồn tại: `TestAccessibilitySelectValueMatchesSubmittedFormValue` — giá trị AX và
   giá trị submit phải một. Test này là lý do sửa ở (2) và (4) phải làm **cùng lúc**; giữ nó xanh.
6. Keyboard: `FocusNode(sel)` + `MoveSelect(EditDown)` trên markup thụt lề ⇒ value đổi đúng option,
   AX theo đúng DOM (nối BR-101 với BR-139 như TASK-032 đã làm).
7. Tag: `// qc:BR-104` cho test 1, 2, 4 (submit phải mang đúng value), `// qc:BR-106` cho test 1
   vì nó kiểm luôn `EncodeFormURLEncoded`, `// qc:BR-101` cho test 6 (bàn phím đổi selection),
   `// qc:BR-139` cho test 5 đã có sẵn. Chạy `go test -count=1 ./test/qc/ -update` và kiểm tra
   golden chỉ đổi các row tương ứng, rồi nâng sàn trong `docs/qa-ratchet.json` nếu số
   P0/P1 được tag tăng lên.

## 4. Nghiệm thu `[R-7]`

Mỗi proof là **tắt hành vi**, dán `FAIL → revert → PASS`:

* (i) `optionValue` trả `n.TextContent()` thô (bỏ normalize) ⇒ test 1 và 2 **FAIL**.
* (ii) chỉ strip hai đầu, không collapse ⇒ test 2 **FAIL** (mutation dễ lọt nhất vì ca "Charcoal"
  vẫn xanh).
* (iii) `SelectedOption` quay về first-wins ⇒ test 4 **FAIL**.
* (iv) normalize cả nhánh `value` attribute (nếu §2.1 đo được Chromium **không** làm thế) ⇒ test 3
  **FAIL**. Nếu measurement cho thấy Chromium cũng strip attribute, thì chứng minh theo chiều
  ngược lại: bỏ strip attribute ⇒ test 3 **FAIL**.
* (v) sửa `forms.go` nhưng để `accessibility.go` tự tính giá trị riêng ⇒ test 5 **FAIL**. Đây là
  mutation quan trọng nhất của task này.

Proof nào không giết được test nào thì khai trong §Status.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không worktree con. `go test ./internal/archtest/` và
`go vet -all ./...` xanh. Test không cần display. `make parity` phải **không đổi** kết quả
(80/80, average 96.07%, known gaps 5 fixtures/10 bands) — task này không được động vào ảnh; nếu
fixture `forms/*` đổi band thì phải giải thích được bằng số dòng, không được sửa baseline.
Không sửa `docs/qa-test-catalog.md`; đề xuất dòng registry `GOOSIE-FORMVALUE-001` trong báo cáo.

### 5.1 Phép đo Chromium — chạy TRƯỚC khi sửa, và chạy lại sau khi sửa

```
$ node scripts/verify-task037-option-value.js   # exit 0
```

Kịch bản nằm lại trong repo (`scripts/verify-task037-option-value.js`) nên một luật
đã đo có thể đo lại. Máy: macOS 26.6.2 arm64, Playwright 1.63.0, Chromium
**153.0.8010.12**, `node v26.5.0`, `go1.26.10 darwin/arm64`. Output nguyên văn:

```
OPTION name=charcoal -> {"textContent":"\n    Charcoal\n  ","value":"Charcoal","label":"Charcoal"}
OPTION name=hiruns -> {"textContent":"  Hi   there ","value":"Hi there","label":"Hi there"}
OPTION name=tabinside -> {"textContent":"Tab\tin\tside","value":"Tab in side","label":"Tab in side"}
OPTION name=crlfinside -> {"textContent":"a\n   b","value":"a b","label":"a b"}
OPTION name=formfeedinside -> {"textContent":"a\f\fb","value":"a b","label":"a b"}
OPTION name=vtabinside -> {"textContent":"a\u000b\u000bb","value":"a\u000b\u000bb","label":"a\u000b\u000bb"}
OPTION name=edgevtab -> {"textContent":"\u000bleading vt\u000b","value":"\u000bleading vt\u000b","label":"\u000bleading vt\u000b"}
OPTION name=nbsp -> {"textContent":" nbsp edge ","value":" nbsp edge ","label":" nbsp edge "}
OPTION name=blankonly -> {"textContent":"   ","value":"","label":""}
OPTION name=elementkids -> {"textContent":"a bold   c","value":"a bold c","label":"a bold c"}
OPTION name=labelattr -> {"textContent":"text content","value":"text content","label":"  My Label  "}
OPTION name=attrpad -> {"textContent":"ignored text","value":"  pad  me  ","label":"ignored text"}
OPTION name=attrnewline -> {"textContent":"ignored text","value":"a\n b\tc","label":"ignored text"}
OPTION name=attrempty -> {"textContent":"ignored text","value":"","label":"ignored text"}
OPTION name=attrnbsp -> {"textContent":"ignored text","value":" x ","label":"ignored text"}
FORMDATA -> [["charcoal","Charcoal"],["hiruns","Hi there"],["tabinside","Tab in side"],["crlfinside","a b"],["formfeedinside","a b"],["vtabinside","a\u000b\u000bb"],["edgevtab","\u000bleading vt\u000b"],["nbsp"," nbsp edge "],["blankonly",""],["elementkids","a bold c"],["labelattr","text content"],["attrpad","  pad  me  "],["attrnewline","a\n b\tc"],["attrempty",""],["attrnbsp"," x "]]
ENCODED -> "charcoal=Charcoal&hiruns=Hi+there&tabinside=Tab+in+side&crlfinside=a+b&formfeedinside=a+b&vtabinside=a%0B%0Bb&edgevtab=%0Bleading+vt%0B&nbsp=%C2%A0nbsp+edge%C2%A0&blankonly=&elementkids=a+bold+c&labelattr=text+content&attrpad=++pad++me++&attrnewline=a%0A+b%09c&attrempty=&attrnbsp=%C2%A0x%C2%A0"
TIEBREAK two-selected -> {"selectedIndex":1,"value":"b","optionFlags":[false,true,false],"attrFlags":[true,true,false],"formdata":[["c","b"]]}
CLOSED two-selected after ArrowDown -> {"active":"s","selectedIndex":1,"value":"b"}
LISTBOX two-selected initial -> {"active":"s","selectedIndex":1,"value":"b","optionFlags":[false,true,false],"attrFlags":[true,true,false]}
LISTBOX two-selected after ArrowDown -> {"active":"s","selectedIndex":2,"value":"c","optionFlags":[false,false,true],"attrFlags":[true,true,false]}
LISTBOX two-selected after ArrowUp -> {"active":"s","selectedIndex":1,"value":"b","optionFlags":[false,true,false],"attrFlags":[true,true,false]}
LISTBOX single-selected initial -> {"active":"s","selectedIndex":1,"value":"b","optionFlags":[false,true,false],"attrFlags":[false,true,false]}
LISTBOX single-selected after ArrowDown -> {"active":"s","selectedIndex":2,"value":"c","optionFlags":[false,false,true],"attrFlags":[false,true,false]}
INDENTED initial -> {"selectedIndex":0,"value":"Charcoal","optionValue":"Charcoal","optionLabel":"Charcoal"}
INDENTED after ArrowDown -> {"selectedIndex":0,"value":"Charcoal","optionValue":"Charcoal","optionLabel":"Charcoal"}
```

### 5.2 Luật đã hiện thực, trích đúng dòng đo ở §5.1

| Luật | Dòng đo | Code |
| --- | --- | --- |
| Text content: bỏ ASCII whitespace hai đầu, **collapse mọi chuỗi whitespace bên trong thành một U+0020** | `OPTION name=charcoal`, `hiruns`, `tabinside`, `crlfinside`, `formfeedinside`, `elementkids`, `blankonly` | `optionTextValue` (`internal/engine/focus.go`) |
| Tập byte bị collapse **không** gồm U+000B, **không** gồm U+00A0 — nên đây không phải `strings.TrimSpace` hay `unicode.IsSpace` | `vtabinside`/`edgevtab` giữ `a\u000b\u000bb`; `nbsp` giữ `\u00a0` hai đầu | `isASCIISpace` (`focus.go`) |
| `value` attribute: dùng **nguyên bản**, kể cả whitespace và cả chuỗi rỗng (`value=""` submit `""`, không fallback về text) | `attrpad` → `"  pad  me  "`, `attrnewline` → `"a\n b\tc"`, `attrempty` → `""`, `attrnbsp` → `"\u00a0x\u00a0"`; `FORMDATA`/`ENCODED` cùng dòng | `optionValue` nhánh attribute, giữ nguyên |
| Nhiều option cùng `selected`: cái **cuối** thắng; Chromium **không** viết lại content attribute | `TIEBREAK two-selected -> {"selectedIndex":1,"value":"b","optionFlags":[false,true,false],"attrFlags":[true,true,false],"formdata":[["c","b"]]}` | `SelectedOption` (`focus.go`) |
| Bàn phím bước từ option đang thắng; markup **một** option selected không đổi hành vi | `LISTBOX two-selected initial/after ArrowDown/after ArrowUp` (1→2→1) và control `LISTBOX single-selected *` (1→2) | `MoveSelect` (`internal/engine/select.go`) |
| Label AX của option dùng **chung** helper với value | `OPTION name=charcoal -> {"value":"Charcoal","label":"Charcoal"}`, `INDENTED initial` | `internal/engine/accessibility.go:174` (label option), `:192` (label fallback của combo) |

Ghi chú thành thật: select **đóng** (không có `size`) không responded với phím mũi
tên trong headless Chromium trên máy này — `CLOSED two-selected after ArrowDown ->
{"active":"s","selectedIndex":1,"value":"b"}` — nên tie-break của bàn phím đo trên
listbox `size=3`, và `INDENTED after ArrowDown` chỉ xác nhận giá trị khởi điểm
`"Charcoal"`. Phần "một phím mũi tên bước sang option kế" là hợp đồng bàn phím
TASK-030 của Goosie, đã có oracle riêng.

### 5.3 Oracle mới, và bằng chứng nó đỏ trên `main`

`test/engine/option_value_test.go` (7 test, không cần display):
`TestSubmittedOptionValueStripsSurroundingWhitespace`,
`TestSubmittedOptionValueCollapsesInnerWhitespaceRuns` (9 case),
`TestOptionValueAttributeIsSubmittedVerbatim` (4 case + encoding),
`TestLastAuthoredSelectedOptionWins`,
`TestAccessibilityOptionLabelAndValueShareNormalization`,
`TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX`,
`TestMoveSelectStepsFromTheLastAuthoredSelection`.
Sửa một expectation cũ: `test/engine/select_keyboard_test.go:TestSelectMoveCollapsesMultipleAuthoredSelections` (o2 → o3, theo §5.1).

Chạy bộ test mới với `internal/engine/` đưa về trạng thái `main` — trên branch này
implement đã commit nên `git stash` không hoàn tác được gì, phải checkout thẳng:

```
$ git checkout 6071b8b -- internal/engine        # exit 0
$ go test -count=1 ./test/engine/ -run '...'     # exit 1, output dưới đây
$ git checkout HEAD -- internal/engine           # exit 0, `git status` chỉ còn file docs
```

```
$ go test -count=1 ./test/engine/ -run 'TestSubmittedOptionValueStripsSurroundingWhitespace|TestSubmittedOptionValueCollapsesInnerWhitespaceRuns|TestOptionValueAttributeIsSubmittedVerbatim|TestLastAuthoredSelectedOptionWins|TestAccessibilityOptionLabelAndValueShareNormalization|TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX|TestMoveSelectStepsFromTheLastAuthoredSelection'
exit 1
--- FAIL: TestSubmittedOptionValueStripsSurroundingWhitespace (0.00s)
    option_value_test.go:70: submitted c = "\n  Charcoal\n", want "Charcoal": the option's text content is indented over three lines and Chromium reports the stripped word
--- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns (0.00s)
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/inner_run_of_spaces (0.00s)
        option_value_test.go:114: submitted value = "  Hi   there ", want "Hi there" (text content was "  Hi   there ")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_inside (0.00s)
        option_value_test.go:114: submitted value = "Tab\tin\tside", want "Tab in side" (text content was "Tab\tin\tside")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/newline_and_spaces_inside (0.00s)
        option_value_test.go:114: submitted value = "a\n   b", want "a b" (text content was "a\n   b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/form_feed_inside (0.00s)
        option_value_test.go:114: submitted value = "a\f\fb", want "a b" (text content was "a\f\fb")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_and_newline_inside (0.00s)
        option_value_test.go:114: submitted value = "a\t\n b", want "a b" (text content was "a\t\n b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/text_split_over_element_children (0.00s)
        option_value_test.go:114: submitted value = "a bold   c", want "a bold c" (text content was "a <b>bold</b>   c")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/only_whitespace (0.00s)
        option_value_test.go:114: submitted value = "   ", want "" (text content was "   ")
--- FAIL: TestLastAuthoredSelectedOptionWins (0.00s)
    option_value_test.go:178: submitted c = "a", want "b": the last option authored selected wins
--- FAIL: TestAccessibilityOptionLabelAndValueShareNormalization (0.00s)
    option_value_test.go:215: combo box announces value "\n  Charcoal\n", want "Charcoal"
--- FAIL: TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX (0.00s)
    option_value_test.go:302: FocusedSelectValue before any key = "\n  Charcoal\n", want "Charcoal"
--- FAIL: TestMoveSelectStepsFromTheLastAuthoredSelection (0.00s)
    option_value_test.go:363: value before any key = "1", want 2: the last authored selection wins
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.564s
FAIL
```

Riêng `TestOptionValueAttributeIsSubmittedVerbatim` xanh từ đầu tới cuối: nó là
**chốt chặn** (branch attribute vốn đã nguyên bản), không phải test hồi quy — §5.4
(iv) chứng minh nó có răng.

### 5.4 Sáu phép đột biến [R-7] — mỗi phép đều giết ít nhất một test

Mỗi phép: sửa source → `go test -count=1 ./test/engine/ -run ...` → dán output →
`git checkout HEAD -- <file>` (exit 0) → chạy tiếp phép sau. Cả sáu phép được chạy lại
trong **một lượt script** sau commit `d4dfd7c`, nên mọi dòng số test được trích dưới đây
khớp với file `test/engine/option_value_test.go` đã commit. Cùng lượt đó kết thúc bằng
một lần chạy control với implement nguyên vẹn — **exit 0**:

```
=== CONTROL: implementation restored, same package run ===
EXIT: 0
ok  	github.com/vyquocvu/goosie/test/engine	0.807s
```

(`go test -count=1 ./...` toàn repo cũng exit 0 — §5.6.)

**(i) `optionValue` trả `n.TextContent()` thô — bỏ normalize.** `go test -count=1 ./test/engine/ -run 'TestSubmittedOptionValueStripsSurroundingWhitespace|TestSubmittedOptionValueCollapsesInnerWhitespaceRuns'` → **exit 1**:

```
--- FAIL: TestSubmittedOptionValueStripsSurroundingWhitespace (0.00s)
    option_value_test.go:70: submitted c = "\n  Charcoal\n", want "Charcoal": the option's text content is indented over three lines and Chromium reports the stripped word
--- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns (0.00s)
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/inner_run_of_spaces (0.00s)
        option_value_test.go:114: submitted value = "  Hi   there ", want "Hi there" (text content was "  Hi   there ")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_inside (0.00s)
        option_value_test.go:114: submitted value = "Tab\tin\tside", want "Tab in side" (text content was "Tab\tin\tside")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/newline_and_spaces_inside (0.00s)
        option_value_test.go:114: submitted value = "a\n   b", want "a b" (text content was "a\n   b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/form_feed_inside (0.00s)
        option_value_test.go:114: submitted value = "a\f\fb", want "a b" (text content was "a\f\fb")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_and_newline_inside (0.00s)
        option_value_test.go:114: submitted value = "a\t\n b", want "a b" (text content was "a\t\n b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/text_split_over_element_children (0.00s)
        option_value_test.go:114: submitted value = "a bold   c", want "a bold c" (text content was "a <b>bold</b>   c")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/only_whitespace (0.00s)
        option_value_test.go:114: submitted value = "   ", want "" (text content was "   ")
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.811s
FAIL
```

**(ii) Chỉ strip hai đầu, không collapse** (phép dễ lọt nhất; test 1 vẫn xanh, đúng như
§4 cảnh báo). `go test -count=1 ./test/engine/ -run 'TestSubmittedOptionValueStripsSurroundingWhitespace|TestSubmittedOptionValueCollapsesInnerWhitespaceRuns|TestAccessibilityOptionLabelAndValueShareNormalization'` → **exit 1**:

```
--- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns (0.00s)
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/inner_run_of_spaces (0.00s)
        option_value_test.go:114: submitted value = "Hi   there", want "Hi there" (text content was "  Hi   there ")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_inside (0.00s)
        option_value_test.go:114: submitted value = "Tab\tin\tside", want "Tab in side" (text content was "Tab\tin\tside")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/newline_and_spaces_inside (0.00s)
        option_value_test.go:114: submitted value = "a\n   b", want "a b" (text content was "a\n   b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/form_feed_inside (0.00s)
        option_value_test.go:114: submitted value = "a\f\fb", want "a b" (text content was "a\f\fb")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/tabs_and_newline_inside (0.00s)
        option_value_test.go:114: submitted value = "a\t\n b", want "a b" (text content was "a\t\n b")
    --- FAIL: TestSubmittedOptionValueCollapsesInnerWhitespaceRuns/text_split_over_element_children (0.00s)
        option_value_test.go:114: submitted value = "a bold   c", want "a bold c" (text content was "a <b>bold</b>   c")
--- FAIL: TestAccessibilityOptionLabelAndValueShareNormalization (0.00s)
    option_value_test.go:245: option node is labelled "Hi   there" and valued "Hi   there", want "Hi there" for both
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	1.344s
FAIL
```

**(iii) `SelectedOption` quay về first-wins.** `go test -count=1 ./test/engine/ -run 'TestLastAuthoredSelectedOptionWins|TestAccessibilitySelectValueMatchesSubmittedFormValue|TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX'` → **exit 1** (chỉ oracle tie-break đỏ; oracle AX cũ và test keyboard markup một-option-selected vẫn xanh, đúng vì chúng không có hai `selected`):

```
--- FAIL: TestLastAuthoredSelectedOptionWins (0.00s)
    option_value_test.go:178: submitted c = "a", want "b": the last option authored selected wins
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	1.031s
FAIL
```

**(iv) Normalize cả nhánh `value` attribute** (measurement ở §5.1 nói Chromium **không** làm thế, nên đây là hướng đột biến đúng của §4). `go test -count=1 ./test/engine/ -run 'TestOptionValueAttributeIsSubmittedVerbatim|TestSubmittedOptionValueStripsSurroundingWhitespace|TestLastAuthoredSelectedOptionWins'` → **exit 1**, trong khi test 1 và test 4 vẫn xanh:

```
--- FAIL: TestOptionValueAttributeIsSubmittedVerbatim (0.00s)
    --- FAIL: TestOptionValueAttributeIsSubmittedVerbatim/padded_attribute (0.00s)
        option_value_test.go:147: submitted value = "pad me", want "  pad  me  " verbatim from the value attribute
    --- FAIL: TestOptionValueAttributeIsSubmittedVerbatim/attribute_with_newline_and_tab (0.00s)
        option_value_test.go:147: submitted value = "a b c", want "a\n b\tc" verbatim from the value attribute
    option_value_test.go:156: encoded form = "c=pad+me", want "c=++pad++me++": Chromium sends "  pad  me  " for this attribute
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.754s
FAIL
```

**(v) `accessibility.go` tự tính giá trị select từ text của option được chọn** —
phép quan trọng nhất. `go test -count=1 ./test/engine/ -run 'TestAccessibilitySelectValueMatchesSubmittedFormValue|TestAccessibilityOptionLabelAndValueShareNormalization'`. Lần chạy đầu, oracle chéo cũ chết còn test mới thì không
(tất cả option của nó có text == value), nên test đã được siết thêm một case
`<option selected value="slate-blue">` thụt lề; chạy lại cả hai: **exit 1**:

```
--- FAIL: TestAccessibilitySelectValueMatchesSubmittedFormValue (0.00s)
    --- FAIL: TestAccessibilitySelectValueMatchesSubmittedFormValue/value_attribute (0.00s)
        accessibility_test.go:375: accessibility announces "Blue" but the form submits "blue": one control, two answers
--- FAIL: TestAccessibilityOptionLabelAndValueShareNormalization (0.00s)
    option_value_test.go:261: combo box announces value "Slate blue", want the value attribute "slate-blue" it submits
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.742s
FAIL
```

**(vi) `MoveSelect` bước từ `selected` đầu tiên** → test 7 và test keyboard cũ
cùng đỏ, còn **mọi** test keyboard trên markup một option selected vẫn xanh
(§2.3 yêu cầu đúng ca đó):

`go test -count=1 ./test/engine/ -run 'TestMoveSelectStepsFromTheLastAuthoredSelection|TestSelectMoveCollapsesMultipleAuthoredSelections|TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX|TestSelectArrowKeysMoveSelection|TestSelectSkipsDisabledOptions|TestSelectHomeEndGoToTheEdges|TestSelectKeysAreConsumedAtTheEnds|TestSelectMoveChangesSubmittedValue'` → **exit 1**:

```
--- FAIL: TestMoveSelectStepsFromTheLastAuthoredSelection (0.00s)
    option_value_test.go:369: value after EditDown = "2", want 3: a move steps from the winning selection
--- FAIL: TestSelectMoveCollapsesMultipleAuthoredSelections (0.00s)
    select_keyboard_test.go:246: move from a two-selected document landed on "o2", want o3: the last authored selection is what a move steps from
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.807s
FAIL
```

Không phép nào sống. (v) là phép duy nhất cần siết oracle — khai ở §5.8.

### 5.5 Tag QC: **đề xuất**, chưa commit — và bằng chứng vì sao không thể commit

Ba file `test/qc/traceability.golden.json`, `docs/qa-ratchet.json`,
`docs/qa-test-catalog.md` thuộc quyền integrator, nên branch này không động vào.
Dòng tag đề nghị, đặt ngay trên `func` tương ứng trong `test/engine/option_value_test.go`:

| Test | Dòng tag |
| --- | --- |
| `TestSubmittedOptionValueStripsSurroundingWhitespace` | `// qc:BR-104 BR-106` |
| `TestSubmittedOptionValueCollapsesInnerWhitespaceRuns` | `// qc:BR-104` |
| `TestOptionValueAttributeIsSubmittedVerbatim` | `// qc:BR-104` |
| `TestLastAuthoredSelectedOptionWins` | `// qc:BR-104` |
| `TestAccessibilityOptionLabelAndValueShareNormalization` | `// qc:BR-139 BR-104` |
| `TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX` | `// qc:BR-101 BR-139` |
| `TestMoveSelectStepsFromTheLastAuthoredSelection` | `// qc:BR-101` |

Đã thử **lại** đúng trình tự §3.7 trên branch này với file test đã commit (`d4dfd7c`):
đặt 7 dòng tag tạm → `go test -count=1 ./test/qc/` **exit 1** → `-update` → `git diff` →
revert cả tag lẫn golden (`git checkout HEAD -- ...`, exit 0). Đặt tag mà chưa
regenerate golden thì đỏ ở `traceability_test.go:112`:

```
--- FAIL: TestTraceability (0.22s)
    traceability_test.go:112: traceability.golden.json is stale; run go test ./test/qc -update
```

Chạy `go test -count=1 ./test/qc/ -update` (exit 0) rồi `git diff --stat` thì golden chỉ
đổi **bốn row BR-101/BR-104/BR-106/BR-139**, không row nào khác:

```
 test/qc/traceability.golden.json | 16 +++++++++++++---
 1 file changed, 13 insertions(+), 3 deletions(-)
```

So sánh hai biến thể golden bằng JSON cho ra đúng bảy ref mới, phân phối như bảng tag:

| Row | Ref thêm vào mảng `tests` |
| --- | --- |
| BR-101 | `option_value_test.go:TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX`, `:TestMoveSelectStepsFromTheLastAuthoredSelection` |
| BR-104 | `:TestAccessibilityOptionLabelAndValueShareNormalization`, `:TestLastAuthoredSelectedOptionWins`, `:TestOptionValueAttributeIsSubmittedVerbatim`, `:TestSubmittedOptionValueCollapsesInnerWhitespaceRuns`, `:TestSubmittedOptionValueStripsSurroundingWhitespace` |
| BR-106 | `:TestSubmittedOptionValueStripsSurroundingWhitespace` |
| BR-139 | `:TestAccessibilityOptionLabelAndValueShareNormalization`, `:TestKeyboardSelectOnIndentedMarkupMovesAndUpdatesAX` |

Đếm lại theo đúng `tagged_groups()` của `scripts/rc-sweep.sh:106-119` (đếm **case** có
`tests` không rỗng, không đếm test) trên **cả hai** biến thể golden:

```
base golden  : p0_tagged=19 p01_tagged=75  floors=19/75
tagged golden: p0_tagged=19 p01_tagged=75  floors=19/75
```

vì **sàn ratchet không đổi** — cả bốn case đã có ít nhất một test được tag từ trước,
thêm test chỉ làm mảng `tests` dài ra. `docs/qa-ratchet.json` không cần nâng. Đề xuất
dòng registry cho defect:
**`GOOSIE-FORMVALUE-001`** — *"giá trị `<option>` không normalize (whitespace hai đầu +
chuỗi bên trong) và tie-break `selected` lấy cái đầu tiên; submit nhận
`?c=%0A++Charcoal%0A` và AX báo một giá trị khác giá trị submit"* — method INT,
BR-101/BR-104/BR-106/BR-139, đóng bằng `test/engine/option_value_test.go` +
oracle chéo `TestAccessibilitySelectValueMatchesSubmittedFormValue`.
Sau khi revert, `go test -count=1 ./test/qc/` **exit 0** (branch xanh thật, không phải
xanh do chưa tag).

### 5.6 Các gate đã chạy trên branch (commit `06aee6c` + `d4dfd7c`)

```
gofmt -l internal test                                  → rỗng, exit 0
go vet -all ./...                                       → exit 0, không output
go test -count=1 ./...                                  → exit 0 (42 package ok, 0 FAIL)
go test -count=1 ./internal/archtest/                   → exit 0 (ok)
go test -count=1 ./test/gate -run TestGate -v           → exit 0 (8 top-level PASS, 0 FAIL)
go test -count=1 ./test/paint/ ./test/raster/ ./test/surface/ ./test/gate/ → exit 0 (4 ok — nền cho §5.7)
go test -count=1 -race ./test/engine/ ./internal/engine/ ./test/qc/ → exit 0
node scripts/verify-task037-option-value.js             → exit 0 (output ở §5.1)
```

### 5.7 `make parity`: **đã không chạy**, và vì sao kết quả không thể đổi

Ràng buộc của task nói `make parity` phải không đổi (80/80, 96.07%, gaps 5/10) nhưng
hướng dẫn vận hành lần này cấm chạy `testdata/parity.py --render` (thư mục `/tmp`
render chung đang thuộc một agent khác) và cấm `live-parity.py` (sweep 100 trang đang
chạy ở checkout chính). Nên con số 80/80 **không** được đo lại ở đây — đó là phần
integrator phải chạy. Bằng chứng gián tiếp rằng nó không đổi — `git diff --stat 6071b8b HEAD`
(chỉ code + test; file docs chứa §5 này là file thứ bảy và thuần tài liệu):

```
 internal/engine/accessibility.go       |   6 +-
 internal/engine/focus.go               |  77 ++++++-
 internal/engine/select.go              |  15 +-
 scripts/verify-task037-option-value.js | 152 +++++++++++++
 test/engine/option_value_test.go       | 387 +++++++++++++++++++++++++++++++++
 test/engine/select_keyboard_test.go    |  11 +-
 6 files changed, 632 insertions(+), 16 deletions(-)
```

Không một dòng nào trong `internal/paint`, `internal/layout`, `internal/frame`,
`internal/raster`, `internal/surface`, `testdata/**`, `MIN_SCORED_FIXTURES`
(hay `testdata/parity.py`): **không fixture nào được thêm**, nên không có chuyện nâng sàn
fixture. Đường vẽ select (`internal/paint/builder_controls.go:271` `paintSelect`) đọc
`TextContent()` và attribute `selected` trực tiếp, không đi qua `optionValue`, nên ảnh
pixel không thể đổi; tie-break của nó vốn đã là "cái cuối thắng" nên từ lần sửa này nó
**đồng ý kiến** với value thay vì lệch. Suite `./test/paint/ ./test/raster/
./test/surface/ ./test/gate/` xanh.

### 5.8 Nợ còn lại — khai rõ, không để lần sau đoán

* **Tag QC + golden**: chưa nộp (xem §5.5). Branch không tự tag vì sẽ làm
  `./test/qc` đỏ.
* **`make parity` / live sweep**: chưa chạy lại ở lần này (§5.7).
* **`option:checked` trong CSS** (`internal/css/selector.go:882-910`) vẫn phân định
  tie-break theo **`selected` đầu tiên**: guard `&& selected == nil` ở `:901` và
  `return selected == nil && first == n` ở `:909`. Với markup hai option cùng
  `selected`, `:checked` giờ chọn option **khác** option mà form submit và AX
  công bố. **Chưa oracle nào chắn lệch này**: test duy nhất về `option:checked`
  (`test/css/interactive_test.go:202-214`) chỉ đặt `selected` lên **một** option nên
  nó vẫn xanh — đây là lệch thật chưa được đo, không phải lệch đã có test đỏ.
  Ngoài phạm vi §2 (và sửa nó là đụng `internal/css`), nên nợ.
* **`<optgroup>`**: `SelectedOption` và `accessibility.go` vẫn chỉ duyệt con trực tiếp,
  nên option lồng trong optgroup không tới được bằng bàn phím và không có node AX —
  chính là §6, task riêng.
* **`label` attribute của `<option>`**: Chromium trả `option.label` nguyên bản
  (`"  My Label  "`, measurement `labelattr`) và dùng nó làm text hiển thị; Goosie vẫn
  lấy text content cho cả label lẫn value. Không đổi vì §2.4 chỉ yêu cầu label và value
  **chung một helper normalize**.
* **`paintSelect` vẫn vẽ text thô**: thụt lề ba dòng thì chữ trong ô vẫn là chuỗi gốc
  chưa collapse (§2.5: không đụng paint). Đây là lý do thật sự khiến ảnh có thể lệch
  Chromium về khoảng trắng trong ô select — nhưng corpus hiện tại không buộc, và task
  này cấm đụng ảnh.
* **`option.value` trong JS** (`internal/js/js.go:809-823` `__getValue__`) trả `""` cho
  option không có `value` attribute, thay vì text đã normalize như Chromium đo ở §5.1.
  Ngoài phạm vi: `internal/js` không nằm trong ba đường (submit / bàn phím / AX) của task.
* **`<select multiple>`, `size>1`, `<datalist>`, và việc input/textarea có cần normalize
  tương tự**: §6.
* **Phép (v) cần siết oracle**: bản đầu của `TestAccessibilityOptionLabelAndValueShareNormalization`
  **sống sót** đột biến (v) vì mọi option nó dùng có text == value. Đã sửa bằng case
  `value="slate-blue"` (commit `d4dfd7c`). Đây là lỗ hổng oracle do chính task này tìm ra,
  ghi lại để lần sau viết cross-check thì chọn markup mà hai đường *lệch nhau hữu ý*.

## 6. Ngoài phạm vi

`<optgroup>` — engine đã ghi nhận chưa hỗ trợ ngay trong code
(`internal/engine/select.go:46-48`: option lồng trong optgroup không tới được bằng bàn phím), nên
đó là task riêng; `<select multiple>`, `<datalist>`, `size>1`, và việc textarea/input có cần
normalize tương tự hay không.
