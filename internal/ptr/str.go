package ptr

// StrPtr receives a string and returns a pointer to that string.
// This is useful when you need to pass a string value as a pointer.
func StrPtr(v string) *string {
	return &v
}

// PtrStr receives a pointer to a string and returns the string value.
// If the pointer is nil, it returns an empty string.
func PtrStr(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}
