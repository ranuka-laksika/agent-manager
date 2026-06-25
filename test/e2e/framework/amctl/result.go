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

	. "github.com/onsi/gomega"
)

// Result is the captured outcome of one amctl invocation.
type Result struct {
	Args     []string
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Combined renders the invocation for assertion failure messages.
func (r Result) Combined() string {
	return fmt.Sprintf("args=%v exit=%d\n--- stdout ---\n%s\n--- stderr ---\n%s",
		r.Args, r.ExitCode, r.Stdout, r.Stderr)
}

// EnvelopeError mirrors the CLI's clierr.CLIError JSON shape.
type EnvelopeError struct {
	Status  int     `json:"status"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Reason  *string `json:"reason"`
}

// Envelope is the --json output envelope written to stdout by every command.
type Envelope struct {
	Instance string          `json:"instance"`
	Org      string          `json:"org"`
	Project  string          `json:"project"`
	Data     json.RawMessage `json:"data"`
	Error    *EnvelopeError  `json:"error"`
}

// Envelope parses stdout as the --json envelope.
func (r Result) Envelope(g Gomega) Envelope {
	var env Envelope
	g.Expect(json.Unmarshal(r.Stdout, &env)).To(Succeed(), "parse JSON envelope: %s", r.Combined())
	return env
}

// DecodeData asserts the command succeeded (exit 0, no error envelope) and
// decodes the envelope's data field into T.
func DecodeData[T any](g Gomega, r Result) T {
	g.Expect(r.ExitCode).To(Equal(0), "expected success: %s", r.Combined())
	env := r.Envelope(g)
	g.Expect(env.Error).To(BeNil(), "expected no error envelope: %s", r.Combined())
	var out T
	g.Expect(json.Unmarshal(env.Data, &out)).To(Succeed(), "decode data: %s", r.Combined())
	return out
}

// ExpectError asserts the command failed (non-zero exit) and returns its error envelope.
func (r Result) ExpectError(g Gomega) EnvelopeError {
	g.Expect(r.ExitCode).NotTo(Equal(0), "expected failure: %s", r.Combined())
	env := r.Envelope(g)
	g.Expect(env.Error).NotTo(BeNil(), "expected error envelope: %s", r.Combined())
	return *env.Error
}
