package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
)

func historyObservation(
	minute int,
	conditions ...configv1.ClusterOperatorStatusCondition,
) Observation {
	return Observation{
		ObservedAt: time.Date(
			2026,
			9,
			19,
			10,
			minute,
			0,
			0,
			time.UTC,
		),
		Operator: testOperator(conditions...),
	}
}

func TestAnalyzeHistory(t *testing.T) {
	steady := []configv1.ClusterOperatorStatusCondition{
		{
			Type:   configv1.OperatorAvailable,
			Status: configv1.ConditionTrue,
		},
		{
			Type:   configv1.OperatorProgressing,
			Status: configv1.ConditionFalse,
		},
		{
			Type:   configv1.OperatorDegraded,
			Status: configv1.ConditionFalse,
		},
	}

	progressing := []configv1.ClusterOperatorStatusCondition{
		{
			Type:   configv1.OperatorAvailable,
			Status: configv1.ConditionTrue,
		},
		{
			Type:   configv1.OperatorProgressing,
			Status: configv1.ConditionTrue,
		},
		{
			Type:   configv1.OperatorDegraded,
			Status: configv1.ConditionFalse,
		},
	}

	t.Run(
		"reports changes only between adjacent observations",
		func(t *testing.T) {
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
				t.Fatalf(
					"transitions = %d, want 2",
					len(got.Transitions),
				)
			}

			first := got.Transitions[0]

			if first.Condition != configv1.OperatorProgressing ||
				first.From != configv1.ConditionFalse ||
				first.To != configv1.ConditionTrue ||
				!first.FromTime.Equal(
					historyObservation(0).ObservedAt,
				) ||
				!first.ToTime.Equal(
					historyObservation(5).ObservedAt,
				) {
				t.Errorf(
					"unexpected first transition: %+v",
					first,
				)
			}

			second := got.Transitions[1]

			if second.Condition != configv1.OperatorProgressing ||
				second.From != configv1.ConditionTrue ||
				second.To != configv1.ConditionFalse {
				t.Errorf(
					"unexpected second transition: %+v",
					second,
				)
			}
		},
	)

	t.Run(
		"unchanged reports do not create transitions",
		func(t *testing.T) {
			got, err := analyzeHistory([]Observation{
				historyObservation(0, steady...),
				historyObservation(5, steady...),
			})

			if err != nil {
				t.Fatal(err)
			}

			if len(got.Transitions) != 0 ||
				got.UncomparedPairs != 0 {
				t.Fatalf("unexpected report: %+v", got)
			}
		},
	)

	t.Run(
		"missing condition does not get bridged",
		func(t *testing.T) {
			got, err := analyzeHistory([]Observation{
				historyObservation(0, steady...),
				historyObservation(
					5,
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorDegraded,
						Status: configv1.ConditionFalse,
					},
				),
				historyObservation(10, progressing...),
			})

			if err != nil {
				t.Fatal(err)
			}

			if len(got.Transitions) != 0 ||
				got.UncomparedPairs != 2 {
				t.Fatalf("unexpected report: %+v", got)
			}
		},
	)

	t.Run(
		"Unknown is a reported status, not a verdict",
		func(t *testing.T) {
			got, err := analyzeHistory([]Observation{
				historyObservation(
					0,
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionUnknown,
					},
				),
				historyObservation(
					5,
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionTrue,
					},
				),
			})

			if err != nil {
				t.Fatal(err)
			}

			if len(got.Transitions) != 1 ||
				got.Transitions[0].From !=
					configv1.ConditionUnknown ||
				got.Transitions[0].To !=
					configv1.ConditionTrue ||
				got.UncomparedPairs != 2 {
				t.Fatalf("unexpected report: %+v", got)
			}
		},
	)

	invalid := []struct {
		name         string
		observations []Observation
		want         string
	}{
		{
			name:         "empty",
			observations: nil,
			want:         "no observations",
		},
		{
			name: "missing timestamp",
			observations: []Observation{
				{
					Operator: testOperator(steady...),
				},
			},
			want: "observedAt is missing",
		},
		{
			name: "backward time",
			observations: []Observation{
				historyObservation(5, steady...),
				historyObservation(0, steady...),
			},
			want: "must be later",
		},
		{
			name: "same time",
			observations: []Observation{
				historyObservation(0, steady...),
				historyObservation(0, steady...),
			},
			want: "must be later",
		},
		{
			name: "different operator",
			observations: func() []Observation {
				a := historyObservation(5, steady...)
				a.Operator.Name = "network"

				return []Observation{
					historyObservation(0, steady...),
					a,
				}
			}(),
			want: "operator changed",
		},
		{
			name: "duplicate Available",
			observations: []Observation{
				historyObservation(
					0,
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionFalse,
					},
				),
			},
			want: "duplicate Available",
		},
		{
			name: "invalid Progressing",
			observations: []Observation{
				historyObservation(
					0,
					configv1.ClusterOperatorStatusCondition{
						Type: configv1.OperatorProgressing,
						Status: configv1.ConditionStatus(
							"Broken",
						),
					},
				),
			},
			want: "invalid Progressing status",
		},
		{
			name: "invalid ClusterOperator",
			observations: []Observation{
				func() Observation {
					a := historyObservation(
						0,
						steady...,
					)
					a.Operator.Kind = "Pod"

					return a
				}(),
			},
			want: "unsupported resource",
		},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := analyzeHistory(tc.observations)

			if err == nil ||
				!strings.Contains(err.Error(), tc.want) {
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
			path := filepath.Join(
				t.TempDir(),
				"history.jsonl",
			)

			if err := os.WriteFile(
				path,
				[]byte(tc.text),
				0600,
			); err != nil {
				t.Fatal(err)
			}

			got, err := readHistory(path)

			if tc.wantError != "" {
				if err == nil ||
					!strings.Contains(
						err.Error(),
						tc.wantError,
					) {
					t.Fatalf(
						"error = %v, want %q",
						err,
						tc.wantError,
					)
				}

				return
			}

			if err != nil ||
				len(got) != tc.wantCount {
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
