package dev.hitpipe.keycloak.email;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Instant;
import java.util.Arrays;
import java.util.Locale;
import java.util.Objects;
import java.util.function.IntUnaryOperator;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/** Secret material is supplied to this IdP extension, never read from its SQL tables. */
public final class EmailCodes implements AutoCloseable {
    public static final int CODE_BOUND = 100_000_000;
    private final byte[] challengeKey;
    private final byte[] rateKey;
    private final int keyVersion;
    private final IntUnaryOperator random;
    private volatile boolean closed;

    public record Context(String realmId, String userId, String email, String flowId, long generation) {
        public Context {
            Objects.requireNonNull(realmId);
            Objects.requireNonNull(userId);
            Objects.requireNonNull(email);
            Objects.requireNonNull(flowId);
            if (realmId.isBlank() || userId.isBlank() || email.isBlank() || flowId.isBlank()
                    || realmId.length() > 255 || userId.length() > 255 || generation < 0) {
                throw new IllegalArgumentException("Invalid email-code context");
            }
        }

        @Override
        public String toString() {
            return "EmailCodeContext[generation=" + generation + "]";
        }
    }

    public EmailCodes(byte[] challengeKey, int keyVersion, byte[] stableRateKey) {
        this(challengeKey, keyVersion, stableRateKey, secureRandom()::nextInt);
    }

    // Allows a test to prove that a failed CSPRNG never falls back to predictable codes.
    EmailCodes(byte[] challengeKey, int keyVersion, byte[] stableRateKey, IntUnaryOperator random) {
        Objects.requireNonNull(challengeKey);
        Objects.requireNonNull(stableRateKey);
        if (challengeKey.length < 32 || stableRateKey.length < 32 || keyVersion <= 0) {
            throw new IllegalArgumentException("Email-code keys need at least 256 bits and a positive version");
        }
        this.challengeKey = challengeKey.clone();
        this.rateKey = stableRateKey.clone();
        this.keyVersion = keyVersion;
        this.random = Objects.requireNonNull(random);
        // Validate availability before the extension can accept a challenge.
        hmac(this.challengeKey, "key-check");
        hmac(this.rateKey, "key-check");
    }

    public String generateCode() {
        requireOpen();
        int value = random.applyAsInt(CODE_BOUND);
        if (value < 0 || value >= CODE_BOUND) {
            throw new IllegalStateException("Secure random generator returned an invalid value");
        }
        return String.format(Locale.ROOT, "%08d", value);
    }

    String generateReplacementCode(Context context, long previousGeneration, Instant previousExpiry,
                                   byte[] previousCodeMac) {
        for (int attempt = 0; attempt < 8; attempt++) {
            String code = generateCode();
            if (!matches(previousCodeMac, codeMac(context, previousGeneration, previousExpiry, code))) {
                return code;
            }
        }
        throw new IllegalStateException("Secure random generator repeatedly returned the previous code");
    }

    int keyVersion() {
        requireOpen();
        return keyVersion;
    }

    byte[] emailMac(Context context) {
        return hmac(challengeKey, "email", context.realmId(), context.userId(), context.email());
    }

    byte[] flowMac(Context context) {
        return hmac(challengeKey, "flow", context.realmId(), context.userId(), context.flowId());
    }

    byte[] revokedFlowMac(Context context) {
        return hmac(challengeKey, "revoked-flow", context.realmId(), context.userId(), context.flowId());
    }

    byte[] recipientMac(Context context) {
        return hmac(rateKey, "recipient", context.realmId(), context.email().toLowerCase(Locale.ROOT));
    }

    byte[] codeMac(Context context, long generation, Instant expiresAt, String code) {
        return hmac(challengeKey, "code", context.realmId(), context.userId(), context.email(),
                context.flowId(), Long.toString(generation), expiresAt.toString(), code);
    }

    static boolean matches(byte[] expected, byte[] actual) {
        return MessageDigest.isEqual(expected, actual);
    }

    private byte[] hmac(byte[] key, String... fields) {
        requireOpen();
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key, "HmacSHA256"));
            for (String field : fields) {
                byte[] bytes = field.getBytes(StandardCharsets.UTF_8);
                mac.update(ByteBuffer.allocate(Integer.BYTES).putInt(bytes.length).array());
                mac.update(bytes);
            }
            return mac.doFinal();
        } catch (GeneralSecurityException failure) {
            throw new IllegalStateException("Email-code authentication is unavailable", failure);
        }
    }

    private static SecureRandom secureRandom() {
        try {
            return SecureRandom.getInstanceStrong();
        } catch (GeneralSecurityException failure) {
            throw new IllegalStateException("Secure random generation is unavailable", failure);
        }
    }

    private void requireOpen() {
        if (closed) {
            throw new IllegalStateException("Email-code keys are closed");
        }
    }

    @Override
    public void close() {
        closed = true;
        Arrays.fill(challengeKey, (byte) 0);
        Arrays.fill(rateKey, (byte) 0);
    }
}
