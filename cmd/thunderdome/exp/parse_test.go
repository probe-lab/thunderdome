package exp

import (
	"context"
	"strings"
	"testing"
)

func TestExamplesCorrectness(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "tweedles",
			json: tweedles,
		},
		{
			name: "test",
			json: testExperiment,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(context.Background(), strings.NewReader(tc.json))
			if err != nil {
				t.Errorf("experiment json invalid: %v", err)
			}
		})
	}
}
