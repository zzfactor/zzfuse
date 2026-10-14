package util

// Ptr returns a pointer to the given value.
// This is a generic helper that works with any type.
func Ptr[T any](v T) *T { return &v }

// PtrOrNil returns a pointer to the value if the condition is true, otherwise nil.
// Useful for optional fields that should be nil when empty/zero.
func PtrOrNil[T comparable](v T, condition bool) *T {
	if condition {
		return &v
	}
	return nil
}

// Deref returns the value pointed to by ptr, or zero value if ptr is nil.
func Deref[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

// DerefOr returns the value pointed to by ptr, or defaultVal if ptr is nil.
func DerefOr[T any](ptr *T, defaultVal T) T {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// PtrFromString returns a pointer to the given string.
// Deprecated: Use Ptr[string](s) instead.
func PtrFromString(s string) *string { return &s }

// PtrFromInt64 returns a pointer to the given int64.
// Deprecated: Use Ptr[int64](i) instead.
func PtrFromInt64(i int64) *int64 { return &i }

// PtrFromFloat64 returns a pointer to the given float64.
// Deprecated: Use Ptr[float64](f) instead.
func PtrFromFloat64(f float64) *float64 { return &f }

// PtrFromInt32 returns a pointer to the given int32.
// Deprecated: Use Ptr[int32](i) instead.
func PtrFromInt32(i int32) *int32 { return &i }

// PtrFromBool returns a pointer to the given bool.
// Deprecated: Use Ptr[bool](b) instead.
func PtrFromBool(b bool) *bool { return &b }

// PtrFromInt returns a pointer to the given int.
// Deprecated: Use Ptr[int](i) instead.
func PtrFromInt(i int) *int { return &i }

// NonEmptyStringPtr returns a pointer to the string if it's non-empty, otherwise nil.
func NonEmptyStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
