<script>
  import { games, navigate, toast, syncActivity, askConfirm } from '../lib/stores.js';
  import { api, native, coverURL, gameCover } from '../lib/api.js';
  import { addExclusion, addNegation, removeDirectExclusion } from '../lib/ignorerules.js';
  import { manualUploadOutcome } from '../lib/uploadActivity.js';
  import { peerRequiringSavePath } from '../lib/syncOutcome.js';
  import { t, locale } from '../lib/i18n.js';

  export let params = {};

  $: game = $games[params.gameId];
  $: activity = $syncActivity[params.gameId];

  let tab = 'snapshots';
  let newBranch = '';
  // Default to copying: a branch that keeps your save can never surprise you.
  let branchCopySave = true;
  let branchDialog = false;
  let snapshotComment = '';
  let busy = false;
  let browsing = null; // {snapshotId, files}

  // GameDetail is reused (not remounted) when navigating between games, so
  // reset per-game view state whenever the game id changes — otherwise the
  // previous game's cloud list / open file browser would leak across.
  let loadedFor = null;
  $: if (params.gameId !== loadedFor) {
    loadedFor = params.gameId;
    tab = 'snapshots';
    browsing = null;
    cloudSnaps = null;
    cloudLoading = false;
  }

  // Editable per-game configuration (loaded from the game, saved via PATCH).
  let cfg = null;
  $: if (game && (cfg === null || cfg._id !== game.id)) {
    cfg = {
      _id: game.id,
      appId: game.appId ?? '',
      exePath: game.exePath ?? '',
      coverUrl: game.coverUrl ?? '',
      autoSync: game.autoSync ?? true,
      maxSnapshots: game.maxSnapshots ?? 5,
      maxManualSnapshots: game.maxManualSnapshots ?? 0,
      syncIgnore: game.syncIgnore ?? ''
    };
  }
  // Cover preview for the config editor: a custom URL wins, else the proxied
  // Steam art for the App ID being edited.
  $: cfgCover =
    cfg?.coverUrl && !cfg.coverUrl.includes('steamstatic.com') ? cfg.coverUrl : coverURL(cfg?.appId);

  // ── Picking files to exclude ─────────────────────────────────────
  // The pattern box on its own asks you to type a filename you have to
  // already know, for a folder you cannot see, in a syntax you have to learn
  // — and gives no answer until the file turns up on another machine days
  // later. This lists what is actually in the game's folders and marks each
  // file with the verdict the sync engine itself would reach, so the rule and
  // its effect are on screen together.
  let showFiles = false;
  let saveFiles = null;
  let filesTruncated = false;
  let filesError = '';
  $: excludedCount = (saveFiles ?? []).filter((f) => f.excluded).length;

  // Reset with the game, like the other per-game view state above.
  $: if (params.gameId !== filesLoadedFor) {
    filesLoadedFor = params.gameId;
    showFiles = false;
    saveFiles = null;
    filesError = '';
  }
  let filesLoadedFor = null;

  // Ask the daemon to judge a rule list against the real folders. The rules
  // are sent rather than saved first: the point of a verdict is watching it
  // change as you type, and saving to find out would mean writing a rule to
  // discover whether it was the rule you meant.
  async function refreshSaveFiles(rulesText) {
    if (!game) return;
    try {
      const q = encodeURIComponent(rulesText ?? '');
      const res = await api.get(`/api/games/${game.id}/save-files?rules=${q}`);
      saveFiles = res?.files ?? [];
      filesTruncated = !!res?.truncated;
      filesError = '';
    } catch (e) {
      filesError = $locale === 'zh-CN' ? $t('game.filesError') : e.message;
    }
  }

  async function toggleFilePicker() {
    showFiles = !showFiles;
    if (showFiles && saveFiles === null) await refreshSaveFiles(cfg?.syncIgnore ?? '');
  }

  // Typing in the box re-judges the list, but not on every keystroke.
  let previewTimer = null;
  let lastPreviewed = null;
  $: if (showFiles && cfg && cfg.syncIgnore !== lastPreviewed) {
    lastPreviewed = cfg.syncIgnore;
    clearTimeout(previewTimer);
    const text = cfg.syncIgnore ?? '';
    previewTimer = setTimeout(() => refreshSaveFiles(text), 250);
  }

  // Ticking a file adds its pattern. Unticking removes the lines that name it
  // outright — and if it is still caught after that, by a wildcard or a folder
  // rule, adds a "!" exception, which is the only thing that can rescue one
  // file from a broader rule without abandoning the rule.
  //
  // Whether it is still caught is a question only the daemon can answer: it
  // holds the matcher the sync engine itself uses. The text editing around
  // that answer lives in ../lib/ignorerules.js, where it is tested.
  async function toggleFileExcluded(f) {
    if (!cfg) return;
    let text;
    if (!f.excluded) {
      text = addExclusion(cfg.syncIgnore, f.path);
    } else {
      text = removeDirectExclusion(cfg.syncIgnore, f.path);
      await refreshSaveFiles(text);
      const still = (saveFiles ?? []).find((x) => x.path === f.path && x.location === f.location);
      if (still?.excluded) text = addNegation(text, f.path);
    }
    cfg.syncIgnore = text;
    lastPreviewed = text;
    await refreshSaveFiles(text);
  }

  // Cloud explorer state.
  let cloudSnaps = null;
  let cloudLoading = false;

  $: branches = game ? Object.values(game.branches ?? {}) : [];
  $: allSnapshots = branches
    .flatMap((b) => (b.snapshots ?? []).map((s) => ({ ...s, branch: b.name })))
    .sort((a, b) => (a.timestamp < b.timestamp ? 1 : -1));

  const fmtTime = (time, language) => (time ? new Date(time).toLocaleString(language) : '—');
  const fmtSize = (n) => (n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1024).toFixed(1) + ' KB');

  async function run(label, fn) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (label) toast(label, 'success');
    } catch (e) {
      toast($locale === 'zh-CN' && !/\p{Script=Han}/u.test(e.message) ? $t('game.operationFailed') : e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const syncNow = () => run($t('game.syncTriggered'), async () => {
    const response = await api.post(`/api/games/${game.id}/sync`);
    const peer = peerRequiringSavePath(response);
    if (peer) throw new Error($t('game.sync.pathMappingRequired', { peer }));
  });
  const takeSnapshot = () =>
    run($t('game.snapshotCreated'), async () => {
      await api.post(`/api/games/${game.id}/snapshot`, { comment: snapshotComment });
      snapshotComment = '';
    });
  const rollback = async (snap) => {
    if (!(await askConfirm($t('game.restoreConfirm', { id: snap.id }), { title: $t('game.restoreTitle'), confirmText: $t('game.restore') }))) return;
    return run($t('game.restored', { id: snap.id }), () => api.post(`/api/games/${game.id}/rollback`, { snapshotId: snap.id }));
  };
  // Creating a branch asks what it starts from in its own dialog rather than
  // a checkbox beside the name field. The two answers do materially
  // different things to the save folder on the next switch, which is not a
  // decision to make by noticing a tickbox.
  function openBranchDialog() {
    if (!newBranch || busy) return;
    branchCopySave = true; // the safe answer is the one pre-selected
    branchDialog = true;
  }
  // Escape closes it, as in every other dialog in the app.
  function onBranchKeydown(e) {
    if (branchDialog && e.key === 'Escape') branchDialog = false;
  }
  const createBranch = () => {
    branchDialog = false;
    return run($t(branchCopySave ? 'game.branchCopied' : 'game.branchEmptyCreated'), async () => {
      await api.post(`/api/games/${game.id}/branch`, {
        name: newBranch,
        copyCurrentSave: branchCopySave,
      });
      newBranch = '';
    });
  };
  const switchBranch = (name) =>
    run($t('game.branchSwitched', { name }), () => api.post(`/api/games/${game.id}/branch/switch`, { name }));
  async function deleteBranch(name) {
    if (!(await askConfirm($t('game.deleteBranchConfirm', { name }), { title: $t('game.deleteBranchTitle'), confirmText: $t('game.delete'), danger: true }))) return;
    run($t('game.branchDeleted', { name }), () => api.del(`/api/games/${game.id}/branch/${encodeURIComponent(name)}`));
  }
  async function deleteSnapshot(snap) {
    if (!(await askConfirm($t('game.deleteSnapshotConfirm', { id: snap.id }), { title: $t('game.deleteSnapshotTitle'), confirmText: $t('game.delete'), danger: true }))) return;
    run($t('game.snapshotDeleted'), () => api.del(`/api/games/${game.id}/snapshot/${snap.id}`));
  }

  async function browseSnapshot(snap) {
    try {
      const files = await api.get(`/api/games/${game.id}/snapshot/${snap.id}/files`);
      browsing = { snapshotId: snap.id, files };
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('game.operationFailed') : e.message, 'error');
    }
  }

  const restoreFile = async (relPath) => {
    if (!(await askConfirm($t('game.restoreFileConfirm', { path: relPath, id: browsing.snapshotId }), { title: $t('game.restoreFileTitle'), confirmText: $t('game.restore') }))) return;
    return run($t('game.fileRestored', { path: relPath }), () =>
      api.post(`/api/games/${game.id}/snapshot/${browsing.snapshotId}/restore-file`, { relPath })
    );
  };

  async function untrack() {
    if (!(await askConfirm($t('game.untrackConfirm', { name: game.name }), { title: $t('game.untrackTitle'), confirmText: $t('game.untrack'), danger: true }))) return;
    const gameId = game.id;
    const gameName = game.name;
    await run($t('game.untracked'), () => api.del(`/api/games/${gameId}`));
    navigate('home');

    // Cloud copies would otherwise linger forever — offer to clean them up.
    try {
      const settings = await api.get('/api/settings');
      if (settings.cloudSync?.enabled) {
        if (
          await askConfirm(
            $t('game.cloudDeleteConfirm', { name: gameName }),
            { title: $t('game.cloudDeleteTitle'), confirmText: $t('game.cloudDelete'), cancelText: $t('game.keepCloud'), danger: true }
          )
        ) {
          const res = await api.post(`/api/cloud/delete-game/${gameId}`);
          toast(
            res.deleted > 0
              ? $t('game.cloudDeleted', { count: res.deleted })
              : $t('game.noCloudSnapshots'),
            'success'
          );
        }
      }
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('game.cloudCleanupFailed') : `Cloud cleanup failed: ${e.message}`, 'error');
    }
  }

  // ── Extra save locations ─────────────────────────────────────────
  //
  // A location learned from a peer or a backup arrives named but not placed:
  // the name travels between devices, the folder it lives in does not. Those
  // show as needing a folder, because until one is chosen the location is
  // silently skipped by every sync and every restore — and silence is exactly
  // what makes that dangerous.
  let locations = [];
  let locationsFor = null;
  let newLocation = '';
  $: if (game && tab === 'config' && locationsFor !== game.id) loadLocations();

  async function loadLocations() {
    locationsFor = game.id;
    try {
      locations = (await api.get(`/api/games/${game.id}/roots`)) ?? [];
    } catch {
      locations = [];
    }
  }

  async function pickLocation(name) {
    const label = String(name ?? '').trim();
    if (!label || busy) return;
    const dir = await native.selectDirectory($t('game.pickLocation', { label, name: game.name }));
    if (!dir) return;
    await run($t('game.locationAdded', { label }), async () => {
      await api.post(`/api/games/${game.id}/roots`, { name: label, path: dir });
      newLocation = '';
      locationsFor = null; // force a reload
      await loadLocations();
    });
  }

  async function removeLocation(name) {
    if (
      !(await askConfirm(
        $t('game.removeLocationConfirm', { name, gameName: game.name }),
        { title: $t('game.removeLocationTitle'), confirmText: $t('game.remove'), danger: true }
      ))
    )
      return;
    await run($t('game.locationRemoved', { name }), async () => {
      await api.del(`/api/games/${game.id}/roots/${encodeURIComponent(name)}`);
      locationsFor = null;
      await loadLocations();
    });
  }

  // Linked copies — cross-device "same game" links (the manual counterpart
  // to App-ID matching). Loaded lazily when the Manage tab is opened.
  let aliases = [];
  let aliasesFor = null;
  let linkTarget = '';
  $: if (game && tab === 'danger' && aliasesFor !== game.id) loadAliases();
  $: otherGames = Object.values($games).filter((g) => g.id !== params.gameId);

  // Games tracked on paired devices. Without these the picker can only offer
  // entries from this machine, which merges local duplicates but can never
  // connect a title tracked under one name here and another name there —
  // the case App-ID matching can't cover, because a save sitting in
  // AppData\<company>\<game> carries no App ID anywhere in its path.
  let peerGames = [];
  let peerGamesFor = null;
  let peerGamesLoading = false;
  $: if (game && tab === 'danger' && peerGamesFor !== game.id) loadPeerGames();

  async function loadPeerGames() {
    peerGamesFor = game.id;
    peerGamesLoading = true;
    peerGames = [];
    try {
      const peers = await api.get('/api/peers');
      // Ask each device separately and keep whatever answers: one being
      // offline must not cost you the ability to link against the others.
      const results = await Promise.allSettled(
        (peers ?? []).map(async (p) => ({
          peer: p,
          games: await api.get(`/api/peers/${p.id}/games`),
        }))
      );
      const local = new Set(Object.keys($games));
      peerGames = results
        .filter((r) => r.status === 'fulfilled')
        .flatMap((r) =>
          (r.value.games ?? [])
            // An id we already track is the same entry, not a link target.
            .filter((g) => !local.has(g.id))
            .map((g) => ({ ...g, peerName: r.value.peer.name }))
        );
    } catch {
      peerGames = [];
    } finally {
      peerGamesLoading = false;
    }
  }

  async function loadAliases() {
    aliasesFor = game.id;
    try {
      aliases = await api.get(`/api/games/${game.id}/aliases`);
    } catch {
      aliases = [];
    }
  }
  async function linkGame() {
    if (!linkTarget) return;
    const other = $games[linkTarget];
    const remote = peerGames.find((g) => g.id === linkTarget);

    // Two different operations behind one button, and the difference matters
    // to the user: merging a local entry removes it from this library, while
    // linking another device's entry changes nothing here at all. Saying
    // "removed from your library" for the second would be a lie about a
    // destructive step that isn't happening.
    const message = remote
      ? $t('game.linkRemoteConfirm', { other: remote.name, peer: remote.peerName, name: game.name })
      : $t('game.linkLocalConfirm', { other: other?.name ?? linkTarget, name: game.name });

    const ok = await askConfirm(message, { title: $t('game.linkTitle'), confirmText: $t('game.link') });
    if (!ok) return;
    const canonicalId = game.id;
    await run($t('game.linked'), () => api.post(`/api/games/${canonicalId}/link`, { alias: linkTarget }));
    linkTarget = '';
    await loadAliases();
  }
  async function unlink(aliasId) {
    await run($t('game.unlinked'), () => api.del(`/api/games/${game.id}/alias/${aliasId}`));
    await loadAliases();
  }

  // ── Configuration ────────────────────────────────────────────────
  // A Steam App ID is digits. Saving something else stores a value that
  // matches nothing, launches nothing and fetches no art, with no complaint
  // at any point — which reads as the features being broken rather than the
  // field being wrong. Catch it at the edit instead.
  let appIdError = '';
  async function saveConfig() {
    const appId = (cfg.appId ?? '').trim();
    if (appId !== '' && !/^\d+$/.test(appId)) {
      appIdError = $t('game.appIdError');
      return;
    }
    appIdError = '';
    await run($t('game.configSaved'), () =>
      api.patch(`/api/games/${game.id}`, {
        appId,
        exePath: cfg.exePath,
        coverUrl: cfg.coverUrl,
        autoSync: cfg.autoSync,
        maxSnapshots: Number(cfg.maxSnapshots),
        maxManualSnapshots: Number(cfg.maxManualSnapshots),
        syncIgnore: cfg.syncIgnore ?? ''
      })
    );
  }

  async function browseExe() {
    const file = await native.selectFile($t('game.selectExecutable'));
    if (file) cfg.exePath = file;
  }

  // ── Cloud explorer ───────────────────────────────────────────────
  async function loadCloudSnaps() {
    cloudLoading = true;
    cloudSnaps = null;
    try {
      cloudSnaps = await api.get(`/api/cloud/snapshots/${game.id}`);
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('game.cloudLoadFailed') : e.message, 'error');
      cloudSnaps = [];
    } finally {
      cloudLoading = false;
    }
  }

  $: if (game && tab === 'cloud' && cloudSnaps === null && !cloudLoading) loadCloudSnaps();

  const restoreCloud = async (snap) => {
    if (!(await askConfirm($t('game.restoreCloudConfirm', { id: snap.snapshotId }), { title: $t('game.restoreCloudTitle'), confirmText: $t('game.downloadRestore') }))) return;
    return run($t('game.restoredCloud'), async () => {
      await api.post(`/api/cloud/restore/${game.id}`, { fileName: snap.name });
    });
  };

  const uploadToCloud = () =>
    run(null, async () => {
      const res = await api.post(`/api/cloud/sync-local/${game.id}`);
      if (manualUploadOutcome(res) === 'conflict') {
        toast($t('cloud.upload.conflict', { uploaded: res.uploaded, conflicts: res.conflicts, failed: res.failed }), 'error');
      } else if (manualUploadOutcome(res) === 'failed') {
        toast($t('game.uploadPartial', { uploaded: res.uploaded, failed: res.failed, skipped: res.skipped }), 'error');
      } else {
        toast($t('game.uploadDone', { uploaded: res.uploaded, skipped: res.skipped }), 'success');
      }
      await loadCloudSnaps();
    });

  async function launchGame() {
    await run($t('game.launching'), () => api.post(`/api/games/${game.id}/launch`));
  }

  let editPath = false;
  let pathDraft = '';
  // Reveal the save location in Explorer / Finder / the Linux file manager.
  // The bridge returns a message when it can't (e.g. the folder was deleted).
  async function openSaveFolder() {
    const problem = await native.openFolder(game.savePath);
    if (problem) toast($locale === 'zh-CN' ? $t('game.openFolderFailed') : problem, 'error');
  }

  async function savePath() {
    await run($t('game.pathUpdated'), () => api.patch(`/api/games/${game.id}`, { savePath: pathDraft }));
    editPath = false;
  }

</script>

{#if !game}
  <div class="empty"><h3>{$t('game.notFound')}</h3></div>
{:else}
  <div class="head">
    <button class="btn icon back" on:click={() => navigate('home')} title={$t('game.back')}>←</button>
    {#if gameCover(game)}
      <img
        class="head-cover"
        src={gameCover(game)}
        alt=""
        on:load={(e) => (e.currentTarget.style.display = '')}
        on:error={(e) => (e.currentTarget.style.display = 'none')}
      />
    {/if}
    <div class="title-block">
      <h2 class="page-title">{game.name}</h2>
      <div class="sub">
        {$t('game.branch')} <strong>{game.activeBranch}</strong>
        {#if activity?.state === 'running'}
          · <span class="syncing">{$t('game.syncing', { percent: activity.percentage ?? 0 })}</span>
        {/if}
      </div>
    </div>
    <div class="head-actions">
      {#if game.appId || game.exePath}
        <button class="btn" disabled={busy} on:click={launchGame}>▶ {$t('game.launch')}</button>
      {/if}
      <button class="btn primary" disabled={busy} on:click={syncNow}>⟳ {$t('game.syncNow')}</button>
    </div>
  </div>

  <div class="path-line">
    {#if editPath}
      <input class="path-input" bind:value={pathDraft} />
      <button class="btn small" on:click={async () => (pathDraft = (await native.selectDirectory($t('game.selectSaveFolder'))) || pathDraft)}>{$t('game.browse')}</button>
      <button class="btn small primary" on:click={savePath}>{$t('game.save')}</button>
      <button class="btn small" on:click={() => (editPath = false)}>{$t('game.cancel')}</button>
    {:else}
      <span class="path" title={game.savePath}>{game.savePath}</span>
      <button class="btn small" on:click={openSaveFolder} title={$t('game.openFolderHint')}>
        📂 {$t('game.openFolder')}
      </button>
      <button class="btn small" on:click={() => { pathDraft = game.savePath; editPath = true; }}>{$t('game.edit')}</button>
    {/if}
  </div>

  <div class="pill-tabs tabs">
    <button class:active={tab === 'snapshots'} on:click={() => (tab = 'snapshots')}>{$t('game.snapshots')}</button>
    <button class:active={tab === 'branches'} on:click={() => (tab = 'branches')}>{$t('game.branches')}</button>
    <button class:active={tab === 'cloud'} on:click={() => (tab = 'cloud')}>☁️ {$t('game.cloud')}</button>
    <button class:active={tab === 'config'} on:click={() => (tab = 'config')}>{$t('game.config')}</button>
    <button class:active={tab === 'danger'} on:click={() => (tab = 'danger')}>{$t('game.manage')}</button>
  </div>

  {#if tab === 'snapshots'}
    <div class="card snap-new">
      <input placeholder={$t('game.snapshotComment')} bind:value={snapshotComment} />
      <button class="btn primary" disabled={busy} on:click={takeSnapshot}>📸 {$t('game.snapshotNow')}</button>
    </div>

    {#if browsing}
      <div class="card browse">
        <div class="browse-head">
          <h3>{$t('game.filesInSnapshot', { id: browsing.snapshotId })}</h3>
          <button class="btn small" on:click={() => (browsing = null)}>{$t('game.close')}</button>
        </div>
        {#each browsing.files.filter((f) => !f.isDir) as f}
          <div class="file-row">
            <span class="file-path">{f.path}</span>
            <span class="file-size">{fmtSize(f.size)}</span>
            <button class="btn small" disabled={busy} on:click={() => restoreFile(f.path)}>{$t('game.restoreFile')}</button>
          </div>
        {/each}
      </div>
    {/if}

    {#if allSnapshots.length === 0}
      <div class="empty"><h3>{$t('game.noSnapshots')}</h3><p>{$t('game.noSnapshotsHint')}</p></div>
    {:else}
      <div class="snap-list">
        {#each allSnapshots as snap (snap.id)}
          <div class="card snap">
            <div class="snap-info">
              <div class="snap-top">
                <span class="snap-id">{snap.id}</span>
                <span class="badge offline">{snap.branch}</span>
                {#if snap.isSystemAuto}<span class="badge offline">{$t('game.auto')}</span>{/if}
              </div>
              <div class="snap-comment">{snap.comment}</div>
              <div class="snap-meta">{fmtTime(snap.timestamp, $locale)} · {fmtSize(snap.sizeBytes)}</div>
            </div>
            <div class="snap-actions">
              <button class="btn small" on:click={() => browseSnapshot(snap)}>{$t('game.browseFiles')}</button>
              <button class="btn small primary" disabled={busy} on:click={() => rollback(snap)}>{$t('game.restore')}</button>
              <button class="btn small danger" disabled={busy} on:click={() => deleteSnapshot(snap)}>{$t('game.delete')}</button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {:else if tab === 'branches'}
    <div class="card branch-new">
      <div class="snap-new-row">
        <input
          placeholder={$t('game.newBranchPlaceholder')}
          bind:value={newBranch}
          on:keydown={(e) => e.key === 'Enter' && openBranchDialog()}
        />
        <button class="btn primary" disabled={!newBranch || busy} on:click={openBranchDialog}>+ {$t('game.createBranch')}</button>
      </div>
      <span class="hint">
        {$t('game.branchIntro')}
      </span>
    </div>
    <div class="snap-list">
      {#each branches as branch (branch.name)}
        <div class="card snap">
          <div class="snap-info">
            <div class="snap-top">
              <span class="snap-id">{branch.name}</span>
              {#if branch.name === game.activeBranch}<span class="badge online">{$t('game.active')}</span>{/if}
            </div>
            <div class="snap-meta">{$t('game.snapshotCount', { count: branch.snapshots?.length ?? 0 })}</div>
          </div>
          {#if branch.name !== game.activeBranch}
            <div class="branch-actions">
              <button class="btn small primary" disabled={busy} on:click={() => switchBranch(branch.name)}>
                {$t('game.switchTo')}
              </button>
              {#if branch.name !== 'main'}
                <button class="btn small danger" disabled={busy} on:click={() => deleteBranch(branch.name)}>
                  {$t('game.delete')}
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
    <p class="branch-hint">
      {$t('game.branchSwitchHint')}
    </p>
  {:else if tab === 'cloud'}
    <div class="card">
      <div class="cloud-head">
        <div>
          <h3>☁️ {$t('game.cloudFor', { name: game.name })}</h3>
          <p class="cloud-sub">{$t('game.cloudSubtitle')}</p>
        </div>
        <div class="cloud-actions">
          <button class="btn small" disabled={busy} on:click={loadCloudSnaps}>↻ {$t('game.refresh')}</button>
          <button class="btn small primary" disabled={busy} on:click={uploadToCloud}>↑ {$t('game.uploadLocal')}</button>
        </div>
      </div>

      {#if cloudLoading}
        <div class="cloud-loading"><span class="cspin"></span> {$t('game.cloudLoading')}</div>
      {:else if !cloudSnaps || cloudSnaps.length === 0}
        <div class="cloud-empty">
          <p>{$t('game.cloudEmpty')}</p>
          <p class="cloud-hint">
            {$t('game.cloudEmptyBefore')} <button class="linklike" on:click={() => navigate('cloud')}>{$t('nav.cloud')}</button>{$t('game.cloudEmptyAfter')}
          </p>
        </div>
      {:else}
        <table class="cloud-table">
          <thead>
            <tr><th>{$t('game.branch')}</th><th>{$t('game.date')}</th><th>{$t('game.size')}</th><th></th></tr>
          </thead>
          <tbody>
            {#each cloudSnaps as snap (snap.name)}
              <tr>
                <td><span class="badge offline">{snap.branch}</span></td>
                <td class="mono">{new Date(snap.createdTime).toLocaleString($locale)}</td>
                <td class="mono">{fmtSize(snap.sizeBytes)}</td>
                <td class="right">
                  <button class="btn small primary" disabled={busy} on:click={() => restoreCloud(snap)}>{$t('game.restore')}</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {:else if tab === 'config'}
    {#if cfg}
      <div class="card config-card">
        <div class="config-cover">
          {#if cfgCover}
            <img src={cfgCover} alt="" on:error={(e) => (e.currentTarget.style.display = 'none')} />
          {:else}
            <div class="config-cover-fallback">🎮</div>
          {/if}
        </div>
        <div class="config-fields">
          <h3>{$t('game.configTitle')}</h3>
          <div class="field">
            <label for="c-appid">{$t('game.steamAppId')}</label>
            <input id="c-appid" placeholder={$t('game.appIdPlaceholder')} bind:value={cfg.appId} />
            <span class="hint">
              {$t('game.steamAppIdHint')}
            </span>
            {#if appIdError}<span class="hint hint-error">{appIdError}</span>{/if}
          </div>
          <div class="field">
            <label for="c-exe">{$t('game.exePath')}</label>
            <div class="path-row">
              <input id="c-exe" placeholder={$t('game.exePlaceholder')} bind:value={cfg.exePath} />
              <button class="btn" on:click={browseExe}>{$t('game.browse')}</button>
            </div>
          </div>
          <div class="field">
            <label for="c-cover">{$t('game.coverUrl')}</label>
            <input id="c-cover" placeholder={$t('game.coverPlaceholder')} bind:value={cfg.coverUrl} />
            <span class="hint">{$t('game.coverHint')}</span>
          </div>
          <label class="check">
            <input type="checkbox" bind:checked={cfg.autoSync} />
            {$t('game.autoSync')}
          </label>
          <div class="field" style="margin-top: 12px;">
            <label for="c-max">{$t('game.autoSnapshotLimit')}</label>
            <input id="c-max" type="number" min="0" bind:value={cfg.maxSnapshots} />
            <span class="hint">
              {$t('game.autoSnapshotHint')}
            </span>
          </div>
          <div class="field">
            <label for="c-max-manual">{$t('game.manualSnapshotLimit')}</label>
            <input id="c-max-manual" type="number" min="0" bind:value={cfg.maxManualSnapshots} />
            <span class="hint">
              {$t('game.manualSnapshotHint')}
            </span>
          </div>
          <div class="excludes">
            <h4>{$t('game.excludeTitle')}</h4>
            <p class="hint">
              {$t('game.excludeIntro')}
            </p>
            <textarea
              class="exclude-box"
              rows="4"
              spellcheck="false"
              placeholder={'Config.gs\n*.log\nlogs/'}
              bind:value={cfg.syncIgnore}
            ></textarea>
            <span class="hint">
              {$t('game.excludePatternHint')}
            </span>
            <span class="hint">
              {$t('game.excludeSnapshotHint')}
            </span>

            <div class="picker-head">
              <button class="btn small" on:click={toggleFilePicker}>
                {showFiles ? '▾' : '▸'} {$t('game.pickFiles')}
              </button>
              {#if showFiles && saveFiles}
                <span class="hint picker-count">
                  {$t('game.excludedCount', { excluded: excludedCount, total: saveFiles.length })}
                  {#if filesTruncated}· {$t('game.filesTruncated', { count: saveFiles.length })}{/if}
                </span>
              {/if}
            </div>

            {#if showFiles}
              {#if filesError}
                <p class="hint err">{filesError}</p>
              {:else if !saveFiles}
                <p class="hint"><span class="cspin"></span> {$t('game.readingFiles')}</p>
              {:else if saveFiles.length === 0}
                <p class="hint">{$t('game.noFiles')}</p>
              {:else}
                <div class="file-picker">
                  {#each saveFiles as f (f.location + '/' + f.path)}
                    <label class="file-row" class:excluded={f.excluded}>
                      <input type="checkbox" checked={f.excluded} on:change={() => toggleFileExcluded(f)} />
                      <span class="file-name">
                        {#if f.location}<span class="file-loc">{f.location} ›</span>{/if}{f.path}
                      </span>
                      <span class="file-verdict">{$t(f.excluded ? 'game.wontSync' : 'game.willSync')}</span>
                    </label>
                  {/each}
                </div>
                <span class="hint">
                  {$t('game.filePickerHint')}
                  {#if locations.length > 0}
                    {$t('game.filePickerLocationsHint')}
                  {/if}
                </span>
              {/if}
            {/if}
          </div>
          <div class="locations">
            <h4>{$t('game.locationsTitle')}</h4>
            <p class="hint">
              {$t('game.locationsIntro')}
            </p>
            <div class="loc-row">
              <span class="loc-name">{$t('game.mainSave')}</span>
              <span class="loc-path" title={game.savePath}>{game.savePath}</span>
            </div>
            {#each locations as loc (loc.name)}
              <div class="loc-row" class:unmapped={!loc.mapped}>
                <span class="loc-name">{loc.name}</span>
                {#if loc.mapped}
                  <span class="loc-path" title={loc.path}>{loc.path}</span>
                {:else}
                  <span class="loc-path missing">
                    {$t('game.unmappedLocation')}
                  </span>
                {/if}
                <button class="btn small" disabled={busy} on:click={() => pickLocation(loc.name)}>
                  {$t(loc.mapped ? 'game.change' : 'game.chooseFolder')}
                </button>
                <button class="btn small danger" disabled={busy} on:click={() => removeLocation(loc.name)}>
                  {$t('game.remove')}
                </button>
              </div>
            {/each}
            <div class="loc-add">
              <input placeholder={$t('game.newLocationPlaceholder')} bind:value={newLocation} />
              <button class="btn" disabled={!newLocation || busy} on:click={() => pickLocation(newLocation)}>
                + {$t('game.addFolder')}
              </button>
            </div>
            <span class="hint">
              {$t('game.locationNameHint')}
            </span>
          </div>
          <div class="config-save">
            <button class="btn primary" disabled={busy} on:click={saveConfig}>{$t('game.saveConfig')}</button>
          </div>
        </div>
      </div>
    {/if}
  {:else}
    <div class="card">
      <h3>{$t('game.linkedCopies')}</h3>
      <p class="danger-desc">
        {$t('game.linkedCopiesIntro')}
      </p>
      {#if aliases.length > 0}
        <div class="alias-list">
          {#each aliases as a}
            <div class="alias-row">
              <span class="alias-id" title={a.savePath || a.id}>
                🔗 {a.name || a.id}{a.savePath ? ` — ${a.savePath}` : ''}
              </span>
              <button class="btn small" disabled={busy} on:click={() => unlink(a.id)}>{$t('game.unlink')}</button>
            </div>
          {/each}
        </div>
      {/if}
      {#if otherGames.length > 0 || peerGames.length > 0}
        <div class="link-row">
          <select bind:value={linkTarget}>
            <option value="">{$t('game.chooseLink')}</option>
            {#if otherGames.length > 0}
              <optgroup label={$t('game.linkLocalGroup')}>
                {#each otherGames as g}
                  <!-- Same-named entries are normal now that one game can be
                       tracked at several save locations, so show the path too —
                       otherwise duplicates are indistinguishable in this list. -->
                  <option value={g.id}>{g.name} — {g.savePath}</option>
                {/each}
              </optgroup>
            {/if}
            {#if peerGames.length > 0}
              <optgroup label={$t('game.linkPeerGroup')}>
                {#each peerGames as g}
                  <option value={g.id}>{g.name} — {g.peerName}</option>
                {/each}
              </optgroup>
            {/if}
          </select>
          <button class="btn small primary" disabled={busy || !linkTarget} on:click={linkGame}>{$t('game.link')}</button>
        </div>
        {#if peerGamesLoading}
          <p class="danger-desc">{$t('game.checkingPeers')}</p>
        {/if}
      {:else if peerGamesLoading}
        <p class="danger-desc">{$t('game.checkingPeers')}</p>
      {:else}
        <p class="danger-desc">
          {$t('game.noLinkTargets')}
        </p>
      {/if}
    </div>

    <div class="card" style="margin-top: 16px;">
      <h3>{$t('game.untrackTitle')}</h3>
      <p class="danger-desc">
        {$t('game.untrackHint', { name: game.name })}
      </p>
      <button class="btn danger" disabled={busy} on:click={untrack}>{$t('game.untrackThis')}</button>
    </div>
  {/if}
{/if}

<svelte:window on:keydown={onBranchKeydown} />

{#if branchDialog && game}
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <div class="overlay" on:click={() => (branchDialog = false)}>
    <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
    <div class="modal card" on:click|stopPropagation>
      <h3>🌱 {$t('game.newBranchTitle', { name: newBranch })}</h3>
      <p class="desc">{$t('game.branchStartQuestion')}</p>

      <label class="choice" class:sel={branchCopySave}>
        <input type="radio" bind:group={branchCopySave} value={true} />
        <span class="c-body">
          <span class="c-title">{$t('game.copyCurrent')}</span>
          <span class="c-desc">
            {$t('game.copyCurrentHint', { name: newBranch })}
          </span>
        </span>
      </label>

      <label class="choice" class:sel={!branchCopySave}>
        <input type="radio" bind:group={branchCopySave} value={false} />
        <span class="c-body">
          <span class="c-title">{$t('game.freshStart')}</span>
          <span class="c-desc">
            {$t('game.freshStartHint', { name: newBranch, current: game.activeBranch })}
          </span>
        </span>
      </label>

      <div class="actions">
        <button class="btn" on:click={() => (branchDialog = false)}>{$t('game.cancel')}</button>
        <button class="btn primary" disabled={busy} on:click={createBranch}>{$t('game.createBranch')}</button>
      </div>
      <p class="hint-line">
        🛡️ {$t('game.branchSafetyHint')}
      </p>
    </div>
  </div>
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 6px;
  }
  .back {
    font-size: 1rem;
  }
  .head-cover {
    height: 52px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
  }
  .title-block {
    flex: 1;
    min-width: 0;
  }
  .sub {
    color: var(--text-dim);
    font-size: 0.85rem;
    margin-top: 2px;
  }
  .syncing {
    color: var(--accent);
    font-weight: 600;
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }
  .path-line {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 18px 50px;
  }
  .path {
    font-size: 0.8rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path-input {
    flex: 1;
    padding: 6px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.82rem;
    outline: none;
  }
  .hint-error {
    color: var(--danger);
  }
  .tabs {
    margin-bottom: 18px;
  }
  .snap-new {
    display: flex;
    gap: 10px;
    padding: 14px;
    margin-bottom: 14px;
  }
  /* Same card as .snap-new, stacked so the choice and its consequence sit
     under the name field rather than beside it. */
  .branch-new {
    padding: 14px;
    margin-bottom: 14px;
  }
  .branch-new .snap-new-row {
    display: flex;
    gap: 10px;
  }
  .branch-new .hint {
    display: block;
    margin-top: 10px;
  }
  /* The new-branch dialog. Deliberately the same furniture as the conflict
     modal — both are "this is about to touch your save folder, choose". */
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
    width: 520px;
    max-width: calc(100vw - 48px);
    max-height: calc(100vh - 80px);
    overflow-y: auto;
  }
  .modal h3 {
    margin-bottom: 8px;
    overflow-wrap: anywhere;
  }
  .modal .desc {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-bottom: 14px;
  }
  .choice {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px;
    margin-bottom: 10px;
    cursor: pointer;
    transition:
      border-color 0.13s ease,
      background 0.13s ease;
  }
  .choice:hover {
    border-color: var(--border-strong);
  }
  .choice.sel {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .choice input {
    margin-top: 1px;
  }
  .c-body {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .c-title {
    font-size: 0.9rem;
    font-weight: 600;
  }
  .c-desc {
    font-size: 0.8rem;
    color: var(--text-dim);
    line-height: 1.5;
  }
  .modal .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 14px;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
  .snap-new input,
  .branch-new .snap-new-row input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .snap-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .snap {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
  }
  .snap-info {
    flex: 1;
    min-width: 0;
  }
  .snap-top {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }
  .snap-id {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .snap-comment {
    font-size: 0.83rem;
    color: var(--text-dim);
    margin-bottom: 3px;
  }
  .snap-meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .snap-actions {
    display: flex;
    gap: 6px;
  }
  .browse {
    margin-bottom: 14px;
  }
  .browse-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
  }
  .file-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 4px;
    border-bottom: 1px solid var(--border);
    font-size: 0.85rem;
  }
  .file-row:last-child {
    border-bottom: none;
  }
  .file-path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file-size {
    color: var(--text-faint);
    font-size: 0.78rem;
  }
  .branch-actions {
    display: flex;
    gap: 6px;
  }
  .branch-hint {
    margin-top: 12px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  .danger-desc {
    color: var(--text-dim);
    font-size: 0.88rem;
    margin: 8px 0 14px;
  }

  /* Linked copies */
  .alias-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
  }
  .alias-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 7px 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .alias-id {
    font-family: monospace;
    font-size: 0.82rem;
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .link-row {
    display: flex;
    gap: 10px;
    align-items: center;
    margin-top: 14px;
  }
  .link-row select {
    flex: 1;
    min-width: 0;
    padding: 8px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
  }

  /* Cloud explorer */
  .cloud-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 14px;
    flex-wrap: wrap;
  }
  .cloud-sub {
    font-size: 0.82rem;
    color: var(--text-faint);
    margin-top: 3px;
  }
  .cloud-actions {
    display: flex;
    gap: 8px;
  }
  .cloud-loading,
  .cloud-empty {
    padding: 30px 10px;
    text-align: center;
    color: var(--text-faint);
  }
  .cloud-hint {
    font-size: 0.82rem;
    margin-top: 6px;
  }
  .linklike {
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    padding: 0;
    font: inherit;
  }
  .cspin {
    display: inline-block;
    width: 13px;
    height: 13px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    vertical-align: middle;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .cloud-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }
  .cloud-table th {
    text-align: left;
    color: var(--text-faint);
    font-weight: 600;
    font-size: 0.75rem;
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
  }
  .cloud-table td {
    padding: 9px 10px;
    border-bottom: 1px solid var(--border);
  }
  .cloud-table tr:last-child td {
    border-bottom: none;
  }
  .cloud-table .mono {
    font-family: 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.78rem;
    color: var(--text-dim);
  }
  .cloud-table .right {
    text-align: right;
  }

  /* Configuration panel */
  .config-card {
    display: flex;
    gap: 20px;
    align-items: flex-start;
  }
  .config-cover {
    flex-shrink: 0;
    width: 160px;
    aspect-ratio: 460 / 215;
    border-radius: var(--radius);
    overflow: hidden;
    border: 1px solid var(--border);
    background: var(--bg);
  }
  .config-cover img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .config-cover-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2rem;
  }
  .config-fields {
    flex: 1;
    min-width: 0;
  }
  .config-fields h3 {
    margin-bottom: 14px;
  }
  .config-fields .field {
    margin-bottom: 14px;
  }
  .excludes {
    margin-top: 18px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  .excludes h4 {
    font-size: 0.92rem;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .exclude-box {
    width: 100%;
    margin-top: 8px;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 0.82rem;
    line-height: 1.6;
    resize: vertical;
    outline: none;
  }
  .exclude-box:focus {
    border-color: var(--accent);
  }
  .excludes .hint + .hint {
    margin-top: 6px;
  }
  .picker-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 12px;
    flex-wrap: wrap;
  }
  .picker-count {
    margin-top: 0;
  }
  .file-picker {
    margin-top: 8px;
    max-height: 280px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
  }
  .file-row {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 5px 10px;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    font-size: 0.8rem;
  }
  .file-row:last-child {
    border-bottom: 0;
  }
  .file-row:hover {
    background: var(--bg-elev, rgba(127, 127, 127, 0.06));
  }
  .file-name {
    flex: 1;
    min-width: 0;
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    word-break: break-all;
  }
  /* The location a file lives in, when the game has more than its save
     folder — the same filename can appear in two of them. */
  .file-loc {
    color: var(--accent);
    margin-right: 6px;
  }
  .file-verdict {
    flex: 0 0 auto;
    font-size: 0.72rem;
    color: var(--text-faint);
  }
  .file-row.excluded .file-name {
    text-decoration: line-through;
    color: var(--text-faint);
  }
  .file-row.excluded .file-verdict {
    color: var(--warn, var(--accent));
  }
  .hint.err {
    color: var(--danger, #e5484d);
  }
  .locations {
    margin-top: 18px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  .locations h4 {
    font-size: 0.92rem;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .loc-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    margin-top: 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: 0.84rem;
  }
  .loc-row.unmapped {
    border-color: rgba(251, 191, 36, 0.4);
  }
  .loc-name {
    flex: none;
    min-width: 90px;
    font-weight: 600;
  }
  .loc-path {
    flex: 1;
    min-width: 0;
    color: var(--text-dim);
    font-size: 0.78rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .loc-path.missing {
    color: var(--warn);
  }
  .loc-add {
    display: flex;
    gap: 8px;
    margin-top: 10px;
  }
  .loc-add input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.86rem;
    outline: none;
  }
  .config-save {
    display: flex;
    justify-content: flex-end;
    margin-top: 8px;
  }
  @media (max-width: 720px) {
    .config-card {
      flex-direction: column;
    }
    .config-cover {
      width: 100%;
    }
  }
</style>
