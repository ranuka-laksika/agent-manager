#!/usr/bin/env bash
# Pre-flight / config tests for the advanced VM installer: config validation
# (lib-advanced.sh), the cert-manager DNS-01 renderers (lib-certmanager.sh), and the
# advisory DNS check. Run: bash deployments/vm/tests/preflight.sh
# AMP_HOST_*/DOMAIN_BASE/AMP_AGENTS_BASE are consumed by sourced lib functions; the
# source boundary hides that from shellcheck. _resolve_host stubs are invoked indirectly.
# shellcheck disable=SC2034,SC2329
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib-advanced.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-advanced.sh"
# shellcheck source=../lib-vm.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-vm.sh"
# shellcheck source=../lib-certmanager.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-certmanager.sh"

FAILLOG="$(mktemp)"
trap 'rm -f "$FAILLOG"' EXIT
assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then printf 'ok   - %s\n' "$label"
  else printf 'FAIL - %s\n      expected: %q\n      actual:   %q\n' "$label" "$expected" "$actual"; echo 1 >>"$FAILLOG"; fi
}

DOMAIN_BASE=amp.mycompany.com
AMP_AGENTS_BASE=agents.amp.mycompany.com
AMP_HOST_CONSOLE=console.amp.mycompany.com
AMP_HOST_API=api.amp.mycompany.com
AMP_HOST_THUNDER=thunder.amp.mycompany.com
AMP_HOST_OBSERVER=observer.amp.mycompany.com
AMP_HOST_GATEWAY=gateway.amp.mycompany.com
AMP_HOST_CP=cp.amp.mycompany.com

# --- cert_dns_names: fixed hosts + the two dynamic wildcards ---
sans="$(cert_dns_names)"
assert_eq "cert SANs include console"          "yes" "$(grep -qxF 'console.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include cp (external gw)" "yes" "$(grep -qxF 'cp.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include agents wildcard"  "yes" "$(grep -qxF '*.agents.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include env-Thunder wild" "yes" "$(grep -qxF '*.thunder.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
# CP omitted when external gateways are off.
AMP_HOST_CP="" sans_nocp="$(AMP_HOST_CP="" cert_dns_names)"
assert_eq "cert SANs omit cp when unset"       "no"  "$(grep -qxF 'cp.amp.mycompany.com' <<<"$sans_nocp" && echo yes || echo no)"

# --- validate_dns01_config: provider + credential presence (appends to CONFIG_ERRORS) ---
run_validate() { CONFIG_ERRORS=(); validate_dns01_config; echo "${#CONFIG_ERRORS[@]}"; }

n="$(DNS_PROVIDER=cloudflare CLOUDFLARE_API_TOKEN=tok run_validate)"
assert_eq "cloudflare with token: 0 errors" "0" "$n"
n="$(DNS_PROVIDER=cloudflare run_validate)"                # token missing
assert_eq "cloudflare without token: error" "1" "$n"
n="$(DNS_PROVIDER=route53 AWS_ACCESS_KEY_ID=a AWS_SECRET_ACCESS_KEY=s AWS_REGION=us-east-1 run_validate)"
assert_eq "route53 with all creds: 0 errors" "0" "$n"
n="$(DNS_PROVIDER=route53 AWS_ACCESS_KEY_ID=a run_validate)"   # 2 missing
assert_eq "route53 missing creds: 2 errors" "2" "$n"
n="$(DNS_PROVIDER=bogus run_validate)"
assert_eq "unknown provider: error" "1" "$n"

# --- render_acme_clusterissuer emits the right provider solver block ---
issuer_cf="$(DNS_PROVIDER=cloudflare ACME_EMAIL=o@x.com render_acme_clusterissuer amp-acme amp-creds)"
assert_eq "cloudflare issuer has cloudflare solver" "yes" "$(grep -q 'cloudflare:' <<<"$issuer_cf" && echo yes || echo no)"
assert_eq "cloudflare issuer references cred secret" "yes" "$(grep -q 'name: amp-creds' <<<"$issuer_cf" && echo yes || echo no)"
issuer_r53="$(DNS_PROVIDER=route53 ACME_EMAIL=o@x.com AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=AK render_acme_clusterissuer amp-acme amp-creds)"
assert_eq "route53 issuer has route53 solver" "yes" "$(grep -q 'route53:' <<<"$issuer_r53" && echo yes || echo no)"
assert_eq "route53 issuer sets region"        "yes" "$(grep -q 'region: us-east-1' <<<"$issuer_r53" && echo yes || echo no)"

# --- render_wildcard_certificate covers the SANs and references the issuer ---
cert="$(render_wildcard_certificate amp-tls amp-tls amp-acme)"
assert_eq "cert manifest lists agents wildcard" "yes" "$(grep -q '\*.agents.amp.mycompany.com' <<<"$cert" && echo yes || echo no)"
assert_eq "cert manifest references issuer"     "yes" "$(grep -q 'name: amp-acme' <<<"$cert" && echo yes || echo no)"

# --- render_consolidated_gateway: :443 HTTPS Terminate, from All, cert ref ---
gw="$(render_consolidated_gateway amp-gw amp-tls 443)"
assert_eq "gateway listens :443"        "yes" "$(grep -q 'port: 443' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway terminates TLS"      "yes" "$(grep -q 'mode: Terminate' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway allows all routes"   "yes" "$(grep -q 'from: All' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway references cert sec" "yes" "$(grep -q 'name: amp-tls' <<<"$gw" && echo yes || echo no)"

# --- render_k3d_advanced_config: publishes :443 (public) + loopback-binds plane ports ---
k3d_out="$(printf 'ports:\n  - port: 8080:8080\n    nodeFilters:\n      - loadbalancer\n' | render_k3d_advanced_config)"
assert_eq "k3d advanced publishes 443"        "yes" "$(grep -q -- '- port: 443:443' <<<"$k3d_out" && echo yes || echo no)"
assert_eq "k3d advanced loopback-binds 8080"  "yes" "$(grep -q -- '- port: 127.0.0.1:8080:8080' <<<"$k3d_out" && echo yes || echo no)"

# --- validate_dns is advisory: records errors but never fails (no more hard-fail mode) ---
_resolve_host() { echo "198.51.100.5"; }        # resolves to a non-candidate IP
validate_dns 203.0.113.10; rc=$?
assert_eq "validate_dns advisory rc=0"       "0"  "$rc"
assert_eq "validate_dns records the mismatch" "yes" "$([[ ${#DNS_ERRORS[@]} -gt 0 ]] && echo yes || echo no)"
unset -f _resolve_host

if [[ -s "$FAILLOG" ]]; then echo "PREFLIGHT TESTS FAILED"; exit 1; fi
echo "ALL PREFLIGHT TESTS PASSED"
