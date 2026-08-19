package roleplay

import "testing"

func TestOpaqueRoleplayIdentitiesAreExact(t *testing.T) {
	tests := []struct {
		name string
		id   string
		kind identityKind
		ok   bool
	}{
		{name: "world", id: "rpw_0123456789abcdef0123456789abcdef", kind: worldIdentity, ok: true},
		{name: "character", id: "rpc_0123456789abcdef0123456789abcdef", kind: characterIdentity, ok: true},
		{name: "event", id: "rpe_0123456789abcdef0123456789abcdef", kind: eventIdentity, ok: true},
		{name: "knowledge", id: "rpk_0123456789abcdef0123456789abcdef", kind: knowledgeIdentity, ok: true},
		{name: "wrong prefix", id: "rpc_0123456789abcdef0123456789abcdef", kind: worldIdentity},
		{name: "uppercase", id: "rpw_0123456789ABCDEF0123456789abcdef", kind: worldIdentity},
		{name: "short", id: "rpw_0123", kind: worldIdentity},
		{name: "semantic", id: "rpw_kingdom", kind: worldIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateIdentity(test.id, test.kind)
			if test.ok && err != nil {
				t.Fatal(err)
			}
			if !test.ok && err == nil {
				t.Fatalf("validateIdentity(%q) succeeded", test.id)
			}
		})
	}
}

func TestProjectionLimitFailsLoudly(t *testing.T) {
	for _, limit := range []int{0, -1, MaxProjectionEvents + 1} {
		if err := validateProjectionLimit(limit); err == nil {
			t.Fatalf("limit %d succeeded", limit)
		}
	}
	if err := validateProjectionLimit(MaxProjectionEvents); err != nil {
		t.Fatal(err)
	}
}
