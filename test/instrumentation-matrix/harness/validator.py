"""ContractValidator — loads a versioned JSON-schema bundle for a provider and
validates captured spans against it (shape + coverage).
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator

from harness.classify import classify_span, span_has_tool_event

_HERE = Path(__file__).resolve().parent.parent
_CONTRACTS = _HERE / "contracts"


def _str_to_number(value: str, *, integer_only: bool) -> int | float | None:
    """Parse a numeric string to int/float, or None if it isn't numeric.

    integer_only rejects non-integral values (e.g. "1.5" for an integer field)
    so a genuine type mismatch still surfaces rather than being silently fixed.
    """
    try:
        f = float(value)
    except (TypeError, ValueError):
        return None
    if f.is_integer():
        return int(f)
    return None if integer_only else f


def _coerce_numeric_attributes(
    span: dict[str, Any], schema: dict[str, Any]
) -> dict[str, Any]:
    """Return a copy of *span* with string attribute values coerced to int/float
    for the attributes the kind schema declares as number/integer.

    Only schema-declared numeric attributes are touched — a string-typed
    attribute that happens to hold "1" (an id, or literal content) is left
    alone. Returns the span unchanged when there is nothing to coerce.
    """
    attr_props = (
        schema.get("properties", {}).get("attributes", {}).get("properties", {})
    )
    attrs = span.get("attributes")
    if not attr_props or not isinstance(attrs, dict):
        return span

    coerced: dict[str, Any] | None = None
    for key, spec in attr_props.items():
        declared = spec.get("type")
        types = declared if isinstance(declared, list) else [declared]
        if "number" not in types and "integer" not in types:
            continue
        value = attrs.get(key)
        if not isinstance(value, str):
            continue
        integer_only = "integer" in types and "number" not in types
        num = _str_to_number(value, integer_only=integer_only)
        if num is None:
            continue
        if coerced is None:
            coerced = dict(attrs)
        coerced[key] = num

    if coerced is None:
        return span
    return {**span, "attributes": coerced}


@dataclass
class ValidationResult:
    ok: bool
    span_name: str
    kind: str
    message: str = ""
    path: str = ""


@dataclass
class CoverageResult:
    ok: bool
    expected: list[str]
    actual: set[str]
    missing: set[str]


class ContractValidator:
    def __init__(self, schema_id: str, kind_schemas: dict[str, Draft202012Validator]):
        self.schema_id = schema_id
        self._validators = kind_schemas

    @classmethod
    def load(cls, schema_id: str) -> "ContractValidator":
        bundle_dir = _CONTRACTS / schema_id / "kinds"
        validators: dict[str, Draft202012Validator] = {}
        for schema_file in bundle_dir.glob("*.schema.json"):
            kind = schema_file.stem.removesuffix(".schema")
            schema = json.loads(schema_file.read_text())
            validators[kind] = Draft202012Validator(schema)
        if not validators:
            raise FileNotFoundError(f"no schemas under {bundle_dir}")
        return cls(schema_id, validators)

    def validate(
        self, span: dict[str, Any], kind: str, *, coerce_numeric: bool = False
    ) -> ValidationResult:
        validator = self._validators.get(kind)
        if validator is None:
            return ValidationResult(
                ok=False,
                span_name=span.get("name", "?"),
                kind=kind,
                message=f"no schema for kind '{kind}' in bundle {self.schema_id}",
            )
        # Heavy tier reads spans back through the observer/OpenSearch round-trip,
        # which returns every attribute value as a string. Coerce the
        # schema-declared numeric attributes back to numbers before validating
        # so the round-trip's stringification isn't reported as a type error.
        # Off by default → the emission tier stays strict and still catches
        # genuine instrumentation stringification (see FINDINGS F-001).
        if coerce_numeric:
            span = _coerce_numeric_attributes(span, validator.schema)
        errors = sorted(validator.iter_errors(span), key=lambda e: list(e.path))
        if not errors:
            return ValidationResult(ok=True, span_name=span.get("name", "?"), kind=kind)
        e = errors[0]
        return ValidationResult(
            ok=False,
            span_name=span.get("name", "?"),
            kind=kind,
            message=e.message,
            path="/" + "/".join(str(p) for p in e.absolute_path),
        )

    def validate_all(
        self, spans: list[dict[str, Any]], *, coerce_numeric: bool = False
    ) -> list[ValidationResult]:
        return [
            self.validate(s, classify_span(s), coerce_numeric=coerce_numeric)
            for s in spans
        ]

    def validate_resource(self, resource: dict[str, Any]) -> ValidationResult:
        path = _CONTRACTS / self.schema_id / "resource.schema.json"
        v = Draft202012Validator(json.loads(path.read_text()))
        errors = sorted(v.iter_errors(resource), key=lambda e: list(e.path))
        if not errors:
            return ValidationResult(ok=True, span_name="<resource>", kind="resource")
        e = errors[0]
        return ValidationResult(
            ok=False,
            span_name="<resource>",
            kind="resource",
            message=e.message,
            path="/" + "/".join(str(p) for p in e.absolute_path),
        )

    def assert_coverage(
        self, spans: list[dict[str, Any]], expected_kinds: list[str]
    ) -> CoverageResult:
        actual = {classify_span(s) for s in spans}
        # F-009: a tool call may be an event on an LLM span rather than its own
        # span. Count that as tool-kind coverage.
        if any(span_has_tool_event(s) for s in spans):
            actual.add("tool")
        missing = set(expected_kinds) - actual
        return CoverageResult(
            ok=not missing, expected=expected_kinds, actual=actual, missing=missing
        )
