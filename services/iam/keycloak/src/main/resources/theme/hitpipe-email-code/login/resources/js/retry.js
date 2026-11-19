const retryHint = document.querySelector("#hitpipe-email-code-retry[data-ready-text]");
const form = document.getElementById("hitpipe-email-code-form");
if (retryHint && form) {
    const seconds = Number(form.dataset.retrySeconds);
    if (Number.isFinite(seconds) && seconds > 0) {
        window.setTimeout(() => {
            retryHint.textContent = retryHint.dataset.readyText;
            form.dataset.retrySeconds = "0";
        }, seconds * 1_000);
    }
}
