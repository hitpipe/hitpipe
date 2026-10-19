package readback

import "testing"

func TestEvaluateFixtureRejectsMissingFile(t *testing.T) {
	if _, err := EvaluateFixture([]byte("{")); err == nil {
		t.Fatal("expected malformed fixture error")
	}
}
