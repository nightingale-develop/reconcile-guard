package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func historyObservation(minute int, conditions ...Condition) Observation {
	return Observation{
		ObservedAt: time.Date(2026, 9, 19, 10, minute, 0, 0, time.UTC),
		Operator:   testOperator(conditions...),
	}
}

func TestAnalyzeHistory(t *testing.T) {
	steady := []Condition{
		{Type: "Available", Status: "True"},
		{Type: "Progressing", Status: "False"},
		{Type: "Degraded", Status: "False"},
	}

	progressing := []Condition{
		{Type: "Available", Status: "True"},
		{Type: "Progressing", Status: "True"},
		{Type: "Degraded", Status: "False"},
	}

	t.Run("reports changes only between adjacent observations", func(t *testing.T) {
		got, err := analyzeHistory([]Observation{
			historyObservation(0, steady...),
			historyObservation(5, progressing...),
			historyObservation(10, steady...),
		})

		if err != nil {
			t.Fatal(err)
		}

		if got.Operator != "ingress" ||
			got.Observations != 3 ||
			got.UncomparedPairs != 0 {
			t.Fatalf("unexpected summary: %+v", got)
		}

		if len(got.Transitions) != 2 {
			t.Fatalf("transitions = %d, want 2", len(got.Transitions))
		}

		first := got.Transitions[0]

		if first.Condition != "Progressing" ||
			first.From != "False" ||
			first.To != "True" ||
			!first.FromTime.Equal(historyObservation(0).ObservedAt) ||
			!first.ToTime.Equal(historyObservation(5).ObservedAt) {
			t.Errorf("unexpected first transition: %+v", first)
		}

		second := got.Transitions[1]

		if second.Condition != "Progressing" ||
			second.From != "True" ||
			second.To != "False" {
			t.Errorf("unexpected second transition: %+v", second)
		}
	})

	t.Run("unchanged reports do not create transitions", func(t *testing.T) {
		got, err := analyzeHistory([]Observation{
			historyObservation(0, steady...),
			historyObservation(5, steady...),
		})

		if err != nil {
			t.Fatal(err)
		}

		if len(got.Transitions) != 0 || got.UncomparedPairs != 0 {
			t.Fatalf("unexpected report: %+v", got)
		}
	})

	t.Run("missing condition does not get bridged", func(t *testing.T) {
		got, err := analyzeHistory([]Observation{
			historyObservation(0, steady...),
			historyObservation(5,
				Condition{Type: "Available", Status: "True"},
				Condition{Type: "Degraded", Status: "False"},
			),
			historyObservation(10, progressing...),
		})

		if err != nil {
			t.Fatal(err)
		}

		if len(got.Transitions) != 0 || got.UncomparedPairs != 2 {
			t.Fatalf("unexpected report: %+v", got)
		}
	})

	t.Run("Unknown is a reported status, not a verdict", func(t *testing.T) {
		got, err := analyzeHistory([]Observation{
			historyObservation(0,
				Condition{Type: "Progressing", Status: "Unknown"},
			),
			historyObservation(5,
				Condition{Type: "Progressing", Status: "True"},
			),
		})

		if err != nil {
			t.Fatal(err)
		}

		if len(got.Transitions) != 1 ||
			got.Transitions[0].From != "Unknown" ||
			got.Transitions[0].To != "True" ||
			got.UncomparedPairs != 2 {
			t.Fatalf("unexpected report: %+v", got)
		}
	})

	invalid := []struct {
		name         string
		observations []Observation
		want         string
	}{
		{
			"empty",
			nil,
			"no observations",
		},
		{
			"missing timestamp",
			[]Observation{{Operator: testOperator(steady...)}},
			"observedAt is missing",
		},
		{
			"backward time",
			[]Observation{
				historyObservation(5, steady...),
				historyObservation(0, steady...),
			},
			"must be later",
		},
		{
			"same time",
			[]Observation{
				historyObservation(0, steady...),
				historyObservation(0, steady...),
			},
			"must be later",
		},
		{
			"different operator",
			func() []Observation {
				a := historyObservation(5, steady...)
				a.Operator.Metadata.Name = "network"

				return []Observation{
					historyObservation(0, steady...),
					a,
				}
			}(),
			"operator changed",
		},
		{
			"duplicate Available",
			[]Observation{
				historyObservation(0,
					Condition{Type: "Available", Status: "True"},
					Condition{Type: "Available", Status: "False"},
				),
			},
			"duplicate Available",
		},
		{
			"invalid Progressing",
			[]Observation{
				historyObservation(0,
					Condition{Type: "Progressing", Status: "Broken"},
				),
			},
			"invalid Progressing status",
		},
		{
			"invalid ClusterOperator",
			[]Observation{
				func() Observation {
					a := historyObservation(0, steady...)
					a.Operator.Kind = "Pod"
					return a
				}(),
			},
			"unsupported resource",
		},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := analyzeHistory(tc.observations)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf(
					"error = %v, want a message containing %q",
					err,
					tc.want,
				)
			}
		})
	}
}

func TestReadHistory(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantCount int
		wantError string
	}{
		{
			name: "two observations separated by a blank line",
			text: `{"observedAt":"2026-09-19T10:00:00Z","operator":{}}

{"observedAt":"2026-09-19T10:05:00Z","operator":{}}
`,
			wantCount: 2,
		},
		{
			name:      "invalid JSON reports physical line",
			text:      "\n{invalid\n",
			wantError: "line 2: decode JSON",
		},
		{
			name:      "invalid timestamp",
			text:      `{"observedAt":"yesterday","operator":{}}`,
			wantError: "decode JSON",
		},
		{
			name:      "empty",
			text:      " \n\n",
			wantError: "no observations",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")

			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}

			got, err := readHistory(path)

			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}

				return
			}

			if err != nil || len(got) != tc.wantCount {
				t.Fatalf(
					"got %d observations, error %v; want %d",
					len(got),
					err,
					tc.wantCount,
				)
			}
		})
	}
}
