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

import React, {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";
import {
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
  HighContrastTheme,
  PaleGrayTheme,
} from "@wso2/oxygen-ui";

export type AppThemeKey =
  | "acrylic-orange"
  | "acrylic-purple"
  | "pale-gray"
  | "high-contrast";

interface ThemeRegistryEntry {
  label: string;
  color: string;
  description: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  themeObject: any;
}

/** Single source of truth for all theme metadata and theme objects. */
const THEME_REGISTRY: Record<AppThemeKey, ThemeRegistryEntry> = {
  "acrylic-orange": {
    label: "Acrylic Orange",
    color: "#FF7400",
    description: "Vibrant orange accent",
    themeObject: AcrylicOrangeTheme,
  },
  "acrylic-purple": {
    label: "Acrylic Purple",
    color: "#646cff",
    description: "Blue-purple accent",
    themeObject: AcrylicPurpleTheme,
  },
  "pale-gray": {
    label: "Pale Gray",
    color: "#212121",
    description: "Subtle neutral tones",
    themeObject: PaleGrayTheme,
  },
  "high-contrast": {
    label: "High Contrast",
    color: "#0000FF",
    description: "Maximum legibility",
    themeObject: HighContrastTheme,
  },
};

export interface AppThemeOption {
  key: AppThemeKey;
  label: string;
  /** Representative primary color for the preview swatch. */
  color: string;
  description: string;
}

export const APP_THEME_OPTIONS: AppThemeOption[] = (
  Object.entries(THEME_REGISTRY) as [AppThemeKey, ThemeRegistryEntry][]
).map(([key, { label, color, description }]) => ({ key, label, color, description }));

const THEME_STORAGE_KEY = "amp:theme";
const DEFAULT_THEME: AppThemeKey = "acrylic-orange";

function readStoredThemeKey(): AppThemeKey {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored && stored in THEME_REGISTRY) return stored as AppThemeKey;
  } catch {
    // localStorage not available (e.g. private mode)
  }
  return DEFAULT_THEME;
}

interface AppThemeContextValue {
  themeKey: AppThemeKey;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  themeObject: any;
  setThemeKey: (key: AppThemeKey) => void;
}

const AppThemeContext = createContext<AppThemeContextValue | null>(null);

export const AppThemeProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [themeKey, setThemeKeyState] = useState<AppThemeKey>(readStoredThemeKey);

  const setThemeKey = useCallback((key: AppThemeKey) => {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, key);
    } catch {
      // ignore write failures
    }
    setThemeKeyState(key);
  }, []);

  const ctxValue = useMemo(
    () => ({ themeKey, themeObject: THEME_REGISTRY[themeKey].themeObject, setThemeKey }),
    [themeKey, setThemeKey],
  );

  return (
    <AppThemeContext.Provider value={ctxValue}>
      {children}
    </AppThemeContext.Provider>
  );
};

export const useAppTheme = (): AppThemeContextValue => {
  const ctx = useContext(AppThemeContext);
  if (!ctx) throw new Error("useAppTheme must be used within AppThemeProvider");
  return ctx;
};
