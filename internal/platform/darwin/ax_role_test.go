//go:build darwin

package darwin

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

func TestAXRoleCodeSelectRoles(t *testing.T) {
	if got := axRoleCode(ax.RoleComboBox); got != 12 {
		t.Fatalf("RoleComboBox maps to %d, want GOOSIE_AX_COMBO_BOX (12)", got)
	}
	if got := axRoleCode(ax.RoleOption); got != 13 {
		t.Fatalf("RoleOption maps to %d, want GOOSIE_AX_OPTION (13)", got)
	}
}
