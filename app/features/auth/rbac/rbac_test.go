package rbac

import "testing"

func TestParseRole(t *testing.T) {
	for _, valid := range []string{"admin", "editor", "user"} {
		if _, ok := ParseRole(valid); !ok {
			t.Errorf("expected %q to be a known role", valid)
		}
	}
	if _, ok := ParseRole("root"); ok {
		t.Error("expected unknown role to be rejected")
	}
	if _, ok := ParseRole(""); ok {
		t.Error("expected empty role to be rejected")
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		actor string
		min   Role
		want  bool
	}{
		{"admin", RoleEditor, true},
		{"admin", RoleAdmin, true},
		{"editor", RoleAdmin, false},
		{"editor", RoleEditor, true},
		{"user", RoleEditor, false},
		{"user", RoleUser, true},
		{"nope", RoleUser, false},
	}
	for _, c := range cases {
		if got := AtLeast(c.actor, c.min); got != c.want {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", c.actor, c.min, got, c.want)
		}
	}
}

func TestHasRole(t *testing.T) {
	if !HasRole("admin", RoleAdmin, RoleEditor) {
		t.Error("admin should satisfy [admin editor]")
	}
	if HasRole("user", RoleAdmin, RoleEditor) {
		t.Error("user should not satisfy [admin editor]")
	}
	if HasRole("unknown", RoleUser) {
		t.Error("unknown role should never satisfy a requirement")
	}
}
