// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package spec

type CreateDeploymentPipelineRequest struct {
	// DisplayName is the human-readable name for the pipeline.
	DisplayName string `json:"displayName"`
	// Description is an optional description.
	Description *string `json:"description,omitempty"`
	// ProjectName optionally links the pipeline to a project.
	ProjectName *string `json:"projectName,omitempty"`
	// PromotionPaths defines the ordered promotion chain.
	PromotionPaths []PromotionPath `json:"promotionPaths"`
}
