/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { IconButton, TextField } from "@wso2/oxygen-ui";
import { Copy } from "@wso2/oxygen-ui-icons-react";

export const commandTextFieldSx = {
  "& .MuiInputBase-input": {
    fontFamily: "monospace",
    fontSize: "0.875rem",
  },
};

export interface CommandFieldProps {
  value: string;
  multiline?: boolean;
  minRows?: number;
  onCopy: () => void;
  copyLabel: string;
}

export function CommandField({
  value,
  multiline,
  minRows = 1,
  onCopy,
  copyLabel,
}: CommandFieldProps) {
  return (
    <TextField
      fullWidth
      multiline={multiline}
      minRows={minRows}
      value={value}
      slotProps={{
        input: {
          readOnly: true,
          endAdornment: (
            <IconButton size="small" onClick={onCopy} aria-label={`Copy ${copyLabel}`}>
              <Copy size={16} />
            </IconButton>
          ),
        },
      }}
      sx={commandTextFieldSx}
    />
  );
}
