package readback

import "testing"

func TestEvaluateFixtureRejectsMissingFile(t *testing.T) {
	if _, err := EvaluateFixture("missing-fixture.json"); err == nil { t.Fatal("expected missing fixture error") }
}
