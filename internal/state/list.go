package state

// List is a JSON list that always encodes as an array, never as null, so
// every list key in a state file is present as [] when empty.
type List[T any] []T

// MarshalJSON encodes a nil list as [].
func (l List[T]) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return marshalUnescaped([]T(l))
}
