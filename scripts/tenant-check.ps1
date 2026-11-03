# tenant-check.ps1 - is this Windows laptop ready to be a Deevnet tenant developer?
# tenant-check.sh is the same for macOS, Linux and WSL.
#
#   powershell -ExecutionPolicy Bypass -File tenant-check.ps1                tools, then the site's services (on DVNTM-TD)
#   powershell -ExecutionPolicy Bypass -File tenant-check.ps1 -Offline       tools only (at home, before the meetup)
#   powershell -ExecutionPolicy Bypass -File tenant-check.ps1 -WriteCa .     also write the Deevnet Root CA as .\deevnet-root-ca.pem
#
# Read-only apart from -WriteCa: it looks for tools, resolves names and opens
# connections. Each missing tool comes with the winget command that installs it.
#
#   ok       present / reachable
#   MISSING  required - the exit status says so
#   warn     only needed for some of the work (devices, taking it home)
#
# Plain ASCII on purpose: Windows PowerShell 5.1 reads a script with no BOM
# as the local code page.

param([switch]$Offline, [string]$WriteCa = '')

Set-StrictMode -Version 2

$Version   = '@VERSION@'
$Downloads = 'https://downloads.mobile.deevnet.net'

# --- Site root (Deevnet Root CA, valid to 2046-10-02; ADR-0030) -------------------
# SHA-256 fingerprint F6:8A:BD:B3:1E:A5:6D:0A:88:1F:31:28:56:8A:4C:14:B0:3A:3F:5C:3F:38:CC:F1:7C:C4:F0:09:8B:EB:94:52
$CaPem = @'
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
'@
$CaFile = Join-Path ([IO.Path]::GetTempPath()) ("deevnet-root-ca-" + [Guid]::NewGuid() + ".pem")
Set-Content -Path $CaFile -Value $CaPem -Encoding Ascii
$CaCert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList $CaFile
if ($WriteCa) {
  New-Item -ItemType Directory -Force -Path $WriteCa | Out-Null
  $out = Join-Path $WriteCa 'deevnet-root-ca.pem'
  Set-Content -Path $out -Value $CaPem -Encoding Ascii
  Write-Host "wrote $out"
}

# --- Where are we ------------------------------------------------------------------
$cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$Arch = switch ($cpu) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { $cpu } }
$HasWinget = [bool](Get-Command winget -ErrorAction SilentlyContinue)

function Hint([string]$Tool) {
  switch ($Tool) {
    'git'       { 'winget install Git.Git   (also brings openssl and ssh-keygen)' }
    'terraform' { "winget install Hashicorp.Terraform   (or unzip $Downloads/terraform/ into a folder on PATH)" }
    'provider'  { "powershell -ExecutionPolicy Bypass -File install-provider.ps1   (from $Downloads/scripts/)" }
    'curl'      { 'curl.exe ships with Windows 10 1803 and later: update Windows' }
    'openssl'   { 'winget install Git.Git, then add C:\Program Files\Git\usr\bin to PATH' }
    'mosquitto' { 'winget install EclipseFoundation.Mosquitto, then add C:\Program Files\mosquitto to PATH' }
    'python'    { 'winget install Python.Python.3.12   (the Microsoft Store "python" stub does not count)' }
    'mpremote'  { 'python -m pip install --user mpremote' }
    'podman'    { 'winget install RedHat.Podman, then podman machine init; podman machine start' }
    'ssh'       { 'Settings > System > Optional features > OpenSSH Client' }
    default     { "install $Tool" }
  }
}

$script:P = 0; $script:M = 0; $script:W = 0
function Ok([string]$What)                 { Write-Host ("  ok       " + $What); $script:P++ }
function Miss([string]$What, [string]$How) { Write-Host ("  MISSING  " + $What); Write-Host ("           -> " + $How); $script:M++ }
function Warn([string]$What, [string]$How) { Write-Host ("  warn     " + $What); Write-Host ("           -> " + $How); $script:W++ }
function Have([string]$Cmd) { [bool](Get-Command $Cmd -ErrorAction SilentlyContinue) }

# The Microsoft Store's python.exe is a stub that opens the Store: on PATH, not a Python.
function Have-Python {
  foreach ($c in 'python', 'py') {
    $cmd = Get-Command $c -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source -notlike '*\WindowsApps\*') { return $true }
  }
  return $false
}

Write-Host "Deevnet tenant check - windows/$Arch ($(if ($HasWinget) { 'winget' } else { 'no winget' }))"
Write-Host ''
Write-Host '== To apply your Terraform'
if (Have git) { Ok 'git' } else { Miss 'git' (Hint git) }
if (Have terraform) {
  $tv = ''
  try { $tv = (terraform version -json | ConvertFrom-Json).terraform_version } catch { }
  if ($tv -match '^(0\.|1\.[0-4]\.)') { Miss "terraform $tv (need 1.5 or later)" (Hint terraform) } else { Ok "terraform $tv" }
} else { Miss 'terraform' (Hint terraform) }
$pdir = Join-Path $env:APPDATA "terraform.d\plugins\registry.terraform.io\deevnet\deevnet\$Version\windows_$Arch"
if (Test-Path (Join-Path $pdir 'terraform-provider-deevnet*.exe')) { Ok "deevnet provider $Version" }
else { Miss "deevnet provider $Version (not in $pdir)" (Hint provider) }
if (Have curl.exe) { Ok 'curl.exe' } else { Miss 'curl.exe' (Hint curl) }
# On macOS and Linux openssl checks the CA's fingerprint; here PowerShell can
# (Before You Start), so openssl is only needed to hash a Pi's password.
if (Have openssl) { Ok 'openssl' } else { Warn 'openssl, to hash a password for a Pi' (Hint openssl) }

Write-Host ''
Write-Host '== To talk to your devices'
if ((Have mosquitto_pub) -and (Have mosquitto_sub)) { Ok 'mosquitto_pub / mosquitto_sub' }
elseif (Test-Path 'C:\Program Files\mosquitto\mosquitto_pub.exe') { Warn 'mosquitto is installed but not on PATH' 'add C:\Program Files\mosquitto to PATH' }
else { Warn 'mosquitto_pub / mosquitto_sub' (Hint mosquitto) }
if (Have Resolve-DnsName) { Ok 'Resolve-DnsName (in place of dig)' } else { Warn 'Resolve-DnsName' 'Windows 8 or later has it' }
if (Have-Python) { Ok 'python' } else { Warn 'python' (Hint python) }
if (Have mpremote) { Ok 'mpremote (Pico)' } else { Warn 'mpremote, to copy files to a Pico' (Hint mpremote) }

Write-Host ''
Write-Host '== To take it home on a Pi'
if ((Have podman) -or (Have docker)) {
  Ok ("container build (" + $(if (Have podman) { 'podman' } else { 'docker' }) + ") - your app image must be arm64")
} else { Warn 'podman or docker, to build your app for the Pi' (Hint podman) }
if (Have ssh) { Ok 'ssh (OpenSSH client)' } else { Warn 'ssh, to reach your workload and your Pi' (Hint ssh) }
Ok '.local names (built into Windows 10 and later)'
Write-Host "  -        Raspberry Pi Imager and an SD card reader: $Downloads/tools/"

if (-not $Offline) {
  Write-Host ''
  Write-Host '== The site''s services (run this on DVNTM-TD)'

  function Check-Https([string]$Url, [string]$Label) {
    # curl.exe, not curl: in Windows PowerShell 5.1 curl is Invoke-WebRequest.
    $code = & curl.exe -s -o NUL -w '%{http_code}' --max-time 8 --ssl-revoke-best-effort --cacert $CaFile $Url
    if ($code -match '^[1-5][0-9][0-9]$') { Ok "$Label $Url (HTTP $code, TLS verified)" }
    else { Miss "$Label $Url did not answer" 'are you on DVNTM-TD? if so, tell the operator' }
  }

  # A TLS handshake, verified against the Deevnet Root CA alone: the chain the
  # server sends must end at that root, and name the host.
  function Check-Tls([string]$TlsHost, [int]$Port, [string]$Label) {
    $why = ''
    $tcp = New-Object System.Net.Sockets.TcpClient
    try {
      $conn = $tcp.BeginConnect($TlsHost, $Port, $null, $null)
      if (-not $conn.AsyncWaitHandle.WaitOne(8000)) { throw 'no answer' }
      $tcp.EndConnect($conn)
      $seen = @{ Cert = $null; Errors = $null; Chain = @() }
      $cb = [System.Net.Security.RemoteCertificateValidationCallback]{
        param($s, $cert, $chain, $errors)
        $seen.Cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 -ArgumentList $cert
        $seen.Errors = $errors
        if ($chain) { $seen.Chain = @($chain.ChainElements | ForEach-Object { $_.Certificate }) }
        return $true
      }
      $ssl = New-Object System.Net.Security.SslStream -ArgumentList $tcp.GetStream(), $false, $cb
      $ssl.AuthenticateAsClient($TlsHost)
      $ssl.Dispose()
      if ($seen.Errors -band [System.Net.Security.SslPolicyErrors]::RemoteCertificateNameMismatch) { throw "certificate does not name $TlsHost" }
      $c = New-Object System.Security.Cryptography.X509Certificates.X509Chain
      $c.ChainPolicy.RevocationMode = 'NoCheck'
      $c.ChainPolicy.VerificationFlags = 'AllowUnknownCertificateAuthority'
      [void]$c.ChainPolicy.ExtraStore.Add($CaCert)
      foreach ($e in $seen.Chain) { [void]$c.ChainPolicy.ExtraStore.Add($e) }
      [void]$c.Build($seen.Cert)
      $bad = @($c.ChainStatus | Where-Object { $_.Status -ne 'UntrustedRoot' -and $_.Status -ne 'NoError' })
      $root = $c.ChainElements[$c.ChainElements.Count - 1].Certificate
      if ($bad.Count -gt 0) { throw (($bad | ForEach-Object { $_.StatusInformation.Trim() }) -join '; ') }
      if ($root.Thumbprint -ne $CaCert.Thumbprint) { throw "chain ends at $($root.Subject), not the Deevnet Root CA" }
    } catch {
      $why = $_.Exception.Message
    } finally {
      $tcp.Close()
    }
    if ($why) { Miss "$Label ${TlsHost}:$Port $why" 'are you on DVNTM-TD? if so, tell the operator' }
    else { Ok "$Label ${TlsHost}:$Port (TLS verified)" }
  }

  Check-Https 'https://api.mobile.deevnet.net/healthz' 'Deevnet API'
  Check-Https 'https://tfstate.mobile.deevnet.net/minio/health/live' 'state store'
  Check-Tls 'mqtt.mobile.deevnet.net' 8883 'MQTT broker'
  Check-Tls 'logs.mobile.deevnet.net' 443 'log store'
  Check-Https 'https://grafana.mobile.deevnet.net/api/health' 'Grafana'
  Check-Https "$Downloads/" 'tenant downloads'
}

Remove-Item -Force $CaFile -ErrorAction SilentlyContinue
Write-Host ''
if ($script:M -gt 0) { Write-Host "RESULT: $($script:M) missing, $($script:W) to consider, $($script:P) ok"; exit 1 }
Write-Host "RESULT: ready ($($script:P) ok, $($script:W) to consider)"
