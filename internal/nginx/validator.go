package nginx

import (
	"context"
	"fmt"
	"os/exec"
)

// Validator runs `nginx -t` to check whether the on-disk configuration is
// syntactically and semantically valid.
type Validator struct {
	BinaryPath string // e.g. "nginx" or "/usr/sbin/nginx"
}

// NewValidator returns a Validator using the given nginx binary path.
func NewValidator(binaryPath string) *Validator {
	if binaryPath == "" {
		binaryPath = "nginx"
	}
	return &Validator{BinaryPath: binaryPath}
}

// Result captures the outcome of a configuration test.
type Result struct {
	OK     bool
	Output string // combined stdout+stderr from nginx -t
}

// Test runs `nginx -t` and reports whether the configuration is valid.
func (v *Validator) Test(ctx context.Context) (Result, error) {
	cmd := exec.CommandContext(ctx, v.BinaryPath, "-t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if isExitError(err, &exitErr) {
			return Result{OK: false, Output: string(out)}, nil
		}
		return Result{}, fmt.Errorf("running %s -t: %w", v.BinaryPath, err)
	}
	return Result{OK: true, Output: string(out)}, nil
}

func isExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}
