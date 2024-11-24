package ptr

// BoolPtr receives a bool and returns a pointer to that bool.
// This is useful when you need to pass a boolean value as a pointer.
func BoolPtr(v bool) *bool {
	return &v
}

// PtrBool receives a pointer to a bool and returns the boolean value.
// If the pointer is nil, it returns false.
func PtrBool(v *bool) bool {
	if v == nil {
		return false
	}

	return *v
}
