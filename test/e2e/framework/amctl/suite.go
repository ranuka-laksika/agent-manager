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

package amctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// suiteConfig holds optional behavior layered onto a suite registration.
type suiteConfig struct {
	// sharedProc1 runs on parallel process 1 during SynchronizedBeforeSuite's
	// first phase, after the binary is built. Its bytes are broadcast to every
	// process. nil when the suite needs no shared setup.
	sharedProc1 func() []byte
	// sharedEveryProc runs on every process during the second phase, after
	// login, receiving the bytes sharedProc1 returned. nil when unused.
	sharedEveryProc func([]byte)
}

// SuiteOption configures optional behavior on RegisterSuite.
type SuiteOption func(*suiteConfig)

// WithSharedSetup injects suite-specific provisioning into the single
// SynchronizedBeforeSuite the harness owns. proc1 runs exactly once on parallel
// process 1 and returns an opaque payload; everyProc runs on every process with
// that payload. This lets a suite provision a shared fixture (e.g. a CLI-owned
// agent) once instead of racing to create it from a per-process BeforeAll, while
// keeping this package free of any resource specifics. Ginkgo permits only one
// SynchronizedBeforeSuite per suite, so suites that need shared setup must route
// it through here rather than registering their own.
func WithSharedSetup(proc1 func() []byte, everyProc func([]byte)) SuiteOption {
	return func(c *suiteConfig) {
		c.sharedProc1 = proc1
		c.sharedEveryProc = everyProc
	}
}

// suitePayload is the SynchronizedBeforeSuite wire format: the built binary path
// plus an opaque shared-setup blob produced on process 1 and broadcast to all.
type suitePayload struct {
	Bin    string `json:"bin"`
	Shared []byte `json:"shared,omitempty"`
}

// RegisterSuite wires the per-suite CLI setup and returns a Harness that is
// populated before specs run. Call it from a CLI suite's package scope:
//
//	var H = amctl.RegisterSuite()
//
// The binary is built once (parallel process 1) and shared; each process then
// gets its own isolated $HOME and logs in. Pass WithSharedSetup to provision a
// suite-wide fixture exactly once alongside the binary build.
func RegisterSuite(opts ...SuiteOption) *Harness {
	h := &Harness{}
	var sc suiteConfig
	for _, opt := range opts {
		opt(&sc)
	}

	ginkgo.SynchronizedBeforeSuite(func() []byte {
		// Process 1 only: build (or locate) the binary a single time, then run
		// any shared setup so it happens exactly once across all processes.
		payload := suitePayload{Bin: buildAmctl()}
		if sc.sharedProc1 != nil {
			payload.Shared = sc.sharedProc1()
		}
		data, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred(), "marshal suite payload")
		return data
	}, func(data []byte) {
		// Every process: isolated config home + real login.
		var payload suitePayload
		Expect(json.Unmarshal(data, &payload)).To(Succeed(), "unmarshal suite payload")

		h.bin = payload.Bin
		if os.Getenv("AMCTL_BIN") == "" {
			h.builtBinDir = filepath.Dir(h.bin)
		}
		h.cfg = framework.LoadConfig()
		h.org = h.cfg.DefaultOrg

		framework.WaitForAPIReady(h.cfg)

		home, err := os.MkdirTemp("", fmt.Sprintf("amctl-home-%d-", ginkgo.GinkgoParallelProcess()))
		Expect(err).NotTo(HaveOccurred(), "create temp HOME")
		h.home = home

		h.Login(Default)

		if sc.sharedEveryProc != nil {
			sc.sharedEveryProc(payload.Shared)
		}
	})

	ginkgo.SynchronizedAfterSuite(func() {
		// Every process: drop its isolated home.
		if h.home != "" {
			_ = os.RemoveAll(h.home)
		}
	}, func() {
		// Process 1 only: drop the built binary (skipped when AMCTL_BIN was used).
		if h.builtBinDir != "" {
			_ = os.RemoveAll(h.builtBinDir)
		}
	})

	return h
}
