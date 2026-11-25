package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Commit trailers (QR-04).
const (
	TrailerTool    = "Pmcp-Tool"
	TrailerAction  = "Pmcp-Action"
	TrailerRequest = "Pmcp-Request"
	TrailerDigest  = "Pmcp-State-Digest"
	TrailerResult  = "Pmcp-Result"
)

// CommitMessage is a pmcp state commit's message.
type CommitMessage struct {
	Tool      string
	Action    string
	Verdict   string
	RequestID string
	Body      string
	Digest    string
	Result    *Result
}

// Build renders the message: subject, body and trailers.
func (m CommitMessage) Build() (string, error) {
	stored := *m.Result
	stored.NextRequest = nil
	stored.PublishedRevision = nil
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(stored); err != nil {
		return "", err
	}
	short := m.RequestID
	if len(short) > 8 {
		short = short[:8]
	}
	body := strings.TrimSpace(strings.ReplaceAll(m.Body, "\r", ""))
	var builder strings.Builder
	fmt.Fprintf(&builder, "pmcp(%s): %s %s [%s]\n\n", m.Tool, m.Action, m.Verdict, short)
	if body != "" {
		builder.WriteString(body + "\n\n")
	}
	fmt.Fprintf(&builder, "%s: %s\n", TrailerTool, m.Tool)
	fmt.Fprintf(&builder, "%s: %s\n", TrailerAction, m.Action)
	fmt.Fprintf(&builder, "%s: %s\n", TrailerRequest, m.RequestID)
	fmt.Fprintf(&builder, "%s: %s\n", TrailerDigest, m.Digest)
	fmt.Fprintf(&builder, "%s: %s", TrailerResult, strings.TrimRight(encoded.String(), "\n"))
	return builder.String(), nil
}

// ParseTrailers returns the pmcp trailers of a commit message. A commit
// without a Pmcp-Tool trailer is not a pmcp commit.
func ParseTrailers(message string) map[string]string {
	trailers := map[string]string{}
	for _, line := range strings.Split(message, "\n") {
		key, value, found := strings.Cut(line, ": ")
		if !found || !strings.HasPrefix(key, "Pmcp-") {
			continue
		}
		trailers[key] = strings.TrimSpace(value)
	}
	return trailers
}

// StoredResult decodes the Pmcp-Result trailer.
func StoredResult(trailers map[string]string) (*Result, error) {
	raw, ok := trailers[TrailerResult]
	if !ok {
		return nil, fmt.Errorf("commit has no %s trailer", TrailerResult)
	}
	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
