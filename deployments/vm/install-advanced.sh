#!/usr/bin/env bash
# install-advanced.sh — config-driven Agent Manager install on a VM with Docker.
# Run ON the target VM with sudo. Custom domain + publicly-trusted TLS via cert-manager's
# ACME DNS-01 challenge (kgateway terminates TLS on :443 — no Caddy, no lego). Works on a
# public OR private VM: issuance is egress-only (the ACME CA reads a DNS TXT record).
# See --init for the annotated config template.
#
# Usage:
#   sudo ./install-advanced.sh --config amp-config.env
#   ./install-advanced.sh --init > amp-config.env      # emit annotated template
#   sudo ./install-advanced.sh --config amp-config.env --dry-run   # validate + render only
set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# This installer wraps the quick-start installer (install.sh + k3d-config.yaml).
QS_DIR="$(cd "${VM_DIR}/../quick-start" && pwd)"

log() { printf '\033[0;34m[install-advanced]\033[0m %s\n' "$*"; }
die() { printf '\033[0;31m[install-advanced] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# shellcheck source=lib-vm.sh
source "${VM_DIR}/lib-vm.sh"
# shellcheck source=lib-advanced.sh
source "${VM_DIR}/lib-advanced.sh"
# shellcheck source=lib-bootstrap.sh
source "${VM_DIR}/lib-bootstrap.sh"
# shellcheck source=lib-certmanager.sh
source "${VM_DIR}/lib-certmanager.sh"

print_template() {
  cat <<'TEMPLATE'
# amp-config.env — Agent Manager advanced VM install configuration.
# Sourced by install-advanced.sh. Lines are shell assignments.
#
# TLS is always publicly-trusted certificates issued in-cluster by cert-manager
# using the ACME DNS-01 challenge (kgateway terminates TLS on :443 — there is no
# Caddy). The ACME CA validates by reading a DNS TXT record, so the VM needs NO
# inbound access for issuance (egress only) and this works on a private VM too.

# --- Required ---
AMP_VERSION=0.15.0                 # amp/v* release tag (see github.com/wso2/agent-manager/releases)
DOMAIN_BASE=amp.mycompany.com      # service hosts derived as <svc>.<DOMAIN_BASE>
ACME_EMAIL=ops@mycompany.com       # ACME account contact (required)

# --- DNS-01 provider (you must control the DNS zone for DOMAIN_BASE) ---
# cert-manager writes a TXT record to prove control of the zone, then issues a
# wildcard certificate covering every service host, the deployed-agent wildcard, and
# the env-Thunder wildcard. Set DNS_PROVIDER and that provider's credentials below;
# the installer turns them into the Kubernetes Secret the ClusterIssuer references.
DNS_PROVIDER=cloudflare            # cloudflare | route53 | clouddns | azuredns
#   Cloudflare:        CLOUDFLARE_API_TOKEN=...            (scoped Zone.DNS:Edit token)
#   AWS Route53:       AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... AWS_REGION=us-east-1
#   Google Cloud DNS:  GCP_PROJECT=... GCP_SERVICE_ACCOUNT_FILE=/opt/amp/gcp-sa.json
#   Azure DNS:         AZURE_TENANT_ID=... AZURE_CLIENT_ID=... AZURE_CLIENT_SECRET=... \
#                      AZURE_SUBSCRIPTION_ID=... AZURE_RESOURCE_GROUP=...
# ACME_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory  # optional: LE staging for testing

# --- Optional ---
EXTERNAL_GATEWAYS=true             # expose the cp endpoint for external data-plane gateways

# --- Optional per-service host overrides (default: <svc>.<DOMAIN_BASE>) ---
# HOST_CONSOLE=console.amp.mycompany.com
# HOST_API=api.amp.mycompany.com
# HOST_THUNDER=thunder.amp.mycompany.com
# HOST_OBSERVER=observer.amp.mycompany.com
# HOST_GATEWAY=gateway.amp.mycompany.com
# HOST_CP=cp.amp.mycompany.com
# AGENTS_BASE=agents.amp.mycompany.com   # deployed-agent wildcard base
TEMPLATE
}

CONFIG_FILE="" DRY_RUN="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --init) print_template; exit 0 ;;
    --config) CONFIG_FILE="${2:?--config requires a path}"; shift 2 ;;
    --dry-run) DRY_RUN="true"; shift ;;
    -h|--help) grep '^#' "$0" | grep -v '^#!' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

[[ -n "$CONFIG_FILE" ]] || die "--config <file> is required (or --init to emit a template)"

# Load + validate config, derive hostnames.
load_config "$CONFIG_FILE" || die "could not load config: $CONFIG_FILE"
if ! validate_config; then
  printf '%s\n' "${CONFIG_ERRORS[@]}" >&2
  die "config validation failed (${#CONFIG_ERRORS[@]} error(s)) — fix amp-config.env and re-run"
fi
# Declare the host vars in this scope so the lib-vm.sh cores (dynamic scope) see them.
AMP_HOST_CONSOLE="" AMP_HOST_API="" AMP_HOST_THUNDER="" AMP_HOST_OBSERVER=""
AMP_HOST_GATEWAY="" AMP_HOST_CP="" AMP_AGENTS_BASE=""
derive_hosts

# Names of the cert-manager + gateway resources the installer creates post-install.
DNS01_SECRET="amp-dns01-credentials"
ACME_ISSUER="amp-acme-dns01"
WILDCARD_CERT="amp-wildcard-tls"
WILDCARD_SECRET="amp-wildcard-tls"
CONSOLIDATED_GATEWAY="amp-consolidated-gateway"
GATEWAY_NS="openchoreo-control-plane"

# preflight_dns — advisory only. DNS-01 needs NO inbound and does NOT require the service
# hostnames to point at this VM (the ACME CA proves control by reading a TXT record the
# provider API writes). The A records only matter for clients reaching the services, so
# report whether they resolve here without ever aborting the install.
preflight_dns() {
  local -a cand=(); local ip pub
  while IFS= read -r ip; do [[ -n "$ip" ]] && cand+=("$ip"); done < <(_local_ips)
  pub="$(_public_ip)"; [[ -n "$pub" ]] && cand+=("$pub")
  (( ${#cand[@]} )) || { log "Could not determine the VM's IP for the DNS check; skipping."; return 0; }
  validate_dns "${cand[@]}" >/dev/null 2>&1 || true   # advisory: validate_dns hard-fails only in the (removed) letsencrypt mode
  if (( ${#DNS_ERRORS[@]} == 0 )); then
    log "DNS check: all service hostnames resolve to this VM."
  else
    log "DNS check (advisory): some hostnames don't resolve to this VM yet — point your DNS (or client /etc/hosts) at it before connecting. Certificate issuance itself needs no inbound and no A records."
  fi
}

# apply_advanced_tls — after the base install, create the cert-manager DNS-01 resources
# (provider Secret + ACME ClusterIssuer + wildcard Certificate) and the single :443 HTTPS
# Gateway that terminates TLS with the issued cert, then wait for issuance. This replaces
# the old lego + Caddy path entirely; cert-manager auto-renews the cert in-cluster.
apply_advanced_tls() {
  log "Applying cert-manager DNS-01 resources (provider=${DNS_PROVIDER}) + consolidated :443 gateway"
  { render_dns01_credentials_secret "$DNS01_SECRET"
    echo "---"
    render_acme_clusterissuer "$ACME_ISSUER" "$DNS01_SECRET"
    echo "---"
    render_wildcard_certificate "$WILDCARD_CERT" "$WILDCARD_SECRET" "$ACME_ISSUER"
    echo "---"
    render_consolidated_gateway "$CONSOLIDATED_GATEWAY" "$WILDCARD_SECRET" 443
  } | kubectl apply -f - || die "failed to apply cert-manager/gateway resources"

  log "Waiting for cert-manager to issue the wildcard cert via DNS-01 (can take a few minutes)…"
  kubectl wait --for=condition=Ready "certificate/${WILDCARD_CERT}" -n "$GATEWAY_NS" --timeout=600s \
    || die "cert-manager did not issue the cert — inspect: kubectl describe certificate ${WILDCARD_CERT} -n ${GATEWAY_NS}; kubectl get challenge -A"

  # Routes not repointed via chart/config overrides (platform + env Thunder, the
  # api-platform gateway/LLM-proxy routes) are attached to the consolidated gateway here.
  # amp-console/api/gateway-mgmt (ocIngress override) and deployed agents (DP external
  # ingress override) are already born on it; observer via its ocIngress override.
  attach_routes_to_consolidated_gateway
}

# attach_routes_to_consolidated_gateway — ensure every HTTPRoute in the platform
# namespaces also lists the consolidated :443 gateway as a parent, so it is served on
# :443 regardless of which plane gateway the chart/OpenChoreo generated it against. Adds
# the parentRef only when absent (idempotent), so re-runs are safe.
attach_routes_to_consolidated_gateway() {
  local ns route parent
  parent="{\"group\":\"gateway.networking.k8s.io\",\"kind\":\"Gateway\",\"name\":\"${CONSOLIDATED_GATEWAY}\",\"namespace\":\"${GATEWAY_NS}\"}"
  for ns in openchoreo-control-plane openchoreo-observability-plane openchoreo-data-plane \
            $(kubectl get ns -o name 2>/dev/null | sed 's|namespace/||' | grep -E '^(amp-thunder|dp-)' || true); do
    while IFS= read -r route; do
      [[ -z "$route" ]] && continue
      # Skip routes that already parent the consolidated gateway.
      if kubectl get httproute "$route" -n "$ns" -o json 2>/dev/null \
          | grep -q "\"name\":\"${CONSOLIDATED_GATEWAY}\""; then continue; fi
      kubectl patch httproute "$route" -n "$ns" --type=json \
        -p "[{\"op\":\"add\",\"path\":\"/spec/parentRefs/-\",\"value\":${parent}}]" >/dev/null 2>&1 \
        && log "  attached ${ns}/${route} to ${CONSOLIDATED_GATEWAY}"
    done < <(kubectl get httproute -n "$ns" -o name 2>/dev/null | sed 's|httproute.gateway.networking.k8s.io/||')
  done
}

run_advanced_install() {
  [[ "$(id -u)" -eq 0 ]] || die "run with sudo — this opens the firewall and creates the cluster"

  log "Phase 1/3: preflight (verify tools + firewall)"
  verify_prerequisites
  ensure_inotify_limits
  ensure_firewall 443     # inbound :443 for client traffic; DNS-01 issuance itself needs no inbound
  ensure_disk
  preflight_dns

  log "Phase 2/3: install Agent Manager (cert-manager DNS-01, no Caddy) — 8-15 min"
  # Hostname-driven helm overrides (same cores as the simple path), plus advanced-only
  # overrides that make the fixed-service routes parent the consolidated :443 gateway.
  # shellcheck disable=SC2034  # arrays are inherited by the subshell that sources install.sh
  mapfile -t AMP_HELM_ARGS < <(amp_helm_args)
  AMP_HELM_ARGS+=(--set "ocIngress.gatewayName=${CONSOLIDATED_GATEWAY}"
                  --set "ocIngress.gatewayNamespace=${GATEWAY_NS}")
  # shellcheck disable=SC2034
  mapfile -t THUNDER_HELM_ARGS < <(thunder_helm_args)
  # shellcheck disable=SC2034
  mapfile -t GATEWAY_HELM_ARGS < <(gateway_helm_args)
  # shellcheck disable=SC2034
  mapfile -t CP_HELM_ARGS < <(cp_helm_args)
  # shellcheck disable=SC2034
  mapfile -t PLATFORM_RESOURCES_HELM_ARGS < <(build_platform_resources_helm_args)
  # shellcheck disable=SC2034
  mapfile -t OBSERVABILITY_HELM_ARGS < <(observability_helm_args)
  OBSERVABILITY_HELM_ARGS+=(--set "amObserver.ocIngress.gatewayName=${CONSOLIDATED_GATEWAY}"
                            --set "amObserver.ocIngress.gatewayNamespace=${GATEWAY_NS}")

  DP_EXTERNAL_INGRESS="$(dataplane_external_ingress)"; export DP_EXTERNAL_INGRESS
  # Deployed-agent routes are generated by OpenChoreo against this gateway; point them at
  # the consolidated :443 gateway (install.sh reads these; defaults keep the simple path).
  export DP_EXTERNAL_GATEWAY_NAME="${CONSOLIDATED_GATEWAY}"
  export DP_EXTERNAL_GATEWAY_NAMESPACE="${GATEWAY_NS}"
  export VERSION="$AMP_VERSION"
  export SHOW_LOCALHOST_URLS=false

  # Env-Thunder deployment-wide config (inherited by install_default_env_thunder). The
  # wildcard cert is publicly trusted, so env-Thunder trusts platform Thunder's HTTPS via
  # the container's default trust store — no custom CA bundle needed. AMP_HOST_THUNDER is
  # "thunder.<DOMAIN_BASE>"; stripping "thunder." gives env-Thunder's base domain.
  export THUNDER_HOST_BASE_DOMAIN="${AMP_HOST_THUNDER#thunder.}"
  export TLS_ENABLED=true
  export SKIP_CA_BUNDLE_TRUST=true
  export PLATFORM_THUNDER_ISSUER="https://${AMP_HOST_THUNDER}"
  export PLATFORM_THUNDER_JWKS_URL="https://${AMP_HOST_THUNDER}/oauth2/jwks"

  # k3d: publish :443 (the consolidated gateway) to the host; keep the plane ports
  # loopback-bound (only :443 faces the network).
  render_k3d_advanced_config <"${QS_DIR}/k3d-config.yaml" >/tmp/k3d-config-vm.yaml
  export K3D_CONFIG=/tmp/k3d-config-vm.yaml
  render_coredns_vm_config "k3d-amp-local-server-0" >/tmp/coredns-amp-vm.yaml
  export COREDNS_FILE=/tmp/coredns-amp-vm.yaml

  log "Running base installer with custom-domain overrides (DNS-01)"
  local rc=0
  ( set +e; source "${QS_DIR}/install.sh" ) || rc=$?
  [[ "$rc" -eq 0 ]] || die "Base installer exited $rc"

  log "Phase 3/3: issue TLS certificate (cert-manager DNS-01) + expose :443"
  apply_advanced_tls

  log "Done. Access URLs:"
  cat <<EOF

  Console:   https://${AMP_HOST_CONSOLE}
  API:       https://${AMP_HOST_API}
  Thunder:   https://${AMP_HOST_THUNDER}
  Observer:  https://${AMP_HOST_OBSERVER}
  OTel ingest: https://${AMP_HOST_GATEWAY}/otel
  Deployed agents: https://<org>-<project>.${AMP_AGENTS_BASE}/...
EOF
  [[ -n "$AMP_HOST_CP" ]] && echo "  Gateway control plane: https://${AMP_HOST_CP}  (connect external gateways here; registration token is secret-bearing)"
}

if [[ "$DRY_RUN" == "true" ]]; then
  log "DRY RUN — derived hosts:"
  printf '  console=%s api=%s thunder=%s observer=%s gateway=%s cp=%s agents=%s\n' \
    "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" "$AMP_HOST_OBSERVER" \
    "$AMP_HOST_GATEWAY" "${AMP_HOST_CP:-<none>}" "$AMP_AGENTS_BASE"
  log "DRY RUN — amp helm args:"; amp_helm_args
  log "DRY RUN — wildcard cert SANs:"; cert_dns_names
  log "DRY RUN — cert-manager resources:"
  render_dns01_credentials_secret "$DNS01_SECRET"; echo "---"
  render_acme_clusterissuer "$ACME_ISSUER" "$DNS01_SECRET"; echo "---"
  render_wildcard_certificate "$WILDCARD_CERT" "$WILDCARD_SECRET" "$ACME_ISSUER"; echo "---"
  render_consolidated_gateway "$CONSOLIDATED_GATEWAY" "$WILDCARD_SECRET" 443
  log "DRY RUN — DNS pre-flight (advisory):"; preflight_dns
  exit 0
fi

run_advanced_install
