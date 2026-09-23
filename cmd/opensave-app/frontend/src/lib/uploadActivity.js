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
  if ((result?.failed ?? 0) > 0) return 'failed';
  if (result?.uploaded === 0 && (result?.skipped ?? 0) > 0) return 'already-current';
  return 'uploaded';
}
