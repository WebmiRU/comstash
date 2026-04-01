package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultGitTimeout = 2 * time.Minute
	longGitTimeout    = 10 * time.Minute
)

func runGit(args ...string) error {
	return runGitWithTimeout(defaultGitTimeout, args...)
}

func runGitOutput(args ...string) (string, error) {
	return runGitOutputWithTimeout(defaultGitTimeout, args...)
}

func runGitWithTimeout(timeout time.Duration, args ...string) error {
	_, err := runGitCommand(timeout, args...)
	return err
}

func runGitOutputWithTimeout(timeout time.Duration, args ...string) (string, error) {
	return runGitCommand(timeout, args...)
}

func gitCloneMirror(sourceURL string, repoPath string) error {
	return runGitWithTimeout(longGitTimeout, "clone", "--mirror", sourceURL, repoPath)
}

func gitFetchMirror(repoPath string) error {
	return runGitWithTimeout(longGitTimeout, "-C", repoPath, "fetch", "--prune", "--tags", "origin")
}

func gitVerifyRef(repoPath string, ref string) error {
	_, err := runGitOutput("-C", repoPath, "rev-parse", "--verify", ref+"^{commit}")
	return err
}

func gitArchiveZip(repoPath string, ref string, zipPath string) error {
	return runGitWithTimeout(longGitTimeout, "-C", repoPath, "archive", "--format=zip", "--output", zipPath, ref)
}

func runGitCommand(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git %s timed out after %s", strings.Join(args, " "), timeout)
		}

		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
