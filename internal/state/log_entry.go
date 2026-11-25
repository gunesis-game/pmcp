package state

// LogEntry is one event in a standard cycle's checkpoint log.
type LogEntry struct {
	At      string `json:"at"`
	Event   string `json:"event"`
	Status  string `json:"status"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// UnmarshalJSON requires exactly the entry's field set.
func (l *LogEntry) UnmarshalJSON(data []byte) error {
	type plain LogEntry
	return decodeExact(data, (*plain)(l))
}
