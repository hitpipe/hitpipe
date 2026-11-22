[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$module = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$run = 'hit91-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$pg = 'docker.io/library/postgres:18.7-alpine@sha256:fa1b61f5a4844f4d4b73c9b15716cfc75fe70c931f2d32723630f3b900056366'
$go = 'docker.io/library/golang:1.27.2-bookworm@sha256:55395706e9703db746cc507abfc4eb2aea75918f8a8024e4848e2cb81004f5ad'
$migration = Join-Path ([System.IO.Path]::GetTempPath()) "$run.sql"
$sql = [System.IO.File]::ReadAllText((Join-Path $module 'migrations/001_delivery.sql')) -replace '\r\n', "`n"
$app = "postgresql://hp_delivery_app@$($run)-pg/delivery?sslmode=disable&pool_max_conns=5"
$admin = "postgresql://postgres@$($run)-pg/delivery?sslmode=disable&pool_max_conns=2"
$mounts = @('--mount', "type=bind,source=$module,target=/src,readonly",
            '--mount', "type=volume,source=$run,target=/go/pkg/mod")
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE -ne 0) {
        throw "Docker exited $LASTEXITCODE for $($args[0])"
    }
}
try {
    Write-Output "HIT-91 fixture $(Get-Date -AsUTC -Format o)"
    [System.IO.File]::WriteAllText($migration, $sql, [System.Text.UTF8Encoding]::new($false))
    $sqlDigest = (Get-FileHash -LiteralPath $migration -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($sqlDigest -ne '9b23a5a0f8617025f20012f38d0b961010d9d4fbbbc47c3059c1b651a16f155e') {
        throw 'Migration does not match the accepted SQL'
    }
    Write-Output "Migration SHA-256 $sqlDigest"
    Write-Output 'Commands: go test -v -race -count=1 ./...; go vet ./...'
    Invoke-Docker version --format '{{.Server.Version}}'
    Invoke-Docker image inspect $pg $go --format '{{.Id}} {{.Os}}/{{.Architecture}} {{index .RepoDigests 0}}'
    Invoke-Docker volume create $run
    Invoke-Docker network create --internal $run
    Invoke-Docker run --rm --platform linux/amd64 @mounts -w /src $go go mod download
    Invoke-Docker run --detach --platform linux/amd64 --name "$run-pg" --network $run -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=delivery $pg
    Invoke-Docker cp $migration "$($run)-pg:/tmp/001_delivery.sql"
    Invoke-Docker cp (Join-Path $module 'tests/hit91_fixture.sql') "$($run)-pg:/tmp/hit91_fixture.sql"
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        & docker exec "$run-pg" pg_isready -q -h 127.0.0.1 -U postgres -d delivery
        if ($LASTEXITCODE -eq 0) { break }
        Start-Sleep -Seconds 1
    }
    if ($LASTEXITCODE -ne 0) { throw 'Isolated PostgreSQL did not become ready' }
    Invoke-Docker exec "$run-pg" psql -U postgres -d delivery -f /tmp/hit91_fixture.sql
    Invoke-Docker exec "$run-pg" psql -U postgres -d delivery -c "SELECT version(),current_setting('fsync') AS fsync,current_setting('full_page_writes') AS full_page_writes,current_setting('synchronous_commit') AS synchronous_commit"
    $runnerEnvironment = @('-e', 'GOPROXY=off', '-e', 'GOSUMDB=off', '-e', 'HIT91_REQUIRE_POSTGRES=1',
                           '-e', "HIT91_TEST_APP_DSN=$app", '-e', "HIT91_TEST_ADMIN_DSN=$admin")
    Invoke-Docker run --rm --platform linux/amd64 --network $run @mounts -w /src @runnerEnvironment $go sh -ec 'go version; go test -v -race -count=1 ./...; echo go-test-exit=0; go vet ./...; echo go-vet-exit=0'
} finally {
    & docker rm -f -v "$run-pg" 2>$null
    & docker network rm $run 2>$null
    & docker volume rm $run 2>$null
    if (Test-Path -LiteralPath $migration) { Remove-Item -LiteralPath $migration }
}
