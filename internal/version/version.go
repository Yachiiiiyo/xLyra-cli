// Package version holds build-time version metadata and the control-plane protocol version.
package version

// Build-time variables injected via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = ""
)

// ProtocolVersion identifies the control-plane UDS request/response wire protocol version.
const ProtocolVersion = 1

// MinServerSchemaVersion identifies the minimum xlyra-server control-plane schema version
// required for full CLI command parity (e.g. model merge, site-model bind-canonical/pagination,
// route overview/candidates/select/failover/traces).
const MinServerSchemaVersion = 2

// BuildInfo describes the compiled xlyra-cli binary and its supported protocol version.
type BuildInfo struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
	BuildTime              string `json:"build_time,omitempty"`
	ProtocolVersion        int    `json:"protocol_version"`
	MinServerSchemaVersion int    `json:"min_server_schema_version"`
	OS                     string `json:"os"`
	Arch                   string `json:"arch"`
}

// Info returns build metadata populated with the current runtime OS and architecture.
func Info(goos, goarch string) BuildInfo {
	return BuildInfo{
		Version:                Version,
		Commit:                 Commit,
		BuildTime:              BuildTime,
		ProtocolVersion:        ProtocolVersion,
		MinServerSchemaVersion: MinServerSchemaVersion,
		OS:                     goos,
		Arch:                   goarch,
	}
}
