package dev.hitpipe.keycloak.email;

import jakarta.ws.rs.core.MultivaluedMap;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Function;
import org.keycloak.authentication.RequiredActionContext;
import org.keycloak.authentication.RequiredActionProvider;
import org.keycloak.email.EmailException;
import org.keycloak.email.EmailTemplateProvider;
import org.keycloak.forms.login.LoginFormsProvider;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.utils.KeycloakModelUtils;
import org.keycloak.sessions.AuthenticationSessionModel;

final class EmailCodeAction implements RequiredActionProvider {
    private static final System.Logger LOG = System.getLogger(EmailCodeAction.class.getName());
    private static final String GENERATION = "hitpipe.email-code.generation";
    private static final String RETRY_AT = "hitpipe.email-code.retry-at";
    private static final String DELIVERY = "hitpipe.email-code.delivery";
    private static final String REQUEST_VERIFIED_USER = "hitpipe.email-code.request-verified-user";
    private static final String UNAVAILABLE = "hpEmailCodeUnavailable";
    private static final String STALE = "hpEmailCodeStale";
    private static final String SENT = "hpEmailCodeSent";
    private static final String DELIVERY_UNKNOWN = "hpEmailCodeDeliveryUnknown";
    private final EmailCodeStore store;
    private final Clock clock;

    EmailCodeAction(EmailCodeStore store, Clock clock) {
        this.store = store;
        this.clock = clock;
    }

    @Override
    public void evaluateTriggers(RequiredActionContext context) {
        Object verifiedInRequest = context.getSession().getAttribute(REQUEST_VERIFIED_USER);
        if (!context.getUser().isEmailVerified() && !userContext(context).equals(verifiedInRequest)) {
            context.getAuthenticationSession().addRequiredAction(EmailCodeActionFactory.ID);
        }
    }

    @Override
    public void requiredActionChallenge(RequiredActionContext context) {
        if (finishIfAlreadyVerified(context)) {
            return;
        }
        if (!canRun(context)) {
            challenge(context, UNAVAILABLE, null);
            return;
        }
        AuthenticationSessionModel auth = context.getAuthenticationSession();
        if (auth.getAuthNote(GENERATION) != null) {
            String delivery = auth.getAuthNote(DELIVERY);
            challenge(context, DELIVERY_UNKNOWN.equals(delivery) ? delivery : null, SENT.equals(delivery) ? delivery : null);
            return;
        }
        issue(context);
    }

    @Override
    public void processAction(RequiredActionContext context) {
        if (!canRun(context)) {
            challenge(context, UNAVAILABLE, null);
            return;
        }
        MultivaluedMap<String, String> form = context.getHttpRequest().getDecodedFormParameters();
        String operation = singleValue(form, "operation");
        if ("resend".equals(operation)) {
            issue(context);
        } else if ("verify".equals(operation)) {
            verify(context, singleValue(form, "generation"), singleValue(form, "code"));
        } else {
            challenge(context, "hpEmailCodeBadRequest", null);
        }
    }

    private void issue(RequiredActionContext context) {
        if (!hasSmtp(context)) {
            challenge(context, UNAVAILABLE, null);
            return;
        }
        Instant now = clock.instant();
        EmailCodes.Context input = codeContext(context, 0);
        EmailCodeStore.IssueResult issue;
        try {
            issue = committed(context, session -> store.issue(session, input, now));
        } catch (RuntimeException exception) {
            LOG.log(System.Logger.Level.WARNING, "Email code issuance did not complete");
            challenge(context, UNAVAILABLE, null);
            return;
        }
        AuthenticationSessionModel auth = context.getAuthenticationSession();
        if (issue.generation() > 0) {
            auth.setAuthNote(GENERATION, Long.toString(issue.generation()));
        }
        switch (issue.status()) {
            case ISSUED -> {
                setRetryAt(auth, issue.retryAt());
                // The issue transaction has closed. Mail transport never holds a database lock.
                auth.setAuthNote(DELIVERY, DELIVERY_UNKNOWN);
                try {
                    Map<String, Object> mailAttributes = new HashMap<>();
                    mailAttributes.put("verificationCode", issue.code());
                    context.getSession().getProvider(EmailTemplateProvider.class)
                            .setRealm(context.getRealm())
                            .setUser(context.getUser())
                            .send("hpEmailCodeSubject", "hitpipe-email-code.ftl", mailAttributes, input.email());
                } catch (EmailException | RuntimeException exception) {
                    LOG.log(System.Logger.Level.WARNING, "Email code delivery could not be confirmed");
                    challenge(context, DELIVERY_UNKNOWN, null);
                    return;
                }
                auth.setAuthNote(DELIVERY, SENT);
                challenge(context, null, SENT);
            }
            case COOLDOWN -> {
                setRetryAt(auth, issue.retryAt());
                challenge(context, "hpEmailCodeCooldown", null);
            }
            case RATE_LIMIT -> {
                setRetryAt(auth, issue.retryAt());
                challenge(context, "hpEmailCodeRateLimited", null);
            }
            case ALREADY_VERIFIED, CONTEXT_INVALID -> challenge(context, STALE, null);
        }
    }

    private void verify(RequiredActionContext context, String generationText, String code) {
        long generation = generation(generationText);
        if (generation < 1) {
            challenge(context, STALE, null);
            return;
        }
        EmailCodes.Context input = codeContext(context, generation);
        EmailCodeStore.SubmitResult result;
        try {
            result = committed(context, session -> store.submit(session, input, code, clock.instant()));
        } catch (RuntimeException exception) {
            LOG.log(System.Logger.Level.WARNING, "Email verification transaction did not complete");
            challenge(context, UNAVAILABLE, null);
            return;
        }
        switch (result) {
            case ACCEPTED -> {
                // The HTTP session may still have a user model loaded before the commit.
                // Prevent a redundant trigger in this request without writing emailVerified again.
                context.getSession().setAttribute(REQUEST_VERIFIED_USER, userContext(context));
                finish(context);
            }
            case INVALID -> challenge(context, "hpEmailCodeInvalid", null);
            case EXPIRED -> challenge(context, "hpEmailCodeExpired", null);
            case LOCKED -> challenge(context, "hpEmailCodeLocked", null);
            case STALE, CONTEXT_INVALID -> challenge(context, STALE, null);
        }
    }

    private static <T> T committed(RequiredActionContext context, Function<KeycloakSession, T> operation) {
        KeycloakSession request = context.getSession();
        return KeycloakModelUtils.runJobInTransactionWithResult(
                request.getKeycloakSessionFactory(), request.getContext(), session -> {
                    T result = operation.apply(session);
                    if (session.getTransactionManager().getRollbackOnly()) {
                        throw new IllegalStateException("Email verification transaction marked for rollback");
                    }
                    return result;
                }, EmailCodeActionFactory.ID);
    }

    private static EmailCodes.Context codeContext(RequiredActionContext context, long generation) {
        AuthenticationSessionModel auth = context.getAuthenticationSession();
        String flow = auth.getParentSession().getId() + "/" + auth.getTabId() + "/" + auth.getClient().getId();
        return new EmailCodes.Context(context.getRealm().getId(), context.getUser().getId(),
                context.getUser().getEmail(), flow, generation);
    }

    private record UserContext(String realm, String user, String email) {}

    private static UserContext userContext(RequiredActionContext context) {
        return new UserContext(context.getRealm().getId(), context.getUser().getId(), context.getUser().getEmail());
    }

    private boolean canRun(RequiredActionContext context) {
        String email = context.getUser().getEmail();
        return store != null && email != null && !email.isBlank()
                && !context.getRealm().isVerifyEmail() && !context.getRealm().isDuplicateEmailsAllowed();
    }

    private static boolean hasSmtp(RequiredActionContext context) {
        Map<String, String> smtp = context.getRealm().getSmtpConfig();
        return smtp != null && !smtp.getOrDefault("host", "").isBlank() && !smtp.getOrDefault("from", "").isBlank();
    }

    private static boolean finishIfAlreadyVerified(RequiredActionContext context) {
        if (!context.getUser().isEmailVerified()) {
            return false;
        }
        finish(context);
        return true;
    }

    private static void finish(RequiredActionContext context) {
        AuthenticationSessionModel auth = context.getAuthenticationSession();
        auth.removeAuthNote(GENERATION);
        auth.removeAuthNote(RETRY_AT);
        auth.removeAuthNote(DELIVERY);
        context.success();
    }

    private void challenge(RequiredActionContext context, String error, String info) {
        AuthenticationSessionModel auth = context.getAuthenticationSession();
        long generation = generation(auth.getAuthNote(GENERATION));
        LoginFormsProvider form = context.form()
                .setAttribute("hpGeneration", generation)
                .setAttribute("hpRetrySeconds", retrySeconds(auth))
                .setAttribute("hpStoreReady", canRun(context))
                .setAttribute("hpCanResend", canRun(context) && hasSmtp(context))
                .setAttribute("hpStatus", status(error, info));
        if (error != null) {
            form.setError(error);
        } else if (info != null) {
            form.setInfo(info);
        }
        context.challenge(form.createForm("hitpipe-email-code.ftl"));
    }

    private static String status(String error, String info) {
        if (error == null) {
            return SENT.equals(info) ? "sent" : "awaiting";
        }
        return switch (error) {
            case DELIVERY_UNKNOWN -> "smtp_failed";
            case "hpEmailCodeCooldown" -> "cooldown";
            case "hpEmailCodeRateLimited" -> "rate_limited";
            case "hpEmailCodeInvalid" -> "invalid";
            case "hpEmailCodeLocked" -> "locked";
            case "hpEmailCodeExpired" -> "expired";
            case STALE -> "stale";
            default -> "unavailable";
        };
    }

    private long retrySeconds(AuthenticationSessionModel auth) {
        String note = auth.getAuthNote(RETRY_AT);
        if (note == null) {
            return 0;
        }
        try {
            return Math.max(0, Math.ceilDiv(Duration.between(clock.instant(), Instant.parse(note)).toMillis(), 1000));
        } catch (DateTimeParseException | ArithmeticException exception) {
            return 0;
        }
    }

    private static void setRetryAt(AuthenticationSessionModel auth, Instant retryAt) {
        if (retryAt == null) {
            auth.removeAuthNote(RETRY_AT);
        } else {
            auth.setAuthNote(RETRY_AT, retryAt.toString());
        }
    }

    private static long generation(String value) {
        if (value == null || value.isEmpty() || value.length() > 19) {
            return 0;
        }
        try {
            return Math.max(0, Long.parseLong(value));
        } catch (NumberFormatException exception) {
            return 0;
        }
    }

    private static String singleValue(MultivaluedMap<String, String> form, String name) {
        List<String> values = form.get(name);
        return values != null && values.size() == 1 ? values.getFirst() : null;
    }

    @Override
    public void close() {
        // The request owns its own Keycloak providers.
    }
}
