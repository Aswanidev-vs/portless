package nfr

import (
	"context"
	"testing"
	"time"

	"gotunnel/internal/state"
)

func TestSyntheticScaleAndRecoveryValidation(t *testing.T) {
	validator := NewValidator(ValidatorConfig{})
	validator.AddRequirement(&NFRRequirement{ID: "scale_10k", Type: NFRScalability, Threshold: 10000, Unit: "tunnels"})
	validator.AddRequirement(&NFRRequirement{ID: "recovery_30s", Type: NFRRecovery, Threshold: 30, Unit: "seconds"})

	for i := 0; i < 10000; i++ {
		if err := validator.ValidateScalability("scale_10k", i+1); err != nil {
			t.Fatalf("validate scalability: %v", err)
		}
	}

	start := time.Now()
	store := state.NewMemoryStore()
	replicator := state.NewReplicator(state.Config{NodeID: "node-b"}, store)
	replicator.RegisterNode("node-a", "http://node-a")
	if err := replicator.Start(context.Background()); err != nil {
		t.Fatalf("start replicator: %v", err)
	}
	replicator.RemoveNode("node-a")
	recoveryTime := time.Since(start)
	if err := validator.ValidateRecovery("recovery_30s", recoveryTime); err != nil {
		t.Fatalf("validate recovery: %v", err)
	}

	statuses := validator.Validate(context.Background())
	if statuses["scale_10k"] != NFRStatusPassing {
		t.Fatalf("expected scale requirement to pass, got %s", statuses["scale_10k"])
	}
	if statuses["recovery_30s"] != NFRStatusPassing {
		t.Fatalf("expected recovery requirement to pass, got %s", statuses["recovery_30s"])
	}
}
