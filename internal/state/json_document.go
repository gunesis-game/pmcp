package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// JSONDocument is a JSON value that keeps object key order, so pmcp can
// change one key of a user-owned file (the policy) without reordering or
// dropping anything else.
type JSONDocument struct {
	keys   []string
	fields map[string]*JSONDocument
	leaf   json.RawMessage
}

// ParseJSONDocument parses data, keeping key order.
func ParseJSONDocument(data []byte) (*JSONDocument, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty JSON document")
	}
	if trimmed[0] != '{' {
		if !json.Valid(trimmed) {
			return nil, errors.New("invalid JSON")
		}
		return &JSONDocument{leaf: append(json.RawMessage{}, trimmed...)}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	doc := &JSONDocument{fields: map[string]*JSONDocument{}}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", token)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		child, err := ParseJSONDocument(raw)
		if err != nil {
			return nil, err
		}
		if _, exists := doc.fields[key]; !exists {
			doc.keys = append(doc.keys, key)
		}
		doc.fields[key] = child
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return doc, nil
}

// Set replaces (or appends) the value at a dotted path of object keys.
func (d *JSONDocument) Set(path string, value any) error {
	encoded, err := marshalUnescaped(value)
	if err != nil {
		return err
	}
	parent, key, err := d.parentOf(path)
	if err != nil {
		return err
	}
	if _, exists := parent.fields[key]; !exists {
		parent.keys = append(parent.keys, key)
	}
	parent.fields[key] = &JSONDocument{leaf: encoded}
	return nil
}

// Delete removes the key at a dotted path. It reports whether it existed.
func (d *JSONDocument) Delete(path string) bool {
	parent, key, err := d.parentOf(path)
	if err != nil {
		return false
	}
	if _, exists := parent.fields[key]; !exists {
		return false
	}
	delete(parent.fields, key)
	kept := parent.keys[:0]
	for _, existing := range parent.keys {
		if existing != key {
			kept = append(kept, existing)
		}
	}
	parent.keys = kept
	return true
}

func (d *JSONDocument) parentOf(path string) (*JSONDocument, string, error) {
	parts := strings.Split(path, ".")
	current := d
	for _, part := range parts[:len(parts)-1] {
		if current.fields == nil {
			return nil, "", fmt.Errorf("%s: not an object", path)
		}
		next, ok := current.fields[part]
		if !ok || next.fields == nil {
			return nil, "", fmt.Errorf("%s: %q is not an object", path, part)
		}
		current = next
	}
	if current.fields == nil {
		return nil, "", fmt.Errorf("%s: not an object", path)
	}
	return current, parts[len(parts)-1], nil
}

// Encode writes the document in canonical form.
func (d *JSONDocument) Encode() ([]byte, error) {
	var compact bytes.Buffer
	if err := d.writeCompact(&compact); err != nil {
		return nil, err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	indented.WriteByte('\n')
	return indented.Bytes(), nil
}

func (d *JSONDocument) writeCompact(buf *bytes.Buffer) error {
	if d.fields == nil {
		return json.Compact(buf, d.leaf)
	}
	buf.WriteByte('{')
	for i, key := range d.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encodedKey, err := marshalUnescaped(key)
		if err != nil {
			return err
		}
		buf.Write(encodedKey)
		buf.WriteByte(':')
		if err := d.fields[key].writeCompact(buf); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}
