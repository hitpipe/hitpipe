package dispatch

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/jackc/pgx/v5"
)

const applyPath = "/internal/v1/delivery-observations"

type Publisher struct {
	db        *Postgres
	files     ContentReader
	gate      intake.StartGate
	recipient *ClickUp
}

// NewPublisher binds a provisioned Processing origin and client identity.
// An outbox row cannot supply either, and the transport never follows redirects.
func NewPublisher(db *Postgres, files ContentReader, gate intake.StartGate, origin string,
	roots *x509.CertPool, cert tls.Certificate) (*Publisher, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || roots == nil || len(cert.Certificate) == 0 ||
		cert.PrivateKey == nil || db == nil || files == nil || gate == nil {
		return nil, ErrUnwired
	}
	client := clickUpAt(*u, roots)
	client.tlsConfig.Certificates = []tls.Certificate{cert}
	return &Publisher{db, files, gate, client}, nil
}

type claimed struct {
	key, lease uuid.UUID
	ref        intake.ContentRef
	digest     string
}

// Publish sends at most one physical POST. An unacknowledged claim is retried
// with the same durable file, message ID and digest after its next due time.
func (p *Publisher) Publish(ctx context.Context, workspace, message uuid.UUID, epoch int64) error {
	if workspace == uuid.Nil || message == uuid.Nil || epoch < 1 {
		return intake.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var c claimed
	var ex *exchange
	start, stop := context.WithTimeout(ctx, 2*time.Second)
	err := p.gate.WithOpen(start, workspace, epoch, func() error {
		var err error
		c, err = p.db.claimObservation(start, workspace, message)
		if err != nil {
			return err
		}
		body, err := p.files.Read(c.ref, time.Now())
		if err != nil {
			return intake.ErrUnavailable
		}
		var o intake.TaskObservation
		if json.Unmarshal(body, &o) != nil || o.Workspace != workspace || o.MessageID != message || o.MessageDigest != c.digest {
			return intake.ErrConflict
		}
		wire, err := intake.EncodeObservation(&o)
		if err != nil || !bytes.Equal(wire, body) {
			return intake.ErrConflict
		}
		dst := p.recipient.origin
		dst.Path = applyPath
		req, err := http.NewRequest(http.MethodPost, dst.String(), bytes.NewReader(body))
		if err != nil {
			return intake.ErrUnavailable
		}
		req.Close = true
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", uuid.NewString())
		ex, err = p.recipient.start(start, req, func() error { return p.db.publicationAllowed(start, o, c, epoch) })
		return err
	})
	stop()
	if ex != nil {
		defer ex.conn.Close()
	}
	if c.lease == uuid.Nil {
		if err == nil {
			return intake.ErrDenied
		}
		return err
	}
	ack := false
	if err == nil && ex != nil {
		status, body, readErr := ex.readBounded(ctx)
		ack = readErr == nil && status == http.StatusAccepted && acceptedReceipt(body, message)
		if !ack {
			err = intake.ErrUnavailable
		}
	}
	finish, stopFinish := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stopFinish()
	if e := p.db.finishPublication(finish, workspace, c, ack); e != nil {
		return e
	}
	return err
}

func acceptedReceipt(body []byte, message uuid.UUID) bool {
	if _, err := jcs.Transform(body); err != nil {
		return false
	}
	var f map[string]json.RawMessage
	if json.Unmarshal(body, &f) != nil {
		return false
	}
	for k := range f {
		switch k {
		case "message_id", "disposition", "record_id", "revision":
		default:
			return false
		}
	}
	var id uuid.UUID
	var disposition string
	if json.Unmarshal(f["message_id"], &id) != nil || id != message || json.Unmarshal(f["disposition"], &disposition) != nil {
		return false
	}
	if raw, ok := f["record_id"]; ok {
		var record uuid.UUID
		if json.Unmarshal(raw, &record) != nil || record == uuid.Nil {
			return false
		}
	}
	if raw, ok := f["revision"]; ok {
		var rev int64
		if json.Unmarshal(raw, &rev) != nil || rev < 1 || rev > 9007199254740991 {
			return false
		}
	}
	return disposition == "APPLIED" || disposition == "DUPLICATE" || disposition == "HELD"
}

func (p *Postgres) claimObservation(ctx context.Context, ws, id uuid.UUID) (claimed, error) {
	row := claimed{key: id, lease: uuid.New()}
	available := true
	err := p.scoped(ctx, Job{Workspace: ws}, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE hp_delivery.outbox
			SET state='CLAIMED',claim_token=$3,lease_until=clock_timestamp()+interval '15 seconds',delivery_count=delivery_count+1
			WHERE workspace_id=$1 AND message_id=$2 AND kind='TASK_OBSERVATION'
			  AND expires_at>clock_timestamp() AND next_attempt_at<=clock_timestamp() AND delivery_count<8
			  AND (state='PENDING' OR (state='CLAIMED' AND lease_until<=clock_timestamp()))
			RETURNING payload_ref,payload_sha256,payload_bytes,expires_at,digest`, ws, id, row.lease).
			Scan(&row.ref.ID, &row.ref.SHA256, &row.ref.Size, &row.ref.Expires, &row.digest)
		if errors.Is(err, pgx.ErrNoRows) {
			available = false
			released, err := tx.Exec(ctx, `UPDATE hp_delivery.outbox SET state='HELD',claim_token=NULL,lease_until=NULL
				WHERE workspace_id=$1 AND message_id=$2 AND kind='TASK_OBSERVATION'
				  AND (expires_at<=clock_timestamp() OR delivery_count>=8)
				  AND (state='PENDING' OR (state='CLAIMED' AND lease_until<=clock_timestamp()))`, ws, id)
			if err != nil {
				return intake.ErrUnavailable
			}
			if released.RowsAffected() == 1 {
				_, err = tx.Exec(ctx, `DELETE FROM hp_delivery.job_index WHERE workspace_id=$1 AND job_id=$2`, ws, id)
				if err != nil {
					return intake.ErrUnavailable
				}
			}
			return nil
		}
		if err != nil {
			return intake.ErrUnavailable
		}
		_, err = tx.Exec(ctx, `UPDATE hp_delivery.job_index j SET due_at=o.lease_until,claim_token=o.claim_token,lease_until=o.lease_until
			FROM hp_delivery.outbox o WHERE j.workspace_id=$1 AND j.job_id=$2
			  AND o.workspace_id=j.workspace_id AND o.message_id=j.job_id`, ws, id)
		if err != nil {
			return intake.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return claimed{}, err
	}
	if !available {
		return claimed{}, intake.ErrNotFound
	}
	return row, nil
}

func (p *Postgres) publicationAllowed(ctx context.Context, o intake.TaskObservation, c claimed, epoch int64) error {
	return p.scoped(ctx, Job{Workspace: o.Workspace}, func(tx pgx.Tx) error {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM hp_delivery.effect_observations v
			JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id)
			JOIN hp_delivery.destination_connections d USING(workspace_id,connection_id)
			JOIN hp_delivery.outbox b ON b.workspace_id=v.workspace_id AND b.message_id=$2
			WHERE v.workspace_id=$1 AND v.observation_id=$3 AND v.lineage_id=$4 AND l.case_id=$5
			  AND d.security_epoch=$6 AND d.revoked_at IS NULL
			  AND b.state='CLAIMED' AND b.claim_token=$7 AND b.lease_until>clock_timestamp()
			  AND b.expires_at>clock_timestamp() FOR SHARE OF d`,
			o.Workspace, c.key, o.Observation, o.Lineage, o.Case, epoch, c.lease).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return intake.ErrDenied
		}
		if err != nil {
			return intake.ErrUnavailable
		}
		return nil
	})
}

func (p *Postgres) finishPublication(ctx context.Context, ws uuid.UUID, c claimed, ack bool) error {
	return p.scoped(ctx, Job{Workspace: ws}, func(tx pgx.Tx) error {
		var state string
		var next time.Time
		err := tx.QueryRow(ctx, `UPDATE hp_delivery.outbox SET
			state=CASE WHEN $4 THEN 'ACKED' WHEN delivery_count>=8 OR expires_at<=clock_timestamp()
			  THEN 'HELD' ELSE 'PENDING' END,
			next_attempt_at=clock_timestamp()+make_interval(secs=>LEAST(300,power(2,delivery_count))),
			claim_token=NULL,lease_until=NULL
			WHERE workspace_id=$1 AND message_id=$2 AND claim_token=$3 AND state='CLAIMED'
			RETURNING state,next_attempt_at`, ws, c.key, c.lease, ack).Scan(&state, &next)
		if err != nil {
			return intake.ErrUnavailable
		}
		if state == "PENDING" {
			_, err = tx.Exec(ctx, `UPDATE hp_delivery.job_index SET due_at=$3,claim_token=NULL,lease_until=NULL WHERE workspace_id=$1 AND job_id=$2`, ws, c.key, next)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM hp_delivery.job_index WHERE workspace_id=$1 AND job_id=$2`, ws, c.key)
		}
		if err != nil {
			return intake.ErrUnavailable
		}
		return nil
	})
}
