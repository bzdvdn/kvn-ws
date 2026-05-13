package nat

import (
	"testing"
)

// @sk-test routing-split-tunnel#T2.5: TestNFTManagerInterface (AC-007)
func TestNFTManagerInterface(t *testing.T) {
	mgr := NewNFTManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	var _ Manager = mgr
}
