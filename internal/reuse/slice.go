// Package reuse provides operations on reusable scratch storage.
package reuse

// Exact releases unused backing storage by copying only when capacity exceeds
// length. Use for finalized payloads that no longer need room for growth.
func Exact[T any](values []T) []T {
	if len(values) == cap(values) {
		return values
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
}

// Resize reuses storage when capacity permits. Growth starts with zeroed values;
// callers fill the result rather than relying on old contents after growth.
func Resize[T any](values []T, length int) []T {
	if length > cap(values) {
		return make([]T, length)
	}
	return values[:length]
}

// Reset clears references throughout the backing array and keeps bounded
// scratch storage. Oversized arrays are released instead of retained in a pool.
func Reset[T any](values []T, maxCapacity int) []T {
	if cap(values) > maxCapacity {
		return nil
	}
	clear(values[:cap(values)])
	return values[:0]
}

// UpdateAt appends or replaces the next observed value and reports a change.
// Callers visit indices in order and truncate the slice after the last value.
func UpdateAt[T comparable](values *[]T, index int, value T) bool {
	if index == len(*values) {
		*values = append(*values, value)
		return true
	}
	if (*values)[index] == value {
		return false
	}
	(*values)[index] = value
	return true
}
