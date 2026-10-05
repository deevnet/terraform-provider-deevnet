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
# mobile site's root CA is embedded below, so the local download is verified with
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
CA="$TMP/deevnet-root-ca.pem"
cat > "$CA" <<'PEM'
-----BEGIN CERTIFICATE-----
MIIFUzCCAzugAwIBAgIQRsdIVg5XBHXG6FyTpxtZNTANBgkqhkiG9w0BAQsFADBC
MRAwDgYDVQQKDAdEZWV2bmV0MRQwEgYDVQQLDAtEZWV2bmV0IFBLSTEYMBYGA1UE
AwwPRGVldm5ldCBSb290IENBMB4XDTI2MTAwNDEwNDYyNVoXDTQ2MTAwNDEwNDYy
NVowQjEQMA4GA1UECgwHRGVldm5ldDEUMBIGA1UECwwLRGVldm5ldCBQS0kxGDAW
BgNVBAMMD0RlZXZuZXQgUm9vdCBDQTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCC
AgoCggIBAIji/nhm9S3Uf9JwUgJR6u6qjgI2oKBeQKoqtWlL3Sc22naaTSWBrM2V
BvSJ8TJfLJ25Z4OTlSRHBGKxBu9YQU3TPIEorxUMdRav1L+o2J3b/EMrS6MR0dNe
n9ifEYlWNGccJ1UVdJ8RItXCB+PVtWrHit54UaqtRd6F4M6QsOd7zyoyoCRXmCHi
n5Bt9ggu1fBLvy/x5tTIcMM14eK8Nm3G/K9/VJHtQFp6kGwSL4CjTTG89PV6RZFO
agfacj1yQr14pPsMg8hCaTlipXiRY4pb5trAEia3DtvWfxOMZL9mfvbYhngceoc+
bYY8r53zDZpFBDcFxmQgtYouvRjXr2wg5qD0iIlXYnfKkCzWMQt7gF6/goK54GUT
aNrwk5PZ136RSLRpE5knfwCCB9DvS5sCqIT+dyLPIfxgh6vyD/4/ffgzNIpB5I7n
dBx89W4U0yFRuNb6W842sIdI/LQ49VbL4OE3hql127BOwV74UIghIuOZ+DiIU7R2
xSAmk2N/lfQKhRD7Aha7MZz4qeVQoWLTCam/7a0gf28sk4oXkBCiJ4w1ievsFNV7
VO7BuCLzBKrHdVkwXuntOuYe7niN5fF4NrxAU3z7COBjAX3dR+n1zcsz1mqkymR0
PFp8CoMfQLiiM3dhlXRYJAPNp04mAuG2lVPx2bl+tAXe3IEmU6SZAgMBAAGjRTBD
MBIGA1UdEwEB/wQIMAYBAf8CAQIwDgYDVR0PAQH/BAQDAgEGMB0GA1UdDgQWBBTE
BPg3Qx5ktLloUrHpDc+KLOmxtzANBgkqhkiG9w0BAQsFAAOCAgEAIe0XNeXOUcpQ
9SgLjruq1lPb5aPtDi2K69kd5RCoJgqvr6YesCiBUCfbvsssViqyodCb1uQg119L
FhEUwt3oj2vK+7eP7eUKdH8itYA69UuAk3djSFeFLWctnuK5NIeqkLpWwF/PHlS6
W/JAbK/nwCFLilZVZQaXVbwy+NdUgpALo9CLa6k4boGsm9p1V92WlbUddqADC0nF
wBXboYguz8teOOLS0GTAi7iuScniD3q83iP+72HxzWK9wl9p/SUdrgdK5Ur2mRzM
Lo9iSSCA/WmEnXNhRRjwWugiMsRTKKEgbehhTp2T/IllxO82UJaOricf2rr/yQSe
L7Hi3tlhfz5/p8EHRiXn+f6umNxOSBUWgNdqgMLii1vy8A3v+++nI7NmFvsnIzNN
plIYVLSZvJtR6JwB/82945eky1cqrdVY/gmzf8ToJ+uvNVT8thZSApnC6IoVLg5G
vEtl2pWIao0q0ls9qsr10K8RSgTWqa8Np53Hs3mK3/H2Al1CdH5U+CHDOmuuXcZp
pJbFsspPYfE0qLHb75zvYJOmwwAjhxXcmL/dTEVcc91WnuL8IQW6vFU3rIcO5Zi5
+Oc4leUEthiqfcJmxkE8xvXhchVQjUP2F/cMejandgu94Vk8IMMsWTM/QifnL1K+
NwUaqobwJXKkFzNpQTyAHoeZSWVPCi4=
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
