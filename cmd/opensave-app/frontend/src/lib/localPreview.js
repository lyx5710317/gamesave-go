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
  if (!Array.isArray(groups)) return null;
  const gameIds = new Set();
  const snapshotNames = new Set();
  for (const group of groups) {
    if (!group || typeof group.gameId !== 'string' || !group.gameId.trim() ||
        !Array.isArray(group.snapshots) || gameIds.has(group.gameId)) return null;
    gameIds.add(group.gameId);
    for (const snapshot of group.snapshots) {
      if (!snapshot || typeof snapshot.name !== 'string' || !snapshot.name.trim() ||
          snapshotNames.has(snapshot.name)) return null;
      snapshotNames.add(snapshot.name);
    }
  }

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

// Matching game IDs are a review hint only. Snapshot ZIP inventory has no
// trusted content hash or ancestry, and different IDs can still be the same
// game on two independently configured devices.
export function summarizeJoinOverlap(scan, remoteInventory) {
  if (scan?.complete !== true || !Array.isArray(scan?.library?.games) ||
      !Array.isArray(remoteInventory?.games)) return null;
  const localIds = new Set();
  const localGames = scan.library.games;
  for (const game of localGames) {
    if (!game || typeof game.gameId !== 'string' || !game.gameId.trim() ||
        localIds.has(game.gameId)) return null;
    localIds.add(game.gameId);
  }
  const remoteIds = new Set(remoteInventory.games.map((game) => game.gameId));
  return {
    overlapping: localGames.filter((game) => remoteIds.has(game.gameId)).map((game) => ({
      gameId: game.gameId,
      name: game.name || game.gameId
    }))
  };
}

// A readable vault document is only a discovery result. It is not proof of
// strong conditional writes or permission to join another device.
export function summarizeRemoteVault(result) {
  const status = result?.status;
  if (status === 'valid') {
    if (!Number.isSafeInteger(result.revision) || result.revision < 1 ||
        !Number.isSafeInteger(result.deviceCount) || result.deviceCount < 1) {
      return { status: 'unavailable' };
    }
    return { status, revision: result.revision, deviceCount: result.deviceCount };
  }
  if (['missing', 'unsupported', 'invalid', 'upgrade-required'].includes(status)) return { status };
  return { status: 'unavailable' };
}
