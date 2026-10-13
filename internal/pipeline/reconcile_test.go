package pipeline

import (
	"testing"
	"time"
)

func TestArtifactReconciliationProvisionalPolicy(t *testing.T) {
	if DefaultArtifactOrphanGracePeriod != 7*24*time.Hour {
		t.Fatalf("orphan grace = %s, want provisional 7 days", DefaultArtifactOrphanGracePeriod)
	}
	if DefaultArtifactReconciliationInterval != 24*time.Hour {
		t.Fatalf("reconciliation interval = %s, want provisional 24 hours", DefaultArtifactReconciliationInterval)
	}
}
