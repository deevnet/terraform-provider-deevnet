#!/bin/bash
#
# install-provider.sh - put the Terraform providers a Deevnet tenant needs
# where terraform finds them, on macOS or Linux, with no Go and no registry.
#
#   bash install-provider.sh            from the site's tenant downloads (fast, local)
#   bash install-provider.sh --github   from GitHub releases (off-site)
#
# It installs, into terraform's implicit local mirror
# (~/.terraform.d/plugins/registry.terraform.io/<ns>/<name>/<ver>/<os>_<arch>/):
#   deevnet/deevnet  @VERSION@   the Deevnet provider
#   grafana/grafana  @GRAFANA@   for dashboards as code (local mirror only;
#                                off-site, terraform fetches it from the registry)
#
# Every zip is checked against its SHA256SUMS before it is unpacked. The
# mobile site's CA is embedded below, so the local download is verified with
# nothing else on the laptop. Needs only curl, unzip and shasum or sha256sum.

set -euo pipefail

VERSION="${DEEVNET_PROVIDER_VERSION:-@VERSION@}"
GRAFANA="@GRAFANA@"
BASE="${DEEVNET_DOWNLOADS:-https://downloads.mobile.deevnet.net:8443}"
GITHUB="https://github.com/deevnet/terraform-provider-deevnet/releases/download/v$VERSION"
SOURCE=local
case "${1:-}" in
  --github) SOURCE=github ;;
  "") ;;
  *) echo "usage: bash $0 [--github]" >&2; exit 2 ;;
esac

case "$(uname -s)" in Darwin) OS=darwin ;; Linux) OS=linux ;; *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) ARCH=amd64 ;; arm64|aarch64) ARCH=arm64 ;; *) echo "unsupported CPU: $(uname -m)" >&2; exit 1 ;; esac
for t in curl unzip; do command -v $t >/dev/null || { echo "missing: $t" >&2; exit 1; }; done
if command -v sha256sum >/dev/null; then SHA="sha256sum"; else SHA="shasum -a 256"; fi

TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
CA="$TMP/site-ca.pem"
cat > "$CA" <<'PEM'
-----BEGIN CERTIFICATE-----
MIIDOzCCAiOgAwIBAgIUNFURi+BYDXa773TCiFB57bjC7OkwDQYJKoZIhvcNAQEL
BQAwJTEjMCEGA1UEAxMaRGVldm5ldCBtb2JpbGUgaW50ZXJuYWwgQ0EwHhcNMjYw
OTE3MjM1NjA2WhcNMzYwOTE0MjM1NjM2WjAlMSMwIQYDVQQDExpEZWV2bmV0IG1v
YmlsZSBpbnRlcm5hbCBDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AM7UYiY+Zar9LzJZubkQqVFulWBxXkxteJt6n6qQzQ4JBpJ+Fns12ZOazFQPJhcE
Lrjsod9f+xmo5VlObQewKCCd8EsK+DiXOI2ky44rfzoFRGJ95N/bNp+ohS/neBIi
oDix1feTVx0d6A+Oto/2vGJzqWbb0EAOwnFKowqdfUNS40cgSeVo3bm03uN9iOmk
Zmn/cjfI2ZEfcJ007ADxiJmX7bfXhOUbvrMKoShM3mmpOC8TVo4c8b5bsHwjOUJU
BIo6KHsOCBu1EePUXvz7vMC7//EWu1FMUp0UcLE0pxuLHMrUA80BVqqbmVJpB3AK
4bK6mAG9zGEeJtE+N2muk3ECAwEAAaNjMGEwDgYDVR0PAQH/BAQDAgEGMA8GA1Ud
EwEB/wQFMAMBAf8wHQYDVR0OBBYEFJKXklMLQkz8utmOfxlCD3cWlqnGMB8GA1Ud
IwQYMBaAFJKXklMLQkz8utmOfxlCD3cWlqnGMA0GCSqGSIb3DQEBCwUAA4IBAQCQ
D/V6muGHw5C8GFRUcIxK4VY/bhl5omj/IC1ma3a5iZMTWnyQKRHQT8EXDKWySjKK
603JD3EVRyvIWJKYa0tpdv0oBR+vUIpGJiC1CNHQfXyXtHHYWqE7vR0tuMFRZ0xn
b3EXKC3FuOSicwGsIZ4ezbqMxEHueniJ6Upw24PXvtLd0WI0JlQXu0+3OHaSkRQf
0bNw/Ie/GbiQ5cys2K60X27kl+X/HxthCPnB+2VhFoE4zRfLUMm23rlJOQN4t+8W
3Fp1LHrcQzpZdlilEJH7wBdqyQ/RDp4Tv/RSd0pfsbbqmjPoZOLx3GCWjCVY93GH
gmqLFDJRFNdwqOjx4/3N
-----END CERTIFICATE-----
PEM

fetch() { # url dest
  # Say what is coming: the Grafana provider is ~28 MB, and a silent curl
  # that long looks like a hang.
  echo "downloading $(basename "$1") ..."
  if [ "$SOURCE" = local ]; then curl -fsSL --cacert "$CA" -o "$2" "$1"; else curl -fsSL -o "$2" "$1"; fi
}

# install <namespace> <name> <version> <base-url> <sums-file-name>
install_one() {
  local ns=$1 name=$2 ver=$3 base=$4 sums=$5
  local zip="terraform-provider-${name}_${ver}_${OS}_${ARCH}.zip"
  local dest="$HOME/.terraform.d/plugins/registry.terraform.io/$ns/$name/$ver/${OS}_${ARCH}"
  fetch "$base/$zip" "$TMP/$zip"
  fetch "$base/$sums" "$TMP/$sums"
  local want got
  want=$(awk -v z="$zip" '$2==z{print $1}' "$TMP/$sums")
  got=$($SHA "$TMP/$zip" | awk '{print $1}')
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "checksum mismatch for $zip - refusing to install it" >&2; exit 1
  fi
  mkdir -p "$dest"
  unzip -qo "$TMP/$zip" -d "$dest"
  echo "installed $ns/$name $ver (${OS}_${ARCH}) -> $dest"
}

if [ "$SOURCE" = local ]; then
  install_one deevnet deevnet "$VERSION" "$BASE/provider/$VERSION" SHA256SUMS
  install_one grafana grafana "$GRAFANA" "$BASE/providers/grafana/$GRAFANA" "terraform-provider-grafana_${GRAFANA}_SHA256SUMS"
else
  install_one deevnet deevnet "$VERSION" "$GITHUB" SHA256SUMS
fi
echo
echo "Pin it with:  deevnet = { source = \"deevnet/deevnet\", version = \"~> ${VERSION%.*}\" }"
