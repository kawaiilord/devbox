# Project change records

The owner requires every delivered change to be recorded and reported, including
additions, modifications and removals, so a release can be reviewed or rolled back.

- Maintain a dated record in `docs/changes/` for each task/release. Include the
  concrete behavior, business rules, validation results, known limits, commit/CI
  identifiers, deployed asset hashes, baseline version and rollback instructions.
- Before deploying, preserve the previous web files and server binary/image. If
  persistence changes, take a database backup and explain compatibility. Record
  backup locations in the deployment manifest; never commit credentials or tokens.
- Prefer additive database changes so code can be rolled back without losing new
  user activity. Do not restore an old database merely to undo a UI/server release.
- Report additions, changes, removals and the rollback reference in the final
  response or an attached release report. Distinguish verified work from pending work.
- Continue within the owner's existing deployment authorization. These records
  do not introduce an extra approval requirement.
