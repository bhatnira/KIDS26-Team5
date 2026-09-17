package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// SlurmDispatcher implements Scheduler using SSH + sbatch/squeue/sacct.
type SlurmDispatcher struct {
	cfg setting.SlurmConfig
}

// NewSlurmDispatcher creates a new Slurm dispatcher from config.
func NewSlurmDispatcher(cfg setting.SlurmConfig) *SlurmDispatcher {
	cfg.Defaults()
	return &SlurmDispatcher{cfg: cfg}
}

// Name returns the scheduler type.
func (s *SlurmDispatcher) Name() string {
	return "slurm"
}

// sshClient establishes an SSH connection to the Slurm cluster.
func (s *SlurmDispatcher) sshClient() (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: s.cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: use known hosts in production
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	return ssh.Dial("tcp", addr, config)
}

// runCommand executes a command over SSH and returns stdout.
func (s *SlurmDispatcher) runCommand(cmd string) (string, error) {
	client, err := s.sshClient()
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// Dispatch submits a job via sbatch.
func (s *SlurmDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResponse, error) {
	// Build the sbatch script
	script := s.buildBatchScript(req)

	// Write the script to a temp file and submit via stdin
	cmd := fmt.Sprintf("cat << 'SBATCH_EOF' | sbatch\n%s\nSBATCH_EOF", script)

	output, err := s.runCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("sbatch failed: %w", err)
	}

	// Parse "Submitted batch job 12345"
	jobID := strings.TrimSpace(strings.TrimPrefix(output, "Submitted batch job "))
	if jobID == "" {
		return nil, fmt.Errorf("unexpected sbatch output: %s", output)
	}

	log.L().Info("slurm job dispatched",
		zap.String("job_id", req.JobID),
		zap.String("slurm_id", jobID))

	return &DispatchResponse{DispatchedID: jobID}, nil
}

// buildBatchScript generates a Slurm batch script from the dispatch request.
func (s *SlurmDispatcher) buildBatchScript(req DispatchRequest) string {
	var b strings.Builder

	// Slurm directives
	b.WriteString("#!/bin/bash\n")
	b.WriteString(fmt.Sprintf("#SBATCH --job-name=%s\n", req.JobID))
	b.WriteString(fmt.Sprintf("#SBATCH --partition=%s\n", s.cfg.Partition))
	if s.cfg.Account != "" {
		b.WriteString(fmt.Sprintf("#SBATCH --account=%s\n", s.cfg.Account))
	}
	b.WriteString(fmt.Sprintf("#SBATCH --time=%s\n", s.cfg.TimeLimit))
	b.WriteString(fmt.Sprintf("#SBATCH --cpus-per-task=%d\n", s.cfg.Cores))
	b.WriteString(fmt.Sprintf("#SBATCH --mem=%dM\n", s.cfg.Memory))
	b.WriteString(fmt.Sprintf("#SBATCH --output=%s/%s-%%j.out\n", s.cfg.WorkDir, req.JobID))
	b.WriteString(fmt.Sprintf("#SBATCH --error=%s/%s-%%j.err\n", s.cfg.WorkDir, req.JobID))

	if s.cfg.QoS != "" {
		b.WriteString(fmt.Sprintf("#SBATCH --qos=%s\n", s.cfg.QoS))
	}

	b.WriteString("\n")

	// Environment setup
	b.WriteString("set -euo pipefail\n")
	b.WriteString(fmt.Sprintf("export NXF_OPTS=\"-Xms512m -Xmx2g\"\n"))
	b.WriteString(fmt.Sprintf("export NXF_WORK=%s/work\n", s.cfg.WorkDir))
	b.WriteString("\n")

	// Download Nextflow if not in PATH
	b.WriteString("if ! command -v nextflow &> /dev/null; then\n")
	b.WriteString(fmt.Sprintf("  curl -s https://get.nextflow.io | bash -s -- -d %s/bin\n", s.cfg.WorkDir))
	b.WriteString(fmt.Sprintf("  export PATH=%s/bin:$PATH\n", s.cfg.WorkDir))
	b.WriteString("fi\n")
	b.WriteString("\n")

	// Write dispatch payload to file
	b.WriteString(fmt.Sprintf("PAYLOAD_FILE=%s/%s-payload.json\n", s.cfg.WorkDir, req.JobID))
	b.WriteString(fmt.Sprintf("cat > \"$PAYLOAD_FILE\" << 'PAYLOAD_EOF'\n"))
	b.Write(req.Payload)
	b.WriteString("\nPAYLOAD_EOF\n")
	b.WriteString("\n")

	// Extract payload fields and run Nextflow
	b.WriteString("REPO=$(jq -r '.repository // empty' \"$PAYLOAD_FILE\")\n")
	b.WriteString("REV=$(jq -r '.revision // empty' \"$PAYLOAD_FILE\")\n")
	b.WriteString("PARAMS_FILE=$(mktemp)\n")
	b.WriteString("jq -r '.params // {}' \"$PAYLOAD_FILE\" > \"$PARAMS_FILE\"\n")
	b.WriteString("\n")

	b.WriteString("set -- nextflow run \"$REPO\"\n")
	b.WriteString("[ -n \"$REV\" ] && set -- \"$@\" -r \"$REV\"\n")
	b.WriteString("set -- \"$@\" -profile docker -work-dir \"$NXF_WORK\"\n")
	b.WriteString("set -- \"$@\" --outdir results\n")
	b.WriteString("[ -s \"$PARAMS_FILE\" ] && [ \"$(jq 'length' \"$PARAMS_FILE\")\" != \"0\" ] && set -- \"$@\" -params-file \"$PARAMS_FILE\"\n")
	b.WriteString("\n")
	b.WriteString("echo \"Executing: $@\"\n")
	b.WriteString("exec \"$@\"\n")

	return b.String()
}

// Deregister cancels a Slurm job.
func (s *SlurmDispatcher) Deregister(ctx context.Context, jobID string) error {
	cmd := fmt.Sprintf("scancel %s", jobID)
	_, err := s.runCommand(cmd)
	if err != nil {
		return fmt.Errorf("scancel failed: %w", err)
	}

	log.L().Info("slurm job cancelled", zap.String("slurm_id", jobID))
	return nil
}

// GetStatus queries the job status via sacct.
func (s *SlurmDispatcher) GetStatus(ctx context.Context, jobID string) (JobStatus, error) {
	cmd := fmt.Sprintf("sacct -j %s --format=State --noheader --parsable2", jobID)
	output, err := s.runCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("sacct failed: %w", err)
	}

	// sacct returns one line per task; take the first non-empty line
	for _, line := range strings.Split(output, "\n") {
		state := strings.TrimSpace(line)
		if state != "" {
			return mapSlurmState(state), nil
		}
	}

	return JobStatusPending, nil
}

// mapSlurmState converts Slurm state strings to our JobStatus.
func mapSlurmState(state string) JobStatus {
	switch strings.ToUpper(state) {
	case "PENDING", "PD":
		return JobStatusPending
	case "RUNNING", "R":
		return JobStatusRunning
	case "COMPLETED", "CD":
		return JobStatusCompleted
	case "FAILED", "F", "TIMEOUT", "NODE_FAIL", "OUT_OF_MEMORY", "DEADLINE":
		return JobStatusFailed
	case "CANCELLED", "CA", "CANCELLED+", "PREEMPTED", "PRF":
		return JobStatusCancelled
	case "SUSPENDED", "S":
		return JobStatusPending // treat suspended as pending
	default:
		return JobStatusPending
	}
}

// StreamLogs reads the job's stdout/stderr file via SSH.
func (s *SlurmDispatcher) StreamLogs(ctx context.Context, jobID string, logType LogType) (LogStream, error) {
	// Determine the log file path
	var suffix string
	switch logType {
	case LogTypeStderr:
		suffix = "err"
	default:
		suffix = "out"
	}

	// Try to find the log file (Slurm appends job ID to the pattern)
	logFile := fmt.Sprintf("%s/%s-*.%s", s.cfg.WorkDir, jobID, suffix)

	client, err := s.sshClient()
	if err != nil {
		return nil, fmt.Errorf("ssh connect: %w", err)
	}

	// Use a pipe to stream the output
	pr, pw := io.Pipe()

	go func() {
		defer client.Close()
		defer pw.Close()

		// First, wait for the file to exist and tail it
		session, err := client.NewSession()
		if err != nil {
			pw.CloseWithError(fmt.Errorf("ssh session: %w", err))
			return
		}
		defer session.Close()

		// tail -f the log file, retrying if it doesn't exist yet
		cmd := fmt.Sprintf("for i in $(seq 1 30); do [ -f %s ] && break; sleep 2; done; tail -n +1 -f %s 2>/dev/null || true", logFile, logFile)

		session.Stdout = pw
		session.Stderr = pw

		if err := session.Run(cmd); err != nil {
			// Ignore errors from tail when file doesn't exist
			log.L().Warn("slurm log stream ended", zap.String("job_id", jobID), zap.Error(err))
		}
	}()

	return pr, nil
}

// HealthCheck verifies SSH connectivity to the Slurm cluster.
func (s *SlurmDispatcher) HealthCheck(ctx context.Context) error {
	client, err := s.sshClient()
	if err != nil {
		return fmt.Errorf("slurm ssh health check failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("slurm ssh session failed: %w", err)
	}
	defer session.Close()

	// Simple command to verify Slurm is available
	if err := session.Run("sinfo --version 2>/dev/null || echo ok"); err != nil {
		return fmt.Errorf("slurm sinfo check failed: %w", err)
	}

	return nil
}
