package ptr

import (
	"testing"
)

func TestStrPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVal  string
		wantNil  bool
		wantZero bool
	}{
		{
			name:     "regular string",
			input:    "hello",
			wantVal:  "hello",
			wantNil:  false,
			wantZero: false,
		},
		{
			name:     "empty string",
			input:    "",
			wantVal:  "",
			wantNil:  false,
			wantZero: true,
		},
		{
			name:     "string with spaces",
			input:    "   ",
			wantVal:  "   ",
			wantNil:  false,
			wantZero: false,
		},
		{
			name:     "unicode string",
			input:    "Hello, 世界",
			wantVal:  "Hello, 世界",
			wantNil:  false,
			wantZero: false,
		},
		{
			name:     "special characters",
			input:    "!@#$%^&*()",
			wantVal:  "!@#$%^&*()",
			wantNil:  false,
			wantZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StrPtr(tt.input)

			if got == nil && !tt.wantNil {
				t.Errorf("StrPtr(%q) = nil, want non-nil", tt.input)
				return
			}

			if got != nil && tt.wantNil {
				t.Errorf("StrPtr(%q) = %v, want nil", tt.input, *got)
				return
			}

			if got != nil {
				if *got != tt.wantVal {
					t.Errorf("StrPtr(%q) = %q, want %q", tt.input, *got, tt.wantVal)
				}

				// Test that we get a different pointer each time
				got2 := StrPtr(tt.input)
				if got == got2 {
					t.Errorf("StrPtr(%q) returned same pointer on second call", tt.input)
				}

				// Verify zero value behavior
				if (tt.wantZero && *got != "") || (!tt.wantZero && *got == "") {
					t.Errorf("StrPtr(%q) zero value behavior incorrect", tt.input)
				}
			}
		})
	}
}

func TestStrPtr_ModifyOriginal(t *testing.T) {
	// Test that modifying the input doesn't affect the pointer
	original := "test"
	ptr := StrPtr(original)
	original = "modified"

	if *ptr != "test" {
		t.Errorf("StrPtr result was affected by modifying original value, got %q, want %q", *ptr, "test")
	}
}

func TestStrPtr_ModifyResult(t *testing.T) {
	// Test that modifying the result doesn't affect other pointers
	ptr1 := StrPtr("test")
	ptr2 := StrPtr("test")

	*ptr1 = "modified"
	if *ptr2 != "test" {
		t.Errorf("Modifying one pointer affected another, got %q, want %q", *ptr2, "test")
	}
}

func TestPtrStr(t *testing.T) {
	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{
			name:  "nil pointer",
			input: nil,
			want:  "",
		},
		{
			name:  "empty string pointer",
			input: StrPtr(""),
			want:  "",
		},
		{
			name:  "regular string pointer",
			input: StrPtr("hello"),
			want:  "hello",
		},
		{
			name:  "string with spaces",
			input: StrPtr("   "),
			want:  "   ",
		},
		{
			name:  "unicode string",
			input: StrPtr("Hello, 世界"),
			want:  "Hello, 世界",
		},
		{
			name:  "special characters",
			input: StrPtr("!@#$%^&*()"),
			want:  "!@#$%^&*()",
		},
		{
			name:  "very long string",
			input: StrPtr(string(make([]byte, 1000))), // 1000 zero bytes
			want:  string(make([]byte, 1000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PtrStr(tt.input)
			if got != tt.want {
				t.Errorf("PtrStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPtrStr_ModifyOriginal(t *testing.T) {
	// Test that modifying the original pointer value affects the result
	str := "test"
	ptr := &str
	result := PtrStr(ptr)
	str = "modified"

	// The result should not change as it's a copy
	if result != "test" {
		t.Errorf("PtrStr result was affected by modifying original value, got %q, want %q", result, "test")
	}
}

func TestPtrStr_ZeroValue(t *testing.T) {
	// Test behavior with zero value of string
	var zeroStr string // zero value of string
	got := PtrStr(&zeroStr)
	if got != "" {
		t.Errorf("PtrStr with zero value string = %q, want empty string", got)
	}
}

func TestPtrStr_ConsistentNilBehavior(t *testing.T) {
	// Test that multiple calls with nil return the same result
	first := PtrStr(nil)
	second := PtrStr(nil)
	if first != second {
		t.Errorf("Inconsistent nil behavior: first call = %q, second call = %q", first, second)
	}
}
