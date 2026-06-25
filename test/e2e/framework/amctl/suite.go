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
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// RegisterSuite wires the per-suite CLI setup and returns a Harness that is
// populated before specs run. Call it from a CLI suite's package scope:
//
//	var H = amctl.RegisterSuite()
//
// The binary is built once (parallel process 1) and shared; each process then
// gets its own isolated $HOME and logs in.
func RegisterSuite() *Harness {
	h := &Harness{}

	ginkgo.SynchronizedBeforeSuite(func() []byte {
		// Process 1 only: build (or locate) the binary a single time.
		return []byte(buildAmctl())
	}, func(binPath []byte) {
		// Every process: isolated config home + real login.
		h.bin = string(binPath)
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
