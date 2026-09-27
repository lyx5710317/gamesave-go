// Never expose raw filesystem/archive errors in safety-critical restores.
export function fileRestoreFailureKey(error) {
  const codes = {
    file_restore_location: 'location', file_restore_target: 'target',
    file_restore_archive: 'archive', file_restore_safety: 'safety',
    file_restore_changed: 'changed', file_restore_publish: 'publish'
  };
  return Object.hasOwn(codes, error?.code) ? `game.fileRestoreFailure.${codes[error.code]}` : null;
}

// Allowlist whole-restore preflight failures just like single-file failures.
export function restorePreflightFailureKey(error) {
  const codes = { restore_archive: 'archive', restore_location: 'location', restore_safety: 'safety' };
  return Object.hasOwn(codes, error?.code) ? `game.restoreFailure.${codes[error.code]}` : null;
}

// Translate only known generated safety comments; preserve stored metadata and
// user-written comments, including those that resemble a system comment.
export function snapshotComment(snapshot, t) {
  const comment = snapshot?.comment ?? '';
  if (!snapshot?.isSystemAuto) return comment;
  const whole = /^Pre-rollback safety restore point \(before restoring ([A-Za-z0-9_-]+)\)$/.exec(comment);
  if (whole) return t('game.safetyRestoreComment', { id: whole[1] });
  const single = /^Safety snapshot before single-file restore from ([A-Za-z0-9_-]+)$/.exec(comment);
  if (single) return t('game.safetyFileRestoreComment', { id: single[1] });
  return comment;
}
