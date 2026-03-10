package node

import "testing"

func TestTaskRequirement_Matches(t *testing.T) {
	cap := Capability{NodeID: "coder2", HasGPU: true, HasAudioOut: false, HasBrowser: true}
	if !(TaskRequirement{NeedsGPU: true}).Matches(cap) {
		t.Fatal("expected GPU requirement to match")
	}
	if (TaskRequirement{NeedsAudioOut: true}).Matches(cap) {
		t.Fatal("expected audio requirement to fail")
	}
	if !(TaskRequirement{NeedsBrowser: true}).Matches(cap) {
		t.Fatal("expected browser requirement to match")
	}
}
