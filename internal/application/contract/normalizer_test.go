package contract

import "testing"

func TestNormalizeRequest_TTS(t *testing.T) {
	c, err := NormalizeRequest("TTS実装して")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.Goal != "TTS実装して" {
		t.Fatalf("unexpected goal: %q", c.Goal)
	}
	if len(c.Acceptance) < 3 {
		t.Fatalf("expected tts acceptance criteria, got %v", c.Acceptance)
	}
}

func TestNormalizeRequest_Empty(t *testing.T) {
	_, err := NormalizeRequest("   ")
	if err == nil {
		t.Fatal("expected error for empty request")
	}
}

func TestNormalizeRequest_Generic(t *testing.T) {
	c, err := NormalizeRequest("ログ確認して")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.Goal != "ログ確認して" {
		t.Fatalf("unexpected goal: %q", c.Goal)
	}
	if len(c.Acceptance) == 0 || len(c.Verification) == 0 {
		t.Fatalf("generic contract is incomplete: %+v", c)
	}
}
