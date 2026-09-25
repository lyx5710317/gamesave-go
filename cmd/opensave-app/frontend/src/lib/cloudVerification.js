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
