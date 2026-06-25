// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package amctl is a resource-agnostic harness for black-box e2e testing of the
// amctl CLI: it builds (or locates) the binary, runs it against an isolated
// config home, and parses the --json envelope output. Resource-specific
// operations live under operations/cli<resource>; this package never references
// projects, agents, etc.
package amctl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Harness runs amctl commands for one Ginkgo process against an isolated $HOME.
type Harness struct {
	bin         string // path to the amctl binary
	home        string // isolated HOME so ~/.amctl/config can't collide
	builtBinDir string // dir to remove on teardown when we built the binary ourselves
	cfg         *framework.Config
	org         string
}

// Org returns the default organization commands operate against.
func (h *Harness) Org() string { return h.org }

// Run executes `amctl <args...>` with the harness's isolated HOME and captures
// stdout, stderr, and the exit code. It never fails the test itself; callers
// assert on the returned Result (see DecodeData / ExpectError).
func (h *Harness) Run(args ...string) Result {
	cmd := exec.Command(h.bin, args...)
	cmd.Env = h.env()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Stdin stays nil (/dev/null) so the CLI sees a non-terminal and never prompts.

	runErr := cmd.Run()
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	} else if runErr != nil {
		code = -1 // process never started (e.g. binary missing)
	}

	return Result{
		Args:     append([]string{}, args...),
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: code,
	}
}

// env returns the parent environment with HOME pointed at the isolated config dir.
func (h *Harness) env() []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if strings.HasPrefix(e, "HOME=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "HOME="+h.home)
}

// buildAmctl returns a usable amctl binary path: $AMCTL_BIN if set, otherwise a
// freshly built binary. Intended to run once (SynchronizedBeforeSuite proc 1).
func buildAmctl() string {
	if bin := os.Getenv("AMCTL_BIN"); bin != "" {
		return bin
	}

	cliDir, err := locateCLIDir()
	Expect(err).NotTo(HaveOccurred(), "locate cli module dir")

	tmpDir, err := os.MkdirTemp("", "amctl-bin-")
	Expect(err).NotTo(HaveOccurred(), "create temp bin dir")

	bin := filepath.Join(tmpDir, "amctl")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/amctl")
	cmd.Dir = cliDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "build amctl:\n%s", string(out))
	return bin
}

// locateCLIDir walks up from this file's location to find the repo's cli module.
func locateCLIDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		cliDir := filepath.Join(dir, "cli")
		if fi, err := os.Stat(filepath.Join(cliDir, "cmd", "amctl")); err == nil && fi.IsDir() {
			return cliDir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not locate cli module dir from %s", thisFile)
}
