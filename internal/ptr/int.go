package ptr

// IntPtr returns a pointer to the given int value.
func IntPtr(v int) *int {
	return &v
}

// PtrInt returns the value of the given int pointer, or 0 if the pointer is nil.
func PtrInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
