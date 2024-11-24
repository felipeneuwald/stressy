package ptr

import (
	"testing"
)

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		wantVal  bool
		wantNil  bool
		wantZero bool
	}{
		{
			name:     "true value",
			input:    true,
			wantVal:  true,
			wantNil:  false,
			wantZero: false,
		},
		{
			name:     "false value",
			input:    false,
			wantVal:  false,
			wantNil:  false,
			wantZero: true,
		},
		{
			name:     "bool zero value",
			input:    bool(false),
			wantVal:  false,
			wantNil:  false,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoolPtr(tt.input)

			if got == nil && !tt.wantNil {
				t.Errorf("BoolPtr(%v) = nil, want non-nil", tt.input)
				return
			}

			if got != nil && tt.wantNil {
				t.Errorf("BoolPtr(%v) = %v, want nil", tt.input, *got)
				return
			}

			if got != nil {
				if *got != tt.wantVal {
					t.Errorf("BoolPtr(%v) = %v, want %v", tt.input, *got, tt.wantVal)
				}

				// Test that we get a different pointer each time
				got2 := BoolPtr(tt.input)
				if got == got2 {
					t.Errorf("BoolPtr(%v) returned same pointer on second call", tt.input)
				}

				// Verify zero value behavior
				if (tt.wantZero && *got != false) || (!tt.wantZero && *got == false) {
					t.Errorf("BoolPtr(%v) zero value behavior incorrect", tt.input)
				}
			}
		})
	}
}

func TestBoolPtr_ModifyOriginal(t *testing.T) {
	// Test that modifying the input doesn't affect the pointer
	original := true
	ptr := BoolPtr(original)
	original = false

	if *ptr != true {
		t.Errorf("BoolPtr result was affected by modifying original value, got %v, want %v", *ptr, true)
	}
}

func TestBoolPtr_ModifyResult(t *testing.T) {
	// Test that modifying the result doesn't affect other pointers
	ptr1 := BoolPtr(true)
	ptr2 := BoolPtr(true)

	*ptr1 = false
	if *ptr2 != true {
		t.Errorf("Modifying one pointer affected another, got %v, want %v", *ptr2, true)
	}
}

func TestBoolPtr_Comparison(t *testing.T) {
	// Test pointer comparison behavior
	ptr1 := BoolPtr(true)
	ptr2 := BoolPtr(true)
	ptr3 := ptr1

	// Different pointers with same value should not be equal
	if ptr1 == ptr2 {
		t.Error("Different pointers with same value should not be equal")
	}

	// Same pointer should be equal
	if ptr1 != ptr3 {
		t.Error("Same pointer should be equal")
	}

	// Values should be equal even if pointers are different
	if *ptr1 != *ptr2 {
		t.Error("Values should be equal even if pointers are different")
	}
}

func TestPtrBool(t *testing.T) {
	tests := []struct {
		name string
		input *bool
		want bool
	}{
		{
			name: "nil pointer",
			input: nil,
			want: false,
		},
		{
			name: "pointer to true",
			input: BoolPtr(true),
			want: true,
		},
		{
			name: "pointer to false",
			input: BoolPtr(false),
			want: false,
		},
		{
			name: "pointer to bool zero value",
			input: BoolPtr(bool(false)),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PtrBool(tt.input)
			if got != tt.want {
				t.Errorf("PtrBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPtrBool_ModifyOriginal(t *testing.T) {
	// Test that modifying the original pointer value affects the result
	val := true
	ptr := &val
	result := PtrBool(ptr)
	val = false

	// The result should not change as it's a copy
	if result != true {
		t.Errorf("PtrBool result was affected by modifying original value, got %v, want %v", result, true)
	}
}

func TestPtrBool_ZeroValue(t *testing.T) {
	// Test behavior with zero value of bool
	var zeroBool bool // zero value of bool is false
	got := PtrBool(&zeroBool)
	if got != false {
		t.Errorf("PtrBool with zero value bool = %v, want false", got)
	}
}

func TestPtrBool_ConsistentNilBehavior(t *testing.T) {
	// Test that multiple calls with nil return the same result
	first := PtrBool(nil)
	second := PtrBool(nil)
	if first != second {
		t.Errorf("Inconsistent nil behavior: first call = %v, second call = %v", first, second)
	}
	if first != false {
		t.Errorf("Nil pointer should return false, got %v", first)
	}
}

func TestPtrBool_PointerReuse(t *testing.T) {
	// Test behavior when reusing the same pointer with different values
	value := true
	ptr := &value
	
	got1 := PtrBool(ptr)
	if got1 != true {
		t.Errorf("First call with true = %v, want true", got1)
	}

	*ptr = false
	got2 := PtrBool(ptr)
	if got2 != false {
		t.Errorf("Second call after changing to false = %v, want false", got2)
	}
}
