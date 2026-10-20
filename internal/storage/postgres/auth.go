package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/legal"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

func conflict(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return repository.ErrConflict
	}
	return err
}
func (d *Database) CreateEmailUser(ctx context.Context, u user.User, hash string, accepted ...legal.Acceptance) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,email,display_name) VALUES($1,$2,$3)", u.ID, u.Email, u.Name); err != nil {
		return conflict(err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO auth_identities(id,user_id,provider,provider_subject,email,password_hash) VALUES($1,$2,'email',$3,$3,$4)", uuid.NewString(), u.ID, u.Email, hash); err != nil {
		return conflict(err)
	}
	if err = recordLegalAcceptances(ctx, tx, u.ID, accepted); err != nil {
		return err
	}
	return tx.Commit()
}

type legalExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func recordLegalAcceptances(ctx context.Context, db legalExecutor, uid string, accepted []legal.Acceptance) error {
	for _, a := range accepted {
		if a.Version == "" || a.AcceptedAt.IsZero() {
			return errors.New("invalid legal acceptance")
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO user_legal_acceptances(user_id,document_version,accepted_at,action) VALUES($1,$2,$3,$4)", uid, a.Version, a.AcceptedAt, a.Action); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) RecordLegalAcceptance(ctx context.Context, uid string, accepted legal.Acceptance) error {
	return recordLegalAcceptances(ctx, d.DB, uid, []legal.Acceptance{accepted})
}

const userColumns = "u.id,coalesce(u.email,''),u.display_name,u.avatar_url,u.onboarding_completed,u.created_at"

func scanUser(row scanner) (*user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.OnboardingCompleted, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}
func (d *Database) providers(ctx context.Context, u *user.User) error {
	if u == nil {
		return nil
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT provider FROM auth_identities WHERE user_id=$1 ORDER BY provider", u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	u.Providers = []string{}
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			return err
		}
		u.Providers = append(u.Providers, p)
	}
	return rows.Err()
}
func (d *Database) EmailIdentity(ctx context.Context, email string) (*user.User, string, error) {
	var u user.User
	var hash string
	err := d.DB.QueryRowContext(ctx, "SELECT "+userColumns+",i.password_hash FROM auth_identities i JOIN users u ON u.id=i.user_id WHERE i.provider='email' AND i.provider_subject=$1", email).Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.OnboardingCompleted, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	err = d.providers(ctx, &u)
	return &u, hash, err
}
func (d *Database) OAuthUser(ctx context.Context, i repository.Identity, link string, accepted ...legal.Acceptance) (*user.User, error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,1))", i.Provider+":"+i.Subject); err != nil {
		return nil, err
	}
	u, err := scanUser(tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM auth_identities i JOIN users u ON u.id=i.user_id WHERE i.provider=$1 AND i.provider_subject=$2", i.Provider, i.Subject))
	if err != nil {
		return nil, err
	}
	if u != nil {
		if link != "" && u.ID != link {
			return nil, repository.ErrConflict
		}
	} else {
		id := link
		if id == "" {
			id = uuid.NewString()
			// A matching contact email never authenticates an existing account or
			// creates a duplicate. Its owner must sign in and explicitly link.
			if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,email,display_name,avatar_url) VALUES($1,nullif($2,''),$3,$4)", id, i.Email, i.Name, i.AvatarURL); err != nil {
				return nil, conflict(err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO auth_identities(id,user_id,provider,provider_subject,email) VALUES($1,$2,$3,$4,nullif($5,''))", uuid.NewString(), id, i.Provider, i.Subject, i.Email); err != nil {
			return nil, conflict(err)
		}
		u, err = scanUser(tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users u WHERE u.id=$1", id))
		if err != nil {
			return nil, err
		}
	}
	if i.AvatarURL != "" && u != nil && u.AvatarURL == "" {
		u, err = scanUser(tx.QueryRowContext(ctx, "UPDATE users u SET avatar_url=CASE WHEN avatar_url='' THEN $2 ELSE avatar_url END,updated_at=now() WHERE id=$1 RETURNING "+userColumns, u.ID, i.AvatarURL))
		if err != nil {
			return nil, err
		}
	}
	if err = recordLegalAcceptances(ctx, tx, u.ID, accepted); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	err = d.providers(ctx, u)
	return u, err
}
func (d *Database) CreateSession(ctx context.Context, id, uid string, now, expires time.Time) error {
	_, err := d.DB.ExecContext(ctx, "INSERT INTO sessions(id,user_id,created_at,expires_at,last_seen_at) VALUES($1,$2,$3,$4,$3)", id, uid, now, expires)
	return err
}
func (d *Database) SessionUser(ctx context.Context, id string, now time.Time) (*user.User, error) {
	u, err := scanUser(d.DB.QueryRowContext(ctx, "SELECT "+userColumns+" FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.expires_at>$2", id, now))
	if err != nil || u == nil {
		return u, err
	}
	if _, err = d.DB.ExecContext(ctx, "UPDATE sessions SET last_seen_at=$2::timestamptz WHERE id=$1 AND last_seen_at<$2::timestamptz-interval '5 minutes'", id, now); err != nil {
		return nil, err
	}
	err = d.providers(ctx, u)
	return u, err
}
func (d *Database) RevokeSession(ctx context.Context, id string) error {
	_, err := d.DB.ExecContext(ctx, "DELETE FROM sessions WHERE id=$1", id)
	return err
}
func (d *Database) SaveOAuthState(ctx context.Context, s repository.OAuthState) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM oauth_states WHERE expires_at<now()"); err != nil {
		return err
	}
	accepted, err := json.Marshal(s.LegalAcceptance)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO oauth_states(id,provider,browser_hash,verifier,link_user_id,expires_at,purpose,source,legal_acceptance) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9)", s.ID, s.Provider, s.BrowserHash, s.Verifier, s.LinkUserID, s.ExpiresAt, s.Purpose, s.Source, accepted); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) ConsumeOAuthState(ctx context.Context, id, provider, browser string, now time.Time) (*repository.OAuthState, error) {
	var s repository.OAuthState
	var accepted []byte
	err := d.DB.QueryRowContext(ctx, "DELETE FROM oauth_states WHERE id=$1 AND provider=$2 AND browser_hash=$3 AND expires_at>$4 RETURNING id,provider,browser_hash,verifier,coalesce(link_user_id::text,''),expires_at,purpose,source,legal_acceptance", id, provider, browser, now).Scan(&s.ID, &s.Provider, &s.BrowserHash, &s.Verifier, &s.LinkUserID, &s.ExpiresAt, &s.Purpose, &s.Source, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err == nil && len(accepted) > 0 {
		err = json.Unmarshal(accepted, &s.LegalAcceptance)
	}
	return &s, err
}
func (d *Database) UpdateUser(ctx context.Context, id, name string, avatar *string, complete bool) (*user.User, error) {
	u, err := scanUser(d.DB.QueryRowContext(ctx, "UPDATE users u SET display_name=CASE WHEN $2='' THEN display_name ELSE $2 END,avatar_url=coalesce($4::text,avatar_url),onboarding_completed=onboarding_completed OR $3,updated_at=now() WHERE id=$1 RETURNING "+userColumns, id, name, complete, avatar))
	if err != nil {
		return nil, err
	}
	err = d.providers(ctx, u)
	return u, err
}
