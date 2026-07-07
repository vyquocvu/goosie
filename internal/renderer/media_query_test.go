package renderer

import (
	"testing"
)

func TestMediaQueryEvaluator_BasicTypes(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"empty prelude", "", true},
		{"all media type", "all", true},
		{"screen media type", "screen", true},
		{"print media type", "print", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_WidthQueries(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"max-width satisfied", "(max-width: 1000px)", true},
		{"max-width not satisfied", "(max-width: 600px)", false},
		{"min-width satisfied", "(min-width: 600px)", true},
		{"min-width not satisfied", "(min-width: 1000px)", false},
		{"exact width", "(width: 800px)", true},
		{"screen and max-width", "screen and (max-width: 1000px)", true},
		{"print and max-width", "print and (max-width: 1000px)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_HeightQueries(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"max-height satisfied", "(max-height: 800px)", true},
		{"max-height not satisfied", "(max-height: 400px)", false},
		{"min-height satisfied", "(min-height: 400px)", true},
		{"min-height not satisfied", "(min-height: 800px)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_OrConditions(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"first true", "(max-width: 1000px), (max-width: 500px)", true},
		{"second true", "(max-width: 500px), (max-width: 1000px)", true},
		{"both false", "(max-width: 500px), (max-width: 600px)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_AndConditions(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"both true", "(min-width: 600px) and (max-width: 1000px)", true},
		{"first false", "(min-width: 900px) and (max-width: 1000px)", false},
		{"second false", "(min-width: 600px) and (max-width: 700px)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_Orientation(t *testing.T) {
	landscapeMQ := NewMediaQueryEvaluator(800, 600)
	portraitMQ := NewMediaQueryEvaluator(600, 800)

	tests := []struct {
		name     string
		mq       *MediaQueryEvaluator
		prelude  string
		expected bool
	}{
		{"landscape viewport, landscape query", landscapeMQ, "(orientation: landscape)", true},
		{"landscape viewport, portrait query", landscapeMQ, "(orientation: portrait)", false},
		{"portrait viewport, portrait query", portraitMQ, "(orientation: portrait)", true},
		{"portrait viewport, landscape query", portraitMQ, "(orientation: landscape)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_Negation(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"not print", "not print", true},
		{"not screen", "not screen", false},
		{"not min-width", "not (min-width: 1000px)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_Units(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	tests := []struct {
		name     string
		prelude  string
		expected bool
	}{
		{"em units", "(max-width: 60em)", true},    // 60 * 16 = 960px
		{"rem units", "(max-width: 40rem)", false}, // 40 * 16 = 640px
		{"plain number", "(max-width: 1000)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mq.Evaluate(tt.prelude)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.prelude, result, tt.expected)
			}
		})
	}
}

func TestMediaQueryEvaluator_UpdateViewport(t *testing.T) {
	mq := NewMediaQueryEvaluator(800, 600)

	// Initially max-width: 600px fails
	if mq.Evaluate("(max-width: 600px)") {
		t.Error("Expected (max-width: 600px) to fail with 800px viewport")
	}

	// Update viewport to 500px
	mq.UpdateViewport(500, 400)

	// Now max-width: 600px should pass
	if !mq.Evaluate("(max-width: 600px)") {
		t.Error("Expected (max-width: 600px) to pass with 500px viewport")
	}
}
