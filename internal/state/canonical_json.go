// Package state holds the Go types for every governance state file and the
// canonical JSON encoding pmcp writes them with.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// EncodeCanonical encodes v with 2-space indentation, struct keys in their
// defined order, no HTML escaping and a trailing newline, so diffs of state
// files stay small and stable.
func EncodeCanonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// marshalUnescaped is json.Marshal without HTML escaping, for custom
// marshalers whose output must keep "<" and ">" readable.
func marshalUnescaped(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// DecodeStrict decodes one JSON value into target, rejecting unknown fields
// and trailing data.
func DecodeStrict(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// decodeExact decodes a JSON object into target (a pointer to a struct) and
// requires the object's keys to be exactly the struct's JSON field names.
// Fields tagged omitempty are optional.
func decodeExact(data []byte, target any) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if keys == nil {
		return errors.New("expected an object, got null")
	}
	structType := reflect.TypeOf(target).Elem()
	required, optional := jsonFieldNames(structType)
	for _, name := range required {
		if _, ok := keys[name]; !ok {
			return fmt.Errorf("missing field %q", name)
		}
	}
	for name := range keys {
		if !containsString(required, name) && !containsString(optional, name) {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return DecodeStrict(data, target)
}

func jsonFieldNames(structType reflect.Type) (required []string, optional []string) {
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		if len(parts) > 1 && containsString(parts[1:], "omitempty") {
			optional = append(optional, parts[0])
		} else {
			required = append(required, parts[0])
		}
	}
	return required, optional
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
