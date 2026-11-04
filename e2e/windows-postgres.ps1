# Native Windows proof against a run-owned cluster using the runner's PostgreSQL 17 tools.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (-not $IsWindows) { throw 'Native Windows is required.' }

$repo = Split-Path -Parent $PSScriptRoot
$runID = [guid]::NewGuid().ToString('N')
# Keep the binary outside TEMP: config.BaseDir treats binaries under TEMP as go-run builds.
$scratch = Join-Path $repo ".windows-postgres-$runID"
$package = Join-Path $scratch 'package'
$work = Join-Path $scratch 'work'
$cluster = Join-Path $scratch 'postgres'
$evidence = Join-Path $env:RUNNER_TEMP 'scopeweaver-windows-postgres'
$password = "synthetic_$runID"
$dbUser = "sw_$runID"
$database = "scopeweaver_test_$runID"
$children = [System.Collections.Generic.List[object]]::new()
$logs = [System.Text.StringBuilder]::new()
$clusterStarted = $false
$startedAt = [datetime]::UtcNow
$failure = $null

function Start-Probe([string]$File, [string[]]$Arguments, [string]$Directory, [hashtable]$Environment = @{}) {
    $info = [System.Diagnostics.ProcessStartInfo]::new()
    $info.FileName = $File
    $info.WorkingDirectory = $Directory
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    foreach ($key in @($info.Environment.Keys)) {
        if ($key -match '^(ARTEX_|SCOPEWEAVER_|PG)') { [void]$info.Environment.Remove($key) }
    }
    # An inherited pgpass must never rescue the missing-password case.
    $info.Environment['PGPASSFILE'] = Join-Path $scratch 'empty-pgpass'
    foreach ($key in $Environment.Keys) { $info.Environment[$key] = $Environment[$key] }
    foreach ($argument in $Arguments) { $info.ArgumentList.Add($argument) }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $info
    [void]$process.Start()
    $probe = [pscustomobject]@{
        Process = $process
        Stdout = $process.StandardOutput.ReadToEndAsync()
        Stderr = $process.StandardError.ReadToEndAsync()
        Logged = $false
    }
    $children.Add($probe)
    return $probe
}

function Complete-Probe($Probe, [int]$Timeout = 30000) {
    if (-not $Probe.Process.WaitForExit($Timeout)) {
        $Probe.Process.Kill($true)
        $Probe.Process.WaitForExit()
        throw 'Native probe timed out.'
    }
    $output = $Probe.Stdout.GetAwaiter().GetResult() + $Probe.Stderr.GetAwaiter().GetResult()
    [void]$logs.AppendLine($output)
    $Probe.Logged = $true
    if ($output.Contains($password)) { throw 'A native probe exposed the synthetic password.' }
    return [pscustomobject]@{ ExitCode = $Probe.Process.ExitCode; Output = $output }
}

function Invoke-Checked([string]$File, [string[]]$Arguments, [hashtable]$Environment = @{}, [int]$Timeout = 30000) {
    $result = Complete-Probe (Start-Probe $File $Arguments $repo $Environment) $Timeout
    if ($result.ExitCode -ne 0) { throw "Native command failed: $([IO.Path]::GetFileName($File)), exit $($result.ExitCode)." }
    return $result.Output.Trim()
}

function Free-Port {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try { return $listener.LocalEndpoint.Port } finally { $listener.Stop() }
}

function Start-App([int]$Port, [string]$Credential) {
    $config = Get-Content (Join-Path $repo 'config.example.json') -Raw | ConvertFrom-Json
    $config.database.host = '127.0.0.1'
    $config.database.port = $Port
    $config.database.user = $dbUser
    $config.database.password = $Credential
    $config.database.dbname = $database
    $config.database.sslmode = 'disable'
    $config.PSObject.Properties.Remove('skill_dir')
    $configPath = Join-Path $scratch 'config.json'
    $config | ConvertTo-Json -Depth 10 | Set-Content $configPath -Encoding utf8NoBOM
    return Start-Probe $binary @('-addr', "127.0.0.1:$httpPort", '-data', (Join-Path $scratch 'data'), '-proxy=') $work @{
        SCOPEWEAVER_CONFIG = $configPath
        SCOPEWEAVER_LANGUAGE = 'en'
    }
}

try {
    [void](New-Item -ItemType Directory -Path $package, $work, $evidence -Force)
    Set-Content (Join-Path $scratch 'empty-pgpass') '' -Encoding ascii
    $candidates = @($env:PGBIN)
    $pgRoot = Join-Path $env:ProgramFiles 'PostgreSQL'
    if (Test-Path $pgRoot) {
        $candidates += Get-ChildItem $pgRoot -Directory | ForEach-Object { Join-Path $_.FullName 'bin' }
    }
    $pgBin = $null
    foreach ($candidate in $candidates) {
        if (-not $candidate -or -not (Test-Path (Join-Path $candidate 'postgres.exe'))) { continue }
        $version = Invoke-Checked (Join-Path $candidate 'postgres.exe') @('--version')
        if ($version -match 'PostgreSQL\) 17\.') { $pgBin = $candidate; break }
    }
    if (-not $pgBin) { throw 'Runner-installed PostgreSQL 17 tools are required; no install or service fallback is permitted.' }
    Write-Host "Using native $version"
    $pgCtl = Join-Path $pgBin 'pg_ctl.exe'
    $binary = Join-Path $package 'scopeweaver.exe'
    [void](Invoke-Checked 'go' @('build', '-tags', 'embedui', '-o', $binary, './cmd/artex') @{} 180000)
    $passwordFile = Join-Path $scratch 'init-password'
    Set-Content $passwordFile $password -Encoding ascii
    [void](Invoke-Checked (Join-Path $pgBin 'initdb.exe') @('-D', $cluster, '--username', $dbUser, '--pwfile', $passwordFile, '--auth=scram-sha-256', '--encoding=UTF8', '--no-locale', '--no-instructions'))
    Remove-Item $passwordFile
    $pgPort = Free-Port
    $httpPort = Free-Port
    while ($httpPort -eq $pgPort) { $httpPort = Free-Port }
    $clusterStarted = $true # Cleanup must run even if pg_ctl only partially starts the cluster.
    [void](Invoke-Checked $pgCtl @('-D', $cluster, '-l', (Join-Path $scratch 'postgres.log'), '-w', '-t', '30', 'start', '-o', "-h 127.0.0.1 -p $pgPort") @{} 45000)
    $pgEnvironment = @{ PGHOST = '127.0.0.1'; PGPORT = "$pgPort"; PGUSER = $dbUser; PGPASSWORD = $password }
    [void](Invoke-Checked (Join-Path $pgBin 'createdb.exe') @('--no-password', $database) $pgEnvironment)
    $identity = Invoke-Checked (Join-Path $pgBin 'psql.exe') @('-X', '-A', '-t', '--no-password', '-d', $database, '-v', 'ON_ERROR_STOP=1', '-c', 'SELECT current_database(), inet_server_port(), current_user') $pgEnvironment
    if ($identity -ne "$database|$pgPort|$dbUser") { throw 'PostgreSQL did not select the exact disposable database and custom port.' }

    # Bound but not listening: this endpoint stays closed throughout the failure probe.
    $closed = [System.Net.Sockets.Socket]::new([System.Net.Sockets.AddressFamily]::InterNetwork, [System.Net.Sockets.SocketType]::Stream, [System.Net.Sockets.ProtocolType]::Tcp)
    try {
        $closed.ExclusiveAddressUse = $true
        $closed.Bind([System.Net.IPEndPoint]::new([System.Net.IPAddress]::Loopback, 0))
        $closedPort = $closed.LocalEndPoint.Port
        $result = Complete-Probe (Start-App $closedPort $password)
        if ($result.ExitCode -ne 1 -or $result.Output -notmatch ":$closedPort/" -or $result.Output -notmatch 'open stores:.*connect') {
            throw 'A closed configured PostgreSQL endpoint must fail startup with exit 1 at that exact port.'
        }
    } finally { $closed.Dispose() }
    Write-Host 'PASS: closed PostgreSQL endpoint fails with exit 1; no port fallback'

    $result = Complete-Probe (Start-App $pgPort '')
    if ($result.ExitCode -ne 1 -or $result.Output -notmatch 'password authentication failed|SQLSTATE 28P01|no password supplied|requires a non-empty password') {
        throw 'SCRAM authentication without a password must fail startup with exit 1.'
    }
    Write-Host 'PASS: password-authenticated PostgreSQL rejects the omitted credential'

    $app = Start-App $pgPort $password
    $health = $null
    $deadline = [datetime]::UtcNow.AddSeconds(60)
    while ([datetime]::UtcNow -lt $deadline -and -not $app.Process.HasExited) {
        try { $health = Invoke-RestMethod "http://127.0.0.1:$httpPort/api/health" -TimeoutSec 2 -NoProxy } catch { }
        if ($health) { break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $health -or $health.ok -ne $true -or $health.version -ne 'dev') { throw 'Real backend startup did not return the expected unauthenticated health response.' }
    $setup = Invoke-WebRequest "http://127.0.0.1:$httpPort/setup" -TimeoutSec 5 -NoProxy
    $exportedSetup = Get-Content (Join-Path $repo 'web/out/setup/index.html') -Raw
    if ($setup.StatusCode -ne 200 -or $setup.Headers['Content-Type'] -notmatch 'text/html' -or $setup.Content -ne $exportedSetup -or $setup.Content -notmatch '/_next/') {
        throw 'The real embedded setup page does not match the built frontend.'
    }
    $schema = Invoke-Checked (Join-Path $pgBin 'psql.exe') @('-X', '-A', '-t', '--no-password', '-d', $database, '-v', 'ON_ERROR_STOP=1', '-c', "SELECT current_database(), inet_server_port(), to_regclass('public.agents') IS NOT NULL") $pgEnvironment
    if ($schema -ne "$database|$pgPort|t") { throw 'The application did not initialize the exact disposable database on the custom port.' }
    $app.Process.Kill($true)
    $result = Complete-Probe $app
    $expectedSkillPath = Join-Path $package 'skills'
    if (-not $result.Output.Contains("[config] Skill directory: $expectedSkillPath") -or -not (Test-Path $expectedSkillPath -PathType Container)) {
        throw 'The default skill directory did not resolve beside the executable from a separate working directory.'
    }
    Write-Host 'PASS: authenticated custom-port DB startup, health, embedded /setup and executable-adjacent skills'
} catch {
    $failure = $_
} finally {
    foreach ($child in $children) {
        if (-not $child.Process.HasExited) { $child.Process.Kill($true); $child.Process.WaitForExit() }
        $output = $child.Stdout.GetAwaiter().GetResult() + $child.Stderr.GetAwaiter().GetResult()
        if ($output.Contains($password) -and -not $failure) { $failure = 'A native probe exposed the synthetic password.' }
        if (-not $child.Logged) { [void]$logs.AppendLine($output) }
    }
    if ($clusterStarted) {
        try {
            [void](Invoke-Checked $pgCtl @('-D', $cluster, '-m', 'immediate', '-w', '-t', '30', 'stop') @{} 45000)
        } catch {
            $failure = "PostgreSQL cleanup failed: $_; original failure: $failure"
            # Escalate only the postmaster identified by this run's own data directory.
            $pidFile = Join-Path $cluster 'postmaster.pid'
            if (Test-Path $pidFile) {
                $postmasterID = [int](Get-Content $pidFile -First 1)
                $postmaster = Get-Process -Id $postmasterID -ErrorAction SilentlyContinue
                if ($postmaster -and $postmaster.Path -eq (Join-Path $pgBin 'postgres.exe') -and $postmaster.StartTime.ToUniversalTime() -ge $startedAt) {
                    $postmaster.Kill($true)
                    $postmaster.WaitForExit()
                }
            }
        }
    }
    if (Test-Path (Join-Path $scratch 'postgres.log')) { [void]$logs.AppendLine((Get-Content (Join-Path $scratch 'postgres.log') -Raw)) }
    if ($logs.ToString().Contains($password) -and -not $failure) { $failure = 'A native probe exposed the synthetic password.' }
    # Upload only scrubbed logs, never the configuration, password file or data directory.
    $safeLog = $logs.ToString().Replace($password, '[REDACTED]')
    $safeLog | Set-Content (Join-Path $evidence "$runID.log") -Encoding utf8NoBOM
    Remove-Item $scratch -Recurse -Force
}
if ($failure) {
    Write-Host $safeLog
    throw "$failure".Replace($password, '[REDACTED]')
}
Write-Host 'PASS: run-owned PostgreSQL cluster stopped and scratch data removed'
