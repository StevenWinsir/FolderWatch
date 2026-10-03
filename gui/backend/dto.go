// Package backend adapts the existing application core to a versioned GUI IPC.
// DTOs deliberately contain no core types, snapshot references or native events.
package backend

const ProtocolVersion = 1

const (
	EventStatus  = "session.status"
	EventChanges = "changes.updated"
	EventWarning = "session.warning"
	EventError   = "session.error"
)

type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AppInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuildDate       string `json:"buildDate"`
	Protocol        int    `json:"protocol"`
	HeartbeatMillis int    `json:"heartbeatMillis"`
	LeaseMillis     int    `json:"leaseMillis"`
}

// All uint64 counters and int64 byte counts cross IPC as decimal strings, not
// JavaScript numbers. Sequence orders the entire facade; version is core-owned.
type SessionInfo struct {
	SessionID  string   `json:"sessionId"`
	Root       string   `json:"root"`
	State      string   `json:"state"`
	Sequence   string   `json:"sequence"`
	Generation string   `json:"generation"`
	Version    string   `json:"version"`
	Warning    string   `json:"warning"`
	Problem    *Problem `json:"problem,omitempty"`
}

type Event struct {
	Protocol int         `json:"protocol"`
	Name     string      `json:"name"`
	ClientID string      `json:"clientId"`
	Status   SessionInfo `json:"status"`
	Reload   bool        `json:"reload"`
	Problem  *Problem    `json:"problem,omitempty"`
}

type ConnectionReply struct {
	ClientID string      `json:"clientId"`
	App      AppInfo     `json:"app"`
	Status   SessionInfo `json:"status"`
	Error    *Problem    `json:"error,omitempty"`
}

type SessionRequest struct {
	ClientID  string `json:"clientId"`
	SessionID string `json:"sessionId"`
}

type StartOptions struct {
	ClientID string `json:"clientId"`
	Root     string `json:"root"`
	// Nil means inherit the same user/project/default configuration as the CLI.
	Debounce         *string  `json:"debounce,omitempty"`
	Ignore           []string `json:"ignore,omitempty"`
	RespectGitIgnore *bool    `json:"respectGitIgnore,omitempty"`
	MaxDiffBytes     *string  `json:"maxDiffBytes,omitempty"`
}

type Reply struct {
	Status SessionInfo `json:"status"`
	Error  *Problem    `json:"error,omitempty"`
}

// FolderReply keeps the native picker behind the same small, versioned
// envelope as the rest of the GUI API. An empty path means the user cancelled.
type FolderReply struct {
	Path  string   `json:"path"`
	Error *Problem `json:"error,omitempty"`
}

type ChangesRequest struct {
	SessionRequest
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	Generation string `json:"generation"`
	Version    string `json:"version"`
}

type FileInfo struct {
	SizeBytes      string `json:"sizeBytes"`
	Kind           string `json:"kind"`
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
}

type ChangeSummary struct {
	Path      string    `json:"path"`
	OldPath   string    `json:"oldPath"`
	Kind      string    `json:"kind"`
	Version   string    `json:"version"`
	Before    *FileInfo `json:"before,omitempty"`
	After     *FileInfo `json:"after,omitempty"`
	FirstSeen string    `json:"firstSeen"`
	LastSeen  string    `json:"lastSeen"`
}

type ChangesReply struct {
	SessionID  string          `json:"sessionId"`
	Generation string          `json:"generation"`
	Version    string          `json:"version"`
	Changes    []ChangeSummary `json:"changes"`
	Total      int             `json:"total"`
	NextOffset int             `json:"nextOffset"` // -1 is the final page
	Error      *Problem        `json:"error,omitempty"`
}

type DiffRequest struct {
	SessionRequest
	Path       string `json:"path"`
	Generation string `json:"generation"`
	Version    string `json:"version"` // selected path version, NOT list version
}

type DiffLine struct {
	Kind      string `json:"kind"`
	OldLine   int    `json:"oldLine"`
	NewLine   int    `json:"newLine"`
	Text      string `json:"text"`
	NoNewline bool   `json:"noNewline"`
}

type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []DiffLine `json:"lines"`
}

type DiffResult struct {
	SessionID  string     `json:"sessionId"`
	Path       string     `json:"path"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	Generation string     `json:"generation"`
	Version    string     `json:"version"`
	Hunks      []DiffHunk `json:"hunks"`
}

type DiffReply struct {
	Diff  *DiffResult `json:"diff,omitempty"`
	Error *Problem    `json:"error,omitempty"`
}
