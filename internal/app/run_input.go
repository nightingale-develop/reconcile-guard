package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

type runInput struct {
	Manifest  recording.RunManifest
	Versions  []upgrade.ClusterVersionObservation
	Histories [][]operator.Observation
}

func loadRunInput(directory string) (runInput, error) {
	if _, err := runInputPath(directory, "run.json"); err != nil {
		return runInput{}, err
	}

	manifest, err :=
		recording.ReadRunManifest(directory)
	if err != nil {
		return runInput{}, err
	}

	if manifest.Status != recording.RunStatusStopped {
		return runInput{}, fmt.Errorf("run must be stopped before verification (status %q)", manifest.Status)
	}
	versionPath, err := runInputPath(directory, manifest.Files.ClusterVersion)
	if err != nil {
		return runInput{}, err
	}

	versions, err :=
		upgrade.ReadHistory(versionPath)
	if err != nil {
		return runInput{}, err
	}

	observedClusterID := ""
	for _, observation := range versions {
		id := string(observation.ClusterVersion.Spec.ClusterID)
		if id == "" {
			continue
		}
		if observedClusterID != "" && observedClusterID != id {
			return runInput{}, fmt.Errorf("cluster ID changed within run history")
		}
		observedClusterID = id
	}
	if manifest.Source.ClusterID != observedClusterID {
		return runInput{}, fmt.Errorf("manifest cluster ID does not match ClusterVersion history")
	}
	operatorDirectory, err := runInputPath(directory, manifest.Files.OperatorsDirectory)
	if err != nil {
		return runInput{}, err
	}

	entries, err :=
		os.ReadDir(operatorDirectory)
	if err != nil {
		return runInput{}, err
	}

	expected := make(map[string]bool, len(manifest.Operators))
	for _, name := range manifest.Operators {
		expected[name] = true
	}

	histories := make(
		[][]operator.Observation,
		0,
		len(entries),
	)

	for _, entry := range entries {
		if entry.IsDir() ||
			filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !expected[name] {
			return runInput{}, fmt.Errorf("operator history %q is not listed in manifest", name)
		}
		path, err := runInputPath(directory, filepath.Join(manifest.Files.OperatorsDirectory, entry.Name()))
		if err != nil {
			return runInput{}, err
		}

		observations, err :=
			operator.ReadHistory(path)
		if err != nil {
			return runInput{}, fmt.Errorf("%s: %w", entry.Name(), err)
		}

		for _, observation := range observations {
			if observation.Operator.Name != name {
				return runInput{}, fmt.Errorf("operator history %q contains resource %q", name, observation.Operator.Name)
			}
		}
		delete(expected, name)

		histories = append(
			histories,
			observations,
		)
	}

	if len(expected) != 0 {
		return runInput{}, fmt.Errorf("run is missing operator histories listed in manifest")
	}

	if len(histories) == 0 {
		return runInput{}, fmt.Errorf("run contains no operator histories")
	}

	return runInput{Manifest: manifest, Versions: versions, Histories: histories}, nil
}

func runInputPath(directory, name string) (string, error) {
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return "", fmt.Errorf("invalid run input path %q", name)
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("run input path %q escapes run directory", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return "", fmt.Errorf("run input %q is not a regular file or directory", name)
	}
	return path, nil
}
