#!/bin/bash
#
# tenant-check.sh - is this laptop ready to be a Deevnet tenant developer?
#
#   bash tenant-check.sh              tools, then the site's services (on DVNTM-TD)
#   bash tenant-check.sh --offline    tools only (at home, before the meetup)
#   bash tenant-check.sh --write-ca . also write the site CA as ./site-ca.pem
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

# --- Site CA (mobile internal CA, valid to 2036-09-14) ---------------------------
# SHA-256 fingerprint ED:ED:43:04:B8:40:8A:CE:14:FE:B8:AB:6C:B6:43:BC:A5:56:84:E8:26:A3:C2:75:CD:BD:EB:70:DF:E5:2D:5C
CA=$(mktemp); trap 'rm -f "$CA"' EXIT
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
if [ -n "$WRITE_CA" ]; then
  mkdir -p "$WRITE_CA" && cp "$CA" "$WRITE_CA/site-ca.pem" && echo "wrote $WRITE_CA/site-ca.pem"
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
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "http://tfstate.mobile.deevnet.net:9000/minio/health/live")
  case "$code" in [1-5][0-9][0-9]) ok "state store http://tfstate.mobile.deevnet.net:9000 (HTTP $code)" ;; *) miss "state store tfstate.mobile.deevnet.net:9000 did not answer" "are you on DVNTM-TD? if so, tell the operator" ;; esac
  tls mqtt.mobile.deevnet.net 8883 "MQTT broker"
  tls dv02obs001v01.mobile.deevnet.net 8427 "log store"
  https "https://dv02obs001v01.mobile.deevnet.net:3000/api/health" "Grafana"
  https "$DOWNLOADS/" "tenant downloads"
fi

echo
if [ $M -gt 0 ]; then echo "RESULT: $M missing, $W to consider, $P ok"; exit 1; fi
echo "RESULT: ready ($P ok, $W to consider)"
