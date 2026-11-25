package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Snapshot is the governed repository's state, policy and project documents
// at one revision, parsed for the rules. Files holds the raw content as read;
// the typed fields are what rules change.
type Snapshot struct {
	Files       map[string][]byte
	Registry    *Registry
	Plans       map[string]*PlanRecord
	Checkpoints map[string]*CheckpointRecord
	Evidence    map[string]map[string]*Evidence
	Ledger      *Ledger
	Audit       *AuditState
	Policy      *Policy
	PolicyDoc   *JSONDocument
	ParseErrors []string
}

// ParseSnapshot parses every state file and the policy found in files.
// Problems are collected in ParseErrors instead of failing, so validation
// can report them.
func ParseSnapshot(files map[string][]byte) *Snapshot {
	snap := &Snapshot{Files: copyFiles(files)}
	snap.Registry = parseFile[Registry](snap, CyclesPath)
	snap.Ledger = parseFile[Ledger](snap, DecisionsPath)
	snap.Audit = parseFile[AuditState](snap, AuditPath)
	if plans := parseFile[map[string]*PlanRecord](snap, PlansPath); plans != nil {
		snap.Plans = *plans
	}
	if checkpoints := parseFile[map[string]*CheckpointRecord](snap, CheckpointsPath); checkpoints != nil {
		snap.Checkpoints = *checkpoints
	}
	if evidence := parseFile[map[string]map[string]*Evidence](snap, EvidencePath); evidence != nil {
		snap.Evidence = *evidence
	}
	snap.parsePolicy()
	return snap
}

func parseFile[T any](snap *Snapshot, path string) *T {
	data, ok := snap.Files[path]
	if !ok {
		if snap.IsInitialized() {
			snap.ParseErrors = append(snap.ParseErrors, path+": missing")
		}
		return nil
	}
	var value T
	if err := DecodeStrict(data, &value); err != nil {
		snap.ParseErrors = append(snap.ParseErrors, fmt.Sprintf("%s: %v", path, err))
		return nil
	}
	return &value
}

func (s *Snapshot) parsePolicy() {
	data, ok := s.Files[PolicyPath]
	if !ok {
		return
	}
	policy, err := ParsePolicy(data)
	if err != nil {
		s.ParseErrors = append(s.ParseErrors, fmt.Sprintf("%s: %v", PolicyPath, err))
		return
	}
	doc, err := ParseJSONDocument(data)
	if err != nil {
		s.ParseErrors = append(s.ParseErrors, fmt.Sprintf("%s: %v", PolicyPath, err))
		return
	}
	s.Policy = policy
	s.PolicyDoc = doc
}

// IsInitialized reports whether any state file exists.
func (s *Snapshot) IsInitialized() bool {
	for _, path := range StatePaths {
		if _, ok := s.Files[path]; ok {
			return true
		}
	}
	return false
}

// IsUsable reports whether every state file parsed.
func (s *Snapshot) IsUsable() bool {
	return len(s.ParseErrors) == 0 && s.Registry != nil && s.Plans != nil && s.Checkpoints != nil &&
		s.Evidence != nil && s.Ledger != nil && s.Audit != nil
}

// SetPolicyValue changes one key of the policy, keeping the rest of the file.
func (s *Snapshot) SetPolicyValue(path string, value any) error {
	if s.PolicyDoc == nil {
		return fmt.Errorf("%s is missing", PolicyPath)
	}
	if err := s.PolicyDoc.Set(path, value); err != nil {
		return err
	}
	encoded, err := s.PolicyDoc.Encode()
	if err != nil {
		return err
	}
	policy, err := ParsePolicy(encoded)
	if err != nil {
		return err
	}
	s.Policy = policy
	s.Files[PolicyPath] = encoded
	return nil
}

// Clone returns an independent copy, re-parsed from the encoded state.
func (s *Snapshot) Clone() (*Snapshot, error) {
	files, err := s.EncodedFiles()
	if err != nil {
		return nil, err
	}
	return ParseSnapshot(files), nil
}

// EncodedFiles returns Files with every parsed state file re-encoded from
// the typed fields.
func (s *Snapshot) EncodedFiles() (map[string][]byte, error) {
	files := copyFiles(s.Files)
	encoders := map[string]any{}
	if s.Registry != nil {
		encoders[CyclesPath] = s.Registry
	}
	if s.Plans != nil {
		encoders[PlansPath] = s.Plans
	}
	if s.Checkpoints != nil {
		encoders[CheckpointsPath] = s.Checkpoints
	}
	if s.Evidence != nil {
		encoders[EvidencePath] = s.Evidence
	}
	if s.Ledger != nil {
		encoders[DecisionsPath] = s.Ledger
	}
	if s.Audit != nil {
		encoders[AuditPath] = s.Audit
	}
	for path, value := range encoders {
		encoded, err := EncodeCanonical(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		files[path] = encoded
	}
	return files, nil
}

// ChangedPaths returns the paths whose content differs between two file
// sets, sorted. A path missing from after counts as changed (deleted).
func ChangedPaths(before, after map[string][]byte) []string {
	changed := []string{}
	for path, content := range after {
		if old, ok := before[path]; !ok || !bytes.Equal(old, content) {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

// StateDigest is the SHA-256 over the state files in path order: for each
// file present, its path, a NUL byte, its bytes and a NUL byte.
func StateDigest(files map[string][]byte) string {
	paths := []string{}
	for path := range files {
		if strings.HasPrefix(path, StateDir) && strings.HasSuffix(path, ".json") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write(files[path])
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func copyFiles(files map[string][]byte) map[string][]byte {
	copied := make(map[string][]byte, len(files))
	for path, content := range files {
		copied[path] = append([]byte(nil), content...)
	}
	return copied
}
