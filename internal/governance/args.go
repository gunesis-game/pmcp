package governance

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Args are one action's arguments, decoded and checked on demand (REQ-017).
type Args struct {
	values map[string]json.RawMessage
}

// NewArgs parses the args object of a call. Null or empty means no arguments.
func NewArgs(raw json.RawMessage) (Args, error) {
	args := Args{values: map[string]json.RawMessage{}}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return args, nil
	}
	if err := json.Unmarshal(trimmed, &args.values); err != nil {
		return args, Reject("args must be a JSON object")
	}
	for name, value := range args.values {
		if string(bytes.TrimSpace(value)) == "null" {
			delete(args.values, name)
		}
	}
	return args, nil
}

// ArgsOf builds Args from Go values (used by tests and internal callers).
func ArgsOf(values map[string]any) Args {
	raw, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	args, err := NewArgs(raw)
	if err != nil {
		panic(err)
	}
	return args
}

// Allow rejects any argument not in names.
func (a Args) Allow(names ...string) error {
	unknown := []string{}
	for name := range a.values {
		if !containsText(names, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	sort.Strings(names)
	return Reject("unknown argument(s) %s; accepted: %s", strings.Join(unknown, ", "), strings.Join(names, ", "))
}

// Has reports whether the argument was given.
func (a Args) Has(name string) bool {
	_, ok := a.values[name]
	return ok
}

// String returns an optional non-empty string argument.
func (a Args) String(name string) (string, bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false, Reject("argument %s must be a string", name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, Reject("argument %s must not be empty", name)
	}
	return value, true, nil
}

// RequireString returns a required non-empty string argument.
func (a Args) RequireString(name string) (string, error) {
	value, ok, err := a.String(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", Reject("argument %s is required", name)
	}
	return value, nil
}

// Strings returns an optional list argument: one string or a list of
// non-empty strings.
func (a Args) Strings(name string) ([]string, bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return nil, false, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, false, Reject("argument %s must not be empty", name)
		}
		return []string{single}, true, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false, Reject("argument %s must be a string or a list of strings", name)
	}
	cleaned := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, false, Reject("argument %s must not contain empty strings", name)
		}
		cleaned = append(cleaned, item)
	}
	return cleaned, true, nil
}

// RequireStrings returns a required, non-empty list argument.
func (a Args) RequireStrings(name string) ([]string, error) {
	values, ok, err := a.Strings(name)
	if err != nil {
		return nil, err
	}
	if !ok || len(values) == 0 {
		return nil, Reject("argument %s is required", name)
	}
	return values, nil
}

// Bool returns an optional boolean argument.
func (a Args) Bool(name string) (bool, bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false, Reject("argument %s must be a boolean", name)
	}
	return value, true, nil
}

// Attestation requires the argument to be the boolean true (DR-12).
func (a Args) Attestation(name string) error {
	value, ok, err := a.Bool(name)
	if err != nil || !ok || !value {
		return Reject("argument %s must be true, and only when the statement is genuinely true", name)
	}
	return nil
}

// Int returns an optional integer argument.
func (a Args) Int(name string) (int, bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return 0, false, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false, Reject("argument %s must be an integer", name)
	}
	return value, true, nil
}

// CycleRef returns an optional cycle reference: a UUID, a UUID prefix or a
// sequence number given as an integer or digit string.
func (a Args) CycleRef(name string) (string, bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return "", false, nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.Itoa(number), true, nil
	}
	return a.String(name)
}

// Decode decodes an optional argument into target.
func (a Args) Decode(name string, target any) (bool, error) {
	raw, ok := a.values[name]
	if !ok {
		return false, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return false, Reject("argument %s is malformed: %v", name, err)
	}
	return true, nil
}

// Raw returns the raw arguments, for echoing.
func (a Args) Raw() map[string]json.RawMessage {
	return a.values
}

func containsText(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
