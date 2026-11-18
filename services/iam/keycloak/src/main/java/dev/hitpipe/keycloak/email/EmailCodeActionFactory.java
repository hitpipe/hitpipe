package dev.hitpipe.keycloak.email;

import java.security.MessageDigest;
import java.time.Clock;
import java.time.Duration;
import java.util.Arrays;
import java.util.Base64;
import org.keycloak.Config;
import org.keycloak.authentication.RequiredActionFactory;
import org.keycloak.authentication.RequiredActionProvider;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;
import org.keycloak.models.UserModel;
import org.keycloak.models.utils.KeycloakModelUtils;
import org.keycloak.timer.TimerProvider;

public final class EmailCodeActionFactory implements RequiredActionFactory {
    public static final String ID = "hitpipe-email-code";
    private static final String PURGE = ID + "-purge";
    private static final System.Logger LOG = System.getLogger(EmailCodeActionFactory.class.getName());
    private final Clock clock = Clock.systemUTC();
    private EmailCodes codes;
    private EmailCodeStore store;

    @Override
    public RequiredActionProvider create(KeycloakSession session) {
        return new EmailCodeAction(store, clock);
    }

    EmailCodeStore store() {
        return store;
    }

    @Override
    public String getId() {
        return ID;
    }

    @Override
    public String getDisplayText() {
        return "Verify email with a HitPipe code";
    }

    @Override
    public boolean isOneTimeAction() {
        return true;
    }

    @Override
    public void init(Config.Scope config) {
        byte[] codeKey = null;
        byte[] rateKey = null;
        try {
            codeKey = readKey("HITPIPE_EMAIL_CODE_KEY");
            rateKey = readKey("HITPIPE_EMAIL_RATE_KEY");
            int version = Integer.parseInt(System.getenv("HITPIPE_EMAIL_CODE_KEY_VERSION"));
            if (MessageDigest.isEqual(codeKey, rateKey)) {
                throw new IllegalArgumentException("Email verification requires two distinct keys");
            }
            codes = new EmailCodes(codeKey, version, rateKey);
            store = new EmailCodeStore(codes);
        } catch (RuntimeException exception) {
            LOG.log(System.Logger.Level.WARNING, "Email code keys are unavailable or invalid");
        } finally {
            if (codeKey != null) Arrays.fill(codeKey, (byte) 0);
            if (rateKey != null) Arrays.fill(rateKey, (byte) 0);
        }
    }

    @Override
    public void postInit(KeycloakSessionFactory factory) {
        if (store == null) {
            return;
        }
        factory.register(event -> {
            if (event instanceof UserModel.UserRemovedEvent removed) {
                store.deleteUser(removed.getKeycloakSession(), removed.getRealm().getId(), removed.getUser().getId());
            }
        });
        long interval = Duration.ofMinutes(1).toMillis();
        KeycloakModelUtils.runJobInTransaction(factory, session -> session.getProvider(TimerProvider.class)
                .scheduleTask(tx -> store.purge(tx, clock.instant()), interval, PURGE));
    }

    @Override
    public void close() {
        if (codes != null) {
            codes.close();
        }
    }

    private static byte[] readKey(String variable) {
        String value = System.getenv(variable);
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException("Email verification key is missing");
        }
        return Base64.getDecoder().decode(value);
    }
}
