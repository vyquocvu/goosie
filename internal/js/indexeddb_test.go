package js

import "testing"

func TestIndexedDBFactoryCmpUsesIndexedDBKeyOrder(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.Evaluate(`(function(){var out=[indexedDB===self.indexedDB,Object.prototype.toString.call(indexedDB)];function check(f){try{out.push(f())}catch(e){out.push('ERR:'+e.name+':'+e.message)}}check(function(){return indexedDB.cmp(2,1)});check(function(){return indexedDB.cmp(new Date(0),0)});check(function(){return indexedDB.cmp('',new Date(0))});check(function(){return indexedDB.cmp(new Uint8Array([255,254]),new Uint8Array([255,253,254]))});check(function(){return indexedDB.cmp([1,'a'],[1,'b'])});check(function(){return indexedDB.cmp('\u{10000}','\uE000')});return out.join('|')})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true|[object IDBFactory]|1|1|1|1|-1|-1" {
		t.Fatalf("IndexedDB key comparison = %q", got)
	}
}

func TestIndexedDBFactoryCmpRejectsInvalidKeys(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.Evaluate(`[
		(function(){try{indexedDB.cmp(null, 0)}catch(e){return e.name+':'+e.code+':'+(e instanceof DOMException)}return 'no error'})(),
		(function(){try{indexedDB.cmp(NaN, 0)}catch(e){return e.name+':'+e.code+':'+(e instanceof DOMException)}return 'no error'})(),
		(function(){try{indexedDB.cmp([1, undefined], [1, 2])}catch(e){return e.name+':'+e.code+':'+(e instanceof DOMException)}return 'no error'})(),
		(function(){try{indexedDB.cmp(1)}catch(e){return e.name}return 'no error'})(),
		(function(){try{indexedDB.cmp.call({}, 1, 2)}catch(e){return e.name}return 'no error'})()
	].join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "DataError:0:true|DataError:0:true|DataError:0:true|TypeError|TypeError" {
		t.Fatalf("IndexedDB invalid key exceptions = %q", got)
	}
}

func TestIndexedDBKeyRangeEqualOpenBoundsAreEmptyNotInvalid(t *testing.T) {
	r, err := New(Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.Evaluate(`(function(){
		var open = IDBKeyRange.bound(1, 1, true, false);
		var closed = IDBKeyRange.bound(1, 1);
		return [open.lowerOpen, open.upperOpen, open.includes(1), closed.includes(1)].join('|');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true|false|false|true" {
		t.Fatalf("equal-bound IndexedDB key ranges = %q", got)
	}
}
