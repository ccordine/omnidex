param(
    [ValidateSet("core", "local", "all")]
    [string]$Profile = "all",
    [switch]$WithWhisper,
    [switch]$Yes,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$InstalledPackages = New-Object System.Collections.Generic.List[string]
$Warnings = New-Object System.Collections.Generic.List[string]

function Write-Setup {
    param([string]$Message)
    Write-Host "[setup] $Message"
}

function Write-SetupWarning {
    param([string]$Message)
    Write-Warning "[setup] $Message"
    $Warnings.Add($Message) | Out-Null
}

function Invoke-Step {
    param([string[]]$Command)
    if ($DryRun) {
        Write-Host ("+ " + ($Command -join " "))
        return $true
    }
    & $Command[0] @($Command | Select-Object -Skip 1)
    return ($LASTEXITCODE -eq 0)
}

function Get-PackageTool {
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        return "winget"
    }
    if (Get-Command scoop -ErrorAction SilentlyContinue) {
        return "scoop"
    }
    if (Get-Command choco -ErrorAction SilentlyContinue) {
        return "choco"
    }
    throw "No supported Windows package manager found. Install winget, Scoop, or Chocolatey first."
}

function Test-CommandAvailable {
    param([string]$Name)
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Test-Dependency {
    param([string]$Key)
    switch ($Key) {
        "git" { return Test-CommandAvailable "git" }
        "go" { return Test-CommandAvailable "go" }
        "make" { return (Test-CommandAvailable "make") -or (Test-CommandAvailable "mingw32-make") }
        "curl" { return Test-CommandAvailable "curl" }
        "jq" { return Test-CommandAvailable "jq" }
        "rg" { return Test-CommandAvailable "rg" }
        "docker" { return Test-CommandAvailable "docker" }
        "docker_compose" {
            if (-not (Test-CommandAvailable "docker")) { return $false }
            docker compose version *> $null
            return ($LASTEXITCODE -eq 0)
        }
        "npm" { return Test-CommandAvailable "npm" }
        "ffmpeg" { return Test-CommandAvailable "ffmpeg" }
        "vlc" { return (Test-CommandAvailable "vlc") -or (Test-Path "${env:ProgramFiles}\VideoLAN\VLC\vlc.exe") }
        "tesseract" { return Test-CommandAvailable "tesseract" }
        "python3" { return (Test-CommandAvailable "python") -or (Test-CommandAvailable "py") }
        "pip3" { return (Test-CommandAvailable "pip") -or (Test-CommandAvailable "pip3") }
        "whisper" { return Test-CommandAvailable "whisper" }
        default { return $true }
    }
}

function Get-PackageCandidates {
    param([string]$Tool, [string]$Key)
    switch ($Tool) {
        "winget" {
            switch ($Key) {
                "git" { return @("Git.Git") }
                "go" { return @("GoLang.Go") }
                "make" { return @("GnuWin32.Make") }
                "curl" { return @("cURL.cURL") }
                "jq" { return @("jqlang.jq") }
                "rg" { return @("BurntSushi.ripgrep.MSVC") }
                "docker" { return @("Docker.DockerDesktop") }
                "docker_compose" { return @("Docker.DockerDesktop") }
                "npm" { return @("OpenJS.NodeJS.LTS") }
                "ffmpeg" { return @("Gyan.FFmpeg") }
                "vlc" { return @("VideoLAN.VLC") }
                "tesseract" { return @("UB-Mannheim.TesseractOCR") }
                "python3" { return @("Python.Python.3.12") }
                "pip3" { return @("Python.Python.3.12") }
                default { return @() }
            }
        }
        "scoop" {
            switch ($Key) {
                "git" { return @("git") }
                "go" { return @("go") }
                "make" { return @("make") }
                "curl" { return @("curl") }
                "jq" { return @("jq") }
                "rg" { return @("ripgrep") }
                "docker" { return @("docker") }
                "docker_compose" { return @("docker-compose") }
                "npm" { return @("nodejs-lts") }
                "ffmpeg" { return @("ffmpeg") }
                "vlc" { return @("vlc") }
                "tesseract" { return @("tesseract") }
                "python3" { return @("python") }
                "pip3" { return @("python") }
                default { return @() }
            }
        }
        "choco" {
            switch ($Key) {
                "git" { return @("git") }
                "go" { return @("golang") }
                "make" { return @("make") }
                "curl" { return @("curl") }
                "jq" { return @("jq") }
                "rg" { return @("ripgrep") }
                "docker" { return @("docker-desktop") }
                "docker_compose" { return @("docker-desktop") }
                "npm" { return @("nodejs-lts") }
                "ffmpeg" { return @("ffmpeg") }
                "vlc" { return @("vlc") }
                "tesseract" { return @("tesseract") }
                "python3" { return @("python") }
                "pip3" { return @("python") }
                default { return @() }
            }
        }
    }
}

function Install-PackageCandidate {
    param([string]$Tool, [string]$Package)
    Write-Setup "installing package: $Package"
    switch ($Tool) {
        "winget" {
            $cmd = @("winget", "install", "--id", $Package, "--exact", "--accept-package-agreements", "--accept-source-agreements")
            if ($Yes) { $cmd += "--silent" }
            if (Invoke-Step $cmd) {
                $InstalledPackages.Add($Package) | Out-Null
                return $true
            }
        }
        "scoop" {
            if (Invoke-Step @("scoop", "install", $Package)) {
                $InstalledPackages.Add($Package) | Out-Null
                return $true
            }
        }
        "choco" {
            $cmd = @("choco", "install", $Package)
            if ($Yes) { $cmd += "-y" }
            if (Invoke-Step $cmd) {
                $InstalledPackages.Add($Package) | Out-Null
                return $true
            }
        }
    }
    return $false
}

function Ensure-Dependency {
    param([string]$Tool, [string]$Key)
    if (Test-Dependency $Key) {
        Write-Setup "dependency ready: $Key"
        return
    }

    $candidates = Get-PackageCandidates $Tool $Key
    if ($candidates.Count -eq 0) {
        Write-SetupWarning "no package mapping available for dependency: $Key on $Tool"
        return
    }

    foreach ($candidate in $candidates) {
        if (Install-PackageCandidate $Tool $candidate) {
            if ($DryRun -or (Test-Dependency $Key)) {
                Write-Setup "dependency installed: $Key"
                return
            }
        }
    }
    Write-SetupWarning "dependency still missing after install attempt: $Key"
}

function Install-Whisper {
    if (Test-Dependency "whisper") {
        Write-Setup "whisper CLI already available"
        return
    }
    if (-not (Test-Dependency "python3")) {
        Write-SetupWarning "python is required for whisper CLI"
        return
    }
    Write-Setup "installing whisper CLI via pip (openai-whisper)"
    if (Test-CommandAvailable "pip") {
        Invoke-Step @("pip", "install", "--user", "--upgrade", "openai-whisper") | Out-Null
    } else {
        Invoke-Step @("python", "-m", "pip", "install", "--user", "--upgrade", "openai-whisper") | Out-Null
    }
}

$PackageTool = Get-PackageTool
Write-Setup "detected package manager: $PackageTool"

$Dependencies = New-Object System.Collections.Generic.List[string]
if ($Profile -eq "core" -or $Profile -eq "all") {
    @("git", "go", "make", "curl", "jq", "rg", "docker", "docker_compose", "npm") | ForEach-Object { $Dependencies.Add($_) | Out-Null }
}
if ($Profile -eq "local" -or $Profile -eq "all") {
    @("ffmpeg", "vlc", "tesseract") | ForEach-Object { $Dependencies.Add($_) | Out-Null }
    Write-SetupWarning "Windows local automation support is partial; media/OCR packages are installed, but Linux-specific tools such as pactl, playerctl, iproute, nmcli, and screenshot tools are not mapped."
}
if ($WithWhisper) {
    @("python3", "pip3") | ForEach-Object { $Dependencies.Add($_) | Out-Null }
}

$Dependencies | Select-Object -Unique | ForEach-Object {
    Ensure-Dependency $PackageTool $_
}

if ($WithWhisper) {
    Install-Whisper
}

if ($DryRun) {
    Write-Setup "dry-run completed (no changes were made)"
    exit 0
}

$Missing = New-Object System.Collections.Generic.List[string]
$Dependencies | Select-Object -Unique | ForEach-Object {
    if (-not (Test-Dependency $_)) {
        $Missing.Add($_) | Out-Null
    }
}
if ($WithWhisper -and -not (Test-Dependency "whisper")) {
    $Missing.Add("whisper") | Out-Null
}

if ($InstalledPackages.Count -gt 0) {
    Write-Setup ("installed packages: " + ($InstalledPackages -join " "))
} else {
    Write-Setup "no new packages were installed"
}

if ($Missing.Count -gt 0) {
    Write-SetupWarning ("missing dependencies after setup: " + ($Missing -join " "))
    exit 1
}

if ($Warnings.Count -gt 0) {
    Write-SetupWarning "setup completed with warnings"
} else {
    Write-Setup "setup completed successfully"
}
