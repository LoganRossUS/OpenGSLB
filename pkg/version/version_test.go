package version

import "testing"

func TestGetVersion(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "returns current version",
			expected: Version,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetVersion()
			if result != tt.expected {
				t.Errorf("GetVersion() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestVersionConstant(t *testing.T) {
	if Version == "" {
		t.Error("Version constant should not be empty")
	}
}