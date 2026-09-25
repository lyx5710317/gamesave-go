// Activity is in-memory for the current process. A failed transfer is never
// counted as an already-current/skipped snapshot.
export function summarizeUploadActivity(records) {
  const uploads = Array.isArray(records) ? records : [];
  return {
    running: uploads.filter((record) => record.status === 'running').length,
    failed: uploads.filter((record) => record.status === 'failed').length,
    succeeded: uploads.filter((record) => record.status === 'succeeded').length
  };
}

export function manualUploadOutcome(result) {
  if ((result?.conflicts ?? 0) > 0) return 'conflict';
  if ((result?.failed ?? 0) > 0) return 'failed';
  if (result?.uploaded === 0 && (result?.skipped ?? 0) > 0) return 'already-current';
  return 'uploaded';
}

export function uploadStatusText(record, translate) {
  if (record.status === 'running') return translate('cloud.activity.running');
  if (record.status === 'succeeded') return translate('cloud.activity.succeeded');
  if (record.failure === 'configuration') return translate('cloud.activity.configuration');
  if (record.failure === 'conflict') return translate('cloud.activity.conflict');
  if (record.provider === 'jianguoyun' && record.failure === 'unsafe_condition') {
    const base = translate('cloud.activity.jianguoyun.unsafe_condition');
    const checks = ['probe_cleanup', 'move_first', 'move_overwrite', 'move_contents', 'stage_name'];
    if (!checks.includes(record.safetyCheck)) return base;
    const check = translate(`cloud.activity.jianguoyun.check.${record.safetyCheck}`);
    const status = Number.isInteger(record.httpStatus) && record.httpStatus >= 100 && record.httpStatus <= 599
      ? ` (HTTP ${record.httpStatus})` : '';
    return `${base} · ${check}${status}`;
  }
  if (record.provider === 'jianguoyun' && ['authentication', 'permission', 'quota', 'rate_limit', 'network', 'incomplete_inventory', 'integrity'].includes(record.failure)) {
    return translate(`cloud.activity.jianguoyun.${record.failure}`);
  }
  return translate('cloud.activity.failed');
}
