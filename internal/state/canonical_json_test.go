package state

import (
	"strings"
	"testing"
)

func TestCycleDecodingRequiresExactFields(t *testing.T) {
	valid := `{"id":"a","sequence":1,"execution_order":null,"workflow":"standard","description":"d","status":"pending","created_at":null,"started_at":null,"completed_at":null,"current_phase":"assess","status_reason":null,"eligibility_reason":null,"acceptance_criteria":null,"verification_evidence":null,"outcome":null}`
	var cycle Cycle
	if err := DecodeStrict([]byte(valid), &cycle); err != nil {
		t.Fatal(err)
	}
	if err := DecodeStrict([]byte(strings.Replace(valid, `"outcome":null`, `"outcome":null,"extra":1`, 1)), &cycle); err == nil {
		t.Fatal("an unknown field must be rejected")
	}
	if err := DecodeStrict([]byte(strings.Replace(valid, `,"outcome":null`, ``, 1)), &cycle); err == nil {
		t.Fatal("a missing field must be rejected")
	}
}

func TestCanonicalEncodingKeepsPlaceholdersAndEmptyLists(t *testing.T) {
	encoded, err := EncodeCanonical(Registry{SchemaVersion: 5})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{\n  \"schema_version\": 5,\n  \"cycles\": []\n}\n" {
		t.Fatalf("unexpected encoding: %q", encoded)
	}
	text, _ := EncodeCanonical(List[string]{"<x>"})
	if !strings.Contains(string(text), "<x>") {
		t.Fatalf("HTML must not be escaped: %s", text)
	}
}

func TestJSONDocumentKeepsOrderAndUnknownKeys(t *testing.T) {
	doc, err := ParseJSONDocument([]byte(`{"z":1,"approvals":{"testing_strategy":null,"b":2},"custom":{"keep":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Set("approvals.testing_strategy", "D-001"); err != nil {
		t.Fatal(err)
	}
	doc.Delete("approvals.b")
	encoded, _ := doc.Encode()
	want := "{\n  \"z\": 1,\n  \"approvals\": {\n    \"testing_strategy\": \"D-001\"\n  },\n  \"custom\": {\n    \"keep\": true\n  }\n}\n"
	if string(encoded) != want {
		t.Fatalf("got %s", encoded)
	}
}
