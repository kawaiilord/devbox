package app

import (
	"context"
	"github.com/jackc/pgx/v5/pgconn"
)

// Only the catalog service may use this internal CAS. An empty actor is reserved
// for scheduler/health writes; administrative writes retain permission and audit.
type CatalogRepository interface {
	SaveCatalogDocument(context.Context, AdminDocument, string, string) (AdminDocument, error)
}

func (r *MemoryRepository) SaveCatalogDocument(_ context.Context, d AdminDocument, actor, action string) (AdminDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.Key != "media-catalog" || (actor != "" && !permissionGranted(r.usersByID[actor].User, "source.manage")) {
		return d, ErrForbidden
	}
	if r.adminDocuments[d.Key].Version != d.Version {
		return d, ErrAdminConflict
	}
	d.Version++
	d.Data = append([]byte(nil), d.Data...)
	if r.adminDocuments == nil {
		r.adminDocuments = map[string]AdminDocument{}
	}
	r.adminDocuments[d.Key] = d
	if actor != "" {
		r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "catalog." + action, TargetType: "settings", TargetID: d.Key})
	}
	return d, nil
}

func (r *PostgresRepository) SaveCatalogDocument(ctx context.Context, d AdminDocument, actor, action string) (AdminDocument, error) {
	if d.Key != "media-catalog" {
		return d, ErrForbidden
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return d, e
	}
	defer tx.Rollback(ctx)
	if actor != "" {
		if _, e = postgresAdminActor(ctx, tx, actor, "source.manage"); e != nil {
			return d, e
		}
	}
	var result pgconn.CommandTag
	if d.Version == 0 {
		result, e = tx.Exec(ctx, `INSERT INTO admin_documents(key,version,data,updated_by) VALUES($1,1,$2,NULLIF($3,'')) ON CONFLICT DO NOTHING`, d.Key, []byte(d.Data), actor)
	} else {
		result, e = tx.Exec(ctx, `UPDATE admin_documents SET data=$3,version=version+1,updated_by=COALESCE(NULLIF($4,''),updated_by),updated_at=now() WHERE key=$1 AND version=$2`, d.Key, d.Version, []byte(d.Data), actor)
	}
	if e != nil {
		return d, e
	}
	if result.RowsAffected() != 1 {
		return d, ErrAdminConflict
	}
	if actor != "" {
		if _, e = appendPostgresAudit(ctx, tx, AuditEvent{ActorID: actor, Action: "catalog." + action, TargetType: "settings", TargetID: d.Key}); e != nil {
			return d, e
		}
	}
	d.Version++
	return d, tx.Commit(ctx)
}
