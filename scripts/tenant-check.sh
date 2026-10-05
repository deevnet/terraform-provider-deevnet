#!/bin/bash
#
# tenant-check.sh - is this laptop ready to be a Deevnet tenant developer?
#
#   bash tenant-check.sh              tools, then the site's services (on DVNTM-TD)
#   bash tenant-check.sh --offline    tools only (at home, before the meetup)
#   bash tenant-check.sh --write-ca . also write the Deevnet Root CA as ./deevnet-root-ca.pem
#
# Read-only apart from --write-ca: it looks for tools, resolves names and
# opens connections. macOS first; Linux (dnf or apt) too. Each missing tool
# comes with the command that installs it on this machine.
#
#   ok       present / reachable
#   MISSING  required - the exit status says so
#   warn     only needed for some of the work (devices, taking it home)

set -u

VERSION="@VERSION@"
DOWNLOADS="https://downloads.mobile.deevnet.net:8443"
OFFLINE=0; WRITE_CA=""
while [ $# -gt 0 ]; do
  case "$1" in
    --offline) OFFLINE=1 ;;
    --write-ca) WRITE_CA="${2:?--write-ca needs a directory}"; shift ;;
    *) echo "usage: bash $0 [--offline] [--write-ca DIR]" >&2; exit 2 ;;
  esac
  shift
done

# --- Site root (Deevnet mobile root CA, valid to 2046-10-02; ADR-0030) ---------
# SHA-256 fingerprint F6:8A:BD:B3:1E:A5:6D:0A:88:1F:31:28:56:8A:4C:14:B0:3A:3F:5C:3F:38:CC:F1:7C:C4:F0:09:8B:EB:94:52
CA=$(mktemp); trap 'rm -f "$CA"' EXIT
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
if [ -n "$WRITE_CA" ]; then
  mkdir -p "$WRITE_CA" && cp "$CA" "$WRITE_CA/deevnet-root-ca.pem" && echo "wrote $WRITE_CA/deevnet-root-ca.pem"
fi

# --- Where are we ------------------------------------------------------------------
case "$(uname -s)" in Darwin) OS=darwin; PM=brew ;; Linux) OS=linux
  if command -v dnf >/dev/null; then PM=dnf; elif command -v apt-get >/dev/null; then PM=apt; else PM=other; fi ;;
  *) OS=other; PM=other ;; esac
case "$(uname -m)" in x86_64|amd64) ARCH=amd64 ;; arm64|aarch64) ARCH=arm64 ;; *) ARCH=$(uname -m) ;; esac

# hint <tool>: how to install it here
hint() {
  case "$PM:$1" in
    brew:git) echo "xcode-select --install" ;;
    brew:terraform) echo "brew install hashicorp/tap/terraform   (or unzip $DOWNLOADS/terraform/)" ;;
    dnf:terraform|apt:terraform|other:terraform) echo "unzip terraform from $DOWNLOADS/terraform/ into ~/.local/bin (or HashiCorp's $PM repo)" ;;
    *:provider) echo "bash install-provider.sh   (from $DOWNLOADS/scripts/)" ;;
    brew:dig) echo "brew install bind" ;;  dnf:dig) echo "sudo dnf install bind-utils" ;;  apt:dig) echo "sudo apt install dnsutils" ;;
    brew:mosquitto) echo "brew install mosquitto" ;;  dnf:mosquitto) echo "sudo dnf install mosquitto" ;;  apt:mosquitto) echo "sudo apt install mosquitto-clients" ;;
    brew:python3) echo "brew install python" ;;  dnf:python3) echo "sudo dnf install python3" ;;  apt:python3) echo "sudo apt install python3" ;;
    *:mpremote) echo "python3 -m pip install --user mpremote   (or pipx install mpremote)" ;;
    brew:podman) echo "brew install podman && podman machine init && podman machine start" ;;
    dnf:podman) echo "sudo dnf install podman qemu-user-static" ;;  apt:podman) echo "sudo apt install podman qemu-user-static" ;;
    *:mdns) echo "sudo $PM install nss-mdns avahi (Fedora) / libnss-mdns (Debian), then enable avahi-daemon" ;;
    brew:*) echo "brew install $1" ;;  dnf:*) echo "sudo dnf install $1" ;;  apt:*) echo "sudo apt install $1" ;;
    *) echo "install $1" ;;
  esac
}

P=0; M=0; W=0
ok()   { printf '  ok       %s\n' "$1"; P=$((P+1)); }
miss() { printf '  MISSING  %s\n           -> %s\n' "$1" "$2"; M=$((M+1)); }
warn() { printf '  warn     %s\n           -> %s\n' "$1" "$2"; W=$((W+1)); }
have() { command -v "$1" >/dev/null 2>&1; }

echo "Deevnet tenant check - $OS/$ARCH ($PM)"
echo
echo "== To apply your Terraform"
have git && ok "git" || miss "git" "$(hint git)"
if have terraform; then
  tv=$(terraform version 2>/dev/null | head -1 | sed 's/^Terraform v//')
  case "$tv" in 0.*|1.[0-4].*) miss "terraform $tv (need 1.5 or later)" "$(hint terraform)" ;; *) ok "terraform $tv" ;; esac
else miss "terraform" "$(hint terraform)"; fi
pdir="$HOME/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/$VERSION/${OS}_${ARCH}"
if ls "$pdir"/terraform-provider-deevnet* >/dev/null 2>&1; then ok "deevnet provider $VERSION"
else miss "deevnet provider $VERSION (not in $pdir)" "$(hint provider)"; fi
have curl && ok "curl" || miss "curl" "$(hint curl)"
have openssl && ok "openssl" || miss "openssl" "$(hint openssl)"

echo
echo "== To talk to your devices"
have mosquitto_pub && have mosquitto_sub && ok "mosquitto_pub / mosquitto_sub" || warn "mosquitto_pub / mosquitto_sub" "$(hint mosquitto)"
have dig && ok "dig" || warn "dig" "$(hint dig)"
have python3 && ok "python3" || warn "python3" "$(hint python3)"
have mpremote && ok "mpremote (Pico)" || warn "mpremote, to copy files to a Pico" "$(hint mpremote)"

echo
echo "== To take it home on a Pi"
if have podman || have docker; then ok "container build ($(have podman && echo podman || echo docker)) - your app image must be arm64"
else warn "podman or docker, to build your app for the Pi" "$(hint podman)"; fi
if [ "$OS" = darwin ]; then ok ".local names (built into macOS)"
elif grep -q mdns /etc/nsswitch.conf 2>/dev/null; then ok ".local names (nss-mdns)"
else warn ".local names, to reach <hostname>.local" "$(hint mdns)"; fi
echo "  -        Raspberry Pi Imager and an SD card reader: $DOWNLOADS/tools/"

if [ $OFFLINE -eq 0 ]; then
  echo
  echo "== The site's services (run this on DVNTM-TD)"
  https() { # url label
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 --cacert "$CA" "$1")
    case "$code" in [1-5][0-9][0-9]) ok "$2 $1 (HTTP $code, TLS verified)" ;; *) miss "$2 $1 did not answer" "are you on DVNTM-TD? if so, tell the operator" ;; esac
  }
  tls() { # host port label
    v=$(echo | openssl s_client -connect "$1:$2" -servername "$1" -CAfile "$CA" 2>&1 | grep -m1 'Verify return code')
    case "$v" in *"0 (ok)"*) ok "$3 $1:$2 (TLS verified)" ;; *) miss "$3 $1:$2 ${v:-no answer}" "are you on DVNTM-TD? if so, tell the operator" ;; esac
  }
  https "https://api.mobile.deevnet.net:8080/healthz" "Deevnet API"
  https "https://tfstate.mobile.deevnet.net:9000/minio/health/live" "state store"
  tls mqtt.mobile.deevnet.net 8883 "MQTT broker"
  tls dv02obs001v01.mobile.deevnet.net 8427 "log store"
  https "https://dv02obs001v01.mobile.deevnet.net:3000/api/health" "Grafana"
  https "$DOWNLOADS/" "tenant downloads"
fi

echo
if [ $M -gt 0 ]; then echo "RESULT: $M missing, $W to consider, $P ok"; exit 1; fi
echo "RESULT: ready ($P ok, $W to consider)"
