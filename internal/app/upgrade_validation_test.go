package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/regression"
	"github.com/nightingale-develop/reconcile-guard/internal/result"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func TestRecordedUpgradeRunPipeline(t *testing.T) {
	versions, err := upgrade.ReadHistory("../../testdata/upgrade/cluster-version.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	operators, err := operator.ReadHistory("../../testdata/upgrade/ingress.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	run, err := recording.StartRun(t.TempDir(), "synthetic", "https://synthetic.invalid", versions[0].ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := recording.NewRunRecorder(run.Directory())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		if err := sink.AppendClusterVersion(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range operators {
		if err := sink.AppendOperator(o); err != nil {
			t.Fatal(err)
		}
	}
	if err := run.Finish(recording.RunStatusStopped, versions[len(versions)-1].ObservedAt.Add(time.Second), sink.Snapshot(), nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRunInput(run.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Versions, versions) || len(loaded.Histories) != 1 || !reflect.DeepEqual(loaded.Histories[0], operators) {
		t.Fatal("JSONL round-trip lost upgrade fields")
	}
	if loaded.Manifest.Source.ClusterID != string(versions[0].ClusterVersion.Spec.ClusterID) || loaded.Manifest.Status != recording.RunStatusStopped {
		t.Fatalf("manifest=%+v", loaded.Manifest)
	}
	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		if code := Run([]string{"verify-run", run.Directory(), "--output=" + format}, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("verify %s: code=%d out=%s err=%s", format, code, &out, &stderr)
		}
		if format == "json" {
			var doc result.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Result.Verdict != result.VerdictPass {
				t.Fatalf("result=%+v", doc.Result)
			}
		}
		out.Reset()
		if code := Run([]string{"compare-runs", run.Directory(), run.Directory(), "--output=" + format}, &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("compare %s: code=%d out=%s err=%s", format, code, &out, &stderr)
		}
		if format == "json" {
			var doc regression.Document
			if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Comparison.Verdict != result.VerdictPass || doc.Comparison.Candidate.VerificationVerdict != result.VerdictPass {
				t.Fatalf("comparison=%+v", doc.Comparison)
			}
		}
	}

	path := filepath.Join(run.Directory(), "operators/ingress.jsonl")
	data, err := json.Marshal(operators[0])
	if err != nil {
		t.Fatal(err)
	}
	writeRunTestFile(t, path, append(append(data, '\n'), append(data, '\n')...))
	for _, args := range [][]string{{"verify-run", run.Directory()}, {"compare-runs", run.Directory(), run.Directory()}} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 1 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid history accepted: %d %s %s", code, &out, &stderr)
		}
	}
}
