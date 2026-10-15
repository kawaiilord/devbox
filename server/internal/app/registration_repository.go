package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *MemoryRepository) registerOriginLocked(a *AccountRecord, policy SitePolicy) error {
	if err := validateRegistrationRecord(a.Registration); err != nil {
		return err
	}
	if a.Registration == nil {
		return nil
	}
	if r.devices[a.Registration.DeviceHash].banned {
		return ErrDeviceBlocked
	}
	now := time.Now().UnixMilli()
	buckets := registrationBuckets(a.Registration, policy)
	for _, bucket := range buckets {
		if err := quotaExceeded(bucket, quotaNext(r.registrationQuotas[bucket.Key], now), now); err != nil {
			return err
		}
	}
	if r.registrationQuotas == nil {
		r.registrationQuotas = map[string]registrationQuota{}
	}
	for key, item := range r.registrationQuotas {
		if item.Start+2*registrationWindow.Milliseconds() < now {
			delete(r.registrationQuotas, key)
		}
	}
	for _, bucket := range buckets {
		r.registrationQuotas[bucket.Key] = quotaNext(r.registrationQuotas[bucket.Key], now)
	}
	copy := *a.Registration
	copy.RegisteredAt = now
	a.Registration = &copy
	return nil
}

func (r *MemoryRepository) ReadRegistration(_ context.Context, id string) (RegistrationDetails, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	detail := RegistrationDetails{Related: []RegistrationMatch{}}
	a, exists := r.usersByID[id]
	if !exists {
		return detail, ErrNotFound
	}
	if a.Registration == nil {
		return detail, nil
	}
	copy := *a.Registration
	detail.Record = &copy
	for _, other := range r.usersByID {
		if other.ID == id || other.Registration == nil {
			continue
		}
		if reasons := registrationReasons(copy, *other.Registration); len(reasons) > 0 {
			detail.Related = append(detail.Related, RegistrationMatch{UserID: other.ID, DisplayName: other.DisplayName, Reasons: reasons})
		}
	}
	sort.Slice(detail.Related, func(i, j int) bool { return detail.Related[i].UserID < detail.Related[j].UserID })
	detail.RelatedTotal = len(detail.Related)
	detail.Related = detail.Related[:min(25, len(detail.Related))]
	return detail, nil
}

func insertRegistrationTx(ctx context.Context, tx pgx.Tx, a AccountRecord, policy SitePolicy) error {
	if err := validateRegistrationRecord(a.Registration); err != nil {
		return err
	}
	if a.Registration == nil {
		return nil
	}
	var banned bool
	err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM devices WHERE device_hash=$1 FOR SHARE`, a.Registration.DeviceHash).Scan(&banned)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if banned {
		return ErrDeviceBlocked
	}
	now := time.Now().UnixMilli()
	for _, bucket := range registrationBuckets(a.Registration, policy) {
		var quota registrationQuota
		err := tx.QueryRow(ctx, `INSERT INTO registration_quotas(key_hash,window_start,registrations) VALUES($1,$2,1)
		ON CONFLICT(key_hash) DO UPDATE SET
		window_start=CASE WHEN registration_quotas.window_start<=$3 THEN $2 ELSE registration_quotas.window_start END,
		registrations=CASE WHEN registration_quotas.window_start<=$3 THEN 1 ELSE registration_quotas.registrations+1 END
		RETURNING window_start,registrations`, bucket.Key, now, now-registrationWindow.Milliseconds()).Scan(&quota.Start, &quota.Count)
		if err != nil {
			return err
		}
		if err = quotaExceeded(bucket, quota, now); err != nil {
			return err
		}
	}
	record := *a.Registration
	record.RegisteredAt = now
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO registration_records(user_id,registered_at,source_ip,network_key,device_hash,browser_hash,environment_hash,method,data) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, now, record.SourceIP, record.Network, record.DeviceHash, record.BrowserHash, record.EnvironmentHash, record.Method, body)
	return err
}

func (r *PostgresRepository) ReadRegistration(ctx context.Context, id string) (RegistrationDetails, error) {
	detail := RegistrationDetails{Related: []RegistrationMatch{}}
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT rr.data FROM users u LEFT JOIN registration_records rr ON rr.user_id=u.id WHERE u.id=$1`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrNotFound
	}
	if err != nil || len(raw) == 0 {
		return detail, err
	}
	if err = json.Unmarshal(raw, &detail.Record); err != nil {
		return detail, err
	}
	a := detail.Record
	rows, err := r.pool.Query(ctx, `SELECT rr.user_id,u.display_name,rr.data,count(*) OVER()
		FROM registration_records rr JOIN users u ON u.id=rr.user_id
		WHERE rr.user_id<>$1 AND (rr.source_ip=$2 OR rr.network_key=$3 OR rr.device_hash=$4
		OR ($5<>'' AND rr.browser_hash=$5) OR ($6<>'' AND rr.environment_hash=$6)) ORDER BY rr.user_id LIMIT 25`, id, a.SourceIP, a.Network, a.DeviceHash, a.BrowserHash, a.EnvironmentHash)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RegistrationMatch
		var body []byte
		if err = rows.Scan(&item.UserID, &item.DisplayName, &body, &detail.RelatedTotal); err != nil {
			return detail, err
		}
		var other RegistrationRecord
		if err = json.Unmarshal(body, &other); err != nil {
			return detail, err
		}
		item.Reasons = registrationReasons(*a, other)
		detail.Related = append(detail.Related, item)
	}
	return detail, rows.Err()
}
