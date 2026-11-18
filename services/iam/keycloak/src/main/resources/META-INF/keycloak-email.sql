-- Authored MVP candidate. Apply only through the Keycloak extension migration,
-- in the IdP-owned database, never in the IAM application database.
-- The extension uses its own JPA entity/transaction and SELECT FOR UPDATE.
-- No application service receives an IdP database role or realm-admin token.
CREATE TABLE hitpipe_email_code_challenge (
    realm_id varchar(255) NOT NULL,
    user_id varchar(255) NOT NULL,
    email_mac bytea NOT NULL CHECK (octet_length(email_mac) = 32),
    flow_mac bytea NOT NULL CHECK (octet_length(flow_mac) = 32),
    generation bigint NOT NULL CHECK (generation > 0),
    code_mac bytea NOT NULL CHECK (octet_length(code_mac) = 32),
    key_version integer NOT NULL CHECK (key_version > 0),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    consumed_at timestamptz,
    PRIMARY KEY (realm_id, user_id),
    CHECK (expires_at > issued_at AND expires_at <= issued_at + interval '10 minutes'),
    CHECK (consumed_at IS NULL OR consumed_at >= issued_at)
);

CREATE TABLE hitpipe_email_code_rate (
    realm_id varchar(255) NOT NULL,
    recipient_mac bytea NOT NULL CHECK (octet_length(recipient_mac) = 32),
    send_times timestamptz[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (realm_id, recipient_mac),
    CHECK (cardinality(send_times) <= 5)
);

-- Issuance locks rate then challenge in one transaction, removes timestamps
-- older than one hour, checks five-send rolling limit and 60-second cooldown,
-- advances generation and replaces the old challenge before network SMTP.
-- Code MAC is HMAC-SHA256 with a separately supplied extension secret, covering
-- realm/user/email/flow/generation/expiry/code. A plain code hash is insufficient
-- against offline guessing of an eight-digit code. The key never enters SQL.
-- Submission locks challenge, checks the exact auth-session/email/generation,
-- expiry, attempt limit and MAC, then consumes it and updates emailVerified in
-- the SAME IdP transaction. A transaction rollback cannot produce an accepted
-- code or a HitPipe session. Parallel submissions permit one consumption.
-- Network SMTP is outside the lock/transaction. Unconfirmed delivery is shown
-- truthfully; restart before sending requires a later rate-limited resend.
-- Duplicate realm emails are disabled. Rate identity derives from the IdP's
-- canonical recipient plus realm, with keyed MAC; new tabs/flows do not reset it.
-- Expired challenge/rate records are purged after the last applicable one-hour
-- window; deletion of the IdP user also deletes its challenge. Secret rotation
-- invalidates outstanding challenges rather than accepting an unknown key.
