<#import "template.ftl" as layout>
<@layout.registrationLayout displayInfo=false displayMessage=true; section>
    <#if section == "header">
        ${msg("hpEmailCodeTitle")}
    <#elseif section == "form">
        <p id="hitpipe-email-code-instructions">${msg("hpEmailCodeInstructions")}</p>
        <form id="hitpipe-email-code-form" action="${url.loginAction}" method="post"
              data-status="${hpStatus}" data-retry-seconds="${hpRetrySeconds?c}">
            <input type="hidden" name="generation" value="${hpGeneration?c}">
            <#if hpGeneration gt 0 && hpStoreReady>
                <div class="${properties.kcFormGroupClass!}">
                    <label class="${properties.kcLabelClass!}" for="hitpipe-email-code">${msg("hpEmailCodeLabel")}</label>
                    <input class="${properties.kcInputClass!}" id="hitpipe-email-code" name="code"
                           inputmode="numeric" pattern="[0-9]{8}" minlength="8" maxlength="8"
                           autocomplete="one-time-code" aria-describedby="hitpipe-email-code-instructions"
                           required>
                </div>
                <div class="${properties.kcFormGroupClass!}">
                    <button class="${properties.kcButtonClass!} ${properties.kcButtonPrimaryClass!}"
                            type="submit" name="operation" value="verify">${msg("hpEmailCodeVerify")}</button>
                </div>
            </#if>
            <div class="${properties.kcFormGroupClass!}">
                <#if !hpCanResend>
                    <p id="hitpipe-email-code-retry">${msg("hpEmailCodeResendUnavailable")}</p>
                <#elseif hpRetrySeconds gt 0>
                    <p id="hitpipe-email-code-retry" aria-live="polite"
                       data-ready-text="${msg("hpEmailCodeRetryReady")}">${msg("hpEmailCodeRetryAfter", hpRetrySeconds?c)}</p>
                <#else>
                    <p id="hitpipe-email-code-retry">${msg("hpEmailCodeRetryReady")}</p>
                </#if>
                <button class="${properties.kcButtonClass!} ${properties.kcButtonDefaultClass!}"
                        type="submit" name="operation" value="resend" formnovalidate
                        aria-describedby="hitpipe-email-code-retry" <#if !hpCanResend>disabled</#if>>
                    ${msg("hpEmailCodeResend")}
                </button>
            </div>
        </form>
        <script type="module" src="${url.resourcesPath}/js/retry.js"></script>
    </#if>
</@layout.registrationLayout>
