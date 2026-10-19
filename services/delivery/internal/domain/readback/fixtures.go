package readback

import (
	"encoding/json"
	"fmt"
	"os"
)

type Fixture struct {
	Expected Expected `json:"expected"`
	Observation Observation `json:"observation"`
}

func LoadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil { return Fixture{}, err }
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil { return Fixture{}, err }
	return fixture, nil
}

func EvaluateFixture(path string) (Result, error) {
	fixture, err := LoadFixture(path)
	if err != nil { return Result{}, err }
	if fixture.Expected.Workspace == "" || fixture.Expected.List == "" || fixture.Expected.Title == "" || fixture.Expected.Assignee == "" {
		return Result{}, fmt.Errorf("fixture expected fields are incomplete")
	}
	return Compare(fixture.Expected, fixture.Observation), nil
}
