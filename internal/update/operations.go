package update

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type GitState struct {
	Branch, Commit string
	Dirty          bool
}

func State(ctx context.Context, repository string) (GitState, error) {
	branch, err := git(ctx, repository, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return GitState{}, err
	}
	commit, err := git(ctx, repository, "rev-parse", "HEAD")
	if err != nil {
		return GitState{}, err
	}
	status, err := git(ctx, repository, "status", "--porcelain")
	if err != nil {
		return GitState{}, err
	}
	return GitState{Branch: branch, Commit: commit, Dirty: strings.TrimSpace(status) != ""}, nil
}

func Fetch(ctx context.Context, repository string) error {
	_, err := git(ctx, repository, "fetch", "--prune", "origin")
	return err
}

func History(ctx context.Context, repository string, limit int) ([]string, error) {
	if limit < 1 {
		limit = 20
	}
	output, err := git(ctx, repository, "log", fmt.Sprintf("-%d", limit), "--format=%H %s")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

// InstallCommit deliberately requires a clean checkout and updates in detached
// HEAD. Callers should run it only after presenting the explicit confirmation
// and dirty-tree warning required by the Delbysoft interaction contract.
func InstallCommit(ctx context.Context, repository, commit string) error {
	state, err := State(ctx, repository)
	if err != nil {
		return err
	}
	if state.Dirty {
		return fmt.Errorf("refusing update with uncommitted changes")
	}
	if err := Fetch(ctx, repository); err != nil {
		return err
	}
	_, err = git(ctx, repository, "checkout", "--detach", commit)
	return err
}

func DetachedCommand(repository, commit string) (*exec.Cmd, error) {
	if strings.TrimSpace(repository) == "" || strings.TrimSpace(commit) == "" {
		return nil, fmt.Errorf("repository and commit are required")
	}
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-NoProfile", "-Command", "git -C '"+repository+"' checkout --detach '"+commit+"'"), nil
	}
	return exec.Command("sh", "-lc", "git -C \""+repository+"\" checkout --detach \""+commit+"\""), nil
}

func git(ctx context.Context, repository string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
