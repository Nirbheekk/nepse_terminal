# NEPSE Platform Rollback & Disaster Recovery Plan

**Target System**: NEPSE Data Terminal & Platform API  
**Scope**: Database, Application Binaries, Next.js Frontend, Ingestion Workers  
**Recovery Time Objective (RTO)**: < 15 minutes  
**Recovery Point Objective (RPO)**: < 1 hour (or last scheduled session capture)

---

## 1. Rollback Triggers

A rollback must be initiated immediately upon detection of any of the following:
1. **Critical Data Corruption**: Accounting identity violation in balance sheets or portfolio cost-basis calculation discrepancies.
2. **Database Migration Deadlock**: Migration failure during deployment causing unhandled schema state or service initialization blockage (`/readyz` non-200).
3. **Severe Availability Degradation**: HTTP 5xx error rate exceeding 1% over a 5-minute rolling window post-release.
4. **Security Vulnerability / Tenant Boundary Breach**: Cross-tenant data leakage or unauthorized access token issuance.

---

## 2. Emergency Contacts & Incident Command

| Role | Responsibility | Action Channel |
| :--- | :--- | :--- |
| **Release Commander** | Authorizes rollback initiation | Incident Bridge / Slack #ops-emergency |
| **Database Lead** | Executes SQL down-migrations or point-in-time restore | Primary DB Host Terminal |
| **API / Backend Lead** | Reverts application container images | Container Registry / Docker Host |
| **Frontend Lead** | Flushes CDN caches and reverts web static bundle | Next.js Server Host |

---

## 3. Step-by-Step Rollback Procedures

### Scenario A: Application Binary Rollback (No Database Schema Change)
If the database migration succeeded or was unaffected, and the issue lies in the API, worker, or Next.js container:

1. **Stop Current Ingestion Workers**:
   ```bash
   docker compose stop worker
   ```
2. **Revert API & Web Containers to Previous Known Stable Tag**:
   ```bash
   # Set previous image tag
   export PLATFORM_IMAGE_TAG="v0.9.8-stable"
   docker compose up -d --no-deps api web
   ```
3. **Verify Liveness and Readiness Probes**:
   ```bash
   curl -f http://localhost:8081/healthz
   curl -f http://localhost:8081/readyz
   ```
4. **Restart Ingestion Workers**:
   ```bash
   docker compose start worker
   ```

---

### Scenario B: Database Schema Rollback (Down-Migrations)
If a newly applied migration (e.g. `0014_developer_keys_exports`) causes a failure:

1. **Place Stack in Maintenance Mode**:
   Direct the ingress proxy to serve the 503 Maintenance page to prevent incoming write mutations.

2. **Execute Sequential Down-Migrations**:
   Apply the corresponding `.down.sql` files in reverse order:
   ```bash
   # Example: Reversing migration 0014
   docker compose exec -T postgres psql -U nepse -d nepse -f /migrations/0014_developer_keys_exports.down.sql
   ```

3. **Verify Clean Table Drops & Cascades**:
   ```sql
   SELECT table_name FROM information_schema.tables WHERE table_schema = 'public';
   ```

4. **Revert Binary to Compatible Version**:
   Deploy the binary release corresponding to the target migration level.

5. **Release Maintenance Mode**:
   Restore traffic and verify `/readyz` returns HTTP 200 OK.

---

### Scenario C: Complete Database Restoration from Backup Snapshot
In the event of critical data corruption requiring full restoration to the pre-release state:

1. **Halt All Active Writers & Web Services**:
   ```bash
   docker compose stop api worker web
   ```

2. **Locate Latest Verified Cryptographic Backup**:
   ```bash
   BACKUP_FILE="./var/backups/nepse_db_pre_release.sql.gz"
   CHECKSUM_FILE="${BACKUP_FILE}.sha256"
   
   # Verify SHA-256 integrity checksum
   sha256sum -c "$CHECKSUM_FILE"
   ```

3. **Restore Database from Gzip Snapshot**:
   ```bash
   # Drop existing active connections and restore
   gunzip -c "$BACKUP_FILE" | docker compose exec -T postgres psql -U nepse -d nepse
   ```

4. **Run Verification Suite**:
   ```bash
   # Execute internal store backup and consistency tests
   cd platform && go test -v ./internal/store
   ```

5. **Start Services & Confirm Readiness**:
   ```bash
   docker compose up -d
   until curl -s http://localhost:8081/readyz | grep "Ready"; do sleep 1; done
   echo "Platform successfully restored."
   ```

---

## 4. Post-Rollback Verification & Audit Checklist

- [ ] Liveness probe `/healthz` returns `200 OK`.
- [ ] Readiness probe `/readyz` confirms database connection and applied migration version.
- [ ] Market overview `/api/v1/market/overview` serves correct exchange status.
- [ ] Authentication endpoints `/api/v1/auth/me` validate existing user sessions without forced re-logins.
- [ ] Ingestion reconciliation logs confirm zero contract discrepancies.
- [ ] Post-mortem root cause analysis (RCA) scheduled within 24 hours.
