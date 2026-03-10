package security

import "testing"

func TestSecurityProfile_Validate(t *testing.T) {
	p := StrictProfile()
	if err := p.Validate(); err != nil {
		t.Fatalf("strict profile should be valid: %v", err)
	}
	bad := p
	bad.ApprovalMode = "invalid"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected invalid approval mode error")
	}
	bad = p
	bad.SandboxLevel = "vm"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected invalid sandbox level error")
	}
}

func TestProfiles_ValidateAll(t *testing.T) {
	profiles := []SecurityProfile{
		StrictProfile(),
		BalancedProfile(),
		DevProfile(),
	}
	for _, p := range profiles {
		if err := p.Validate(); err != nil {
			t.Fatalf("profile %s should be valid: %v", p.Name, err)
		}
	}
}
