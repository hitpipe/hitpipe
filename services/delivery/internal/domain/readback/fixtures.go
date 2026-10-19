package readback

import (
	"encoding/json"
	"fmt"
)

type Fixture struct {
	Expected Expected `json:"expected"`
	Observation Observation `json:"observation"`
}

func ParseFixture(data []byte) (Fixture, error) {
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func EvaluateFixture(data []byte) (Result, error) {
	fixture, err := ParseFixture(data)
	if err != nil {
		return Result{}, err
	}
	if fixture.Expected.Workspace == "" || fixture.Expected.List == "" || fixture.Expected.Title == "" || fixture.Expected.Assignee == "" {
		return Result{}, fmt.Errorf("fixture expected fields are incomplete")
	}
	return Compare(fixture.Expected, fixture.Observation), nil
}
