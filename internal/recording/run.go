package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

const RunSchemaVersion = "1"

type RunStatus string

const (
	RunStatusRecording RunStatus = "recording"
	RunStatusStopped   RunStatus = "stopped"
	RunStatusFailed    RunStatus = "failed"
)

type RunSource struct {
	Server    string `json:"server"`
	ClusterID string `json:"clusterId,omitempty"`
}

type RunFiles struct {
	ClusterVersion              string `json:"clusterVersion"`
	OperatorsDirectory          string `json:"operatorsDirectory"`
	MachineConfigPoolsDirectory string `json:"machineConfigPoolsDirectory,omitempty"`
	NodesDirectory              string `json:"nodesDirectory,omitempty"`
}

type RunManifest struct {
	SchemaVersion string     `json:"schemaVersion"`
	RunID         string     `json:"runId"`
	Status        RunStatus  `json:"status"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt,omitempty"`
	ToolVersion   string     `json:"toolVersion"`
	Command       string     `json:"command"`

	Source             RunSource `json:"source"`
	Files              RunFiles  `json:"files"`
	Operators          []string  `json:"operators,omitempty"`
	MachineConfigPools []string  `json:"machineConfigPools,omitempty"`
	Nodes              []string  `json:"nodes,omitempty"`

	Error string `json:"error,omitempty"`
}

type Run struct {
	directory string
	manifest  RunManifest
	mu        sync.Mutex
}

type RunSnapshot struct {
	ClusterID          string
	Operators          []string
	MachineConfigPools []string
	Nodes              []string
}

func StartRun(
	parentDirectory string,
	toolVersion string,
	server string,
	startedAt time.Time,
) (*Run, error) {
	return StartRunForCommand(parentDirectory, toolVersion, server, startedAt, "record-live")
}

func StartRunForCommand(parentDirectory, toolVersion, server string, startedAt time.Time, command string) (*Run, error) {
	if command != "record-live" && command != "observe-upgrade" {
		return nil, fmt.Errorf("unsupported recording command %q", command)
	}
	if parentDirectory == "" {
		return nil, fmt.Errorf(
			"run output directory is required",
		)
	}
	if startedAt.IsZero() {
		return nil, fmt.Errorf("startedAt is required")
	}
	if toolVersion == "" {
		return nil, fmt.Errorf("toolVersion is required")
	}

	startedAt = startedAt.UTC()

	runID := startedAt.Format(
		"20060102T150405.000000000Z",
	)

	directory := filepath.Join(
		parentDirectory,
		runID,
	)

	if err := os.MkdirAll(
		parentDirectory,
		0755,
	); err != nil {
		return nil, fmt.Errorf(
			"create run root: %w",
			err,
		)
	}

	if err := os.Mkdir(
		directory,
		0755,
	); err != nil {
		return nil, fmt.Errorf(
			"create run directory: %w",
			err,
		)
	}

	run := &Run{
		directory: directory,
		manifest: RunManifest{
			SchemaVersion: RunSchemaVersion,
			RunID:         runID,
			Status:        RunStatusRecording,
			StartedAt:     startedAt,
			ToolVersion:   toolVersion,
			Command:       command,
			Source: RunSource{
				Server: server,
			},
			Files: RunFiles{
				ClusterVersion:              "cluster-version.jsonl",
				OperatorsDirectory:          "operators",
				MachineConfigPoolsDirectory: "machine-config-pools",
				NodesDirectory:              "nodes",
			},
		},
	}

	if err := run.write(); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}

	return run, nil
}

func (r *Run) Directory() string {
	return r.directory
}

func (r *Run) Manifest() RunManifest {
	r.mu.Lock()
	defer r.mu.Unlock()

	manifest := r.manifest
	manifest.Operators = slices.Clone(manifest.Operators)
	manifest.MachineConfigPools = slices.Clone(manifest.MachineConfigPools)
	manifest.Nodes = slices.Clone(manifest.Nodes)
	if manifest.EndedAt != nil {
		endedAt := *manifest.EndedAt
		manifest.EndedAt = &endedAt
	}
	return manifest
}

func (r *Run) Finish(
	status RunStatus,
	endedAt time.Time,
	snapshot RunSnapshot,
	runErr error,
) error {
	if status != RunStatusStopped &&
		status != RunStatusFailed {
		return fmt.Errorf(
			"invalid final run status %q",
			status,
		)
	}

	endedAt = endedAt.UTC()

	operators := append(
		[]string(nil),
		snapshot.Operators...,
	)

	sort.Strings(operators)

	machineConfigPools := append(
		[]string(nil),
		snapshot.MachineConfigPools...,
	)

	nodes := append(
		[]string(nil),
		snapshot.Nodes...,
	)

	sort.Strings(machineConfigPools)
	sort.Strings(nodes)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.manifest.Status != RunStatusRecording {
		return fmt.Errorf("run is already finalized")
	}
	if endedAt.IsZero() || endedAt.Before(r.manifest.StartedAt) {
		return fmt.Errorf("endedAt must not be zero or before startedAt")
	}
	if (status == RunStatusFailed) != (runErr != nil) {
		return fmt.Errorf("final status and run error are inconsistent")
	}

	previous := r.manifest
	r.manifest.Status = status
	r.manifest.EndedAt = &endedAt
	r.manifest.Source.ClusterID =
		snapshot.ClusterID
	r.manifest.Operators = operators

	r.manifest.MachineConfigPools = machineConfigPools
	r.manifest.Nodes = nodes

	if runErr != nil {
		r.manifest.Error = runErr.Error()
	} else {
		r.manifest.Error = ""
	}

	if err := validateRunManifest(r.manifest); err != nil {
		r.manifest = previous
		return err
	}
	if err := r.writeLocked(); err != nil {
		r.manifest = previous
		return err
	}
	return nil
}

func (r *Run) write() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.writeLocked()
}

func (r *Run) writeLocked() error {
	data, err := json.MarshalIndent(
		r.manifest,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"encode run manifest: %w",
			err,
		)
	}

	data = append(data, '\n')

	path := filepath.Join(
		r.directory,
		"run.json",
	)

	temp := path + ".tmp"

	if err := os.WriteFile(
		temp,
		data,
		0644,
	); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf(
			"write run manifest: %w",
			err,
		)
	}

	if err := os.Rename(
		temp,
		path,
	); err != nil {
		_ = os.Remove(temp)

		return fmt.Errorf(
			"replace run manifest: %w",
			err,
		)
	}

	return nil
}

func ReadRunManifest(
	directory string,
) (RunManifest, error) {
	path := filepath.Join(
		directory,
		"run.json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		return RunManifest{}, fmt.Errorf(
			"read run manifest: %w",
			err,
		)
	}

	var manifest RunManifest

	if err := json.Unmarshal(
		data,
		&manifest,
	); err != nil {
		return RunManifest{}, fmt.Errorf(
			"decode run manifest: %w",
			err,
		)
	}

	if err := validateRunManifest(manifest); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func validateRunManifest(manifest RunManifest) error {
	if manifest.SchemaVersion !=
		RunSchemaVersion {
		return fmt.Errorf(
			"unsupported run schema version %q",
			manifest.SchemaVersion,
		)
	}

	if manifest.RunID == "" {
		return fmt.Errorf(
			"runId is missing",
		)
	}

	if manifest.StartedAt.IsZero() || manifest.ToolVersion == "" || (manifest.Command != "record-live" && manifest.Command != "observe-upgrade") {
		return fmt.Errorf("run manifest requires startedAt, toolVersion and command record-live or observe-upgrade")
	}
	switch manifest.Status {
	case RunStatusRecording:
		if manifest.EndedAt != nil || manifest.Error != "" {
			return fmt.Errorf("recording run has finalization fields")
		}
	case RunStatusStopped, RunStatusFailed:
		if manifest.EndedAt == nil || manifest.EndedAt.IsZero() || manifest.EndedAt.Before(manifest.StartedAt) {
			return fmt.Errorf("endedAt must not be missing or before startedAt")
		}
		if (manifest.Status == RunStatusFailed) != (manifest.Error != "") {
			return fmt.Errorf("run status and error are inconsistent")
		}
	default:
		return fmt.Errorf("invalid run status %q", manifest.Status)
	}
	paths := []string{
		manifest.Files.ClusterVersion,
		manifest.Files.OperatorsDirectory,
	}

	if manifest.Files.MachineConfigPoolsDirectory != "" {
		paths = append(
			paths,
			manifest.Files.MachineConfigPoolsDirectory,
		)
	}

	if manifest.Files.NodesDirectory != "" {
		paths = append(
			paths,
			manifest.Files.NodesDirectory,
		)
	}

	for _, path := range paths {
		if !filepath.IsLocal(path) ||
			filepath.Clean(path) == "." {
			return fmt.Errorf(
				"run file path must be relative and stay inside the run directory: %q",
				path,
			)
		}
	}
	seen := make(map[string]bool, len(manifest.Operators))
	for _, name := range manifest.Operators {
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
			return fmt.Errorf("invalid operator name %q in run manifest", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate operator %q in run manifest", name)
		}
		seen[name] = true
	}

	for label, names := range map[string][]string{
		"MachineConfigPool": manifest.MachineConfigPools,
		"Node":              manifest.Nodes,
	} {
		seen := make(map[string]bool, len(names))

		for _, name := range names {
			if name == "" ||
				name == "." ||
				name == ".." ||
				filepath.Base(name) != name {
				return fmt.Errorf(
					"invalid %s name %q in run manifest",
					label,
					name,
				)
			}

			if seen[name] {
				return fmt.Errorf(
					"duplicate %s %q in run manifest",
					label,
					name,
				)
			}

			seen[name] = true
		}
	}
	return nil
}
