package caldav

import "testing"

// TestPrivilegeWritableEachGrant closes a pass-12 escaped-canary hole: the only
// writability fixture granted BOTH write AND write-content, so dropping either
// term from writable()'s OR-chain escaped the suite. A write-content-only or
// bind-only share (a common NextCloud share grant) would then be misclassified
// read-only, silently blocking all writes. Assert each grant independently yields
// writable, and that no grant is read-only.
func TestPrivilegeWritableEachGrant(t *testing.T) {
	granted := &struct{}{}
	oneGrant := func(p privilege) privResponse {
		return privResponse{Propstats: []privPropstat{{Prop: privProp{PrivilegeSet: privilegeSet{Privileges: []privilege{p}}}}}}
	}
	cases := []struct {
		name string
		r    privResponse
		want bool
	}{
		{"write", oneGrant(privilege{Write: granted}), true},
		{"write-content", oneGrant(privilege{WriteContent: granted}), true},
		{"bind", oneGrant(privilege{Bind: granted}), true},
		{"all", oneGrant(privilege{All: granted}), true},
		{"read-only (no write grants)", oneGrant(privilege{}), false},
	}
	for _, c := range cases {
		if got := c.r.writable(); got != c.want {
			t.Errorf("%s: writable() = %v, want %v", c.name, got, c.want)
		}
	}
}
