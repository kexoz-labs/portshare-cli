$ErrorActionPreference = 'Stop'

$version = '1.0.6'
$url64 = "https://github.com/kexoz/portshare-cli/releases/download/v$version/portshare-windows-amd64.exe"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $env:ChocolateyBinRoot
  fileType      = 'exe'
  url64bit      = $url64
  softwareName  = 'PortShare*'
  silentArgs    = '/S'
  validExitCodes = @(0)
  checksum64    = 'PLACEHOLDER_SHA256'
  checksumType64 = 'sha256'
}

Install-ChocolateyPackage @packageArgs
