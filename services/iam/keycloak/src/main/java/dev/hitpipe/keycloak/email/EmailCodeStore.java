package dev.hitpipe.keycloak.email;

import jakarta.persistence.EntityManager;
import java.sql.Array;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import org.hibernate.Session;
import org.hibernate.jdbc.ReturningWork;
import org.keycloak.connections.jpa.JpaConnectionProvider;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.RealmModel;
import org.keycloak.models.UserModel;

/** PostgreSQL storage enlisted in the caller's Keycloak transaction. */
public final class EmailCodeStore {
    private static final Duration TTL = Duration.ofMinutes(10);
    private static final Duration WINDOW = Duration.ofHours(1);
    private static final Duration COOLDOWN = Duration.ofSeconds(60);
    private static final int MAX_SENDS = 5;
    private static final int MAX_ATTEMPTS = 5;
    private final EmailCodes codes;

    public enum IssueStatus {
        ISSUED, COOLDOWN, RATE_LIMIT, CONTEXT_INVALID, ALREADY_VERIFIED
    }

    public enum SubmitResult {
        ACCEPTED, INVALID, EXPIRED, LOCKED, STALE, CONTEXT_INVALID
    }

    public record IssueResult(IssueStatus status, long generation, String code,
                              Instant expiresAt, Instant retryAt) {
        @Override
        public String toString() {
            return "EmailCodeIssue[status=" + status + ", generation=" + generation + "]";
        }
    }

    private record Challenge(byte[] emailMac, byte[] flowMac, long generation, byte[] codeMac,
                             int keyVersion, Instant issuedAt, Instant expiresAt,
                             int failedAttempts, Instant consumedAt) {}

    private record LockedUser(UserModel user, boolean verified) {}

    public EmailCodeStore(EmailCodes codes) {
        this.codes = Objects.requireNonNull(codes);
    }

    /** Returns a sendable code only after its challenge and rate reservation have been written. */
    public IssueResult issue(KeycloakSession session, EmailCodes.Context context, Instant now) {
        Instant issuedAt = Objects.requireNonNull(now).truncatedTo(ChronoUnit.MILLIS);
        return transaction(session, connection -> {
            LockedUser locked = lockUser(session, connection, context);
            if (locked == null) {
                return notIssued(IssueStatus.CONTEXT_INVALID, null);
            }
            if (locked.verified()) {
                return notIssued(IssueStatus.ALREADY_VERIFIED, null);
            }

            // A local user lock also serializes an email edit/removal and an initial missing challenge.
            // Issuance always takes the recipient rate lock before the challenge row lock.
            byte[] recipient = codes.recipientMac(context);
            List<Instant> sends = lockRate(connection, context.realmId(), recipient, issuedAt);
            Challenge previous = lockChallenge(connection, context);
            boolean sameContext = previous != null && previous.keyVersion() == codes.keyVersion()
                    && EmailCodes.matches(previous.emailMac(), codes.emailMac(context))
                    && EmailCodes.matches(previous.flowMac(), codes.flowMac(context));
            long activeGeneration = sameContext && previous.consumedAt() == null ? previous.generation() : 0;
            if (previous != null && previous.consumedAt() == null && !sameContext) {
                // A new email/authentication flow revokes the previous challenge even during cooldown.
                try (PreparedStatement statement = connection.prepareStatement("""
                        UPDATE hitpipe_email_code_challenge SET flow_mac = ?
                        WHERE realm_id = ? AND user_id = ?
                        """)) {
                    statement.setBytes(1, codes.revokedFlowMac(context));
                    statement.setString(2, context.realmId());
                    statement.setString(3, context.userId());
                    statement.executeUpdate();
                }
            }
            if (sends.size() >= MAX_SENDS) {
                return new IssueResult(IssueStatus.RATE_LIMIT, activeGeneration, null, null,
                        sends.get(0).plus(WINDOW));
            }
            if (!sends.isEmpty()) {
                Instant retryAt = sends.get(sends.size() - 1).plus(COOLDOWN);
                if (issuedAt.isBefore(retryAt)) {
                    return new IssueResult(IssueStatus.COOLDOWN, activeGeneration, null, null, retryAt);
                }
            }

            long generation = previous == null ? 1 : Math.addExact(previous.generation(), 1);
            Instant expiresAt = issuedAt.plus(TTL);
            String code = sameContext
                    ? codes.generateReplacementCode(context, previous.generation(), previous.expiresAt(), previous.codeMac())
                    : codes.generateCode();
            try (PreparedStatement statement = connection.prepareStatement("""
                    INSERT INTO hitpipe_email_code_challenge
                      (realm_id, user_id, email_mac, flow_mac, generation, code_mac,
                       key_version, issued_at, expires_at, failed_attempts, consumed_at)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL)
                    ON CONFLICT (realm_id, user_id) DO UPDATE SET
                      email_mac = EXCLUDED.email_mac, flow_mac = EXCLUDED.flow_mac,
                      generation = EXCLUDED.generation, code_mac = EXCLUDED.code_mac,
                      key_version = EXCLUDED.key_version, issued_at = EXCLUDED.issued_at,
                      expires_at = EXCLUDED.expires_at, failed_attempts = 0, consumed_at = NULL
                    """)) {
                statement.setString(1, context.realmId());
                statement.setString(2, context.userId());
                statement.setBytes(3, codes.emailMac(context));
                statement.setBytes(4, codes.flowMac(context));
                statement.setLong(5, generation);
                statement.setBytes(6, codes.codeMac(context, generation, expiresAt, code));
                statement.setInt(7, codes.keyVersion());
                statement.setTimestamp(8, Timestamp.from(issuedAt));
                statement.setTimestamp(9, Timestamp.from(expiresAt));
                statement.executeUpdate();
            }
            sends.add(issuedAt);
            saveRate(connection, context.realmId(), recipient, sends);
            return new IssueResult(IssueStatus.ISSUED, generation, code, expiresAt,
                    issuedAt.plus(COOLDOWN));
        });
    }

    /** The caller must commit this transaction before completing the required action. */
    public SubmitResult submit(KeycloakSession session, EmailCodes.Context context, String code, Instant now) {
        Objects.requireNonNull(now);
        return transaction(session, connection -> {
            LockedUser locked = lockUser(session, connection, context);
            if (locked == null) {
                return SubmitResult.CONTEXT_INVALID;
            }
            Challenge challenge = lockChallenge(connection, context);
            if (locked.verified() || challenge == null || challenge.consumedAt() != null
                    || context.generation() != challenge.generation()
                    || challenge.keyVersion() != codes.keyVersion()
                    || !EmailCodes.matches(challenge.emailMac(), codes.emailMac(context))
                    || !EmailCodes.matches(challenge.flowMac(), codes.flowMac(context))) {
                return SubmitResult.STALE;
            }
            if (now.isBefore(challenge.issuedAt()) || !now.isBefore(challenge.expiresAt())) {
                return SubmitResult.EXPIRED;
            }
            if (challenge.failedAttempts() >= MAX_ATTEMPTS) {
                return SubmitResult.LOCKED;
            }
            boolean valid = code != null && code.matches("[0-9]{8}")
                    && EmailCodes.matches(challenge.codeMac(),
                    codes.codeMac(context, challenge.generation(), challenge.expiresAt(), code));
            if (!valid) {
                try (PreparedStatement statement = connection.prepareStatement("""
                        UPDATE hitpipe_email_code_challenge SET failed_attempts = failed_attempts + 1
                        WHERE realm_id = ? AND user_id = ?
                        """)) {
                    statement.setString(1, context.realmId());
                    statement.setString(2, context.userId());
                    statement.executeUpdate();
                }
                return challenge.failedAttempts() + 1 >= MAX_ATTEMPTS
                        ? SubmitResult.LOCKED : SubmitResult.INVALID;
            }

            try (PreparedStatement statement = connection.prepareStatement("""
                    UPDATE hitpipe_email_code_challenge SET consumed_at = ?
                    WHERE realm_id = ? AND user_id = ?
                    """)) {
                statement.setTimestamp(1, Timestamp.from(now));
                statement.setString(2, context.realmId());
                statement.setString(3, context.userId());
                statement.executeUpdate();
            }
            // Uses this same managed session/JPA transaction, including its normal cache invalidation.
            locked.user().setEmailVerified(true);
            return SubmitResult.ACCEPTED;
        });
    }

    /** Purge only after every retained issuance is outside its one-hour window. */
    public void purge(KeycloakSession session, Instant now) {
        Objects.requireNonNull(now);
        transaction(session, connection -> {
            try (PreparedStatement rate = connection.prepareStatement("""
                    DELETE FROM hitpipe_email_code_rate
                    WHERE NOT EXISTS (SELECT 1 FROM unnest(send_times) AS sent_at WHERE sent_at > ?)
                    """)) {
                rate.setTimestamp(1, Timestamp.from(now.minus(WINDOW)));
                rate.executeUpdate();
            }
            try (PreparedStatement challenge = connection.prepareStatement("""
                    DELETE FROM hitpipe_email_code_challenge WHERE issued_at <= ? AND expires_at <= ?
                    """)) {
                challenge.setTimestamp(1, Timestamp.from(now.minus(WINDOW)));
                challenge.setTimestamp(2, Timestamp.from(now));
                challenge.executeUpdate();
            }
            return null;
        });
    }

    /** Invoke synchronously from the Keycloak UserRemovedEvent transaction. */
    public void deleteUser(KeycloakSession session, String realmId, String userId) {
        transaction(session, connection -> {
            try (PreparedStatement statement = connection.prepareStatement("""
                    DELETE FROM hitpipe_email_code_challenge WHERE realm_id = ? AND user_id = ?
                    """)) {
                statement.setString(1, realmId);
                statement.setString(2, userId);
                statement.executeUpdate();
            }
            // Keep recipient rate state until its window ends, including delete/re-register attempts.
            return null;
        });
    }

    void revokeOnEmailChange(KeycloakSession session, String realmId, String userId) {
        RealmModel realm = session.realms().getRealm(realmId);
        UserModel user = realm == null ? null : session.users().getUserById(realm, userId);
        if (user == null) {
            return;
        }
        // Flush the profile update first to keep the user → challenge lock order.
        session.getProvider(JpaConnectionProvider.class).getEntityManager().flush();
        String email = user.getEmail();
        if (email == null || email.isBlank()) {
            deleteUser(session, realmId, userId);
            return;
        }
        EmailCodes.Context updated = new EmailCodes.Context(realmId, userId, email, "account-email-updated", 0);
        transaction(session, connection -> {
            try (PreparedStatement statement = connection.prepareStatement("""
                    UPDATE hitpipe_email_code_challenge SET flow_mac = ?
                    WHERE realm_id = ? AND user_id = ? AND consumed_at IS NULL AND email_mac <> ?
                    """)) {
                statement.setBytes(1, codes.revokedFlowMac(updated));
                statement.setString(2, realmId);
                statement.setString(3, userId);
                statement.setBytes(4, codes.emailMac(updated));
                statement.executeUpdate();
            }
            return null;
        });
    }

    private static IssueResult notIssued(IssueStatus status, Instant retryAt) {
        return new IssueResult(status, 0, null, null, retryAt);
    }

    private static LockedUser lockUser(KeycloakSession session, Connection connection,
                                       EmailCodes.Context context) throws SQLException {
        RealmModel realm = session.realms().getRealm(context.realmId());
        if (realm == null || realm.isDuplicateEmailsAllowed()) {
            return null;
        }
        // This extension serves the local Keycloak JPA user store. A federated/absent user fails closed.
        // The row lock prevents a concurrent email update or deletion from racing consumption.
        try (PreparedStatement statement = connection.prepareStatement("""
                SELECT email, email_verified, enabled FROM user_entity
                WHERE realm_id = ? AND id = ? FOR UPDATE
                """)) {
            statement.setString(1, context.realmId());
            statement.setString(2, context.userId());
            try (ResultSet rows = statement.executeQuery()) {
                if (!rows.next() || !context.email().equals(rows.getString(1)) || !rows.getBoolean(3)) {
                    return null;
                }
                UserModel user = session.users().getUserById(realm, context.userId());
                if (user == null || !user.isEnabled() || !context.email().equals(user.getEmail())) {
                    return null;
                }
                return new LockedUser(user, rows.getBoolean(2) || user.isEmailVerified());
            }
        }
    }

    private static List<Instant> lockRate(Connection connection, String realmId, byte[] recipient,
                                          Instant now) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement("""
                INSERT INTO hitpipe_email_code_rate (realm_id, recipient_mac) VALUES (?, ?)
                ON CONFLICT (realm_id, recipient_mac) DO NOTHING
                """)) {
            statement.setString(1, realmId);
            statement.setBytes(2, recipient);
            statement.executeUpdate();
        }
        try (PreparedStatement statement = connection.prepareStatement("""
                SELECT send_times FROM hitpipe_email_code_rate
                WHERE realm_id = ? AND recipient_mac = ? FOR UPDATE
                """)) {
            statement.setString(1, realmId);
            statement.setBytes(2, recipient);
            try (ResultSet rows = statement.executeQuery()) {
                if (!rows.next()) {
                    throw new SQLException("Email-code rate reservation disappeared");
                }
                Array array = rows.getArray(1);
                try {
                    List<Instant> sends = new ArrayList<>();
                    for (Object value : (Object[]) array.getArray()) {
                        Instant sentAt = value instanceof OffsetDateTime offset
                                ? offset.toInstant() : ((Timestamp) value).toInstant();
                        if (sentAt.isAfter(now.minus(WINDOW))) {
                            sends.add(sentAt);
                        }
                    }
                    sends.sort(Instant::compareTo);
                    return sends;
                } finally {
                    array.free();
                }
            }
        }
    }

    private static void saveRate(Connection connection, String realmId, byte[] recipient,
                                 List<Instant> sends) throws SQLException {
        Array array = connection.createArrayOf("timestamptz", sends.stream().map(Timestamp::from).toArray());
        try (PreparedStatement statement = connection.prepareStatement("""
                UPDATE hitpipe_email_code_rate SET send_times = ? WHERE realm_id = ? AND recipient_mac = ?
                """)) {
            statement.setArray(1, array);
            statement.setString(2, realmId);
            statement.setBytes(3, recipient);
            statement.executeUpdate();
        } finally {
            array.free();
        }
    }

    private static Challenge lockChallenge(Connection connection, EmailCodes.Context context) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement("""
                SELECT email_mac, flow_mac, generation, code_mac, key_version, issued_at,
                       expires_at, failed_attempts, consumed_at
                FROM hitpipe_email_code_challenge WHERE realm_id = ? AND user_id = ? FOR UPDATE
                """)) {
            statement.setString(1, context.realmId());
            statement.setString(2, context.userId());
            try (ResultSet rows = statement.executeQuery()) {
                if (!rows.next()) {
                    return null;
                }
                Timestamp consumedAt = rows.getTimestamp(9);
                return new Challenge(rows.getBytes(1), rows.getBytes(2), rows.getLong(3), rows.getBytes(4),
                        rows.getInt(5), rows.getTimestamp(6).toInstant(), rows.getTimestamp(7).toInstant(),
                        rows.getInt(8), consumedAt == null ? null : consumedAt.toInstant());
            }
        }
    }

    private static <T> T transaction(KeycloakSession session, ReturningWork<T> work) {
        Objects.requireNonNull(session);
        if (!session.getTransactionManager().isActive() || session.getTransactionManager().getRollbackOnly()) {
            throw new IllegalStateException("An active Keycloak transaction is required");
        }
        try {
            EntityManager manager = session.getProvider(JpaConnectionProvider.class).getEntityManager();
            manager.flush();
            return manager.unwrap(Session.class).doReturningWork(work);
        } catch (RuntimeException failure) {
            session.getTransactionManager().setRollbackOnly();
            throw failure;
        }
    }
}
