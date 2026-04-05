package pressure

import "testing"

func TestEvaluate(t *testing.T) {
	th := DefaultThresholds()

	tests := []struct {
		name        string
		rate        float64
		helpers     int
		want        Level
	}{
		{"idle", 0, 10, Normal},
		{"just below warning", 499, 50, Normal},
		{"at warning", 500, 50, Warning},
		{"between warning and high", 750, 50, Warning},
		{"at high", 1000, 50, High},
		{"between high and critical", 1500, 50, High},
		{"at critical rate", 2000, 50, Critical},
		{"above critical rate", 3000, 50, Critical},
		{"helpers escalate normal to warning", 100, 150, Warning},
		{"helpers alone escalate to warning", 0, 200, Warning},
		{"helpers escalate warning to high", 500, 150, High},
		{"helpers escalate high to critical", 1000, 200, Critical},
		{"critical rate stays critical with helpers", 2500, 200, Critical},
		{"critical rate without helpers", 2500, 50, Critical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := th.Evaluate(tt.rate, tt.helpers)
			if got != tt.want {
				t.Errorf("Evaluate(%f, %d) = %v, want %v", tt.rate, tt.helpers, got, tt.want)
			}
		})
	}
}
