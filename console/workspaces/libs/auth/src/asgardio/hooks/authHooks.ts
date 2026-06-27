/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useAsgardeo, useUser } from "@asgardeo/react";
import type { UserInfo } from "../../types";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { globalConfig } from "@agent-management-platform/types";

const decodeJWTPart = (part: string): Record<string, unknown> | null => {
  try {
    const normalized = part.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
    return JSON.parse(window.atob(padded)) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const decodeJWT = (token: string) => {
  const [header, payload] = token.split(".");
  if (!header || !payload) return null;

  return {
    header: decodeJWTPart(header),
    payload: decodeJWTPart(payload),
  };
};

export type AuthHooks = {
  isAuthenticated: boolean;
  userInfo: UserInfo;
  isLoadingUserInfo: boolean;
  isLoadingIsAuthenticated: boolean;
  getToken: () => Promise<string>;
  login: () => void;
  logout: () => Promise<void>;
  trySignInSilently: () => Promise<unknown>;
};

export const useAuthHooks = (): AuthHooks => {
  const {
    signIn,
    getAccessToken,
    signInSilently,
    signOut,
    isSignedIn = false,
    isLoading = false,
    isInitialized = false,
  } = useAsgardeo() ?? {};

  const { flattenedProfile } = useUser();

  const [accessTokenPayload, setAccessTokenPayload] =
    useState<Record<string, unknown> | null>(null);

  const getAccessTokenRef = useRef(getAccessToken);
  getAccessTokenRef.current = getAccessToken;

  useEffect(() => {
    if (!isSignedIn || !isInitialized) {
      if (!isSignedIn) setAccessTokenPayload(null);
      return;
    }
    let cancelled = false;
    const tokenPromise = getAccessTokenRef.current?.();
    if (!tokenPromise) return;
    tokenPromise
      .then((token) => {
        if (cancelled) return;
        if (!token) {
          setAccessTokenPayload(null);
          return;
        }
        const decoded = decodeJWT(token);
        if (!cancelled) setAccessTokenPayload(decoded?.payload ?? null);
      })
      .catch(() => {
        if (!cancelled) setAccessTokenPayload(null);
      });
    return () => {
      cancelled = true;
    };
  }, [isSignedIn, isInitialized]);

  const userInfo = useMemo(() => {
    return {
      ...flattenedProfile,
      familyName: flattenedProfile?.family_name,
      givenName: flattenedProfile?.given_name,
      ...(accessTokenPayload ?? {}),
    } as UserInfo;
  }, [flattenedProfile, accessTokenPayload]);

  const customLogin = () => {
    void signIn?.();
  };

  const handleLogout = useCallback(async () => {
    try {
      await signOut?.();
    } catch (error) {
      console.error("Error during signOut:", error);
    } finally {
      window.location.assign(
        globalConfig.authConfig.afterSignOutUrl ?? "/login",
      );
    }
  }, [signOut]);

  const safeGetToken: () => Promise<string> =
    getAccessToken ??
    (() => Promise.reject(new Error("getAccessToken is not available")));

  const safeSignInSilently: () => Promise<unknown> =
    signInSilently ??
    (() => Promise.reject(new Error("signInSilently is not available")));

  return {
    isAuthenticated: isSignedIn && isInitialized,
    userInfo,
    isLoadingUserInfo: isLoading,
    isLoadingIsAuthenticated: !isInitialized || isLoading,
    getToken: safeGetToken,
    login: customLogin,
    logout: handleLogout,
    trySignInSilently: safeSignInSilently,
  };
};
