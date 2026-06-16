/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Search } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent } from "./DrawerContent";
import { DrawerHeader } from "./DrawerHeader";
import { DrawerWrapper } from "./DrawerWrapper";

export interface SelectionDrawerEmptyState {
  title: string;
  description: string;
}

export interface SelectionDrawerProps<T> {
  /** Whether the drawer is open. */
  open: boolean;
  /** Called when the drawer requests to close (header close, backdrop, or after a selection). */
  onClose: () => void;
  /** Icon shown in the drawer header. */
  icon: ReactNode;
  /** Drawer header title. */
  title: string;
  /** Helper text rendered above the search bar. */
  description: ReactNode;
  /** Placeholder for the search input. */
  searchPlaceholder: string;
  /** Items to list. */
  items: T[];
  /** When true, render loading skeletons instead of the list. */
  isLoading?: boolean;
  /** Stable React key for an item. */
  getItemKey: (item: T) => string;
  /** Whether an item is currently selected. */
  isItemSelected: (item: T) => boolean;
  /** Predicate used to filter items; `query` is already trimmed and lower-cased. */
  matchesSearch: (item: T, query: string) => boolean;
  /** Invoked with the chosen item; the drawer closes automatically afterwards. */
  onSelect: (item: T) => void;
  /** Renders the contents of an item's selection card. */
  renderItem: (item: T, isSelected: boolean) => ReactNode;
  /** Optional accessible label for an item's selection card. */
  getItemAriaLabel?: (item: T, isSelected: boolean) => string;
  /** Empty state shown when there are no items (no active search). */
  emptyState: SelectionDrawerEmptyState;
  /** Empty state shown when a search yields no matches. */
  searchEmptyState: SelectionDrawerEmptyState;
  minWidth?: number | string;
  maxWidth?: number | string;
}

/**
 * A right-side drawer that presents a searchable, single-select list of items as
 * selection cards. The drawer chrome, debounced search, loading, and empty states
 * are handled here; callers supply the data and the per-item rendering.
 */
export function SelectionDrawer<T>({
  open,
  onClose,
  icon,
  title,
  description,
  searchPlaceholder,
  items,
  isLoading = false,
  getItemKey,
  isItemSelected,
  matchesSearch,
  onSelect,
  renderItem,
  getItemAriaLabel,
  emptyState,
  searchEmptyState,
  minWidth = 740,
  maxWidth = 740,
}: SelectionDrawerProps<T>) {
  const [searchQuery, setSearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (searchTimerRef.current) {
        clearTimeout(searchTimerRef.current);
        searchTimerRef.current = null;
      }
    };
  }, []);

  const trimmed = debouncedSearch.trim();
  const filtered = trimmed
    ? items.filter((item) => matchesSearch(item, trimmed.toLowerCase()))
    : items;

  return (
    <DrawerWrapper
      open={open}
      onClose={onClose}
      minWidth={minWidth}
      maxWidth={maxWidth}
    >
      <DrawerHeader icon={icon} title={title} onClose={onClose} />
      <DrawerContent>
        <Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {description}
          </Typography>
          <SearchBar
            placeholder={searchPlaceholder}
            size="small"
            fullWidth
            value={searchQuery}
            onChange={(e) => {
              const val = e.target.value;
              setSearchQuery(val);
              if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
              searchTimerRef.current = setTimeout(
                () => setDebouncedSearch(val),
                250,
              );
            }}
            sx={{ mb: 1 }}
          />
          <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
            {isLoading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} variant="rounded" height={72} />
              ))
            ) : filtered.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<Search size={64} />}
                  title={trimmed ? searchEmptyState.title : emptyState.title}
                  description={
                    trimmed
                      ? searchEmptyState.description
                      : emptyState.description
                  }
                />
              </ListingTable.Container>
            ) : (
              filtered.map((item) => {
                const selected = isItemSelected(item);
                return (
                  <Form.CardButton
                    key={getItemKey(item)}
                    onClick={() => {
                      onSelect(item);
                      onClose();
                    }}
                    selected={selected}
                    aria-label={getItemAriaLabel?.(item, selected)}
                  >
                    <Form.CardContent>
                      {renderItem(item, selected)}
                    </Form.CardContent>
                  </Form.CardButton>
                );
              })
            )}
          </Stack>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
