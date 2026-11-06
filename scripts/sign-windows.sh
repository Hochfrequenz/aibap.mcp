#!/usr/bin/env bash
# Authenticode-signs one Windows binary through Azure Trusted Signing.
#
# Called by GoReleaser as a post-build hook for every build target (see
# .goreleaser.yaml), so it signs the binary in place before it is archived
# and before checksums.txt is computed. Arguments: the binary's path and its
# extension; anything that is not an .exe is left alone.
#
# Runs on Linux: jsign speaks the Trusted Signing REST API directly, so no
# Windows runner or signtool is needed. It authenticates with an access token
# for https://codesigning.azure.net, taken from the Azure CLI session that the
# release workflow's `azure/login` step opened over OIDC. No client secret
# exists anywhere.
#
# Required environment (set by .github/workflows/release.yml):
#   JSIGN_JAR                path to the jsign jar, already hash-checked
#   AZURE_SIGNING_ENDPOINT   e.g. https://neu.codesigning.azure.net/
#   AZURE_SIGNING_ACCOUNT    Trusted Signing account name
#   AZURE_SIGNING_PROFILE    certificate profile name
#
# Without AZURE_SIGNING_ENDPOINT (a local `goreleaser build --snapshot`) the
# binary is left unsigned and a note says so. The release workflow refuses to
# run without the variables and checks every .exe for a signature afterwards,
# so an unsigned binary cannot reach a release through this skip.
set -euo pipefail

bin="$1"
ext="${2:-}"

if [ "$ext" != ".exe" ]; then
  exit 0
fi

if [ -z "${AZURE_SIGNING_ENDPOINT:-}" ]; then
  echo "sign-windows: AZURE_SIGNING_ENDPOINT not set, leaving ${bin} unsigned (local build)" >&2
  exit 0
fi

: "${JSIGN_JAR:?JSIGN_JAR must point at the jsign jar}"
: "${AZURE_SIGNING_ACCOUNT:?}"
: "${AZURE_SIGNING_PROFILE:?}"

# jsign wants the bare host, not the URL the Azure portal shows.
host="${AZURE_SIGNING_ENDPOINT#https://}"
host="${host%%/*}"

# Passed to jsign by variable name (`env:`), so the token never appears on a
# command line or in the log.
AZURE_SIGNING_TOKEN="$(az account get-access-token \
  --resource https://codesigning.azure.net \
  --query accessToken --output tsv)"
export AZURE_SIGNING_TOKEN

java -jar "$JSIGN_JAR" \
  --storetype TRUSTEDSIGNING \
  --keystore "$host" \
  --storepass env:AZURE_SIGNING_TOKEN \
  --alias "${AZURE_SIGNING_ACCOUNT}/${AZURE_SIGNING_PROFILE}" \
  --alg SHA-256 \
  --tsaurl http://timestamp.acs.microsoft.com \
  --tsmode RFC3161 \
  "$bin"

echo "sign-windows: signed ${bin}"
