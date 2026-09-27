// Verification only proves the downloaded ZIP is readable. A same-byte local
// archive adds a content comparison, not cloud-vault identity or ancestry.
export function cloudVerificationFeedback(result) {
  if (!Number.isSafeInteger(result?.sizeBytes) || result.sizeBytes <= 0) {
    throw new Error('invalid cloud verification result');
  }
  switch (result.localComparison) {
    case 'identical': return { key: 'cloud.verify.identical', tone: 'success' };
    case 'different': return { key: 'cloud.verify.different', tone: 'info' };
    case 'unavailable': return { key: 'cloud.verify.unavailable', tone: 'info' };
    default: throw new Error('invalid cloud verification result');
  }
}

// Map only a fixed backend diagnostic. Never display untrusted ZIP entry
// names or turn a generic download failure into a successful verification.
export function cloudArchiveFailureKey(error) {
  return error?.message === 'cloud snapshot archive contains unsafe or conflicting paths; no saves were changed'
    ? 'cloud.restore.unsafeArchive' : null;
}

export function cloudVerificationFailureKey(error) {
  const categories = {
    cloud_verify_disabled: 'disabled',
    cloud_verify_configuration: 'configuration',
    cloud_verify_authentication: 'authentication',
    cloud_verify_permission: 'permission',
    cloud_verify_quota: 'quota',
    cloud_verify_rate_limit: 'rateLimit',
    cloud_verify_network: 'network',
    cloud_verify_missing: 'missing',
    cloud_verify_incomplete_inventory: 'incompleteInventory',
    cloud_verify_ambiguous: 'ambiguous',
    cloud_verify_unsafe_archive: 'unsafeArchive',
    cloud_verify_size_mismatch: 'sizeMismatch',
    cloud_verify_integrity: 'integrity',
    cloud_verify_local_io: 'localIO'
  };
  const category = Object.hasOwn(categories, error?.code) ? categories[error.code] : null;
  return category ? `cloud.verify.failure.${category}` : 'cloud.verify.failed';
}

// Read failures stay visible in the browser, not just in a disappearing toast.
// Only fixed API codes may select copy; never display a raw provider response.
export function cloudReadFailureKey(error) {
  const categories = {
    cloud_read_disabled: 'disabled',
    cloud_read_configuration: 'configuration',
    cloud_read_authentication: 'authentication',
    cloud_read_permission: 'permission',
    cloud_read_quota: 'quota',
    cloud_read_rate_limit: 'rateLimit',
    cloud_read_network: 'network',
    cloud_read_missing: 'missing',
    cloud_read_incomplete_inventory: 'incompleteInventory',
    cloud_read_ambiguous: 'ambiguous',
    cloud_read_local_io: 'localIO'
  };
  const category = Object.hasOwn(categories, error?.code) ? categories[error.code] : 'failed';
  return `cloud.read.failure.${category}`;
}
