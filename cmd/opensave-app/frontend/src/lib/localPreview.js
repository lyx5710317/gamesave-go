// Derived display state for the read-only first-join local scan. Detected
// locations are not treated as remote matches or safe-to-upload games.
export function summarizeLocalPreview(scan) {
  const games = scan?.library?.games ?? [];
  const candidates = scan?.candidates ?? [];
  const unknownCount = candidates.filter((candidate) => !candidate.measured).length;
  return {
    trackedCount: games.length,
    detectedCount: candidates.length,
    unknownCount,
    complete: scan?.complete === true && unknownCount === 0
  };
}
