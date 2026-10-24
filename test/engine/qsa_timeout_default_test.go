//go:build !race

package engine_test

import "time"

// qsaTimeout is the JS runtime timeout for TestManyQuerySelectorAll in normal
// builds. Under the race detector, wrapper construction is slower, so the
// race-tagged file provides a larger value.
const qsaTimeout = 15 * time.Second
