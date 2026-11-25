package state

// Repository paths of the governed layout (D-P3).
const (
	StateDir        = ".pmcp/"
	CyclesPath      = ".pmcp/cycles.json"
	PlansPath       = ".pmcp/plans.json"
	CheckpointsPath = ".pmcp/checkpoints.json"
	EvidencePath    = ".pmcp/evidence.json"
	DecisionsPath   = ".pmcp/decisions.json"
	AuditPath       = ".pmcp/audit.json"
	ProjectDir      = "project/"
	PolicyPath      = "project/policy.json"
	RequirementsDoc = "project/requirements.md"
	ArchitectureDoc = "project/architecture.md"
)

// StatePaths lists the state files in path order.
var StatePaths = []string{AuditPath, CheckpointsPath, CyclesPath, DecisionsPath, EvidencePath, PlansPath}
