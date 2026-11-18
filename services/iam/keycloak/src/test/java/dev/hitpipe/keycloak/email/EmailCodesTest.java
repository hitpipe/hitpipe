package dev.hitpipe.keycloak.email;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Instant;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.Test;

class EmailCodesTest {
    private static final Instant EXPIRY = Instant.parse("2026-11-18T15:10:00Z");
    private static final EmailCodes.Context CONTEXT = context("realm", "user", "alice@example.test", "flow", 1);

    private static byte[] key(int value) {
        byte[] key = new byte[32];
        Arrays.fill(key, (byte) value);
        return key;
    }

    private static EmailCodes.Context context(String realm, String user, String email, String flow, long generation) {
        return new EmailCodes.Context(realm, user, email, flow, generation);
    }

    @Test
    void codeHasExactlyEightDecimalDigitsIncludingLeadingZeroes() {
        try (EmailCodes zero = new EmailCodes(key(1), 1, key(2), bound -> {
            assertEquals(100_000_000, bound);
            return 0;
        }); EmailCodes largest = new EmailCodes(key(1), 1, key(2), bound -> bound - 1)) {
            assertEquals("00000000", zero.generateCode());
            assertEquals("99999999", largest.generateCode());
        }
    }

    @Test
    void secureRandomFailureNeverFallsBackToAnotherGenerator() {
        IllegalStateException unavailable = new IllegalStateException("test random failure");
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2), bound -> {
            throw unavailable;
        })) {
            assertEquals(unavailable, assertThrows(IllegalStateException.class, codes::generateCode));
        }
    }

    @Test
    void invalidRandomOutputFailsClosed() {
        for (int value : new int[] {-1, 100_000_000}) {
            try (EmailCodes codes = new EmailCodes(key(1), 1, key(2), bound -> value)) {
                assertThrows(IllegalStateException.class, codes::generateCode);
            }
        }
    }

    @Test
    void sameContextResendRedrawsThePreviousDigits() {
        AtomicInteger draws = new AtomicInteger();
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2),
                bound -> draws.getAndIncrement() == 0 ? 12_345_678 : 12_345_679)) {
            byte[] previous = codes.codeMac(CONTEXT, 1, EXPIRY, "12345678");
            assertEquals("12345679", codes.generateReplacementCode(CONTEXT, 1, EXPIRY, previous));
            assertEquals(2, draws.get());
        }
    }

    @Test
    void repeatedPreviousDigitsFailClosedAfterBoundedDraws() {
        AtomicInteger draws = new AtomicInteger();
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2), bound -> {
            draws.incrementAndGet();
            return 12_345_678;
        })) {
            byte[] previous = codes.codeMac(CONTEXT, 1, EXPIRY, "12345678");
            assertThrows(IllegalStateException.class,
                    () -> codes.generateReplacementCode(CONTEXT, 1, EXPIRY, previous));
            assertEquals(8, draws.get());
        }
    }

    @Test
    void publicGeneratorUsesCSPRNGAndFixedCodeShape() {
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2))) {
            assertTrue(codes.generateCode().matches("[0-9]{8}"));
        }
    }

    @Test
    void macBindsEveryChallengeDimensionAndCode() {
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2))) {
            byte[] baseline = codes.codeMac(CONTEXT, 1, EXPIRY, "12345678");
            assertEquals(32, baseline.length);
            List<EmailCodes.Context> changed = List.of(
                    context("another-realm", "user", CONTEXT.email(), "flow", 1),
                    context("realm", "another-user", CONTEXT.email(), "flow", 1),
                    context("realm", "user", "other@example.test", "flow", 1),
                    context("realm", "user", CONTEXT.email(), "another-flow", 1));
            for (EmailCodes.Context value : changed) {
                assertFalse(EmailCodes.matches(baseline, codes.codeMac(value, 1, EXPIRY, "12345678")));
            }
            assertFalse(EmailCodes.matches(baseline, codes.codeMac(CONTEXT, 2, EXPIRY, "12345678")));
            assertFalse(EmailCodes.matches(baseline, codes.codeMac(CONTEXT, 1, EXPIRY.plusSeconds(1), "12345678")));
            assertFalse(EmailCodes.matches(baseline, codes.codeMac(CONTEXT, 1, EXPIRY, "12345679")));
        }
    }

    @Test
    void structuredEncodingPreventsConcatenationAmbiguity() {
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2))) {
            EmailCodes.Context first = context("a", "bc", "alice@example.test", "flow", 1);
            EmailCodes.Context second = context("ab", "c", "alice@example.test", "flow", 1);
            assertFalse(EmailCodes.matches(codes.codeMac(first, 1, EXPIRY, "12345678"),
                    codes.codeMac(second, 1, EXPIRY, "12345678")));
        }
    }

    @Test
    void emailAndFlowDomainsAreIndependentAndRevocationCannotMatchAnActiveFlow() {
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2))) {
            EmailCodes.Context sameValue = context("realm", "user", "same", "same", 1);
            assertFalse(EmailCodes.matches(codes.emailMac(sameValue), codes.flowMac(sameValue)));
            assertFalse(EmailCodes.matches(codes.flowMac(sameValue), codes.revokedFlowMac(sameValue)));
        }
    }

    @Test
    void rateIdentitySurvivesNewFlowAndChallengeKeyRotation() {
        try (EmailCodes original = new EmailCodes(key(1), 1, key(2));
             EmailCodes rotated = new EmailCodes(key(3), 2, key(2))) {
            EmailCodes.Context nextFlow = context("realm", "user", "ALICE@EXAMPLE.TEST", "new-flow", 0);
            assertArrayEquals(original.recipientMac(CONTEXT), original.recipientMac(nextFlow));
            assertArrayEquals(original.recipientMac(CONTEXT), rotated.recipientMac(nextFlow));
            assertFalse(EmailCodes.matches(original.emailMac(CONTEXT), rotated.emailMac(CONTEXT)));
            assertNotEquals(original.keyVersion(), rotated.keyVersion());
        }
    }

    @Test
    void rateIdentityBindsRealmAndCanonicalRecipient() {
        try (EmailCodes codes = new EmailCodes(key(1), 1, key(2))) {
            assertFalse(EmailCodes.matches(codes.recipientMac(CONTEXT),
                    codes.recipientMac(context("other", "user", CONTEXT.email(), "flow", 1))));
            assertFalse(EmailCodes.matches(codes.recipientMac(CONTEXT),
                    codes.recipientMac(context("realm", "user", "other@example.test", "flow", 1))));
        }
    }

    @Test
    void callerCannotMutateKeysAfterConstruction() {
        byte[] challengeKey = key(1);
        byte[] rateKey = key(2);
        try (EmailCodes codes = new EmailCodes(challengeKey, 1, rateKey);
             EmailCodes reference = new EmailCodes(key(1), 1, key(2))) {
            Arrays.fill(challengeKey, (byte) 9);
            Arrays.fill(rateKey, (byte) 9);
            assertArrayEquals(reference.emailMac(CONTEXT), codes.emailMac(CONTEXT));
            assertArrayEquals(reference.recipientMac(CONTEXT), codes.recipientMac(CONTEXT));
        }
    }

    @Test
    void invalidKeysAndVersionCannotInitializeExtension() {
        assertThrows(IllegalArgumentException.class, () -> new EmailCodes(new byte[31], 1, key(2)));
        assertThrows(IllegalArgumentException.class, () -> new EmailCodes(key(1), 1, new byte[31]));
        assertThrows(IllegalArgumentException.class, () -> new EmailCodes(key(1), 0, key(2)));
    }

    @Test
    void closedKeysCannotIssueOrAuthenticateCodes() {
        EmailCodes codes = new EmailCodes(key(1), 1, key(2));
        codes.close();
        assertThrows(IllegalStateException.class, codes::generateCode);
        assertThrows(IllegalStateException.class, () -> codes.codeMac(CONTEXT, 1, EXPIRY, "12345678"));
    }

    @Test
    void diagnosticRepresentationsDoNotContainSecretsOrRecipient() {
        String context = CONTEXT.toString();
        assertFalse(context.contains(CONTEXT.email()));
        assertFalse(context.contains(CONTEXT.flowId()));
        EmailCodeStore.IssueResult result = new EmailCodeStore.IssueResult(
                EmailCodeStore.IssueStatus.ISSUED, 1, "12345678", EXPIRY, EXPIRY.minusSeconds(540));
        assertFalse(result.toString().contains("12345678"));
    }
}
