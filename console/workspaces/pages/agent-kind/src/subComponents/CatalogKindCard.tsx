import React from "react";
import { Box, Form, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Link } from "react-router-dom";
import type { AgentKindResponse } from "@agent-management-platform/types";
import { LabelChips } from "@agent-management-platform/shared-component";

interface CatalogKindCardProps {
    item: AgentKindResponse;
    viewPath: string;
}

export const CatalogKindCard: React.FC<CatalogKindCardProps> = ({ item, viewPath }) => {

    const description = item.description ?? "";
    const latestReleaseLabel = item.latestVersion
        ? `Latest: ${item.latestVersion}`
        : null;

    return (
        <Link to={viewPath} style={{ textDecoration: "none" }}>
            <Form.CardButton
                sx={{
                    width: "100%",
                    textAlign: "left",
                    textDecoration: "none",
                    height: 160,
                    display: "flex",
                    flexDirection: "column",
                    justifyContent: "flex-start",
                }}
            >
                <Form.CardHeader
                    sx={{ pb: 0.5 }}
                    title={
                        <Tooltip title={item.displayName} placement="top">
                            <Typography
                                variant="h6"
                                textOverflow="ellipsis"
                                overflow="hidden"
                                whiteSpace="nowrap"
                            >
                                {item.displayName}
                            </Typography>
                        </Tooltip>
                    }
                />
                <Form.CardContent
                    sx={{
                        width: "100%",
                        display: "flex",
                        flexDirection: "column",
                        flexGrow: 1,
                        minHeight: 0,
                        pt: 0,
                    }}
                >
                    {/* Name, description, and version form one tight metadata group. */}
                    <Stack spacing={0.5} minHeight={0}>
                        {/*
                          Box is a plain block element, not a flex item directly —
                          -webkit-line-clamp collapses to zero height (causing the
                          next line to overlap it) when applied straight to a flex
                          child, so it needs this non-flex wrapper to size correctly.
                        */}
                        <Box>
                            <Tooltip title={description} placement="top" disableHoverListener={!description}>
                                <Typography
                                    variant="caption"
                                    color="text.secondary"
                                    sx={{
                                        display: "-webkit-box",
                                        WebkitBoxOrient: "vertical",
                                        WebkitLineClamp: 2,
                                        overflow: "hidden",
                                    }}
                                >
                                    {description || "No description provided."}
                                </Typography>
                            </Tooltip>
                        </Box>
                        {latestReleaseLabel && (
                            <Typography variant="caption" color="text.secondary">
                                {latestReleaseLabel}
                            </Typography>
                        )}
                    </Stack>

                    {/* Labels are a distinct group, anchored to the bottom of the card. */}
                    <Box sx={{ mt: "auto", pt: 1 }}>
                        <LabelChips labels={item.labels} maxVisible={3} />
                    </Box>
                </Form.CardContent>
            </Form.CardButton>
        </Link>
    );
};

export default CatalogKindCard;
