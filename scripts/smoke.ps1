[CmdletBinding()]
param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$DevSecret,
    [int]$TimeoutSeconds = 45,
    [string]$ExpectedVersion = ""
)

$ErrorActionPreference = "Stop"
$BaseUrl = $BaseUrl.TrimEnd('/')
$DevHeaders = @{ "X-Ptium-Dev-Secret" = $DevSecret }

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw "Smoke assertion failed: $Message" }
}

function Invoke-PtiumJson {
    param(
        [ValidateSet("GET", "POST", "PUT", "PATCH", "DELETE")]
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = $DevHeaders,
        [object]$Body
    )
    $Arguments = @{
        Method = $Method
        Uri = "$BaseUrl$Path"
        Headers = $Headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) { $Arguments.Body = ($Body | ConvertTo-Json -Depth 20 -Compress) }
    Invoke-RestMethod @Arguments
}

function Wait-ForPresentation([string]$Id, [string[]]$Statuses) {
    $Deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        $Response = Invoke-PtiumJson -Method GET -Path "/api/v1/presentations/$Id"
        if ($Statuses -contains [string]$Response.data.status) { return $Response.data }
        Start-Sleep -Milliseconds 500
    } while ([DateTimeOffset]::UtcNow -lt $Deadline)
    throw "Presentation $Id did not reach $($Statuses -join ', ') within $TimeoutSeconds seconds."
}

$Ready = Invoke-RestMethod -Uri "$BaseUrl/readyz"
Assert-True ($Ready.data.status -eq "ready") "readiness endpoint"

$HomeResponse = Invoke-WebRequest -Uri "$BaseUrl/"
Assert-True ($HomeResponse.StatusCode -eq 200 -and $HomeResponse.Content.Contains('<div id="root"></div>')) "single-image web application is served"
Assert-True ([string]($HomeResponse.Headers.'X-Content-Type-Options' | Select-Object -First 1) -eq 'nosniff') "browser MIME-sniffing protection"
Assert-True ([string]($HomeResponse.Headers.'X-Frame-Options' | Select-Object -First 1) -eq 'DENY') "browser frame protection"
Assert-True (-not [string]::IsNullOrWhiteSpace([string]$HomeResponse.Headers.'Content-Security-Policy')) "browser content security policy"

$Anonymous = Invoke-WebRequest -Uri "$BaseUrl/api/v1/presentations" -SkipHttpErrorCheck
Assert-True ($Anonymous.StatusCode -eq 401) "presentation API requires authentication"

$AuthConfig = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/config"
Assert-True ($AuthConfig.data.devAuthEnabled -eq $true) "development auth must be enabled for smoke test"
Assert-True ($AuthConfig.data.devAuthHeader -eq "X-Ptium-Dev-Secret") "development header contract"

$PublicSettings = Invoke-RestMethod -Uri "$BaseUrl/api/v1/settings"
Assert-True (-not [string]::IsNullOrWhiteSpace([string]$PublicSettings.data.'branding.product_name')) "safe settings are publicly readable for login branding"
$ConfiguredDefaultSlides = [int]$PublicSettings.data.'generation.default_slide_count'
Assert-True ($ConfiguredDefaultSlides -ge 1) "public generation default is available"

$InvalidUuid = Invoke-WebRequest -Uri "$BaseUrl/api/v1/presentations/not-a-uuid" -Headers $DevHeaders -SkipHttpErrorCheck
Assert-True ($InvalidUuid.StatusCode -eq 400) "invalid resource UUIDs are rejected as client errors"

$TrailingJson = Invoke-WebRequest -Method POST -Uri "$BaseUrl/api/v1/presentations" -Headers $DevHeaders -ContentType "application/json" -Body '{"title":"valid first object","prompt":"contract test"}{}' -SkipHttpErrorCheck
Assert-True ($TrailingJson.StatusCode -eq 400) "trailing JSON tokens are rejected"

$InjectedSlides = Invoke-WebRequest -Method POST -Uri "$BaseUrl/api/v1/presentations" -Headers $DevHeaders -ContentType "application/json" -Body '{"title":"server-owned slides","prompt":"contract test","slides":[]}' -SkipHttpErrorCheck
Assert-True ($InjectedSlides.StatusCode -eq 422) "creation rejects server-owned slide input"

$Me = Invoke-PtiumJson -Method GET -Path "/api/v1/me"
Assert-True (-not [string]::IsNullOrWhiteSpace([string]$Me.data.user.id)) "current user was provisioned"
Assert-True ($Me.data.user.isAdmin -eq $true) "smoke principal is an administrator"

$Profile = Invoke-PtiumJson -Method PUT -Path "/api/v1/profile" -Body @{
    displayName = "Ptium Smoke User"
    company = "Ptium"
    jobTitle = "Integration Tester"
    bio = "Automated end-to-end verification"
    preferences = @{
        language = "ko"
        defaultAudience = "executives"
        defaultTone = "professional"
        defaultTheme = "aurora"
        brandColor = "#8068E8"
    }
}
Assert-True ($Profile.data.displayName -eq "Ptium Smoke User") "profile update"
$MeAfterProfile = Invoke-PtiumJson -Method GET -Path "/api/v1/me"
Assert-True ($MeAfterProfile.data.user.name -eq "Ptium Smoke User") "profile display name is reflected in the current identity"

$Created = Invoke-PtiumJson -Method POST -Path "/api/v1/presentations" -Body @{
    title = "Ptium 통합 검증"
    prompt = "오프라인 배포 준비 상태를 설명하는 경영진용 덱"
    theme = "aurora"
    language = "ko"
    audience = "executives"
    tone = "professional"
    requestedSlideCount = 6
}
$PresentationId = [string]$Created.data.id
Assert-True (-not [string]::IsNullOrWhiteSpace($PresentationId)) "presentation creation"

$null = Invoke-PtiumJson -Method POST -Path "/api/v1/presentations/$PresentationId/generate"
$Generated = Wait-ForPresentation -Id $PresentationId -Statuses @("completed", "failed")
Assert-True ($Generated.status -eq "completed") "fallback generation completed"
Assert-True ($Generated.slides.Count -eq 6) "requested slide count was generated"
Assert-True ($Generated.slideCount -eq 6) "presentation detail reports the actual slide count"
Assert-True ($Generated.slides[0].content.accent -eq "#8068E8") "profile brand color is applied to generated content"
Assert-True (($Generated.slides[1].content | ConvertTo-Json -Depth 20).Contains("Automated end-to-end verification")) "profile context is applied to generated content"

$UpdatedSlides = @($Generated.slides | ForEach-Object {
    @{
        id = $_.id
        position = $_.position
        title = $_.title
        subtitle = $_.subtitle
        content = $_.content
        speakerNotes = $_.speakerNotes
        layout = $_.layout
    }
})
$UpdatedSlides[0].title = "Ptium 검증 완료"
$Updated = Invoke-PtiumJson -Method PATCH -Path "/api/v1/presentations/$PresentationId" -Body @{
    title = "Ptium 통합 검증"
    slides = $UpdatedSlides
}
Assert-True ($Updated.data.slides[0].title -eq "Ptium 검증 완료") "slide editing round trip"

$PptxPath = Join-Path ([System.IO.Path]::GetTempPath()) "ptium-smoke-$PresentationId.pptx"
try {
    Invoke-WebRequest -Uri "$BaseUrl/api/v1/presentations/$PresentationId/export?format=pptx" -Headers $DevHeaders -OutFile $PptxPath
    $PptxBytes = [System.IO.File]::ReadAllBytes($PptxPath)
    Assert-True ($PptxBytes.Length -gt 1000) "PPTX payload is non-empty"
    Assert-True ($PptxBytes[0] -eq 0x50 -and $PptxBytes[1] -eq 0x4b) "PPTX is a ZIP/OOXML package"
} finally {
    if (Test-Path -LiteralPath $PptxPath) { Remove-Item -LiteralPath $PptxPath -Force }
}

$UnknownScopeResponse = Invoke-WebRequest -Method POST -Uri "$BaseUrl/api/v1/api-keys" -Headers $DevHeaders -ContentType "application/json" -Body '{"name":"invalid","scopes":["root:everything"]}' -SkipHttpErrorCheck
Assert-True ($UnknownScopeResponse.StatusCode -eq 422) "unknown API-key scopes are rejected"

$KeyResponse = Invoke-PtiumJson -Method POST -Path "/api/v1/api-keys" -Body @{
    name = "smoke-client"
    scopes = @("presentations:read", "presentations:write", "mcp:use", "api_keys:manage")
    expiresAt = [DateTimeOffset]::UtcNow.AddDays(7).ToString("o")
}
$KeyId = [string]$KeyResponse.data.apiKey.id
$OldSecret = [string]$KeyResponse.data.secret
Assert-True ($OldSecret.StartsWith("ptium_")) "one-time API-key secret returned"

$KeyHeaders = @{ Authorization = "Bearer $OldSecret" }
$KeyList = Invoke-PtiumJson -Method GET -Path "/api/v1/presentations" -Headers $KeyHeaders
Assert-True ($KeyList.meta.total -ge 1) "API key can read owner presentations"
$ListedPresentation = @($KeyList.data | Where-Object { $_.id -eq $PresentationId })[0]
Assert-True ($ListedPresentation.slideCount -eq 6) "presentation list reports the actual slide count"

$ListedKeys = Invoke-PtiumJson -Method GET -Path "/api/v1/api-keys"
$LeakedSecret = ($ListedKeys | ConvertTo-Json -Depth 20).Contains($OldSecret)
Assert-True (-not $LeakedSecret) "API-key list never returns full secrets"

$McpHeaders = @{
    Authorization = "Bearer $OldSecret"
    "MCP-Protocol-Version" = "2025-11-25"
}
$McpInitialize = Invoke-PtiumJson -Method POST -Path "/mcp" -Headers $McpHeaders -Body @{
    jsonrpc = "2.0"
    id = 1
    method = "initialize"
    params = @{ protocolVersion = "2025-11-25"; capabilities = @{}; clientInfo = @{ name = "ptium-smoke"; version = "1.0" } }
}
Assert-True ($McpInitialize.result.serverInfo.name -eq "ptium") "MCP initialize"
if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
    Assert-True ($McpInitialize.result.serverInfo.version -eq $ExpectedVersion) "release image reports version $ExpectedVersion"
}
$McpTools = Invoke-PtiumJson -Method POST -Path "/mcp" -Headers $McpHeaders -Body @{
    jsonrpc = "2.0"
    id = 2
    method = "tools/list"
    params = @{}
}
Assert-True ($McpTools.result.tools.Count -ge 4) "MCP tools/list"
$McpCreated = Invoke-PtiumJson -Method POST -Path "/mcp" -Headers $McpHeaders -Body @{
    jsonrpc = "2.0"
    id = 3
    method = "tools/call"
    params = @{
        name = "ptium.create_presentation"
        arguments = @{ title = "MCP default contract"; prompt = "Verify administrator defaults." }
    }
}
$McpPresentationId = [string]$McpCreated.result.structuredContent.presentation.id
Assert-True (-not [string]::IsNullOrWhiteSpace($McpPresentationId)) "MCP presentation creation"
Assert-True ($McpCreated.result.structuredContent.presentation.requestedSlideCount -eq $ConfiguredDefaultSlides) "MCP omitted slideCount uses the administrator default"

$Rotated = Invoke-PtiumJson -Method POST -Path "/api/v1/api-keys/$KeyId/rotate" -Body @{ graceSeconds = 60 }
$NewKeyId = [string]$Rotated.data.apiKey.id
$NewSecret = [string]$Rotated.data.secret
Assert-True ($NewSecret.StartsWith("ptium_") -and $NewSecret -ne $OldSecret) "API-key rotation creates a new secret"
$null = Invoke-PtiumJson -Method GET -Path "/api/v1/presentations" -Headers $KeyHeaders
$null = Invoke-PtiumJson -Method GET -Path "/api/v1/presentations" -Headers @{ Authorization = "Bearer $NewSecret" }

$null = Invoke-PtiumJson -Method DELETE -Path "/api/v1/api-keys/$KeyId"
$RevokedResponse = Invoke-WebRequest -Uri "$BaseUrl/api/v1/presentations" -Headers $KeyHeaders -SkipHttpErrorCheck
Assert-True ($RevokedResponse.StatusCode -eq 401) "revocation invalidates the predecessor immediately"

$Settings = Invoke-PtiumJson -Method GET -Path "/api/v1/admin/settings"
$SecretSetting = @($Settings.data | Where-Object { $_.key -eq "ai.api_key" })[0]
Assert-True ($null -eq $SecretSetting.value) "sensitive administrator settings are write-only"
$null = Invoke-PtiumJson -Method PATCH -Path "/api/v1/admin/settings" -Body @{
    section = "branding"
    values = @{ product_name = "Ptium"; brand_color = "#8068E8" }
}

$AtomicSettingsFailure = Invoke-WebRequest -Method PATCH -Uri "$BaseUrl/api/v1/admin/settings" -Headers $DevHeaders -ContentType "application/json" -Body '{"settings":[{"key":"branding.product_name","value":"MUST NOT COMMIT"},{"key":"branding.brand_color","value":"invalid"}]}' -SkipHttpErrorCheck
Assert-True ($AtomicSettingsFailure.StatusCode -eq 422) "invalid settings batches are rejected"
$SettingsAfterAtomicFailure = Invoke-PtiumJson -Method GET -Path "/api/v1/admin/settings"
$ProductNameAfterAtomicFailure = @($SettingsAfterAtomicFailure.data | Where-Object { $_.key -eq "branding.product_name" })[0]
Assert-True ($ProductNameAfterAtomicFailure.value -eq "Ptium") "settings batches commit atomically"

$Users = Invoke-PtiumJson -Method GET -Path "/api/v1/admin/users"
Assert-True ($Users.meta.total -ge 1) "administrator user management list"
$SmokeUser = @($Users.data | Where-Object { $_.email -eq $Me.data.user.email })[0]
Assert-True ($SmokeUser.presentationsCount -ge 2) "administrator user list reports the actual presentation count"
$Overview = Invoke-PtiumJson -Method GET -Path "/api/v1/admin/overview"
Assert-True ($Overview.data.presentations -ge 1) "administrator overview"

# Force a controlled generation failure, then prove the administrator incident
# lifecycle can acknowledge and resolve the persisted, redacted failure.
$Failing = $null
try {
    $null = Invoke-PtiumJson -Method PATCH -Path "/api/v1/admin/settings" -Body @{
        section = "ai"
        values = @{ provider = "openai-compatible"; base_url = "http://127.0.0.1:9/v1"; api_key = "smoke-provider-secret" }
    }
    $Failing = Invoke-PtiumJson -Method POST -Path "/api/v1/presentations/generate" -Body @{
        title = "Expected smoke failure"
        prompt = "This deck intentionally exercises incident management."
        theme = "modern"
        language = "en"
        audience = "operators"
        tone = "technical"
        requestedSlideCount = 3
    }
    $FailedDeck = Wait-ForPresentation -Id ([string]$Failing.data.id) -Statuses @("failed")
    Assert-True ($FailedDeck.status -eq "failed") "controlled generation failure"

    $Errors = Invoke-PtiumJson -Method GET -Path "/api/v1/admin/errors?status=open"
    Assert-True ($Errors.meta.total -ge 1) "generation failure persisted for administrators"
    $Incident = @($Errors.data | Where-Object { $_.kind -eq "generation" })[0]
    Assert-True ($null -ne $Incident) "generation incident is queryable"
    Assert-True (-not (($Incident | ConvertTo-Json -Depth 20).Contains("smoke-provider-secret"))) "incident details redact secrets"
    $Acknowledged = Invoke-PtiumJson -Method PATCH -Path "/api/v1/admin/errors/$($Incident.id)" -Body @{ status = "acknowledged"; notes = "Smoke triage" }
    Assert-True ($Acknowledged.data.status -eq "acknowledged") "incident acknowledgement"
    $Resolved = Invoke-PtiumJson -Method PATCH -Path "/api/v1/admin/errors/$($Incident.id)" -Body @{ status = "resolved" }
    Assert-True ($Resolved.data.status -eq "resolved") "incident resolution"
    Assert-True ($Resolved.data.notes -eq "Smoke triage") "incident status updates preserve an omitted operations note"
} finally {
    $null = Invoke-PtiumJson -Method PATCH -Path "/api/v1/admin/settings" -Body @{
        section = "ai"
        values = @{ provider = "fallback"; base_url = "https://api.openai.com/v1"; api_key = "" }
    }
}

$null = Invoke-PtiumJson -Method DELETE -Path "/api/v1/api-keys/$NewKeyId"
$null = Invoke-PtiumJson -Method DELETE -Path "/api/v1/presentations/$McpPresentationId"
if ($null -ne $Failing) { $null = Invoke-PtiumJson -Method DELETE -Path "/api/v1/presentations/$($Failing.data.id)" }

Write-Host "Ptium smoke test passed. presentation=$PresentationId"
