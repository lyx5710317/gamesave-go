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

// The existing cloud browser exposes snapshot inventory, not a validated
// vault head or ancestry. Keep this display-only summary separate from the
// local/remote join planner; an empty array is known-empty, while malformed
// or missing data must never be presented as an empty remote library.
export function summarizeRemoteInventory(groups) {
  if (!Array.isArray(groups) || groups.some((group) =>
    !group || typeof group.gameId !== 'string' || !Array.isArray(group.snapshots)
  )) return null;

  return {
    gameCount: groups.length,
    snapshotCount: groups.reduce((count, group) => count + group.snapshots.length, 0),
    games: groups.map((group) => ({
      gameId: group.gameId,
      name: group.gameName || group.gameId,
      snapshotCount: group.snapshots.length
    }))
  };
}
