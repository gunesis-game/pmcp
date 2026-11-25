package guide

import (
	"testing"

	"github.com/gunesis-game/pmcp/internal/governance"
)

// TestRuleReadSectionsAreServed covers REQ-091.
func TestRuleReadSectionsAreServed(t *testing.T) {
	reads := []string{governance.ReadQuickContract, governance.ReadWorkflow, governance.ReadChoosingCycle, governance.ReadPhases,
		governance.ReadDecisionGate, governance.ReadReconcile, governance.ReadAudit, governance.ReadMainBranch, governance.ReadDocumentOwnership}
	for _, read := range reads {
		if _, ok := ByURI("pmcp://guide/" + read); !ok {
			t.Errorf("guide section %s is not served", read)
		}
	}
	if _, ok := ByURI("pmcp://standards/project-documents"); !ok {
		t.Error("standards section project-documents is not served")
	}
}
