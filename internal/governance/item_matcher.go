package governance

import "strings"

// matchItem finds the index of the text among candidates: an exact match,
// or else a unique prefix match (REQ-040).
func matchItem(kind string, texts []string, wanted string) (int, error) {
	for i, text := range texts {
		if text == wanted {
			return i, nil
		}
	}
	found := -1
	for i, text := range texts {
		if strings.HasPrefix(text, wanted) {
			if found >= 0 {
				return -1, Reject("%s %q matches more than one; use more of its text", kind, wanted)
			}
			found = i
		}
	}
	if found < 0 {
		return -1, Reject("no %s matches %q", kind, wanted)
	}
	return found, nil
}
