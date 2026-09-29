<script>
  import { conflicts, conflictResolution, games, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { demandAttention } from '../lib/notify.js';
  import { t, locale } from '../lib/i18n.js';

  let busy = false;
  let showDiff = false;
  let seen = new Set(); // gameIds already announced
  let applying = new Set(); // gameIds whose resolution runs in the background

  // Conflicts being applied stay hidden: the resolution (possibly a long
  // relay pull) runs in the background and only clears the conflict once
  // it finishes — but the user already made their choice, so don't keep
  // the modal (or its disabled buttons) on screen.
  $: entries = Object.entries($conflicts).filter(([gid]) => !applying.has(gid));
  $: current = entries[0]; // one at a time

  // When a background resolution reports back: on failure the conflict is
  // still active, so un-hide it (the modal reappears with the error toast
  // explaining why). On success the conflict entry is already gone.
  $: onResolutionDone($conflictResolution);
  function onResolutionDone(ev) {
    if (ev && applying.has(ev.gameId)) {
      applying.delete(ev.gameId);
      applying = new Set(applying);
    }
  }
  $: gameName = current ? ($games[current[0]]?.name ?? current[0]) : '';
  $: conflict = current ? current[1] : null;
  $: peerName = conflict ? (conflict.peer.Name ?? conflict.peer.name ?? $t('conflict.otherDevice')) : '';

  // Announce new conflicts: chime + surface the window + toast, same
  // treatment as incoming pairing requests.
  $: onConflicts(entries);
  function onConflicts(list) {
    const fresh = list.filter(([gid]) => !seen.has(gid));
    if (fresh.length > 0) {
      demandAttention();
      const name = $games[fresh[0][0]]?.name ?? $t('conflict.aGame');
      toast($t('conflict.announce', { name }), 'error');
    }
    seen = new Set(list.map(([gid]) => gid));
    // Housekeeping: forget "applying" markers for conflicts that no longer
    // exist (the background resolution finished and cleared them).
    let pruned = false;
    for (const gid of applying) {
      if (!(gid in $conflicts)) {
        applying.delete(gid);
        pruned = true;
      }
    }
    if (pruned) applying = new Set(applying);
  }

  // Which side is further along? Compare last-modified times.
  $: localMs = conflict?.localStats?.latestMtimeMs ?? 0;
  $: remoteMs = conflict?.remoteStats?.latestMtimeMs ?? 0;
  $: newerSide = localMs && remoteMs ? (localMs > remoteMs ? 'local' : remoteMs > localMs ? 'remote' : '') : '';

  // What actually differs, counted per side. The two boxes used to show only
  // each side's whole-save totals, which on a save where one file changed are
  // the same number twice and answer nothing — the question being asked is
  // "what is different", and that was only in the collapsed list below.
  //
  // diffFiles is capped by the server (diffTotal is the true count), so the
  // breakdown is explicitly "of the first N" once it is truncated.
  $: diffFiles = conflict?.diffFiles ?? [];
  $: diffTotal = conflict?.diffTotal ?? 0;
  $: diffCapped = diffTotal > diffFiles.length;
  $: changedCount = diffFiles.filter((d) => d.status === 'changed').length;
  $: onlyLocalCount = diffFiles.filter((d) => d.status === 'only-local').length;
  $: onlyRemoteCount = diffFiles.filter((d) => d.status === 'only-remote').length;

  // Bytes that differ, per side: a changed file counts its own size on each
  // side, a file present on only one side counts only there.
  const sizeOr0 = (n) => (typeof n === 'number' && n > 0 ? n : 0);
  $: localDiffBytes = diffFiles.reduce(
    (n, d) => n + (d.status === 'only-remote' ? 0 : sizeOr0(d.localSize)),
    0
  );
  $: remoteDiffBytes = diffFiles.reduce(
    (n, d) => n + (d.status === 'only-local' ? 0 : sizeOr0(d.remoteSize)),
    0
  );

  // A side with nothing in it is a real state — the save folder was emptied,
  // or this device has never held this game — and "0 files · 0 B" reads like
  // the panel failed to load rather than like information.
  //
  // The "whole save" prefix is load-bearing: this sits directly under the
  // count of what differs, and an unlabelled "3 files" below "2 files differ
  // here" reads as the panel contradicting itself rather than as two
  // different measurements.
  function sideSummary(stats, translate) {
    const files = stats?.files;
    if (files === 0) return translate('conflict.emptyFolder');
    if (typeof files !== 'number') return '';
    return `${translate('conflict.wholeSave', { count: files })} · ${fmtSize(stats?.totalBytes ?? -1)}`;
  }

  async function resolve(resolution) {
    if (!current || busy) return;
    busy = true;
    const [gameId, c] = current;
    const name = gameName;
    try {
      // Returns immediately; the actual apply (possibly a long transfer)
      // runs in the background and reports via the conflict-resolved WS
      // event, which stores.js turns into the outcome toast.
      await api.post(`/api/games/${gameId}/resolve-conflict`, {
        peerId: c.peer.ID ?? c.peer.id,
        resolution
      });
      applying = new Set(applying).add(gameId);
      if (resolution !== 'keep-local') {
        toast($t('conflict.applying', { name, peerName }), 'info');
      }
      showDiff = false;
    } catch (e) {
      toast($t('conflict.operationFailed'), 'error');
    } finally {
      busy = false;
    }
  }

  const fmtMs = (ms, language, translate) => (ms ? new Date(ms).toLocaleString(language) : translate('conflict.unknown'));
  const fmtSize = (n) =>
    n < 0 ? '—' : n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : n >= 1024 ? (n / 1024).toFixed(1) + ' KB' : n + ' B';
  const diffIcon = (s) => (s === 'changed' ? '✱' : s === 'only-remote' ? '+' : '−');
  const diffLabel = (s, translate) =>
    s === 'changed' ? translate('conflict.differs') : s === 'only-remote' ? translate('conflict.onlyOnPeer', { peerName }) : translate('conflict.onlyHere');
</script>

{#if current && conflict}
  <div class="overlay">
    <div class="modal card">
      <h3>⚔️ {$t('conflict.title', { gameName })}</h3>
      <p class="desc">
        {$t('conflict.description', { peerName })}
      </p>

      <div class="versions">
        <div class="version" class:newer={newerSide === 'local'}>
          <div class="v-head">
            <span class="v-title">💻 {$t('conflict.thisDevice')}</span>
            {#if newerSide === 'local'}<span class="v-badge">{$t('conflict.moreRecent')}</span>{/if}
          </div>
          <div class="v-diff">
            <strong>{changedCount + onlyLocalCount}</strong>
            {$t('conflict.differHere')}
            {#if localDiffBytes > 0}<span class="v-diff-bytes">· {fmtSize(localDiffBytes)}</span>{/if}
          </div>
          {#if onlyLocalCount > 0}
            <div class="v-only">{$t('conflict.onlyLocalCount', { count: onlyLocalCount })}</div>
          {/if}
          <div class="v-stats">{sideSummary(conflict.localStats, $t)}</div>
          <div class="v-time">{$t('conflict.lastChange', { time: fmtMs(localMs, $locale, $t) })}</div>
        </div>
        <div class="version" class:newer={newerSide === 'remote'}>
          <div class="v-head">
            <span class="v-title">🖥️ {peerName}</span>
            {#if newerSide === 'remote'}<span class="v-badge">{$t('conflict.moreRecent')}</span>{/if}
          </div>
          <div class="v-diff">
            <strong>{changedCount + onlyRemoteCount}</strong>
            {$t('conflict.differThere')}
            {#if remoteDiffBytes > 0}<span class="v-diff-bytes">· {fmtSize(remoteDiffBytes)}</span>{/if}
          </div>
          {#if onlyRemoteCount > 0}
            <div class="v-only">{$t('conflict.onlyPeerCount', { count: onlyRemoteCount, peerName })}</div>
          {/if}
          <div class="v-stats">{sideSummary(conflict.remoteStats, $t)}</div>
          <div class="v-time">{$t('conflict.lastChange', { time: fmtMs(remoteMs, $locale, $t) })}</div>
        </div>
      </div>
      {#if diffCapped}
        <p class="diff-capped">
          {$t('conflict.capped', { shown: diffFiles.length, total: diffTotal })}
        </p>
      {/if}

      {#if conflict.diffTotal > 0}
        <button class="diff-toggle" on:click={() => (showDiff = !showDiff)}>
          {showDiff ? '▾' : '▸'} {$t('conflict.showDiff', { count: conflict.diffTotal })}
        </button>
        {#if showDiff}
          <div class="diff-list">
            {#each conflict.diffFiles as d (d.path)}
              <div class="diff-row">
                <span class="diff-icon" data-status={d.status}>{diffIcon(d.status)}</span>
                <span class="diff-path" title={d.path}>{d.path}</span>
                <span class="diff-meta">{diffLabel(d.status, $t)}</span>
                <!-- Folders carry no sizes; "— → —" reads as a bug. -->
                {#if d.localSize >= 0 || d.remoteSize >= 0}
                  <span class="diff-sizes">{fmtSize(d.localSize)} → {fmtSize(d.remoteSize)}</span>
                {/if}
              </div>
            {/each}
            {#if conflict.diffTotal > conflict.diffFiles.length}
              <div class="diff-more">{$t('conflict.moreFiles', { count: conflict.diffTotal - conflict.diffFiles.length })}</div>
            {/if}
          </div>
        {/if}
      {/if}

      <div class="actions">
        <button class="btn" disabled={busy} on:click={() => resolve('keep-local')}>{$t('conflict.keepMine')}</button>
        <button class="btn" disabled={busy} on:click={() => resolve('keep-remote')}>{$t('conflict.keepTheirs')}</button>
        <button class="btn primary" disabled={busy} on:click={() => resolve('merge-branch')}>
          {$t('conflict.keepBoth')}
        </button>
      </div>
      <p class="hint-line">
        🛡️ {$t('conflict.safetyHint', { peerName })}
      </p>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 90;
  }
  .modal {
    width: 580px;
    max-width: calc(100vw - 48px);
    max-height: calc(100vh - 80px);
    overflow-y: auto;
  }
  h3 {
    margin-bottom: 8px;
  }
  .desc {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-bottom: 16px;
  }
  .versions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    margin-bottom: 14px;
  }
  .version {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px;
  }
  .version.newer {
    border-color: rgba(74, 222, 128, 0.45);
  }
  .v-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    margin-bottom: 8px;
  }
  .v-title {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .v-badge {
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--success);
    background: rgba(74, 222, 128, 0.12);
    padding: 2px 7px;
    border-radius: 999px;
    white-space: nowrap;
  }
  /* The headline of each box: what differs on that side. The whole-save
     totals moved below it — useful context, but not the decision. */
  .v-diff {
    font-size: 0.84rem;
    color: var(--text);
    margin-bottom: 3px;
  }
  /* Weight alone, not size: enlarging the number knocked it out of line with
     the words around it for no extra clarity. Accent rather than the warning
     amber — a count of what differs is information, not a hazard. */
  .v-diff strong {
    font-weight: 700;
    color: var(--accent);
  }
  .v-diff-bytes {
    color: var(--text-dim);
  }
  .v-only {
    font-size: 0.76rem;
    color: var(--text-dim);
    margin-bottom: 3px;
  }
  /* Ruled off from the difference counts above it: this is context about
     the save as a whole, not another thing that differs. */
  .v-stats {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }
  .diff-capped {
    font-size: 0.74rem;
    color: var(--text-faint);
    margin: -8px 0 12px;
  }
  .v-time {
    font-size: 0.74rem;
    color: var(--text-faint);
  }
  .diff-toggle {
    border: none;
    background: transparent;
    color: var(--text-dim);
    font-size: 0.84rem;
    font-weight: 600;
    cursor: pointer;
    padding: 4px 0;
    margin-bottom: 6px;
  }
  .diff-toggle:hover {
    color: var(--text);
  }
  .diff-list {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    margin-bottom: 14px;
    max-height: 180px;
    overflow-y: auto;
  }
  .diff-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    font-size: 0.8rem;
  }
  .diff-icon {
    width: 16px;
    text-align: center;
    font-weight: 700;
    flex-shrink: 0;
  }
  .diff-icon[data-status='changed'] {
    color: var(--warn);
  }
  .diff-icon[data-status='only-remote'] {
    color: var(--success);
  }
  .diff-icon[data-status='only-local'] {
    color: var(--danger);
  }
  .diff-path {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 0.76rem;
  }
  .diff-meta {
    color: var(--text-faint);
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .diff-sizes {
    color: var(--text-faint);
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .diff-more {
    color: var(--text-faint);
    font-size: 0.76rem;
    padding: 6px 0 2px;
    text-align: center;
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
</style>
