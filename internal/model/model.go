// Package model defines the shared data types used across the tool, including
// the per-folder scan result and the access-status classification.
package model

// ScannedFolder is one hashed folder discovered directly under the
// WebsiteData base path, enriched with its resolved domain and size.
type ScannedFolder struct {
	Hash    string   // the hashed directory name (direct child of base)
	AbsPath string   // absolute path to the folder
	SizeKB  int64    // recursive size in 1024-byte blocks (du -sk semantics)
	Domain  string   // resolved primary domain, or "Unknown Domain"
	Hosts   []string // all hosts parsed from the origin file (partition hosts)
}

// AccessStatus classifies the state of the base path for messaging.
type AccessStatus int

const (
	// StatusOK indicates the base exists with at least one readable folder.
	StatusOK AccessStatus = iota
	// StatusNotFound indicates the base path does not exist (Req 6.3).
	StatusNotFound
	// StatusAccessBlocked indicates the base exists but has zero readable
	// folders, suggesting Full Disk Access is required (Req 6.1).
	StatusAccessBlocked
)
