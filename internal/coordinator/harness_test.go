package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gunesis-game/pmcp/internal/coordinator"
	"github.com/gunesis-game/pmcp/internal/state"
	"github.com/gunesis-game/pmcp/internal/store"
)

// harness drives a coordinator over an in-memory repository with a fixed
// clock and sequential IDs.
type harness struct {
	t     *testing.T
	store *store.MemoryStore
	facts *fakeGitHubFacts
	coord *coordinator.Coordinator
	repo  store.Repo
	clock time.Time
	ids   int
}

func newHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		store: store.NewMemoryStore(),
		facts: newFakeGitHubFacts(),
		repo:  store.Repo{Owner: "gunesis-game", Name: "example"},
		clock: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
	}
	if files == nil {
		files = map[string]string{"README.md": "# example\n"}
	}
	h.store.CreateRepo(h.repo, "main", files)
	h.coord = &coordinator.Coordinator{
		Store: h.store,
		Facts: h.facts,
		Now:   h.tick,
		NewID: h.nextID,
		Sleep: h.sleep,
	}
	return h
}

func (h *harness) tick() time.Time {
	h.clock = h.clock.Add(time.Second)
	return h.clock
}

func (h *harness) sleep(_ context.Context, d time.Duration) error {
	h.clock = h.clock.Add(d)
	return nil
}

func (h *harness) nextID() string {
	h.ids++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", h.ids)
}

func (h *harness) request(tool, action string, args map[string]any) coordinator.Request {
	raw, err := json.Marshal(args)
	if err != nil {
		h.t.Fatal(err)
	}
	return coordinator.Request{Owner: h.repo.Owner, Repo: h.repo.Name, RequestID: h.nextID(), Tool: tool, Action: action, Args: raw}
}

func (h *harness) send(req coordinator.Request) *coordinator.Result {
	return h.coord.Handle(context.Background(), req)
}

func (h *harness) call(tool, action string, args map[string]any) *coordinator.Result {
	return h.send(h.request(tool, action, args))
}

// ok calls and requires status ok.
func (h *harness) ok(tool, action string, args map[string]any) *coordinator.Result {
	h.t.Helper()
	result := h.call(tool, action, args)
	if result.Status != coordinator.StatusOK {
		h.t.Fatalf("%s.%s: status %s: %s (fix: %v)", tool, action, result.Status, result.Summary, deref(result.Fix))
	}
	return result
}

// rejected calls, requires status rejected and an unchanged head.
func (h *harness) rejected(tool, action string, args map[string]any) *coordinator.Result {
	h.t.Helper()
	before := h.head()
	result := h.call(tool, action, args)
	if result.Status != coordinator.StatusRejected {
		h.t.Fatalf("%s.%s: expected rejected, got %s: %s", tool, action, result.Status, result.Summary)
	}
	if h.head() != before {
		h.t.Fatalf("%s.%s: a rejected call moved the head", tool, action)
	}
	if result.PublishedRevision != nil || len(result.ChangedFiles) != 0 {
		h.t.Fatalf("%s.%s: a rejected call reports changes", tool, action)
	}
	return result
}

func (h *harness) head() string {
	head, err := h.store.Head(context.Background(), h.repo, "main")
	if err != nil {
		h.t.Fatal(err)
	}
	return head
}

func (h *harness) push(message string, files map[string]string) string {
	h.t.Helper()
	sha, err := h.store.PushFiles(h.repo, "main", message, files)
	if err != nil {
		h.t.Fatal(err)
	}
	return sha
}

// setPolicy changes policy keys through an ordinary (non-pmcp) commit.
func (h *harness) setPolicy(values map[string]any) {
	h.t.Helper()
	content, ok := h.store.FileAt(h.repo, "main", state.PolicyPath)
	if !ok {
		h.t.Fatal("policy missing")
	}
	doc, err := state.ParseJSONDocument([]byte(content))
	if err != nil {
		h.t.Fatal(err)
	}
	for path, value := range values {
		if err := doc.Set(path, value); err != nil {
			h.t.Fatal(err)
		}
	}
	encoded, err := doc.Encode()
	if err != nil {
		h.t.Fatal(err)
	}
	h.push("Set the project policy", map[string]string{state.PolicyPath: string(encoded)})
}

func expectNext(t *testing.T, result *coordinator.Result, tool, action string) {
	t.Helper()
	if result.NextRequest == nil {
		t.Fatalf("no next_request; expected %s.%s (summary: %s)", tool, action, result.Summary)
	}
	if result.NextRequest.Tool != "pmcp_"+tool || result.NextRequest.Request.Action != action {
		t.Fatalf("next_request is %s.%s, expected pmcp_%s.%s", result.NextRequest.Tool, result.NextRequest.Request.Action, tool, action)
	}
}

func dataOf(t *testing.T, result *coordinator.Result) map[string]any {
	t.Helper()
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func requireContains(t *testing.T, text, part string) {
	t.Helper()
	if !strings.Contains(text, part) {
		t.Fatalf("%q does not contain %q", text, part)
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
