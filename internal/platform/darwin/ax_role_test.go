//go:build darwin

package darwin

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

// qc:BR-139
func TestAXRoleCodeSelectRoles(t *testing.T) {
	if got := axRoleCode(ax.RoleComboBox); got != 12 {
		t.Fatalf("RoleComboBox maps to %d, want GOOSIE_AX_COMBO_BOX (12)", got)
	}
	if got := axRoleCode(ax.RoleOption); got != 13 {
		t.Fatalf("RoleOption maps to %d, want GOOSIE_AX_OPTION (13)", got)
	}
	if got := axRoleCode(ax.RoleHeading); got != 14 {
		t.Fatalf("RoleHeading maps to %d, want GOOSIE_AX_HEADING (14)", got)
	}
	if got := axRoleCode(ax.RoleListBox); got != 15 {
		t.Fatalf("RoleListBox maps to %d, want GOOSIE_AX_LIST_BOX (15)", got)
	}
	if got := ax.RoleListBox.String(); got != "listbox" {
		t.Fatalf("RoleListBox String() = %q, want the ARIA token listbox", got)
	}
}
