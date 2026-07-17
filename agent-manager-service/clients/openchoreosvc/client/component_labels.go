//
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

package client

import "strings"

const systemLabelKeyPrefix = "openchoreo.dev/"

// isSystemLabelKey reports whether key is a reserved openchoreo.dev/* system
// label. User-defined label keys can never match this: the label-key
// validation in utils.ValidateLabels forbids '/' in user keys, so this is a
// permanent, collision-free partition of the label keyspace — no separate
// collision guard is needed anywhere labels are written.
func isSystemLabelKey(key string) bool {
	return strings.HasPrefix(key, systemLabelKeyPrefix)
}

// addUserLabels copies each entry of userLabels into labels, mutating labels
// in place. Safe unconditionally per isSystemLabelKey: user keys can never
// overwrite a system key. Ranging over a nil userLabels map is a no-op.
func addUserLabels(labels map[string]string, userLabels map[string]string) {
	for k, v := range userLabels {
		labels[k] = v
	}
}

// mergeUserLabels rebuilds a component's label set as the system labels from
// existing, plus userLabels. Rebuilding from system-only plus the new user
// set — rather than only adding/overwriting keys on top of the existing map —
// is what makes removing a label work: a key dropped from userLabels is
// simply not carried forward into the result.
func mergeUserLabels(existing *map[string]string, userLabels map[string]string) map[string]string {
	merged := make(map[string]string, len(userLabels))
	if existing != nil {
		for k, v := range *existing {
			if isSystemLabelKey(k) {
				merged[k] = v
			}
		}
	}
	for k, v := range userLabels {
		merged[k] = v
	}
	return merged
}

// extractUserLabels returns only the non-system entries of labels, or nil if
// there are none — keeping the "no user labels" case indistinguishable from
// today's omitempty behavior.
func extractUserLabels(labels *map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	var out map[string]string
	for k, v := range *labels {
		if isSystemLabelKey(k) {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(*labels))
		}
		out[k] = v
	}
	return out
}
