package engine

import (
	"sync"
	"testing"
)

func TestImageReservationExactLimit(t *testing.T) {
	r := newImageReservation(100)
	if !r.reserve(50) {
		t.Fatal("reserve(50) failed")
	}
	if !r.reserve(50) {
		t.Fatal("reserve(50) failed at exact limit")
	}
	if r.reserve(1) {
		t.Fatal("reserve(1) succeeded past limit")
	}
}

func TestImageReservationRollback(t *testing.T) {
	r := newImageReservation(100)
	if !r.reserve(60) {
		t.Fatal("reserve(60) failed")
	}
	r.release(60)
	if !r.reserve(80) {
		t.Fatal("reserve(80) failed after release")
	}
}

func TestImageReservationConcurrent(t *testing.T) {
	const limit = 1000
	r := newImageReservation(limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := int64(0)
	peak := int64(0)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(pixels int64) {
			defer wg.Done()
			if !r.reserve(pixels) {
				return
			}
			mu.Lock()
			reserved += pixels
			if reserved > peak {
				peak = reserved
			}
			mu.Unlock()
			mu.Lock()
			reserved -= pixels
			mu.Unlock()
			r.release(pixels)
		}(int64(100 + i*10))
	}
	wg.Wait()
	if peak > limit {
		t.Errorf("peak reserved = %d, exceeds limit %d", peak, limit)
	}
}

func TestImageReservationMultipleReserves(t *testing.T) {
	r := newImageReservation(1000)
	if !r.reserve(500) {
		t.Fatal("first reserve failed")
	}
	if !r.reserve(400) {
		t.Fatal("second reserve failed")
	}
	if r.reserve(200) {
		t.Fatal("reserve succeeded past limit")
	}
	r.release(500)
	if !r.reserve(200) {
		t.Fatal("reserve after release failed")
	}
}
