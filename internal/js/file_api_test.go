package js

import (
	"bytes"
	goruntime "runtime"
	"testing"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestFileDataTransferInputFilesAndFormDataPreserveIdentity(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var file = new File(['abc'], 'folder/test.txt', {type: 'TEXT/PLAIN', lastModified: 7});
		var transfer = new DataTransfer();
		var dataTransferBrand = transfer instanceof DataTransfer && Object.prototype.toString.call(transfer) === '[object DataTransfer]' && transfer.constructor === DataTransfer;
		var fileItem = transfer.items.add(file);
		var fileItemStable = fileItem === transfer.items[0] && fileItem === transfer.items[0];
		var fileFromItemA = fileItem.getAsFile();
		var fileFromItemB = fileItem.getAsFile();
		var getAsFileReturnsFreshFile = fileFromItemA instanceof File && fileFromItemA !== file &&
			fileFromItemB instanceof File && fileFromItemB !== fileFromItemA &&
			fileFromItemA.name === file.name && fileFromItemA.type === file.type &&
			fileFromItemA.lastModified === file.lastModified && fileFromItemA.size === file.size;
		var input = document.createElement('input');
		input.type = 'file';
		input.name = 'upload';
		input.files = transfer.files;
		var sharedInput = document.createElement('input');
		sharedInput.type = 'file';
		sharedInput.files = transfer.files;
		var inputFileListIdentity = input.files === transfer.files && sharedInput.files === transfer.files;
		input.files = null;
		var nullAssignmentPreservesFiles = input.files === transfer.files && input.files.length === 1;
		var arrayAssignmentRejects = false;
		try { input.files = []; } catch (error) { arrayAssignmentRejects = error instanceof TypeError; }
		var form = document.createElement('form');
		form.appendChild(input);
		document.body.appendChild(form);
		var data = new FormData(form);
		var fakePathMatchesFile = input.value.split(/[\\/]/).pop() === file.name.split(/[\\/]/).pop();
		var fileInputRejectsNonEmpty = false;
		try { input.value = 'not-allowed.txt'; } catch (error) { fileInputRejectsNonEmpty = error.name === 'InvalidStateError'; }
		window.fileAPIRoundTrip = file instanceof File && file instanceof Blob &&
			file.name === 'folder/test.txt' && file.size === 3 && file.type === 'text/plain' &&
			file.lastModified === 7 && transfer.files.length === 1 &&
			dataTransferBrand && fileItemStable && fileItem.kind === 'file' && getAsFileReturnsFreshFile &&
			inputFileListIdentity && nullAssignmentPreservesFiles && arrayAssignmentRejects &&
			input.files.item(0) === file && fakePathMatchesFile && fileInputRejectsNonEmpty &&
			data.get('upload') === file;
		input.value = '';
		window.fileInputClearWorks = input.value === '' && input.files.length === 0 &&
			sharedInput.files.length === 0 && transfer.files.length === 0 &&
			fileItem.kind === '' && fileItem.getAsFile() === null;
	`, "file-api-test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("fileAPIRoundTrip"); !ok || !got.Bool {
		t.Fatalf("File/DataTransfer/FileList/FormData identity = %+v (present %v), want true", got, ok)
	}
	for _, name := range []string{"fileFromItemA", "fileFromItemB"} {
		object, ok := runtime.vm.Get(name).(*goja.Object)
		if !ok || runtime.fileValues[object] == nil {
			t.Fatalf("%s is not a File object", name)
		}
		value := runtime.fileValues[object]
		if !bytes.Equal(value.data, []byte("abc")) || value.name != "folder/test.txt" || value.typeValue != "text/plain" || value.lastModified != 7 {
			t.Errorf("%s File contents/metadata = %q/%q/%q/%d, want abc/folder/test.txt/text/plain/7", name, value.data, value.name, value.typeValue, value.lastModified)
		}
	}
	if got, ok := runtime.Global("fileInputClearWorks"); !ok || !got.Bool {
		t.Fatalf("file input empty-string reset = %+v (present %v), want true", got, ok)
	}
}

func TestDataTransferItemListPreservesMixedInsertionOrder(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var transfer = new DataTransfer();
		var fileA = new File(['a'], 'a.txt', {type: 'text/a'});
		var fileB = new File(['b'], 'b.txt', {type: 'text/b'});
		transfer.items.add('first', 'text/first');
		transfer.items.add(fileA);
		transfer.items.add('second', 'text/second');
		var fileBItem = transfer.items.add(fileB);
		var initialOrder = [0,1,2,3].map(function(i) { return transfer.items[i].type; }).join(',');
		var stableFileItem = transfer.items[3] === fileBItem && transfer.items[3] === fileBItem;
		var initialIndexedAccess = transfer.items[0] === transfer.items[0] && transfer.items[2].type === 'text/second';
		transfer.items.remove(1);
		var orderAfterFileRemoval = [0,1,2].map(function(i) { return transfer.items[i].type; }).join(',');
		var shiftedIndexedAccess = transfer.items[1] === transfer.items[1] && transfer.items[2].type === 'text/b';
		transfer.setData('text/third', 'third');
		var orderAfterSetData = [0,1,2,3].map(function(i) { return transfer.items[i].type; }).join(',');
		transfer.clearData('text/first');
		var orderAfterClearOne = [0,1,2].map(function(i) { return transfer.items[i].type; }).join(',');
		transfer.clearData();
		var fileSurvivesClearData = transfer.items.length === 1 && transfer.items[0] === fileBItem && transfer.files.length === 1;
		transfer.items.clear();
		window.dataTransferMixedOrder = [initialOrder, stableFileItem, orderAfterFileRemoval, orderAfterSetData,
			orderAfterClearOne, fileSurvivesClearData, transfer.items.length === 0, transfer.files.length === 0,
			initialIndexedAccess, shiftedIndexedAccess, transfer.items[0] === undefined].join('|');
	`, "data-transfer-item-order"); err != nil {
		t.Fatal(err)
	}
	got, ok := runtime.Global("dataTransferMixedOrder")
	const want = "text/first,text/a,text/second,text/b|true|text/first,text/second,text/b|text/first,text/second,text/b,text/third|text/second,text/b,text/third|true|true|true|true|true|true"
	if !ok || got.Str != want {
		t.Fatalf("mixed DataTransferItemList order = %q (present %v), want %q", got.Str, ok, want)
	}
}

func TestChangingIntoFilenameStateClearsSelectedFiles(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var file = new File(['x'], 'x.txt');
		var transfer = new DataTransfer();
		transfer.items.add(file);
		var input = document.createElement('input');
		input.type = 'file';
		input.files = transfer.files;
		input.type = 'text';
		input.type = 'file';
		window.typeChangeClearedFiles = input.value === '' && input.files.length === 0;
	`, "input-type-file-transition"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("typeChangeClearedFiles"); !ok || !got.Bool {
		t.Fatalf("entering filename state cleared selected files = %+v (present %v), want true", got, ok)
	}
}

func TestBlobAndFilePartsUseFileAPIBlobPartAlgorithms(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var view = new Uint8Array([0, 128, 255, 65]);
		var rawView = new Blob([view.subarray(1, 3)]);
		var iterable = new Blob(new Set(['left', 'right']));
		var directViewSequence = new Blob(view);
		var composite = new File(['x', view.subarray(3), new Blob(['y'])], 'parts.bin');
		class DerivedFile extends File {}
		var derivedFile = new DerivedFile([], 'derived.bin');
		window.derivedFileInherits = derivedFile instanceof DerivedFile && derivedFile instanceof File && derivedFile instanceof Blob;
		var nativeLines = new Blob(['a\r\nb\rc\nd'], {endings: 'native'});
		var bodyFile = new File([document.body], document.body);
		var invalidSequenceThrows = false;
		var invalidEndingsThrows = false;
		try { new Blob({0: 'wrong', length: 1}); } catch (error) { invalidSequenceThrows = error instanceof TypeError; }
		try { new File([], 'bad', {endings: 'Native'}); } catch (error) { invalidEndingsThrows = error instanceof TypeError; }
	`, "file-api-algorithms"); err != nil {
		t.Fatal(err)
	}
	bytesFor := func(name string) []byte {
		t.Helper()
		value := runtime.vm.Get(name)
		object, ok := value.(*goja.Object)
		if !ok || runtime.fileValues[object] == nil {
			t.Fatalf("%s is not a Blob/File object", name)
		}
		return runtime.fileValues[object].data
	}
	if got := bytesFor("rawView"); !bytes.Equal(got, []byte{128, 255}) {
		t.Errorf("Blob(BufferSource part) bytes = %v, want [128 255]", got)
	}
	if got := string(bytesFor("iterable")); got != "leftright" {
		t.Errorf("Blob(iterable) text = %q, want %q", got, "leftright")
	}
	if got := string(bytesFor("directViewSequence")); got != "012825565" {
		t.Errorf("Blob(typed-array sequence) text = %q, want %q", got, "012825565")
	}
	if got := string(bytesFor("composite")); got != "xAy" {
		t.Errorf("File mixed BlobParts text = %q, want %q", got, "xAy")
	}
	ending := "\n"
	if goruntime.GOOS == "windows" {
		ending = "\r\n"
	}
	if got, want := string(bytesFor("nativeLines")), "a"+ending+"b"+ending+"c"+ending+"d"; got != want {
		t.Errorf("Blob native line endings = %q, want %q", got, want)
	}
	bodyFile := runtime.vm.Get("bodyFile").ToObject(runtime.vm)
	if got := runtime.fileValues[bodyFile].name; got != "[object HTMLBodyElement]" {
		t.Errorf("File object name conversion = %q, want [object HTMLBodyElement]", got)
	}
	if got := len(runtime.fileValues[bodyFile].data); got != 24 {
		t.Errorf("BlobPart DOM object size = %d, want 24", got)
	}
	if value, ok := runtime.Global("derivedFileInherits"); !ok || !value.Bool {
		t.Errorf("File subclass inheritance = %v, want true", value.Bool)
	}
	for _, name := range []string{"invalidSequenceThrows", "invalidEndingsThrows"} {
		value, ok := runtime.Global(name)
		if !ok || !value.Bool {
			t.Errorf("%s = %+v (present %v), want true", name, value, ok)
		}
	}
}

func TestBlobReadMethodsExposeFreshPromisesAndByteStreams(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		const blob = new Blob([new Uint8Array([80, 65, 83, 83])]);
		const bytes1 = blob.bytes();
		const bytes2 = blob.bytes();
		const stream = blob.stream();
		const reader = stream.getReader();
		window.blobReadSurface = [bytes1 instanceof Promise, bytes2 instanceof Promise,
			bytes1 !== bytes2, stream instanceof ReadableStream, stream.locked,
			reader.read() instanceof Promise].join("|");
	`, "blob-read-methods"); err != nil {
		t.Fatal(err)
	}
	if got, ok := runtime.Global("blobReadSurface"); !ok || got.Str != "true|true|true|true|true|true" {
		t.Fatalf("Blob read-method surface = %+v (present %v), want Promise/stream methods", got, ok)
	}
}
