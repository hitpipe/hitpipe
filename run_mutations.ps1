$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$file = Join-Path $root 'services\delivery\internal\domain\readback\readback.go'
$backup = Join-Path $root 'readback.go.baseline'
$log = Join-Path $root 'docs\labs\pr12\ds\evidence\HIT-45-mutations-actual.log'
Copy-Item -LiteralPath $file -Destination $backup -Force
$baseline = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
Set-Content -LiteralPath $log -Value "baseline source_sha=7e815aa790e0ebba58e453e786bda45b9266161d file_sha256=$baseline"
$mutations = @(
  @{id='M1'; description='remove retrieval guard'; old='if !observed.Retrieved {'; new='if false {'; test='TestCompareIncompleteAndUnknownPrecedence'},
  @{id='M2'; description='change malformed handoff outcome'; old='return Result{Outcome: Incomplete, Reason: reason}'; new='return Result{Outcome: Mismatch, Reason: reason}'; test='TestCompareIncompleteAndUnknownPrecedence'},
  @{id='M3'; description='remove title mismatch guard'; old='if normalize(observed.Title) != normalize(expected.Title) {'; new='if false {'; test='TestCompareMismatchEachRequiredField/title'},
  @{id='M4'; description='skip title UTF-8 validation'; old='if !validText(expected.Workspace) || !validText(expected.List) || !validText(expected.Title) || !validText(expected.Assignee) || !validText(expected.Context) || !validText(expected.HandoffBlock) || !validText(observed.Workspace) || !validText(observed.List) || !validText(observed.Title) || !validText(observed.Context) || !validText(observed.HandoffBlock) || !validText(observed.TargetID) || !validText(observed.RetrievalError) {'; new='if !validText(expected.Workspace) || !validText(expected.List) || !validText(expected.Assignee) || !validText(expected.Context) || !validText(expected.HandoffBlock) || !validText(observed.Workspace) || !validText(observed.List) || !validText(observed.Context) || !validText(observed.HandoffBlock) || !validText(observed.TargetID) || !validText(observed.RetrievalError) {'; test='TestReviewComparatorRejectsInvalidTitleAndRetrievalError'},
  @{id='M5'; description='remove assignee comparison'; old='if !containsNormalized(observed.Assignees, expected.Assignee) {'; new='if false {'; test='TestC3FalseConfirmationGuards/missing_assignee'}
)
foreach ($m in $mutations) {
  Copy-Item -LiteralPath $backup -Destination $file -Force
  $text = Get-Content -Raw -LiteralPath $file
  if (-not $text.Contains($m.old)) { throw "mutation source not found $($m.id)" }
  $text = $text.Replace($m.old, $m.new)
  Set-Content -LiteralPath $file -Value $text -NoNewline
  $before = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
  Push-Location (Join-Path $root 'services\delivery')
  $output = & 'C:\Program Files\Go\bin\go.exe' test ./internal/domain/readback -run $m.test -count=1 2>&1
  $exit = $LASTEXITCODE
  Pop-Location
  Add-Content -LiteralPath $log -Value "`n$($m.id) $($m.description) mutated_sha256=$before command=go test ./internal/domain/readback -run $($m.test) -count=1 exit=$exit`n$($output -join "`n")"
  Copy-Item -LiteralPath $backup -Destination $file -Force
  $restored = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
  Add-Content -LiteralPath $log -Value "restored_sha256=$restored expected=$baseline"
}
Push-Location (Join-Path $root 'services\delivery')
$final = & 'C:\Program Files\Go\bin\go.exe' test ./... -count=1 2>&1
$exit = $LASTEXITCODE
Pop-Location
Add-Content -LiteralPath $log -Value "`nFINAL restored test command=go test ./... -count=1 exit=$exit`n$($final -join "`n")"
if ($exit -ne 0) { throw 'final restoration test failed' }
