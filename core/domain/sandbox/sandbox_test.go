package sandbox

import "testing"

func TestFileRequiresCleanAbsolutePath(t *testing.T) {
	for _, file := range []File{
		{Path: "relative.txt"},
		{Path: "/work/../secret.txt"},
	} {
		if err := file.Validate(); err == nil {
			t.Fatalf("expected invalid sandbox file path %q", file.Path)
		}
	}
	if err := (File{Path: "/work/skill/SKILL.md"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkPolicyValidatesActionsAndTargets(t *testing.T) {
	valid := NetworkPolicy{
		DefaultAction: NetworkActionDeny,
		Rules:         []NetworkRule{{Action: NetworkActionAllow, Target: "pypi.org"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := NetworkPolicy{
		DefaultAction: NetworkActionDeny,
		Rules:         []NetworkRule{{Action: NetworkActionAllow}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected empty network target to be rejected")
	}
}
