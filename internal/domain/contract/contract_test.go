package contract

import "testing"

func TestContractValidate(t *testing.T) {
	valid := Contract{
		Goal:         "TTSを導入して動作確認する",
		Acceptance:   []string{"音声生成に成功する"},
		Constraints:  []string{"破壊的操作は禁止"},
		Artifacts:    []string{"generated_audio.wav"},
		Verification: []string{"再生テストに成功する"},
		Rollback:     []string{"設定変更を戻す"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid contract, got err=%v", err)
	}

	invalid := valid
	invalid.Goal = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected validation error for empty goal")
	}
}
