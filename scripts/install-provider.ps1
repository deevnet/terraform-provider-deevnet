# install-provider.ps1 - put the Terraform providers a Deevnet tenant needs
# where terraform finds them, on Windows, with no Go and no registry.
# install-provider.sh is the same for macOS, Linux and WSL.
#
#   powershell -ExecutionPolicy Bypass -File install-provider.ps1           from the site's tenant downloads
#   powershell -ExecutionPolicy Bypass -File install-provider.ps1 -GitHub   from GitHub releases (off-site)
#
# It installs, into terraform's implicit local mirror on Windows
# (%APPDATA%\terraform.d\plugins\registry.terraform.io\<ns>\<name>\<ver>\windows_<arch>\):
#   deevnet/deevnet  @VERSION@   the Deevnet provider
#   grafana/grafana  @GRAFANA@   for dashboards as code (local mirror only;
#                                off-site, terraform fetches it from the registry)
#
# Every zip is checked against its SHA256SUMS before it is unpacked. The
# Deevnet Root CA is embedded below, so the local download is verified with
# nothing else on the laptop. Needs only Windows PowerShell 5.1 (or 7) and the
# curl.exe Windows 10 and 11 ship.
#
# Plain ASCII on purpose: Windows PowerShell 5.1 reads a script with no BOM
# as the local code page.

param([switch]$GitHub)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2

$Version = if ($env:DEEVNET_PROVIDER_VERSION) { $env:DEEVNET_PROVIDER_VERSION } else { '@VERSION@' }
$Grafana = '@GRAFANA@'
$Base    = if ($env:DEEVNET_DOWNLOADS) { $env:DEEVNET_DOWNLOADS } else { 'https://downloads.mobile.deevnet.net' }
$GitHubBase = "https://github.com/deevnet/terraform-provider-deevnet/releases/download/v$Version"

if (-not (Get-Command curl.exe -ErrorAction SilentlyContinue)) {
  throw 'missing: curl.exe (built into Windows 10 1803 and later)'
}

# The platform is the one terraform runs as, which is what it looks for in the
# mirror: an amd64 terraform under emulation on an ARM laptop wants amd64.
$Arch = $null
if (Get-Command terraform -ErrorAction SilentlyContinue) {
  try { $Arch = ((terraform version -json | ConvertFrom-Json).platform -replace '^windows_', '') } catch { $Arch = $null }
}
if (-not $Arch) {
  $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
  switch ($cpu) {
    'AMD64' { $Arch = 'amd64' }
    'ARM64' { $Arch = 'arm64' }
    default { throw "unsupported CPU: $cpu" }
  }
}
if ($Arch -notin @('amd64', 'arm64')) { throw "unsupported platform: windows_$Arch" }

$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("deevnet-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $Tmp | Out-Null
$CA = Join-Path $Tmp 'deevnet-root-ca.pem'
@'
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
'@ | Set-Content -Path $CA -Encoding Ascii

function Fetch([string]$Url, [string]$Dest) {
  # Say what is coming: the Grafana provider is ~28 MB, and a silent download
  # that long looks like a hang.
  Write-Host "downloading $(Split-Path -Leaf $Url) ..."
  # curl.exe, not curl: in Windows PowerShell 5.1 curl is Invoke-WebRequest.
  # Windows' curl checks revocation through Schannel, which refuses a chain
  # whose revocation it cannot check; DVNTM-TD may not reach where a private
  # CA would publish one, so best effort, not refusal. The CA pins the chain.
  if ($GitHub) { & curl.exe -fsSL -o $Dest $Url }
  else { & curl.exe -fsSL --ssl-revoke-best-effort --cacert $CA -o $Dest $Url }
  if ($LASTEXITCODE -ne 0) { throw "download failed (curl exit $LASTEXITCODE): $Url" }
}

function Install-One([string]$Ns, [string]$Name, [string]$Ver, [string]$From, [string]$Sums) {
  $zip  = "terraform-provider-${Name}_${Ver}_windows_${Arch}.zip"
  $dest = Join-Path $env:APPDATA "terraform.d\plugins\registry.terraform.io\$Ns\$Name\$Ver\windows_$Arch"
  Fetch "$From/$zip"  (Join-Path $Tmp $zip)
  Fetch "$From/$Sums" (Join-Path $Tmp $Sums)
  $want = $null
  foreach ($line in Get-Content (Join-Path $Tmp $Sums)) {
    $f = -split $line
    if ($f.Count -ge 2 -and $f[1] -eq $zip) { $want = $f[0].ToLower() }
  }
  $got = (Get-FileHash -Algorithm SHA256 (Join-Path $Tmp $zip)).Hash.ToLower()
  if (-not $want -or $want -ne $got) { throw "checksum mismatch for $zip - refusing to install it" }
  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  Expand-Archive -Force -Path (Join-Path $Tmp $zip) -DestinationPath $dest
  Write-Host "installed $Ns/$Name $Ver (windows_$Arch) -> $dest"
}

try {
  if ($GitHub) {
    Install-One deevnet deevnet $Version $GitHubBase 'SHA256SUMS'
  } else {
    Install-One deevnet deevnet $Version "$Base/provider/$Version" 'SHA256SUMS'
    Install-One grafana grafana $Grafana "$Base/providers/grafana/$Grafana" "terraform-provider-grafana_${Grafana}_SHA256SUMS"
  }
} finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

$minor = $Version -replace '\.[^.]*$', ''
Write-Host ''
Write-Host "Pin it with:  deevnet = { source = `"deevnet/deevnet`", version = `"~> $minor`" }"
