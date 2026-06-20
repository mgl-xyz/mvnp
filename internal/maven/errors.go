package maven

import (
	"errors"
	"strings"
)

var (
	ErrRateLimited      = errors.New("rate limited by Maven Central")
	ErrArtifactNotFound = errors.New("artifact not found")
)

// ShortSkipReason returns a compact user-facing skip reason.
func ShortSkipReason(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(reason, "429"),
		strings.Contains(lower, "abusive"),
		strings.Contains(lower, "wasteful"):
		return "Rate limited"
	case strings.HasPrefix(reason, "property not found in <properties>:"):
		key := strings.TrimSpace(strings.TrimPrefix(reason, "property not found in <properties>:"))
		return "Property missing: " + key
	case strings.HasPrefix(reason, "artifact not found"):
		return "Not found"
	case strings.HasPrefix(reason, "no versions found"):
		return "No versions"
	case strings.HasPrefix(reason, "no matching version"):
		return "No matching version"
	case strings.HasPrefix(reason, "unsupported version"):
		return "Unsupported version"
	case strings.HasPrefix(reason, "metadata request failed"):
		return "Repository error"
	case strings.HasPrefix(reason, "fetch metadata"):
		return "Network error"
	case len(reason) > 48:
		return reason[:45] + "..."
	default:
		return reason
	}
}
