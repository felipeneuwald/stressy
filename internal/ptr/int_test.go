package ptr

import "testing"

func TestIntPtr(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{
			name:  "zero value",
			value: 0,
			want:  0,
		},
		{
			name:  "positive value",
			value: 42,
			want:  42,
		},
		{
			name:  "negative value",
			value: -1,
			want:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr := IntPtr(tt.value)
			if *ptr != tt.want {
				t.Errorf("IntPtr() = %v, want %v", *ptr, tt.want)
			}

			// Ensure modifying the original value doesn't affect the pointer
			tt.value = 999
			if *ptr != tt.want {
				t.Errorf("IntPtr() value changed after original modified = %v, want %v", *ptr, tt.want)
			}
		})
	}
}

func TestPtrInt(t *testing.T) {
	tests := []struct {
		name    string
		ptr     *int
		want    int
		wantNil bool
	}{
		{
			name:    "nil pointer",
			ptr:     nil,
			want:    0,
			wantNil: true,
		},
		{
			name: "zero value",
			ptr:  IntPtr(0),
			want: 0,
		},
		{
			name: "positive value",
			ptr:  IntPtr(42),
			want: 42,
		},
		{
			name: "negative value",
			ptr:  IntPtr(-1),
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PtrInt(tt.ptr)
			if got != tt.want {
				t.Errorf("PtrInt() = %v, want %v", got, tt.want)
			}
		})
	}
}
