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

package framework

import "github.com/onsi/ginkgo/v2"

// AttachOnFailure registers a deferred diagnostic that is added to the Ginkgo report
// (and therefore the JUnit XML, via --junit-report) only when the current spec fails.
//
// Wait helpers that poll with Eventually should capture the last observed state — the
// query scope, the last HTTP status, the last response body — into a variable and pass
// a getter here. On a timeout the raw context lands in the report instead of just a
// terse matcher mismatch, so CI failures are triageable without re-running locally.
//
// The getter is invoked once, at cleanup time, only on failure — so retries never spam
// the report and a passing spec attaches nothing.
func AttachOnFailure(label string, get func() string) {
	ginkgo.DeferCleanup(func() {
		if !ginkgo.CurrentSpecReport().Failed() {
			return
		}
		if get == nil {
			return
		}
		if content := get(); content != "" {
			ginkgo.AddReportEntry(label, content)
		}
	})
}
