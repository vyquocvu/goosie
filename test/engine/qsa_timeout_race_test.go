//go:build race

package engine_test

import "time"

// qsaTimeout is the JS runtime timeout for TestManyQuerySelectorAll under the
// race detector. Race instrumentation slows wrapper construction beyond the
// production default, so this is larger than the non-race value.
const qsaTimeout = 45 * time.Second
