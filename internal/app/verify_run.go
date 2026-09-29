package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nightingale-develop/reconcile-guard/internal/contracts"
	"github.com/nightingale-develop/reconcile-guard/internal/operator"
	"github.com/nightingale-develop/reconcile-guard/internal/recording"
	"github.com/nightingale-develop/reconcile-guard/internal/upgrade"
)

func (c cli) verifyRun(
	args []string,
) int {
	if len(args) != 2 {
		fmt.Fprintln(
			c.stderr,
			"Usage: reconcile-guard verify-run <run-directory>",
		)
		return 1
	}

	directory := args[1]
	if _, err := runInputPath(directory, "run.json"); err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	manifest, err :=
		recording.ReadRunManifest(directory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if manifest.Status != recording.RunStatusStopped {
		fmt.Fprintf(c.stderr, "Error: run must be stopped before verification (status %q)\n", manifest.Status)
		return 1
	}
	versionPath, err := runInputPath(directory, manifest.Files.ClusterVersion)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	versions, err :=
		upgrade.ReadHistory(versionPath)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	observedClusterID := ""
	for _, observation := range versions {
		id := string(observation.ClusterVersion.Spec.ClusterID)
		if id == "" {
			continue
		}
		if observedClusterID != "" && observedClusterID != id {
			fmt.Fprintln(c.stderr, "Error: cluster ID changed within run history")
			return 1
		}
		observedClusterID = id
	}
	if manifest.Source.ClusterID != observedClusterID {
		fmt.Fprintln(c.stderr, "Error: manifest cluster ID does not match ClusterVersion history")
		return 1
	}
	operatorDirectory, err := runInputPath(directory, manifest.Files.OperatorsDirectory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	entries, err :=
		os.ReadDir(operatorDirectory)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
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
			fmt.Fprintf(c.stderr, "Error: operator history %q is not listed in manifest\n", name)
			return 1
		}
		path, err := runInputPath(directory, filepath.Join(manifest.Files.OperatorsDirectory, entry.Name()))
		if err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}

		observations, err :=
			operator.ReadHistory(path)
		if err != nil {
			fmt.Fprintf(
				c.stderr,
				"Error: %s: %v\n",
				entry.Name(),
				err,
			)

			return 1
		}

		for _, observation := range observations {
			if observation.Operator.Name != name {
				fmt.Fprintf(c.stderr, "Error: operator history %q contains resource %q\n", name, observation.Operator.Name)
				return 1
			}
		}
		delete(expected, name)

		histories = append(
			histories,
			observations,
		)
	}

	if len(expected) != 0 {
		fmt.Fprintln(c.stderr, "Error: run is missing operator histories listed in manifest")
		return 1
	}

	if len(histories) == 0 {
		fmt.Fprintln(
			c.stderr,
			"Error: run contains no operator histories",
		)
		return 1
	}

	report, err :=
		contracts.VerifyClusterUpgrade(
			versions,
			histories,
		)
	if err != nil {
		fmt.Fprintln(c.stderr, "Error:", err)
		return 1
	}

	if c.output == outputJSON {
		if err := c.writeJSON(
			"verify-run",
			contracts.ClusterUpgradeResult(report),
		); err != nil {
			fmt.Fprintln(c.stderr, "Error:", err)
			return 1
		}
	} else {
		fmt.Fprintln(
			c.stdout,
			"Run:",
			manifest.RunID,
		)

		fmt.Fprintln(
			c.stdout,
			"Cluster ID:",
			manifest.Source.ClusterID,
		)

		c.printClusterUpgradeReport(report)
	}

	return contractExitCode(report.Verdict)
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
