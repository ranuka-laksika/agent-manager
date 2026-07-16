#!/usr/bin/env bash
# lib-certmanager.sh — render the cert-manager resources that replace the old lego +
# Caddy TLS path for the advanced (DNS-01) VM install. Sourcing only defines functions
# (no side effects); the render_* functions write YAML to stdout, so the caller pipes
# them to `kubectl apply`. cert-manager (installed as a cluster prerequisite) then does
# the ACME DNS-01 challenge, issues a wildcard cert into a Secret, and auto-renews it —
# kgateway terminates TLS on :443 with that Secret. No lego container, no systemd timer.
#
# The caller defines log()/die(); fallbacks are provided so this file is usable standalone.
command -v log >/dev/null 2>&1 || log() { printf '\033[0;34m[certmgr]\033[0m %s\n' "$*"; }
command -v die >/dev/null 2>&1 || die() { printf '\033[0;31m[certmgr] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# The namespaces the cert-manager resources live in. The wildcard cert + its Secret must
# sit in the gateway's namespace so the consolidated :443 Gateway can reference it directly
# (a Gateway listener's certificateRefs is same-namespace by default).
CERT_MANAGER_NS="${CERT_MANAGER_NS:-cert-manager}"
GATEWAY_NS="${GATEWAY_NS:-openchoreo-control-plane}"

# The four DNS providers we support natively (cert-manager has a built-in dns01 solver for
# each). Kept identical to what the old lego path covered.
SUPPORTED_DNS_PROVIDERS="cloudflare route53 clouddns azuredns"

# dns01_required_vars <provider> — print the env-var names that MUST be set for the given
# provider (one per line). Used by validate_dns01_config. Empty output for an unknown
# provider (validate_dns01_config reports the unknown-provider error separately).
dns01_required_vars() {
  case "$1" in
    cloudflare) printf '%s\n' CLOUDFLARE_API_TOKEN ;;
    route53)    printf '%s\n' AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_REGION ;;
    clouddns)   printf '%s\n' GCP_PROJECT GCP_SERVICE_ACCOUNT_FILE ;;
    azuredns)   printf '%s\n' AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET AZURE_SUBSCRIPTION_ID AZURE_RESOURCE_GROUP ;;
  esac
}

# validate_dns01_config — confirm DNS_PROVIDER is one we support and every credential it
# needs is set. Appends to CONFIG_ERRORS (does not reset it — the caller owns that array).
validate_dns01_config() {
  local p="${DNS_PROVIDER:-}" v
  case " ${SUPPORTED_DNS_PROVIDERS} " in
    *" ${p} "*) ;;
    *) CONFIG_ERRORS+=("DNS_PROVIDER must be one of: ${SUPPORTED_DNS_PROVIDERS} (got '${p:-<unset>}')"); return ;;
  esac
  while IFS= read -r v; do
    [[ -n "${!v:-}" ]] || CONFIG_ERRORS+=("${v} is required for DNS_PROVIDER=${p}")
  done < <(dns01_required_vars "$p")
  # clouddns needs the service-account JSON file to exist and be readable.
  if [[ "$p" == clouddns && -n "${GCP_SERVICE_ACCOUNT_FILE:-}" && ! -r "${GCP_SERVICE_ACCOUNT_FILE}" ]]; then
    CONFIG_ERRORS+=("GCP_SERVICE_ACCOUNT_FILE not readable: ${GCP_SERVICE_ACCOUNT_FILE}")
  fi
}

# cert_dns_names — print the SAN hostnames (one per line) the wildcard cert must cover:
# every fixed service host + the deployed-agent wildcard + the env-Thunder wildcard. Reads
# AMP_HOST_*/AMP_AGENTS_BASE from the caller's scope (matches the lib-vm.sh cores). CP is
# omitted when AMP_HOST_CP is empty (external gateways off).
# shellcheck disable=SC2154,SC2153  # AMP_HOST_*/AMP_AGENTS_BASE come from the caller's scope by design.
cert_dns_names() {
  printf '%s\n' "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" \
    "$AMP_HOST_OBSERVER" "$AMP_HOST_GATEWAY"
  [[ -n "${AMP_HOST_CP:-}" ]] && printf '%s\n' "$AMP_HOST_CP"
  # Dynamic tiers (created after install): deployed agents <org>-<project>.<AGENTS_BASE>
  # and env-Thunder <org>-<env>.<THUNDER_HOST>. A wildcard covers each without re-issuing.
  printf '*.%s\n' "$AMP_AGENTS_BASE"
  printf '*.%s\n' "$AMP_HOST_THUNDER"
}

# _dns01_solver_block — print the cert-manager `dns01:` solver body for DNS_PROVIDER,
# indented 10 spaces to sit under `solvers:\n  - dns01:` in render_acme_clusterissuer.
# Credential values come from the config env vars; the referenced Secret is created by
# render_dns01_credentials_secret. Reads DNS_PROVIDER + provider vars from the environment.
_dns01_solver_block() {
  local secret="$1"
  case "$DNS_PROVIDER" in
    cloudflare)
      cat <<EOF
          cloudflare:
            apiTokenSecretRef:
              name: ${secret}
              key: api-token
EOF
      ;;
    route53)
      cat <<EOF
          route53:
            region: ${AWS_REGION}
            accessKeyID: ${AWS_ACCESS_KEY_ID}
            secretAccessKeySecretRef:
              name: ${secret}
              key: secret-access-key
EOF
      [[ -n "${AWS_HOSTED_ZONE_ID:-}" ]] && printf '            hostedZoneID: %s\n' "$AWS_HOSTED_ZONE_ID"
      ;;
    clouddns)
      cat <<EOF
          cloudDNS:
            project: ${GCP_PROJECT}
            serviceAccountSecretRef:
              name: ${secret}
              key: service-account.json
EOF
      ;;
    azuredns)
      cat <<EOF
          azureDNS:
            clientID: ${AZURE_CLIENT_ID}
            clientSecretSecretRef:
              name: ${secret}
              key: client-secret
            subscriptionID: ${AZURE_SUBSCRIPTION_ID}
            tenantID: ${AZURE_TENANT_ID}
            resourceGroupName: ${AZURE_RESOURCE_GROUP}
            hostedZoneName: ${AZURE_HOSTED_ZONE_NAME:-${DOMAIN_BASE}}
EOF
      ;;
    *) die "_dns01_solver_block: unsupported DNS_PROVIDER '${DNS_PROVIDER}'" ;;
  esac
}

# render_dns01_credentials_secret <secret_name> — print an Opaque Secret in CERT_MANAGER_NS
# holding the provider credential the dns01 solver reads. Only the value that must stay
# secret goes here (tokens/keys); non-secret fields like region/project are set inline in
# the ClusterIssuer. Reads DNS_PROVIDER + provider vars from the environment.
render_dns01_credentials_secret() {
  local name="$1"
  printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n' \
    "$name" "$CERT_MANAGER_NS"
  case "$DNS_PROVIDER" in
    cloudflare) printf '  api-token: %s\n' "$(_yaml_quote "$CLOUDFLARE_API_TOKEN")" ;;
    route53)    printf '  secret-access-key: %s\n' "$(_yaml_quote "$AWS_SECRET_ACCESS_KEY")" ;;
    azuredns)   printf '  client-secret: %s\n' "$(_yaml_quote "$AZURE_CLIENT_SECRET")" ;;
    clouddns)
      # The whole service-account JSON is the secret; embed it as a block scalar.
      printf '  service-account.json: |\n'
      sed 's/^/    /' "$GCP_SERVICE_ACCOUNT_FILE"
      ;;
    *) die "render_dns01_credentials_secret: unsupported DNS_PROVIDER '${DNS_PROVIDER}'" ;;
  esac
}

# _yaml_quote <value> — single-quote a scalar for YAML (doubling any embedded single
# quotes), so tokens containing YAML-significant characters are passed through verbatim.
_yaml_quote() { printf "'%s'" "${1//\'/\'\'}"; }

# render_acme_clusterissuer <issuer_name> <cred_secret_name> — print the ACME DNS-01
# ClusterIssuer. cert-manager registers the ACME account (email) and stores its key in
# <issuer>-account-key. ACME_SERVER overrides the CA (e.g. LE staging for testing).
# Reads ACME_EMAIL, ACME_SERVER (optional), DNS_PROVIDER + provider vars.
render_acme_clusterissuer() {
  local issuer="$1" secret="$2" server="${ACME_SERVER:-https://acme-v02.api.letsencrypt.org/directory}"
  cat <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ${issuer}
spec:
  acme:
    email: ${ACME_EMAIL}
    server: ${server}
    privateKeySecretRef:
      name: ${issuer}-account-key
    solvers:
      - dns01:
$(_dns01_solver_block "$secret")
EOF
}

# render_wildcard_certificate <cert_name> <secret_name> <issuer_name> — print the
# Certificate whose issued Secret the consolidated :443 Gateway references. dnsNames come
# from cert_dns_names (fixed hosts + the agent/env-Thunder wildcards). Lives in GATEWAY_NS.
render_wildcard_certificate() {
  local name="$1" secret="$2" issuer="$3" d
  cat <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
spec:
  secretName: ${secret}
  duration: 2160h
  renewBefore: 720h
  privateKey:
    algorithm: RSA
    size: 2048
  dnsNames:
EOF
  while IFS= read -r d; do
    [[ -n "$d" ]] && printf '    - %s\n' "$(_yaml_quote "$d")"
  done < <(cert_dns_names)
  cat <<EOF
  issuerRef:
    name: ${issuer}
    kind: ClusterIssuer
    group: cert-manager.io
EOF
}

# render_consolidated_gateway <name> <cert_secret> [port] — print the single HTTPS
# Gateway that fronts all three planes on :443 (validated in the consolidation spike).
# It terminates TLS with the cert-manager wildcard Secret and accepts HTTPRoutes from
# ANY namespace (from: All), so the control-plane, observability, and data-plane routes
# all attach to it cross-namespace. Lives in GATEWAY_NS (same as the cert Secret).
render_consolidated_gateway() {
  local name="$1" secret="$2" port="${3:-443}"
  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
spec:
  gatewayClassName: kgateway
  listeners:
    - name: https
      port: ${port}
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
          - name: ${secret}
      allowedRoutes:
        namespaces:
          from: All
EOF
}
