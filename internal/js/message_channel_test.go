package js

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestMessageChannelQueuesAndTransfersMessagePorts(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var nested = new MessageChannel();
		var delivered = false;
		var transferredPortWorks = false;
		nested.port2.onmessage = function(event) { transferredPortWorks = event.data === 'through-transfer'; };
		channel.port2.onmessage = function(event) {
			delivered = event.data === 'hello' && event.ports.length === 1 &&
				event.ports[0] instanceof MessagePort &&
				Object.prototype.toString.call(event.ports[0]) === '[object MessagePort]';
			event.ports[0].postMessage('through-transfer');
		};
		channel.port1.postMessage('hello', [nested.port1]);
		window.channelDeliveredBeforeTick = delivered;
	`, "message-channel-test"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Global("channelDeliveredBeforeTick"); got.Bool {
		t.Fatal("MessagePort message was delivered synchronously")
	}
	r.Tick()
	if got, _ := r.Global("delivered"); !got.Bool {
		t.Fatal("MessagePort message or transferred port was not delivered correctly")
	}
	r.Tick()
	if got, _ := r.Global("transferredPortWorks"); !got.Bool {
		t.Fatal("transferred port was not entangled with its original peer")
	}
}

func TestMessagePortMessageEventIsTrustedAndBranded(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var delivered = false;
		var receivedEvent;
		channel.port2.onmessage = function(event) {
			receivedEvent = event;
			delivered = event instanceof MessageEvent && event.isTrusted === true &&
				event.data === 'ping' && event.origin === '' && event.lastEventId === '' &&
				event.source === null && event.ports.length === 0 &&
				event.target === channel.port2 && event.currentTarget === channel.port2;
		};
		channel.port1.postMessage('ping');
	`, "trusted-messageport-event-test"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("delivered"); !got.Bool {
		t.Fatal("MessagePort delivery did not expose a trusted MessageEvent")
	}
	for expr, want := range map[string]string{
		"receivedEvent.currentTarget === null":        "true",
		"receivedEvent.eventPhase === Event.NONE":     "true",
		"receivedEvent.target instanceof MessagePort": "true",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestMessagePortStructuredCloneSnapshotsObjectPayload(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var original = { nested: { value: 1 }, items: ["a"] };
		var received;
		var isIndependentClone = false;
		channel.port2.onmessage = function(event) {
			received = event.data;
			isIndependentClone = event.data !== original && event.data.nested !== original.nested;
		};
		channel.port1.postMessage(original);
		original.nested.value = 2;
		original.items.push("b");
	`, "message-port-clone-test"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []struct {
		name string
		want string
	}{
		{"isIndependentClone", "true"},
		{"received.nested.value", "1"},
		{"received.items.length", "1"},
	} {
		got, err := r.Evaluate(expr.name)
		if err != nil {
			t.Fatalf("evaluate %s: %v", expr.name, err)
		}
		if got != expr.want {
			t.Errorf("%s = %q, want %q", expr.name, got, expr.want)
		}
	}
}

func TestMessagePortStructuredClonePreservesSpecialPrimitivesAndBoxes(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var received;
		channel.port2.onmessage = function(event) { received = event.data; };
		var shared = { value: 1 };
		var payload = {
			undefinedValue: undefined,
			nan: NaN,
			positiveInfinity: Infinity,
			negativeInfinity: -Infinity,
			negativeZero: -0,
			bigint: -90071992547409940009007199254740994000n,
			boxedBoolean: new Boolean(false),
			boxedNumber: new Number(NaN),
			boxedString: new String('lone high surrogate: \uD800'),
			boxedBigInt: Object(-9007199254740994n),
			date: new Date(-8640000000000000),
			regexp: /foo/gy,
			emptyError: new Error(),
			typeError: Object.assign(new TypeError('clone me', {cause: {value: 2}}), {custom: true}),
			array: [undefined, NaN, 0n],
			left: shared,
			right: shared
		};
		payload.self = payload;
		payload.regexp.lastIndex = 7;
		channel.port1.postMessage(payload);
	`, "message-port-special-primitives"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"typeof received.undefinedValue === 'undefined'",
		"Number.isNaN(received.nan)",
		"received.positiveInfinity === Infinity && received.negativeInfinity === -Infinity",
		"Object.is(received.negativeZero, -0)",
		"received.bigint === -90071992547409940009007199254740994000n",
		"received.boxedBoolean instanceof Boolean && received.boxedBoolean.valueOf() === false",
		"received.boxedNumber instanceof Number && Number.isNaN(received.boxedNumber.valueOf())",
		"received.boxedString instanceof String && received.boxedString.valueOf() === 'lone high surrogate: \\uD800'",
		"received.boxedBigInt instanceof Object && received.boxedBigInt.valueOf() === -9007199254740994n",
		"received.date instanceof Date && received.date.getTime() === -8640000000000000",
		"received.regexp instanceof RegExp && received.regexp.source === 'foo' && received.regexp.flags === 'gy' && received.regexp.lastIndex === 0",
		"received.emptyError instanceof Error && !received.emptyError.hasOwnProperty('message') && received.emptyError.message === ''",
		"received.typeError instanceof TypeError && received.typeError.message === 'clone me' && received.typeError.cause.value === 2 && received.typeError.custom === undefined",
		"typeof received.array[0] === 'undefined' && Number.isNaN(received.array[1]) && received.array[2] === 0n",
		"received.self === received && received.left === received.right",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
}

func TestStructuredClonePreservesResizableBuffersAndViewModes(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var sourceBuffer = new ArrayBuffer(4, {maxByteLength: 16});
		new Uint8Array(sourceBuffer).set([11, 12, 13, 14]);
		var trackingView = new Uint8Array(sourceBuffer, 1);
		var fixedView = new Uint16Array(sourceBuffer, 0, 2);
		var trackingDataView = new DataView(sourceBuffer, 1);
	`, "structured-clone-resizable-buffer"); err != nil {
		t.Fatal(err)
	}

	value, err := r.vm.RunString(`({
		buffer: sourceBuffer,
		trackingView,
		fixedView,
		trackingDataView
	})`)
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := cloneMessageValue(r, r, value.(*goja.Object))
	if err != nil {
		t.Fatalf("clone RAB/view graph: %v", err)
	}
	if err := r.vm.GlobalObject().Set("clonedGraph", cloned); err != nil {
		t.Fatal(err)
	}
	got, err := r.Evaluate(`
		var clonedBuffer = clonedGraph.buffer;
		var clonedTrackingView = clonedGraph.trackingView;
		var clonedFixedView = clonedGraph.fixedView;
		var clonedDataView = clonedGraph.trackingDataView;
		var out = new Uint8Array(clonedBuffer);
		var initial = clonedBuffer instanceof ArrayBuffer && clonedBuffer !== sourceBuffer &&
			clonedBuffer.resizable && clonedBuffer.maxByteLength === 16 &&
			out.join(',') === '11,12,13,14' &&
			clonedTrackingView instanceof Uint8Array && clonedTrackingView.buffer === clonedBuffer &&
			clonedTrackingView.length === 3 && clonedTrackingView[0] === 12 &&
			clonedFixedView instanceof Uint16Array && clonedFixedView.length === 2 &&
			clonedDataView instanceof DataView && clonedDataView.byteLength === 3;
		clonedBuffer.resize(8);
		[initial, clonedTrackingView.length, clonedFixedView.length, clonedDataView.byteLength,
		 clonedBuffer.maxByteLength].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate cloned buffer state: %v", err)
	}
	if got != "true|7|2|7|16" {
		t.Fatalf("cloned RAB/view state = %q, want true|7|2|7|16", got)
	}

	if err := r.Run(`sourceBuffer.resize(1);`, "structured-clone-resizable-buffer-oob"); err != nil {
		t.Fatal(err)
	}
	view, ok := r.vm.Get("fixedView").(*goja.Object)
	if !ok {
		t.Fatal("fixedView is not an object")
	}
	if _, err := cloneMessageValue(r, r, view); err == nil {
		t.Fatal("cloning an out-of-bounds fixed-length TypedArray succeeded")
	}
}

func TestMessagePortTransferPreservesResizableBufferAndRejectsOOBViews(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var sourceBuffer = new ArrayBuffer(4, {maxByteLength: 16});
		new Uint8Array(sourceBuffer).set([1, 2, 3, 4]);
		var received;
		channel.port2.onmessage = event => { received = event.data; };
		channel.port1.postMessage(sourceBuffer, [sourceBuffer]);
	`, "messageport-transfer-resizable-buffer"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	got, err := r.Evaluate(`received instanceof ArrayBuffer && received.resizable &&
		received.maxByteLength === 16 && received.byteLength === 4 &&
		sourceBuffer.byteLength === 0 && new Uint8Array(received).join(',') === '1,2,3,4'`)
	if err != nil {
		t.Fatalf("evaluate transferred RAB: %v", err)
	}
	if got != "true" {
		t.Fatalf("transferred RAB check = %q, want true", got)
	}

	if err := r.Run(`
		var oobChannel = new MessageChannel();
		var oobBuffer = new ArrayBuffer(4, {maxByteLength: 8});
		var oobView = new Uint16Array(oobBuffer, 0, 2);
		oobBuffer.resize(1);
		var oobTransferThrew = false;
		try { oobChannel.port1.postMessage(oobView, [oobBuffer]); }
		catch (error) { oobTransferThrew = error.name === 'DataCloneError'; }
		var oobSourceStillAttached = oobBuffer.byteLength === 1;
	`, "messageport-transfer-oob-view"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Global("oobTransferThrew"); !got.Bool {
		t.Fatal("transferring a buffer with an out-of-bounds view did not throw DataCloneError")
	}
	if got, _ := r.Global("oobSourceStillAttached"); !got.Bool {
		t.Fatal("failed OOB view transfer detached its source buffer")
	}
}

func TestStructuredClonePreservesGrowableSharedArrayBuffer(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var sharedBuffer = new SharedArrayBuffer(4, {maxByteLength: 8});
		new Uint8Array(sharedBuffer).set([3, 4, 5, 6]);
	`, "structured-clone-growable-shared-buffer"); err != nil {
		t.Fatal(err)
	}
	value, ok := r.vm.Get("sharedBuffer").(*goja.Object)
	if !ok {
		t.Fatal("sharedBuffer is not an object")
	}
	if _, err := cloneMessageValue(r, r, value); err == nil {
		t.Fatal("SharedArrayBuffer cloned without cross-origin isolation")
	}
	r.crossOriginIsolated = true
	cloned, err := cloneMessageValue(r, r, value)
	if err != nil {
		t.Fatalf("clone SharedArrayBuffer: %v", err)
	}
	if err := r.vm.GlobalObject().Set("clonedSharedBuffer", cloned); err != nil {
		t.Fatal(err)
	}
	got, err := r.Evaluate(`
		var before = clonedSharedBuffer instanceof SharedArrayBuffer &&
			clonedSharedBuffer.growable && clonedSharedBuffer.maxByteLength === 8 &&
			new Uint8Array(clonedSharedBuffer).join(',') === '3,4,5,6';
		clonedSharedBuffer.grow(7);
		[before, clonedSharedBuffer.byteLength, clonedSharedBuffer.maxByteLength].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate SharedArrayBuffer clone: %v", err)
	}
	if got != "true|7|8" {
		t.Fatalf("cloned SharedArrayBuffer state = %q, want true|7|8", got)
	}
}

func TestWebAssemblyMemorySharedConstructorInterop(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.Evaluate(`
		var sharedMemory = new WebAssembly.Memory({initial: 0, maximum: 0, shared: true});
		var memoryBuffer = sharedMemory.buffer;
		[
			typeof WebAssembly.Memory === 'function',
			memoryBuffer.constructor === SharedArrayBuffer,
			memoryBuffer.growable,
			memoryBuffer.maxByteLength,
			sharedMemory.grow(0)
		].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate WebAssembly.Memory: %v", err)
	}
	if got != "true|true|true|0|0" {
		t.Fatalf("WebAssembly.Memory shared-buffer interop = %q, want true|true|true|0|0", got)
	}
}

func TestMessagePortStructuredClonePreservesBlobAndFileData(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var originalBlob = new Blob(['blob payload'], {type: 'text/plain'});
		var originalFile = new File(['file payload'], 'sample.txt', {type: 'text/custom', lastModified: 1234});
		var received;
		var blobText, fileText;
		channel.port2.onmessage = function(event) {
			received = event.data;
			received.blob.text().then(function(value) { blobText = value; });
			received.file.text().then(function(value) { fileText = value; });
		};
		channel.port1.postMessage({blob: originalBlob, file: originalFile});
	`, "message-port-blob-file-clone"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"received.blob !== originalBlob && received.blob instanceof Blob && received.blob.size === 12 && received.blob.type === 'text/plain'",
		"received.file !== originalFile && received.file instanceof File && received.file.size === 12 && received.file.type === 'text/custom'",
		"received.file.name === 'sample.txt' && received.file.lastModified === 1234",
		"blobText === 'blob payload' && fileText === 'file payload'",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
}

func TestMessagePortStructuredClonePreservesFileList(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	originalList := r.newFileList(nil)
	if err := r.vm.Set("originalList", originalList); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(`
		var channel = new MessageChannel();
		var received;
		channel.port2.onmessage = function(event) { received = event.data; };
		channel.port1.postMessage({list: originalList});
	`, "message-port-file-list-clone"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"received.list !== originalList",
		"received.list instanceof FileList",
		"received.list.length === 0 && received.list.item(0) === null",
		"Array.isArray(received.list) === false",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
}

func TestMessagePortStructuredClonePreservesImageData(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var originalImageData = new ImageData(1, 1);
		originalImageData.data.set([100, 101, 102, 103]);
	`, "message-port-image-data-clone"); err != nil {
		t.Fatal(err)
	}
	_, cloneErr := cloneMessageValue(r, r, r.vm.Get("originalImageData"))
	if cloneErr != nil {
		obj := r.vm.Get("originalImageData").(*goja.Object)
		data := obj.Get("data").(*goja.Object)
		_, registered := r.imageDataObjects[obj]
		t.Fatalf("clone ImageData: %v (registered=%v class=%s width=%d height=%d data=%s len=%d)", cloneErr, registered, obj.ClassName(), obj.Get("width").ToInteger(), obj.Get("height").ToInteger(), data.ClassName(), data.Get("length").ToInteger())
	}
	if err := r.Run(`
		var channel = new MessageChannel();
		var receivedImageData;
		channel.port2.onmessage = function(event) { receivedImageData = event.data; };
		channel.port1.postMessage(originalImageData);
	`, "message-port-image-data-post"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"originalImageData instanceof ImageData && receivedImageData instanceof ImageData",
		"receivedImageData !== originalImageData && receivedImageData.data !== originalImageData.data",
		"receivedImageData.width === 1 && receivedImageData.height === 1",
		"receivedImageData.data instanceof Uint8ClampedArray",
		"Array.from(receivedImageData.data).join(',') === '100,101,102,103'",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
	if err := r.Run(`
		var constructedImageData = new ImageData(new Uint8ClampedArray([1, 2, 3, 4]), 1, 1);
	`, "image-data-constructor-test"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Evaluate("constructedImageData.data instanceof Uint8ClampedArray && constructedImageData.data[3] === 4"); err != nil || got != "true" {
		t.Fatalf("ImageData constructor result = %q, err=%v", got, err)
	}
}

func TestMessagePortStructuredClonePreservesImageBitmap(t *testing.T) {
	doc := dom.NewDocument()
	html, body := doc.NewElement("html"), doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	r, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var canvas = document.createElement('canvas');
		canvas.width = 1; canvas.height = 1;
		var createdBitmap;
		createImageBitmap(canvas).then(function(bitmap) { createdBitmap = bitmap; });
	`, "create-image-bitmap-test"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, err := r.Evaluate("createdBitmap instanceof ImageBitmap && createdBitmap.width === 1 && createdBitmap.height === 1"); err != nil || got != "true" {
		t.Fatalf("createImageBitmap result = %q, err=%v", got, err)
	}
	if err := r.Run(`
		var channel = new MessageChannel();
		var receivedBitmap;
		channel.port2.onmessage = function(event) { receivedBitmap = event.data; };
		channel.port1.postMessage(createdBitmap);
	`, "message-port-image-bitmap-post"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"receivedBitmap instanceof ImageBitmap",
		"receivedBitmap !== createdBitmap",
		"receivedBitmap.width === 1 && receivedBitmap.height === 1",
	} {
		got, err := r.Evaluate(expr)
		if err != nil || got != "true" {
			t.Errorf("%s = %q, err=%v", expr, got, err)
		}
	}
}

func TestOffscreenCanvasStructuredCloneCopiesDimensionsAndContext(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`var off = new OffscreenCanvas(10, 10); var offctx = off.getContext('2d'); offctx.fillStyle='red'; offctx.fillRect(1,2,3,4);`, "offscreen-canvas-clone"); err != nil {
		t.Fatal(err)
	}
	source, ok := r.vm.Get("off").(*goja.Object)
	if !ok {
		t.Fatal("OffscreenCanvas missing")
	}
	copy, err := cloneMessageValue(r, r, source)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	r.vm.Set("offcopy", copy)
	for _, expr := range []string{"offcopy instanceof OffscreenCanvas", "offcopy !== off", "offcopy.width === 10 && offcopy.height === 10", "offcopy.getContext('2d') !== offctx"} {
		got, err := r.Evaluate(expr)
		if err != nil || got != "true" {
			t.Errorf("%s = %q (%v)", expr, got, err)
		}
	}
}

func TestOffscreenCanvasWindowTransferClonesAndDetachesSource(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`var transferCanvas = new OffscreenCanvas(10, 10); var transferredCanvas; window.onmessage = e => transferredCanvas = e.data.data; window.postMessage({data: transferCanvas}, '/', [transferCanvas]);`, "offscreen-canvas-transfer"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{"transferredCanvas instanceof OffscreenCanvas", "transferredCanvas !== transferCanvas", "transferredCanvas.width === 10 && transferredCanvas.height === 10", "transferCanvas.width === 0 && transferCanvas.height === 0", "transferCanvas.getContext('2d') === null"} {
		got, err := r.Evaluate(expr)
		if err != nil || got != "true" {
			t.Errorf("%s = %q (%v)", expr, got, err)
		}
	}
}

func TestMessagePortStructuredCloneUsesStoredBlobPrototypeAfterGlobalDeletion(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var blobInterface = globalThis.Blob;
		var originalBlob = new blobInterface(['still clonable'], {type: 'text/plain'});
		delete globalThis.Blob;
		var channel = new MessageChannel();
		var received;
		var clonedText = '';
		channel.port2.onmessage = function(event) {
			received = event.data;
			received.text().then(function(text) { clonedText = text; });
		};
		channel.port1.postMessage(originalBlob);
	`, "message-port-blob-clone-without-global"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for _, expr := range []string{
		"received !== originalBlob",
		"received instanceof blobInterface",
		"received.size === 14 && received.type === 'text/plain'",
		"clonedText === 'still clonable'",
	} {
		got, err := r.Evaluate(expr)
		if err != nil {
			t.Fatalf("evaluate %q: %v", expr, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
}

func TestMessagePortUncloneablePayloadDoesNotDetachTransfers(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var transferable = new MessageChannel();
		var selfTransferThrew = false;
		try { channel.port1.postMessage('invalid', [channel.port1]); }
		catch (error) { selfTransferThrew = error.name === 'DataCloneError'; }
		var sourceStillUsable = false;
		channel.port2.onmessage = function (event) { sourceStillUsable = event.data === 'source-usable'; };
		channel.port1.postMessage('source-usable');
		var threw = false;
		try { channel.port1.postMessage(function () {}, [transferable.port1]); }
		catch (error) { threw = error.name === 'DataCloneError'; }
		var senderPortStillUsable = false;
		transferable.port2.onmessage = function (event) { senderPortStillUsable = event.data === 'still-entangled'; };
		transferable.port1.postMessage('still-entangled');
	`, "message-port-transfer-atomicity"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Global("threw"); !got.Bool {
		t.Fatal("uncloneable payload did not throw DataCloneError")
	}
	if got, _ := r.Global("selfTransferThrew"); !got.Bool {
		t.Fatal("transferring the source MessagePort did not throw DataCloneError")
	}
	r.Tick()
	if got, _ := r.Global("sourceStillUsable"); !got.Bool {
		t.Fatal("failed self-transfer detached the source MessagePort")
	}
	if got, _ := r.Global("senderPortStillUsable"); !got.Bool {
		t.Fatal("failed postMessage detached the MessagePort in its transfer list")
	}
}

func TestMessagePortStartPreservesQueuedMessages(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var received = 0;
		channel.port1.postMessage(7);
	`, "message-port-start-test"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if err := r.Run(`channel.port2.onmessage = function(e) { received = e.data; };`, "message-port-start-handler"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("received"); got.Num != 7 {
		t.Fatalf("received = %v, want queued message 7", got)
	}
}

func TestMessagePortOnMessageIsAnIDLPrototypeAttribute(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var descriptor = Object.getOwnPropertyDescriptor(MessagePort.prototype, 'onmessage');
		var prototypeIDL = !!descriptor && typeof descriptor.get === 'function' && typeof descriptor.set === 'function';
		var ownBeforeSet = Object.prototype.hasOwnProperty.call(channel.port2, 'onmessage');
		var received = 0;
		channel.port1.postMessage(23);
		channel.port2.onmessage = function(event) { received = event.data; };
		var ownAfterSet = Object.prototype.hasOwnProperty.call(channel.port2, 'onmessage');
	`, "message-port-onmessage-idl"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"prototypeIDL", "ownBeforeSet", "ownAfterSet"} {
		got, ok := r.Global(name)
		if !ok {
			t.Fatalf("global %q not found", name)
		}
		want := name == "prototypeIDL"
		if got.Bool != want {
			t.Errorf("%s = %v, want %v", name, got.Bool, want)
		}
	}
	r.Tick()
	if got, ok := r.Global("received"); !ok || got.Num != 23 {
		t.Fatalf("received = %v, exists = %v; want queued message 23", got, ok)
	}
}

func TestMessagePortRemoveEventListenerUsesCallbackAndCapture(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var calls = 0;
		var listener = function() { calls++; };
		channel.port2.addEventListener('message', listener, false);
		channel.port2.addEventListener('message', listener, {capture: true});
		channel.port2.removeEventListener('message', listener, {capture: true});
		channel.port1.postMessage('queued');
		channel.port2.start();
	`, "message-port-remove-listener"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, ok := r.Global("calls"); !ok || got.Num != 1 {
		t.Fatalf("listener calls = %v (exists %v), want 1", got, ok)
	}
}

func TestMessagePortUserActivationSnapshotIsOptIn(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var firstIsObject = false, firstActive = true, firstBeenActive = true, secondIsNull = false;
		channel.port2.onmessage = function(event) {
			if (event.data === 1) {
				firstIsObject = event.userActivation !== null && typeof event.userActivation === 'object';
				firstActive = event.userActivation.isActive;
				firstBeenActive = event.userActivation.hasBeenActive;
			} else secondIsNull = event.userActivation === null;
		};
		channel.port1.postMessage(1, {includeUserActivation: true});
		channel.port1.postMessage(2);
	`, "message-port-user-activation"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	for name, want := range map[string]bool{
		"firstIsObject": true, "firstActive": false, "firstBeenActive": false, "secondIsNull": true,
	} {
		if got, ok := r.Global(name); !ok || got.Bool != want {
			t.Errorf("%s = %+v (exists %v), want %v", name, got, ok, want)
		}
	}
}

func TestBlobWorkerRunsMessageChannelDuringClose(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var workerSource = 'function workerCode() { close(); var channel = new MessageChannel(); channel.port1.postMessage(42); channel.port2.postMessage(42); postMessage(true); } workerCode();';
		var workerBlob = new Blob([workerSource], {type: 'application/javascript'});
		var worker = new Worker(URL.createObjectURL(workerBlob));
		window.workerMessage = 'pending';
		window.workerError = '';
		worker.onmessage = function(event) { window.workerMessage = event.data; };
		worker.onerror = function(event) { window.workerError = event.message; };
	`, "blob-worker-close"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if got, ok := r.Global("workerMessage"); ok && got.Str != "pending" {
			if got.Kind != KindBool || !got.Bool {
				t.Fatalf("worker message = %+v, want boolean true", got)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	errValue, _ := r.Global("workerError")
	t.Fatalf("Blob worker did not post its result; worker error %q", errValue.Str)
}

func TestBlobWorkerTerminatesWithoutDeliveringQueuedMessages(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var worker = new Worker(URL.createObjectURL(new Blob(['onmessage = function() { postMessage("unexpected"); };'])));
		window.terminationMessage = 'none';
		worker.onmessage = function(event) { window.terminationMessage = event.data; };
		worker.terminate();
	`, "blob-worker-terminate"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		r.Tick()
		time.Sleep(time.Millisecond)
	}
	if got, ok := r.Global("terminationMessage"); !ok || got.Str != "none" {
		t.Fatalf("message after Worker.terminate() = %+v (exists %v), want none", got, ok)
	}
}

func TestBlobObjectURLStorageIsRevocableAndBounded(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var blob = new Blob(['worker source']);
		var urls = [];
		for (var i = 0; i < 256; i++) urls.push(URL.createObjectURL(blob));
		var objectURLLimitThrowsRangeError = false;
		try { URL.createObjectURL(blob); } catch (error) { objectURLLimitThrowsRangeError = error instanceof RangeError; }
		URL.revokeObjectURL(urls[0]);
		var replacementURL = URL.createObjectURL(blob);
	`, "blob-object-url-limits"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"objectURLLimitThrowsRangeError": true} {
		if got, ok := r.Global(name); !ok || got.Bool != want {
			t.Errorf("%s = %+v (exists %v), want %v", name, got, ok, want)
		}
	}
	if source, ok := r.objectURLSource(r.vm.Get("urls").ToObject(r.vm).Get("0").String()); ok || source != nil {
		t.Fatalf("revoked URL source remains registered: %q", source)
	}
	if source, ok := r.objectURLSource(r.vm.Get("replacementURL").String()); !ok || string(source) != "worker source" {
		t.Fatalf("replacement URL source = %q (exists %v)", source, ok)
	}
}

func TestMessagePortAddEventListenerDoesNotStartAndOptionsTransferAcceptsArrayBuffer(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		var count = 0;
		channel.port2.addEventListener('message', function() { count++; });
		channel.port1.postMessage('queued');
	`, "message-port-disabled-test"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("count"); got.Num != 0 {
		t.Fatalf("listener ran before port.start(): %v", got.Num)
	}
	if err := r.Run(`channel.port2.start();`, "message-port-start"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("count"); got.Num != 1 {
		t.Fatalf("listener count after start = %v, want 1", got.Num)
	}
	if err := r.Run(`
		var channelWithBuffer = new MessageChannel();
		var buffer = new ArrayBuffer(1);
		new Uint8Array(buffer)[0] = 42;
		var originalBuffer = buffer;
		channelWithBuffer.port1.postMessage(buffer, {transfer: [buffer]});
		channelWithBuffer.port2.onmessage = function(event) {
			window.bufferBytes = event.data.byteLength;
			window.receivedBufferByte = new Uint8Array(event.data)[0];
		};
		window.sourceBufferBytes = originalBuffer.byteLength;
	`, "message-port-arraybuffer-transfer"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("sourceBufferBytes"); got.Num != 0 {
		t.Fatalf("transferred source ArrayBuffer byteLength = %v, want 0", got.Num)
	}
	if got, _ := r.Global("bufferBytes"); got.Num != 1 {
		t.Fatalf("transferred ArrayBuffer byteLength = %v, want 1", got.Num)
	}
	if got, _ := r.Global("receivedBufferByte"); got.Num != 42 {
		t.Fatalf("transferred ArrayBuffer byte = %v, want 42", got.Num)
	}
}

func TestMessagePortTransferBetweenRuntimesDeliversQueuedPeerMessages(t *testing.T) {
	source, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := source.Run(`var channel = new MessageChannel();`, "message-port-source"); err != nil {
		t.Fatal(err)
	}
	transferred, err := source.transferPortsTo(target, source.vm.ToValue([]goja.Value{source.vm.Get("channel").ToObject(source.vm).Get("port1")}))
	if err != nil {
		t.Fatal(err)
	}
	if len(transferred) != 1 {
		t.Fatalf("transferred ports = %d, want 1", len(transferred))
	}
	if err := target.vm.Set("receivedPort", transferred[0]); err != nil {
		t.Fatal(err)
	}
	if err := target.Run(`var received = 0; receivedPort.onmessage = function(event) { received = event.data; };`, "message-port-target"); err != nil {
		t.Fatal(err)
	}
	if err := source.Run(`channel.port2.postMessage(42);`, "message-port-send"); err != nil {
		t.Fatal(err)
	}
	target.Tick()
	if got, _ := target.Global("received"); got.Num != 42 {
		t.Fatalf("transferred peer message = %v, want 42", got.Num)
	}
}

func TestWindowPostMessageExposesFrozenPortsArray(t *testing.T) {
	r, err := New(Options{DOM: dom.NewDocument()})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var channel = new MessageChannel();
		window.receivedPortsLength = -1;
		window.receivedPortsFrozen = false;
		var syntheticEvent = new MessageEvent('message', {ports: [channel.port1]});
		window.syntheticPortsLength = syntheticEvent.ports.length;
		window.syntheticPortsFrozen = Object.isFrozen(syntheticEvent.ports);
		window.onmessage = function(event) {
			window.receivedPortsLength = event.ports.length;
			window.receivedPortsFrozen = Object.isFrozen(event.ports);
		};
		window.postMessage('data', '*', [channel.port1]);
	`, "window-postmessage-ports"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got, _ := r.Global("receivedPortsLength"); got.Num != 1 {
		t.Fatalf("MessageEvent.ports length = %v, want 1", got.Num)
	}
	if got, _ := r.Global("receivedPortsFrozen"); !got.Bool {
		t.Fatal("MessageEvent.ports should be frozen")
	}
	if got, _ := r.Global("syntheticPortsLength"); got.Num != 1 {
		t.Fatalf("constructed MessageEvent.ports length = %v, want 1", got.Num)
	}
	if got, _ := r.Global("syntheticPortsFrozen"); !got.Bool {
		t.Fatal("constructed MessageEvent.ports should be frozen")
	}
}

func TestWindowPostMessageAcrossRuntimesTransfersPortsAsFrozenArray(t *testing.T) {
	source, err := New(Options{URL: "https://example.test/source"})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := New(Options{URL: "https://example.test/target"})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := source.Run(`
		var channel = new MessageChannel();
		channel.port2.onmessage = function(event) { window.reply = event.data; };
	`, "cross-runtime-postmessage-source"); err != nil {
		t.Fatal(err)
	}
	if err := target.Run(`
		window.portsLength = -1;
		window.portsFrozen = false;
		window.onmessage = function(event) {
			window.portsLength = event.ports.length;
			window.portsFrozen = Object.isFrozen(event.ports);
			event.ports[0].postMessage('reply');
		};
	`, "cross-runtime-postmessage-target"); err != nil {
		t.Fatal(err)
	}
	port := source.vm.Get("channel").ToObject(source.vm).Get("port1")
	call := goja.FunctionCall{Arguments: []goja.Value{
		source.vm.ToValue("message"), source.vm.ToValue("*"), source.vm.ToValue([]goja.Value{port}),
	}}
	if value := source.postMessageAcross(call, target, source.vm.GlobalObject()); value == nil {
		t.Fatal("postMessageAcross returned no result")
	}
	for i := 0; i < 4; i++ {
		target.Tick()
		source.Tick()
	}
	if got, _ := target.Global("portsLength"); got.Num != 1 {
		t.Fatalf("cross-runtime MessageEvent.ports length = %v, want 1", got.Num)
	}
	if got, _ := target.Global("portsFrozen"); !got.Bool {
		t.Fatal("cross-runtime MessageEvent.ports should be frozen")
	}
	if got, _ := source.Global("reply"); got.Str != "reply" {
		t.Fatalf("cross-runtime transferred-port response = %+v, want reply", got)
	}
}
