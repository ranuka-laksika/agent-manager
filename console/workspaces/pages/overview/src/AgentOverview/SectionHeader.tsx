/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { ChevronRight } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface SectionHeaderProps {
  title: string;
  viewAllHref: string;
  mb?: number;
}

/**
 * Uppercase caption + "View all" link, shared by every EnvironmentCard
 * section (Agent Identity, Agent Performance, Recent Traces, System Metrics)
 * that links out to its own full listing page.
 */
export const SectionHeader: React.FC<SectionHeaderProps> = ({ title, viewAllHref, mb = 0.5 }) => (
  <Box display="flex" justifyContent="space-between" alignItems="center" mb={mb}>
    <Typography
      variant="caption"
      color="text.secondary"
      fontWeight={600}
      sx={{ textTransform: "uppercase", letterSpacing: "0.05em" }}
    >
      {title}
    </Typography>
    <Button
      size="small"
      variant="text"
      endIcon={<ChevronRight size={14} />}
      component={Link}
      to={viewAllHref}
      sx={{ minWidth: 0, fontSize: "0.75rem" }}
    >
      View all
    </Button>
  </Box>
);
