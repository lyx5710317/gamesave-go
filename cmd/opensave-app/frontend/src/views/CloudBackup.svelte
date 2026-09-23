<script>
  import { gameList, toast, cloudAuthEvent, cloudUploadEvent, backupProgressEvent, askConfirm, settings } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { backdropClose } from '../lib/backdrop.js';
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../lib/i18n.js';
  import { summarizeLocalPreview } from '../lib/localPreview.js';

  // All routed views share the same component contract. This page currently
  // needs no route parameters, but accepting them keeps dynamic navigation
  // warning-free and leaves room for future deep links.
  export let params = {};
  $: params;

  let config = null;
  let localPreview = null;
  let localPreviewBusy = false;
  let localPreviewError = '';
  $: localPreviewSummary = localPreview ? summarizeLocalPreview(localPreview) : null;
  let busy = false;
  let authCode = '';
  let authInProgress = false;
  let authAuto = false; // backend caught the redirect automatically
  let showManualCode = false;

  // The daemon broadcasts cloud-auth when the browser redirect lands on
  // its temporary localhost listener — sign-in completes with no pasting.
  const unsubAuth = cloudAuthEvent.subscribe((ev) => {
    if (!ev || !authInProgress) return;
    cloudAuthEvent.set(null);
    authInProgress = false;
    showManualCode = false;
    if (ev.success) {
      toast($t('cloud.toast.connectedAs', { email: ev.userEmail }), 'success');
      load();
    } else {
      toast(ev.error ?? $t('cloud.toast.signInFailed'), 'error');
    }
  });
  onDestroy(unsubAuth);
  let cloudGames = null; // grouped explorer data (null = not loaded yet)
  let browsing = false;
  let cloudOpen = false; // cloud snapshot browser modal
  let cloudFilter = '';
  let cloudTab = 'all'; // all | cloud | local
  let detailId = null; // gameId drilled into (null = tile grid)
  let uploading = false; // an upload-to-cloud is running (independent of busy
  // so Restore/Delete don't gray out while snapshots are being pushed up)
  let uploadProg = null; // live {done, total, current} from the daemon

  const unsubUpload = cloudUploadEvent.subscribe((ev) => {
    if (!ev) return;
    uploadProg = ev.complete ? null : ev;
  });
  onDestroy(unsubUpload);

  const providers = [
    { id: 'google_drive', labelKey: 'cloud.providers.googleDrive', oauth: true, img: 'cloud/googledrive.png' },
    { id: 'onedrive', labelKey: 'cloud.providers.oneDrive', oauth: true, img: 'cloud/onedrive.png' },
    { id: 'dropbox', labelKey: 'cloud.providers.dropbox', oauth: true, img: 'cloud/dropbox.png' },
    { id: 'local', labelKey: 'cloud.providers.local', oauth: false, icon: 'folder' },
    { id: 'webdav', labelKey: 'cloud.providers.webdav', oauth: false, icon: 'cloud' },
    { id: 'webhook', labelKey: 'cloud.providers.webhook', oauth: false, icon: 'webhook' }
  ];

  const iconPaths = {
    folder: 'M20 6h-8l-2-2H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2z',
    cloud: 'M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z',
    webhook: 'M20 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V8h16v10zM12 10H8v2h4v-2zm4 4h-8v2h8v-2z'
  };

  // The provider the stored OAuth tokens belong to (server truth at load
  // time) — independent of which card is currently selected, so the
  // connected card keeps saying "connected" while you browse the others.
  let connectedProvider = null;

  // Some providers can't reveal the account email (privacy settings / missing
  // scope) — the daemon stores a placeholder then. Only display it if it
  // actually looks like an address.
  const isEmail = (s) => /\S+@\S+\.\S+/.test(s ?? '');

  // Pure function of its arguments so the template call re-renders whenever
  // connectedProvider/config change (a closure over `config` would go stale).
  function providerStatus(id, connected, cfg) {
    if (!cfg) return '';
    if (id === connected) {
      const email = cfg.tokens?.userEmail;
      return isEmail(email) ? email : $t('cloud.status.connected');
    }
    if (['google_drive', 'onedrive', 'dropbox'].includes(id)) return $t('cloud.status.clickToSignIn');
    if (id === cfg.provider && cfg.url) return $t('cloud.status.configured');
    return $t('cloud.status.notConfigured');
  }

  onMount(load);

  async function inspectLocalSaves() {
    localPreviewBusy = true;
    localPreview = null;
    localPreviewError = '';
    try {
      localPreview = await api.get('/api/cloud/join/local-preview');
    } catch (e) {
      localPreviewError = e.message;
    } finally {
      localPreviewBusy = false;
    }
  }

  async function load() {
    try {
      const settings = await api.get('/api/settings');
      config = settings.cloudSync ?? {
        enabled: false, provider: 'local', url: '', username: '', password: '', headers: '{}', folderId: ''
      };
      // OAuth tokens survive a provider switch (so switching back reconnects
      // instantly) — but only the OAuth provider they belong to is
      // "connected"; never badge local/webdav/webhook off someone's tokens.
      connectedProvider =
        config.tokens?.userEmail && ['google_drive', 'onedrive', 'dropbox'].includes(config.provider)
          ? config.provider
          : null;
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function save() {
    busy = true;
    try {
      settings.set(await api.post('/api/settings', { cloudSync: config }));
      toast($t('cloud.toast.settingsSaved'), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // ── Your own OAuth app ───────────────────────────────────────────
  // The daemon has always supported a per-provider client id and secret;
  // nothing in the app set them, so the only route was a hand-written API
  // call. Two things needed it. Google's built-in credentials are a shared
  // app that can expire weekly, and OneDrive has NO built-in id at all —
  // Microsoft does not allow a shared public one — so the OneDrive card could
  // be selected but never connect, and the error said to configure it on a
  // screen that did not exist.
  //
  // OneDrive therefore shows these fields open, as its normal setup rather
  // than as a workaround; the others keep them folded away.
  const providerNeedsOwnApp = (id) => id === 'onedrive';
  let showOwnApp = false;
  let ownAppID = '';
  let ownAppSecret = '';
  let ownAppFor = null;

  // Reload the fields when the selected provider changes, so switching cards
  // never shows one provider's credentials under another's name.
  $: if (config && config.provider !== ownAppFor) {
    ownAppFor = config.provider;
    ownAppID = config.customClientIds?.[config.provider] ?? '';
    ownAppSecret = config.customClientSecrets?.[config.provider] ?? '';
    showOwnApp = providerNeedsOwnApp(config.provider) && !ownAppID;
  }

  async function saveOwnApp() {
    const provider = config.provider;
    // Changing the id invalidates any tokens already held: they were issued to
    // the old app and no refresh will be accepted. Disconnecting is the honest
    // outcome — leaving them in place would show "Connected" over credentials
    // that cannot work.
    const changed = (config.customClientIds?.[provider] ?? '') !== ownAppID.trim();
    const wasConnected = connectedProvider === provider;

    busy = true;
    try {
      settings.set(await api.post('/api/settings', {
        cloudSync: {
          customClientIds: { [provider]: ownAppID.trim() },
          customClientSecrets: { [provider]: ownAppSecret.trim() }
        }
      }));
      if (changed && wasConnected) {
        await api.post('/api/auth/disconnect');
        toast($t('cloud.toast.ownAppChanged'), 'success');
      } else {
        toast($t(ownAppID.trim() ? 'cloud.toast.ownAppSaved' : 'cloud.toast.builtInRestored'), 'success');
      }
      // Reload for the fresh tokens/ids, then put the card back. load() takes
      // the provider from the server, which is whatever was last saved with
      // "Save settings" — so without this, saving credentials for a provider
      // you had only selected drops you back onto the stored one, and the
      // section you were filling in disappears as though nothing happened.
      await load();
      config.provider = provider;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function startAuth() {
    busy = true;
    try {
      const res = await api.post('/api/auth/start', { provider: config.provider });
      native.openExternal(res.authUrl);
      authInProgress = true;
      authAuto = !!res.autoCallback;
      showManualCode = !authAuto;
      if (authAuto) {
        toast($t('cloud.toast.finishAuto'));
      } else {
        toast($t('cloud.toast.finishManual'));
      }
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  function cancelAuth() {
    authInProgress = false;
    showManualCode = false;
    authCode = '';
  }

  async function finishAuth() {
    busy = true;
    try {
      const res = await api.post('/api/auth/callback', { code: authCode.trim() });
      toast($t('cloud.toast.connectedAs', { email: res.userEmail }), 'success');
      authInProgress = false;
      showManualCode = false;
      authCode = '';
      await load();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function disconnect() {
    busy = true;
    try {
      await api.post('/api/auth/disconnect');
      toast($t('cloud.toast.disconnected'));
      await load();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function pickLocalFolder() {
    const dir = await native.selectDirectory($t('cloud.folderPicker'));
    if (dir) config.url = dir;
  }

  // A cloud call that fails on expired credentials means the daemon has
  // just wiped the dead tokens — reload so the "connected" badge flips to
  // "sign in" instead of lying about a working connection.
  function handleCloudError(e) {
    toast(e.message, 'error');
    if (/expired|reconnect|not authenticated|re-auth/i.test(e.message)) load();
  }

  async function browseCloud() {
    browsing = true;
    // keep the current listing visible while refreshing so the detail view
    // doesn't flash back to the loading spinner mid-upload
    try {
      cloudGames = await api.get('/api/cloud/browse');
    } catch (e) {
      handleCloudError(e);
    } finally {
      browsing = false;
    }
  }

  function openCloudBrowser() {
    cloudOpen = true;
    cloudFilter = '';
    cloudTab = 'all';
    detailId = null;
    browseCloud();
  }
  const closeCloudBrowser = () => {
    cloudOpen = false;
    detailId = null;
  };

  // One tile per game — tracked games and cloud-only games merged, so the
  // grid can both browse what's up there and upload what isn't yet.
  $: cloudMap = new Map((cloudGames ?? []).map((g) => [g.gameId, g]));
  $: tiles = [
    ...(cloudGames ?? []).map((g) => {
      const local = $gameList.find((x) => x.id === g.gameId);
      return { id: g.gameId, name: g.gameName, coverUrl: local?.coverUrl, tracked: !!local, cloud: g };
    }),
    ...$gameList
      .filter((lg) => !cloudMap.has(lg.id))
      .map((lg) => ({ id: lg.id, name: lg.name, coverUrl: lg.coverUrl, tracked: true, cloud: null }))
  ];
  $: tabCounts = {
    all: tiles.length,
    cloud: tiles.filter((t) => t.cloud).length,
    local: tiles.filter((t) => !t.cloud).length
  };
  $: filteredTiles = tiles.filter(
    (t) =>
      (cloudTab === 'all' || (cloudTab === 'cloud' ? !!t.cloud : !t.cloud)) &&
      t.name.toLowerCase().includes(cloudFilter.trim().toLowerCase())
  );
  $: detailTile = detailId ? tiles.find((t) => t.id === detailId) : null;

  // Delete only removes the remote copy — local snapshots stay put.
  const deleteCloud = async (g, f) => {
    if (
      !(await askConfirm(
        $t('cloud.delete.message', { snapshot: f.snapshotId, game: g.gameName }),
        { title: $t('cloud.delete.title'), confirmText: $t('cloud.delete.confirm'), danger: true }
      ))
    )
      return;
    busy = true;
    try {
      await api.post(`/api/cloud/delete/${g.gameId}`, { fileName: f.name, id: f.id ?? '' });
      g.snapshots = g.snapshots.filter((x) => x.name !== f.name);
      g.count = g.snapshots.length;
      g.totalSize = g.snapshots.reduce((n, x) => n + x.sizeBytes, 0);
      cloudGames = cloudGames.filter((x) => x.count > 0);
      toast($t('cloud.delete.success'), 'success');
    } catch (e) {
      handleCloudError(e);
    } finally {
      busy = false;
    }
  };

  $: cloudTotals = (cloudGames ?? []).reduce(
    (a, g) => ({ snaps: a.snaps + g.count, size: a.size + g.totalSize }),
    { snaps: 0, size: 0 }
  );

  const restoreCloud = async (gameId, file) => {
    if (!(await askConfirm($t('cloud.restore.message', { snapshot: file.snapshotId }), { title: $t('cloud.restore.title'), confirmText: $t('cloud.restore.confirm') }))) return;
    busy = true;
    try {
      await api.post(`/api/cloud/restore/${gameId}`, { fileName: file.name });
      toast($t('cloud.restore.success'), 'success');
    } catch (e) {
      handleCloudError(e);
    } finally {
      busy = false;
    }
  };

  const uploadLocal = async (gameId) => {
    uploading = true;
    try {
      const res = await api.post(`/api/cloud/sync-local/${gameId}`);
      toast(
        res.uploaded === 0 && res.skipped > 0
          ? $t('cloud.upload.alreadyCurrent', { skipped: res.skipped })
          : $t('cloud.upload.summary', { uploaded: res.uploaded, skipped: res.skipped }),
        'success'
      );
      await browseCloud(); // detailTile re-derives from the fresh listing
    } catch (e) {
      handleCloudError(e);
    } finally {
      uploading = false;
      uploadProg = null;
    }
  };

  // ── .sscb export/import ──────────────────────────────────────────
  // Export: a picker modal lists tracked games plus auto-detected saves;
  // the selection's LIVE saves (with their locations) go into the file.
  let exportOpen = false;
  let exportItems = null; // [{id,name,savePath,appId,cover,tracked}], null while loading
  let exportSel = {};
  let exporting = false;

  // Small cover art next to each row — same Steam box-art the auto-scan
  // grid uses, with the tracked game's own coverUrl taking precedence.
  const portraitUrl = (appId) =>
    `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/library_600x900.jpg`;

  const openExportPicker = async () => {
    exportOpen = true;
    exportItems = null;
    exportSel = {};
    try {
      const [games, scan] = await Promise.all([api.get('/api/games'), api.get('/api/presets/scan')]);
      const tracked = Object.values(games ?? {}).map((g) => ({
        id: g.id, name: g.name, savePath: g.savePath, appId: g.appId,
        // Portrait box art first (matches the auto-scan tiles); the stored
        // coverUrl is Steam's landscape header — wrong shape for tiles.
        cover: (g.appId ? portraitUrl(g.appId) : '') || g.coverUrl || '', tracked: true,
      }));
      const knownPaths = new Set(tracked.map((g) => g.savePath.toLowerCase()));
      const knownIds = new Set(tracked.map((g) => g.id));
      const detected = (scan ?? [])
        .filter((d) => !knownPaths.has(d.savePath.toLowerCase()) && !knownIds.has(d.id))
        // A folder holding no files has nothing to back up. `measured` guards
        // the case where the daemon could not read it — unknown is not empty,
        // and dropping one of those would quietly skip a real save.
        .filter((d) => !(d.measured && d.fileCount === 0))
        .map((d) => ({
          id: d.id, name: d.name, savePath: d.savePath, appId: d.appId,
          cover: d.appId ? portraitUrl(d.appId) : '', tracked: false,
        }));
      for (const g of tracked) exportSel[g.id] = true; // tracked pre-selected
      exportItems = [...tracked, ...detected];
    } catch (e) {
      toast(e.message, 'error');
      exportOpen = false;
    }
  };

  const exportSetAll = (value, trackedOnly = false) => {
    for (const it of exportItems ?? []) {
      exportSel[it.id] = trackedOnly ? value && it.tracked : value;
    }
    exportSel = exportSel;
  };

  $: exportCount = exportItems ? exportItems.filter((it) => exportSel[it.id]).length : 0;
  $: allSelected = !!exportItems && exportItems.length > 0 && exportCount === exportItems.length;

  const runExport = async () => {
    const chosen = (exportItems ?? []).filter((it) => exportSel[it.id]);
    if (!chosen.length) return;
    const target = await native.selectSaveFile($t('cloud.export.pickerTitle'), 'opensave-saves.sscb');
    if (!target) return;
    exporting = true;
    try {
      const res = await api.post('/api/backup/export', {
        targetPath: target,
        games: chosen.map(({ id, name, appId, savePath }) => ({ id, name, appId, savePath })),
      });
      const skipped = res.skipped?.length ?? 0;
      toast(
        $t(skipped ? 'cloud.export.successSkipped' : 'cloud.export.success', { exported: res.exported, skipped }),
        skipped ? 'info' : 'success'
      );
      exportOpen = false;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      exporting = false;
    }
  };

  // Import: mode dialog first. "snapshots" (default) never touches live
  // files; "overwrite" restores every save in the file onto disk — the
  // backend takes safety copies of anything it replaces.
  let importOpen = false;
  let importSrc = '';
  let importMode = 'snapshots';
  let importing = false;

  const pickImportFile = async () => {
    const src = await native.selectBackupFile($t('cloud.import.pickerTitle'));
    if (!src) return;
    importSrc = src;
    importMode = 'snapshots';
    importOpen = true;
  };

  // Live per-game progress while the daemon walks the export/import.
  let backupProg = null;
  const unsubBackupProg = backupProgressEvent.subscribe((ev) => {
    backupProg = ev && !ev.complete ? ev : null;
  });
  onDestroy(unsubBackupProg);

  const runImport = async () => {
    importing = true;
    try {
      const res = await api.post('/api/backup/restore', { sourcePath: importSrc, mode: importMode });
      if (res.legacy) {
        toast($t('cloud.import.legacyResult', { imported: res.imported, skipped: res.skipped }), 'success');
      } else {
        const bits = [];
        if (res.restored) bits.push($t('cloud.import.restored', { count: res.restored }));
        if (res.snapshots) bits.push($t('cloud.import.snapshotsAdded', { count: res.snapshots }));
        if (res.skipped) bits.push($t('cloud.import.skipped', { count: res.skipped }));
        toast($t('cloud.import.finished', { result: bits.join($t('cloud.import.separator')) || $t('cloud.import.nothing') }), res.skipped ? 'info' : 'success');
      }
      importOpen = false;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      importing = false;
    }
  };

  $: currentProvider = providers.find((p) => p.id === config?.provider);
  const fmtSize = (n) => (n >= 1048576 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1024).toFixed(1) + ' KB');
</script>

<div class="head">
  <h2 class="page-title">{$t('cloud.title')}</h2>
</div>

{#if !config}
  <p class="quiet">{$t('cloud.loading')}</p>
{:else}
  <div class="card">
    <div class="provider-label" style="margin-top: 0;">{$t('cloud.selectProvider')}</div>
    <div class="provider-grid">
      {#each providers as p}
        <button
          class="provider-card"
          class:active={config.provider === p.id}
          on:click={() => { config.provider = p.id; cloudGames = null; detailId = null; }}
        >
          {#if p.id === connectedProvider}
            <span class="prov-check" title={$t('cloud.status.connected')}>✓</span>
          {/if}
          <div class="provider-icon">
            {#if p.img}
              <img src={p.img} alt={$t(p.labelKey)} />
            {:else}
              <svg viewBox="0 0 24 24" width="34" height="34" fill="currentColor"><path d={iconPaths[p.icon]} /></svg>
            {/if}
          </div>
          <div class="provider-name">{$t(p.labelKey)}</div>
          <div class="provider-status" class:is-connected={p.id === connectedProvider}>
            {providerStatus(p.id, connectedProvider, config)}
          </div>
        </button>
      {/each}
    </div>

    {#if config.provider === 'local'}
      <div class="field">
        <label for="cb-folder">{$t('cloud.fields.destination')}</label>
        <div class="path-row">
          <input id="cb-folder" bind:value={config.url} placeholder="D:\Backups\OpenSave" />
          <button class="btn" on:click={pickLocalFolder}>{$t('cloud.fields.browse')}</button>
        </div>
      </div>
    {:else if config.provider === 'webdav'}
      <div class="field">
        <label for="cb-url">{$t('cloud.fields.webdavUrl')}</label>
        <input id="cb-url" bind:value={config.url} placeholder="https://nas.local/dav/opensave/" />
      </div>
      <div class="two">
        <div class="field">
          <label for="cb-user">{$t('cloud.fields.username')}</label>
          <input id="cb-user" bind:value={config.username} />
        </div>
        <div class="field">
          <label for="cb-pass">{$t('cloud.fields.password')}</label>
          <input id="cb-pass" type="password" bind:value={config.password} />
        </div>
      </div>
    {:else if config.provider === 'webhook'}
      <div class="field">
        <label for="cb-hook">{$t('cloud.fields.webhookUrl')}</label>
        <input id="cb-hook" bind:value={config.url} />
      </div>
      <div class="field">
        <label for="cb-headers">{$t('cloud.fields.headers')}</label>
        <input id="cb-headers" bind:value={config.headers} placeholder={'{"Authorization": "Bearer …"}'} />
      </div>
    {:else}
      <!-- OAuth providers -->
      {#if connectedProvider === config.provider && config.tokens?.userEmail}
        <div class="acct">
          <div class="acct-icon">
            {#if currentProvider?.img}
              <img src={currentProvider.img} alt="" />
            {:else}
              <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d={iconPaths.cloud} /></svg>
            {/if}
          </div>
          <div class="acct-info">
            <div class="acct-title">{$t('cloud.oauth.connectedTo', { provider: $t(currentProvider?.labelKey ?? '') })}</div>
            <div class="acct-sub">
              <span class="acct-dot"></span>
              {isEmail(config.tokens.userEmail)
                ? config.tokens.userEmail
                : $t('cloud.oauth.signedInUploads')}
            </div>
          </div>
          <button class="btn small danger" disabled={busy} on:click={disconnect}>{$t('cloud.oauth.disconnect')}</button>
        </div>
      {:else}
        {#if connectedProvider}
          <p class="quiet" style="margin-bottom: 10px;">
            {$t('cloud.oauth.replacing', { provider: $t(providers.find((x) => x.id === connectedProvider)?.labelKey ?? '') })}
          </p>
        {/if}
        {#if !authInProgress}
          <button class="btn primary" disabled={busy} on:click={startAuth}>
            {$t('cloud.oauth.signInWith', { provider: $t(currentProvider?.labelKey ?? '') })}
          </button>
          {#if config.provider === 'google_drive'}
            <p class="quiet" style="margin-top: 10px;">
              ⚠️ {$t('cloud.oauth.googleConsentBefore')} <strong>{$t('cloud.oauth.googleConsentAction')}</strong>
              {$t('cloud.oauth.googleConsentAfter')}
            </p>
          {/if}
        {:else}
          <div class="auth-waiting">
            <span class="cspin"></span>
            <div class="auth-waiting-text">
              <strong>{$t('cloud.auth.waiting')}</strong>
              <span class="quiet">{$t('cloud.auth.approve')}</span>
            </div>
            <button class="btn small" on:click={cancelAuth}>{$t('cloud.common.cancel')}</button>
          </div>
          {#if !showManualCode && authAuto}
            <button class="linkish" on:click={() => (showManualCode = true)}>
              {$t('cloud.auth.trouble')}
            </button>
          {/if}
          {#if showManualCode}
            <div class="auth-code">
              <p class="quiet">
                {$t('cloud.auth.manualBefore')} <code>code</code> {$t('cloud.auth.manualAfter')}
              </p>
              <div class="path-row">
                <input placeholder="4/0AY0e-g7…" bind:value={authCode} />
                <button class="btn primary" disabled={!authCode || busy} on:click={finishAuth}>{$t('cloud.auth.connect')}</button>
              </div>
            </div>
          {/if}
        {/if}
      {/if}

      <!-- Your own OAuth app. Folded away for the providers that ship with
           working credentials; opened by default for OneDrive, which has none. -->
      <div class="own-app">
        {#if providerNeedsOwnApp(config.provider) && !ownAppID}
          <p class="quiet own-app-required">
            <strong>{$t('cloud.ownApp.oneDriveTitle')}</strong> {$t('cloud.ownApp.oneDriveBefore')}
            <button class="linkish" on:click={() => native.openExternal('https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade')}>{$t('cloud.ownApp.azurePortal')}</button>
            {$t('cloud.ownApp.oneDriveAfter')} <code>http://localhost/callback</code>{$t('cloud.ownApp.oneDriveEnd')}
          </p>
        {:else}
          <button class="linkish" on:click={() => (showOwnApp = !showOwnApp)}>
            {showOwnApp ? '▾' : '▸'} {$t('cloud.ownApp.toggle')}{ownAppID ? $t('cloud.ownApp.inUse') : ''}
          </button>
        {/if}

        {#if showOwnApp || (providerNeedsOwnApp(config.provider) && !ownAppID)}
          <div class="own-app-body">
            {#if config.provider === 'google_drive'}
              <p class="quiet">
                {$t('cloud.ownApp.googleBefore')}
                <button class="linkish" on:click={() => native.openExternal('https://console.cloud.google.com/apis/credentials')}>{$t('cloud.ownApp.googleConsole')}</button>
                {$t('cloud.ownApp.googleAfter')} <code>http://localhost/callback</code>{$t('cloud.ownApp.redirectEnd')}
              </p>
            {:else if config.provider === 'dropbox'}
              <p class="quiet">
                {$t('cloud.ownApp.dropboxHint')}
              </p>
            {/if}
            <div class="field">
              <label for="cb-clientid">{$t('cloud.ownApp.clientId')}</label>
              <input
                id="cb-clientid"
                bind:value={ownAppID}
                spellcheck="false"
                placeholder={config.provider === 'google_drive'
                  ? '…apps.googleusercontent.com'
                  : $t('cloud.ownApp.clientIdPlaceholder')}
              />
            </div>
            <div class="field">
              <label for="cb-clientsecret">{$t('cloud.ownApp.clientSecret')} <span class="quiet">— {$t('cloud.ownApp.secretHint')}</span></label>
              <input id="cb-clientsecret" type="password" bind:value={ownAppSecret} spellcheck="false" />
            </div>
            <div class="path-row">
              <button class="btn primary" disabled={busy} on:click={saveOwnApp}>{$t('cloud.ownApp.saveCredentials')}</button>
              {#if ownAppID}
                <button
                  class="btn small"
                  disabled={busy}
                  title={$t('cloud.ownApp.builtInTitle')}
                  on:click={() => { ownAppID = ''; ownAppSecret = ''; saveOwnApp(); }}
                >
                  {$t('cloud.ownApp.builtInApp')}
                </button>
              {/if}
            </div>
            {#if connectedProvider === config.provider}
              <p class="quiet">
                {$t('cloud.ownApp.alreadySignedIn')}
              </p>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <div class="actions">
      <span class="quiet" style="margin-right: auto;">
        {$t('cloud.automaticHint', { path: $t('cloud.automaticHintPath') })}
      </span>
      <button class="btn primary" disabled={busy} on:click={save}>{$t('cloud.saveSettings')}</button>
    </div>
  </div>

  <h3 class="section">{$t('cloud.joinPreview.section')}</h3>
  <div class="card preview-card">
    <div class="export-row">
      <div>
        <h3>{$t('cloud.joinPreview.title')}</h3>
        <p class="quiet">{$t('cloud.joinPreview.hint')}</p>
      </div>
      <button class="btn" disabled={localPreviewBusy} on:click={inspectLocalSaves}>
        {localPreviewBusy ? $t('cloud.joinPreview.scanning') : $t('cloud.joinPreview.scan')}
      </button>
    </div>
    {#if localPreviewError}
      <p class="preview-warning" role="alert">{$t('cloud.joinPreview.failed')}: {localPreviewError}</p>
    {:else if localPreview && localPreviewSummary}
      <p class="preview-summary">
        {$t('cloud.joinPreview.summary', { tracked: localPreviewSummary.trackedCount, detected: localPreviewSummary.detectedCount })}
      </p>
      {#if !localPreviewSummary.complete}
        <p class="preview-warning" role="status">
          {$t('cloud.joinPreview.incomplete', { count: localPreviewSummary.unknownCount })}
        </p>
      {/if}
      {#if localPreview.library?.games?.length}
        <h4>{$t('cloud.joinPreview.tracked')}</h4>
        <ul class="preview-list">
          {#each localPreview.library.games as game (game.gameId)}
            <li><span>{game.name}</span><span class="quiet">{$t('cloud.joinPreview.files', { count: game.fileCount })}</span></li>
          {/each}
        </ul>
      {/if}
      {#if localPreview.candidates?.length}
        <h4>{$t('cloud.joinPreview.detected')}</h4>
        <p class="quiet">{$t('cloud.joinPreview.detectedHint')}</p>
        <ul class="preview-list">
          {#each localPreview.candidates as candidate (candidate.id)}
            <li>
              <span>{candidate.name}</span>
              <span class="quiet">{candidate.measured ? $t('cloud.joinPreview.files', { count: candidate.fileCount }) : $t('cloud.joinPreview.unknown')}</span>
            </li>
          {/each}
        </ul>
      {/if}
      <p class="quiet">{$t('cloud.joinPreview.remotePending')}</p>
    {/if}
  </div>

  <h3 class="section">{$t('cloud.snapshots.section')}</h3>
  <div class="card export-row">
    <div>
      <h3>{$t('cloud.snapshots.browseTitle')}</h3>
      <p class="quiet">
        {$t('cloud.snapshots.browseHint')}
      </p>
    </div>
    <button class="btn primary" on:click={openCloudBrowser}>☁️ {$t('cloud.snapshots.browse')}</button>
  </div>

  <h3 class="section">{$t('cloud.backupFile.section')}</h3>
  <div class="card export-row">
    <div>
      <h3>{$t('cloud.backupFile.title')}</h3>
      <p class="quiet">
        {$t('cloud.backupFile.hint')}
      </p>
    </div>
    <div class="export-actions">
      <button class="btn" disabled={busy} on:click={pickImportFile}>{$t('cloud.backupFile.import')}</button>
      <button class="btn primary" disabled={busy} on:click={openExportPicker}>{$t('cloud.backupFile.export')}</button>
    </div>
  </div>
{/if}

{#if exportOpen}
  <div
    class="cloud-overlay"
    use:backdropClose={() => (exportOpen = false)}
    on:keydown={(e) => e.key === 'Escape' && (exportOpen = false)}
    role="presentation"
  >
    <div class="cloud-modal">
      <div class="cloud-modal-head">
        <div>
          <h2>📦 {$t('cloud.export.title')}</h2>
          <p class="cloud-modal-sub">
            {#if !exportItems}
              {$t('cloud.export.looking')}
            {:else}
              {$t('cloud.export.summary', { selected: exportCount, total: exportItems.length })}
            {/if}
          </p>
        </div>
        <div class="cloud-head-actions">
          <button class="btn icon" on:click={() => (exportOpen = false)} title={$t('cloud.common.close')}>✕</button>
        </div>
      </div>

      {#if !exportItems}
        <div class="cloud-loading"><span class="cspin"></span> {$t('cloud.export.loading')}</div>
      {:else}
        <div class="export-toolbar">
          <button class="btn small" on:click={() => exportSetAll(!allSelected)}>
            {allSelected ? $t('cloud.export.unselectAll') : $t('cloud.export.selectAll')}
          </button>
          <button class="btn small" on:click={() => { exportSetAll(false); exportSetAll(true, true); }}>{$t('cloud.export.trackedOnly')}</button>
        </div>
        <div class="cloud-modal-list">
          {#if exportItems.length === 0}
            <div class="cloud-empty">
              <div class="cloud-empty-icon">📦</div>
              <p>{$t('cloud.export.empty')}</p>
            </div>
          {:else}
            <div class="export-grid">
              {#each exportItems as it (it.id)}
                <div
                  class="cover-tile"
                  class:sel={exportSel[it.id]}
                  on:click={() => { exportSel[it.id] = !exportSel[it.id]; }}
                  on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), (exportSel[it.id] = !exportSel[it.id]))}
                  role="button"
                  tabindex="0"
                  title={it.savePath}
                >
                  <div class="cover-art">
                    {#if it.cover}
                      <img
                        src={it.cover}
                        alt={it.name}
                        loading="lazy"
                        on:error={(e) => (e.currentTarget.style.display = 'none')}
                      />
                    {/if}
                    <div class="cover-fallback">
                      <span class="cover-emoji">🎮</span>
                      <span class="cover-fallback-name">{it.name}</span>
                    </div>
                    {#if exportSel[it.id]}
                      <div class="cover-check">✓</div>
                    {/if}
                    <span class="cover-type">{it.tracked ? $t('cloud.export.tracked') : $t('cloud.export.detected')}</span>
                  </div>
                  <div class="cover-name" title={it.name}>{it.name}</div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
        {#if exporting && backupProg}
          <div class="upload-progress">
            <div class="upload-progress-text">
              <span class="cspin"></span>
              {$t('cloud.export.progress', { current: Math.min(backupProg.done + 1, backupProg.total), total: backupProg.total })}
              {#if backupProg.current}&nbsp;— <code>{backupProg.current}</code>{/if}
            </div>
            <div class="upload-bar">
              <div class="upload-bar-fill" style="width: {backupProg.total > 0 ? Math.round((backupProg.done / backupProg.total) * 100) : 8}%"></div>
            </div>
          </div>
        {/if}
        <div class="cloud-modal-foot">
          <span class="quiet">{$t('cloud.export.untrackedHint')}</span>
          <div class="export-foot-actions">
            <button class="btn" on:click={() => (exportOpen = false)}>{$t('cloud.common.cancel')}</button>
            <button class="btn primary" disabled={exporting || exportCount === 0} on:click={runExport}>
              {exporting ? $t('cloud.export.exporting') : $t('cloud.export.exportCount', { count: exportCount })}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if importOpen}
  <div
    class="cloud-overlay"
    use:backdropClose={() => !importing && (importOpen = false)}
    on:keydown={(e) => e.key === 'Escape' && !importing && (importOpen = false)}
    role="presentation"
  >
    <div class="cloud-modal import-modal">
      <div class="cloud-modal-head">
        <div>
          <h2>📥 {$t('cloud.import.title')}</h2>
          <p class="cloud-modal-sub"><code>{importSrc}</code></p>
        </div>
        <div class="cloud-head-actions">
          <button class="btn icon" disabled={importing} on:click={() => (importOpen = false)} title={$t('cloud.common.close')}>✕</button>
        </div>
      </div>

      <div class="import-modes">
        <label class="mode-option" class:selected={importMode === 'snapshots'}>
          <input type="radio" bind:group={importMode} value="snapshots" />
          <div>
            <div class="mode-title">{$t('cloud.import.addToSnapshots')} <span class="badge online">{$t('cloud.import.recommended')}</span></div>
            <p class="quiet">
              {$t('cloud.import.snapshotsHint')}
            </p>
          </div>
        </label>
        <label class="mode-option danger-option" class:selected={importMode === 'overwrite'}>
          <input type="radio" bind:group={importMode} value="overwrite" />
          <div>
            <div class="mode-title">{$t('cloud.import.overwrite')}</div>
            <p class="quiet">
              {$t('cloud.import.overwriteHint')}
            </p>
          </div>
        </label>
      </div>

      {#if importing && backupProg}
        <div class="upload-progress">
          <div class="upload-progress-text">
            <span class="cspin"></span>
            {$t('cloud.import.progress', { current: Math.min(backupProg.done + 1, backupProg.total), total: backupProg.total })}
            {#if backupProg.current}&nbsp;— <code>{backupProg.current}</code>{/if}
          </div>
          <div class="upload-bar">
            <div class="upload-bar-fill" style="width: {backupProg.total > 0 ? Math.round((backupProg.done / backupProg.total) * 100) : 8}%"></div>
          </div>
        </div>
      {/if}
      <div class="cloud-modal-foot">
        <span class="quiet">{$t('cloud.import.safetyHint')}</span>
        <div class="export-foot-actions">
          <button class="btn" disabled={importing} on:click={() => (importOpen = false)}>{$t('cloud.common.cancel')}</button>
          <button
            class="btn {importMode === 'overwrite' ? 'danger' : 'primary'}"
            disabled={importing}
            on:click={runImport}
          >
            {importing ? $t('cloud.import.importing') : importMode === 'overwrite' ? $t('cloud.import.overwriteAction') : $t('cloud.import.addToSnapshots')}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if cloudOpen}
  <div
    class="cloud-overlay"
    use:backdropClose={closeCloudBrowser}
    on:keydown={(e) => e.key === 'Escape' && closeCloudBrowser()}
    role="presentation"
  >
    <div class="cloud-modal">
      <div class="cloud-modal-head">
        <div>
          <h2>☁️ {$t('cloud.browser.title')}</h2>
          <p class="cloud-modal-sub">
            {#if browsing}
              {$t('cloud.browser.reading')}
            {:else if cloudGames}
              {$t('cloud.browser.summary', { games: cloudGames.length, snapshots: cloudTotals.snaps, size: fmtSize(cloudTotals.size) })}
            {:else}
              {$t('cloud.browser.readFailed')}
            {/if}
          </p>
        </div>
        <div class="cloud-head-actions">
          <button class="btn small" disabled={browsing} on:click={browseCloud}>
            {browsing ? $t('cloud.loading') : $t('cloud.browser.refresh')}
          </button>
          <button class="btn icon" on:click={closeCloudBrowser} title={$t('cloud.common.close')}>✕</button>
        </div>
      </div>

      {#if browsing && !cloudGames}
        <div class="cloud-loading"><span class="cspin"></span> {$t('cloud.browser.listing')}</div>
      {:else if cloudGames && detailTile}
        <!-- drill-in: one game's cloud snapshots -->
        <div class="detail-head">
          <button class="btn small" on:click={() => (detailId = null)}>← {$t('cloud.browser.back')}</button>
          <div class="detail-title">
            <strong>{detailTile.name}</strong>
            <span class="quiet">
              {#if detailTile.cloud}
                {$t('cloud.browser.gameSummary', { snapshots: detailTile.cloud.count, size: fmtSize(detailTile.cloud.totalSize) })}
              {:else}
                {$t('cloud.browser.nothingYet')}
              {/if}
            </span>
          </div>
          {#if detailTile.tracked}
            <button class="btn small" disabled={uploading} on:click={() => uploadLocal(detailTile.id)}>
              {uploading ? $t('cloud.browser.uploading') : `⬆ ${$t('cloud.browser.uploadLocal')}`}
            </button>
          {/if}
        </div>
        {#if uploading}
          <div class="upload-progress">
            <div class="upload-progress-text">
              <span class="cspin"></span>
              {#if uploadProg && uploadProg.total > 0}
                {$t('cloud.browser.uploadProgress', { current: Math.min(uploadProg.done + 1, uploadProg.total), total: uploadProg.total })}
                {#if uploadProg.current}&nbsp;— <code>{uploadProg.current}</code>{/if}
              {:else}
                {$t('cloud.browser.checkingUpload')}
              {/if}
            </div>
            <div class="upload-bar">
              <div
                class="upload-bar-fill"
                style="width: {uploadProg && uploadProg.total > 0 ? Math.round((uploadProg.done / uploadProg.total) * 100) : 8}%"
              ></div>
            </div>
          </div>
        {/if}
        <div class="cloud-modal-list">
          {#if detailTile.cloud}
            {#each detailTile.cloud.snapshots as f (f.name)}
              <div class="cloud-row">
                <div class="cloud-info">
                  <div class="cloud-name">{f.snapshotId} <span class="badge offline">{f.branch}</span></div>
                  <div class="cloud-meta">{fmtSize(f.sizeBytes)} · {new Date(f.createdTime).toLocaleString()}</div>
                </div>
                <button
                  class="btn small primary"
                  disabled={busy || !detailTile.tracked}
                  title={detailTile.tracked ? $t('cloud.browser.restoreTitle') : $t('cloud.browser.trackFirst')}
                  on:click={() => restoreCloud(detailTile.id, f)}
                >
                  {$t('cloud.restore.confirm')}
                </button>
                <button
                  class="btn small danger"
                  disabled={busy}
                  title={$t('cloud.browser.deleteTitle')}
                  on:click={() => deleteCloud(detailTile.cloud, f)}
                >
                  {$t('cloud.delete.confirm')}
                </button>
              </div>
            {/each}
          {:else}
            <div class="cloud-empty">
              <div class="cloud-empty-icon">☁️</div>
              <p>{$t('cloud.browser.noGameSnapshots')}</p>
              <p class="quiet">{$t('cloud.browser.useUploadBefore')} <strong>{$t('cloud.browser.uploadLocal')}</strong> {$t('cloud.browser.useUploadAfter')}</p>
            </div>
          {/if}
        </div>
        <div class="cloud-modal-foot">
          <span class="quiet">{$t('cloud.browser.deleteHint')}</span>
          <button class="btn" on:click={closeCloudBrowser}>{$t('cloud.common.close')}</button>
        </div>
      {:else if cloudGames}
        <!-- tile grid -->
        <div class="cloud-toolbar">
          <input class="cloud-search" placeholder={$t('cloud.browser.filter')} bind:value={cloudFilter} />
          <div class="cloud-tabs">
            {#each [['all', 'cloud.browser.tabs.all'], ['cloud', 'cloud.browser.tabs.inCloud'], ['local', 'cloud.browser.tabs.notUploaded']] as [id, labelKey]}
              <button class:active={cloudTab === id} on:click={() => (cloudTab = id)}>
                {$t(labelKey)} <span class="count">{tabCounts[id]}</span>
              </button>
            {/each}
          </div>
        </div>

        <div class="cloud-modal-list">
          <div class="cloud-grid">
            {#each filteredTiles as tile (tile.id)}
              <div
                class="cover-tile"
                on:click={() => (detailId = tile.id)}
                on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), (detailId = tile.id))}
                role="button"
                tabindex="0"
                title={tile.name}
              >
                <div class="cover-art">
                  {#if tile.coverUrl}
                    <img src={tile.coverUrl} alt={tile.name} loading="lazy" on:error={(e) => (e.currentTarget.style.display = 'none')} />
                  {/if}
                  <div class="cover-fallback">
                    <span class="cover-emoji">{tile.cloud ? '☁️' : '💾'}</span>
                    <span class="cover-fallback-name">{tile.name}</span>
                  </div>
                  {#if tile.cloud}
                    <span class="cover-type in-cloud">☁ {tile.cloud.count}</span>
                  {:else}
                    <span class="cover-type">{$t('cloud.browser.localOnly')}</span>
                  {/if}
                  <div class="cover-hover">
                    <button class="btn small primary" on:click|stopPropagation={() => (detailId = tile.id)}>
                      {tile.cloud ? $t('cloud.browser.browse') : $t('cloud.browser.upload')}
                    </button>
                  </div>
                </div>
                <div class="cover-name">{tile.name}</div>
              </div>
            {:else}
              <div class="cloud-grid-empty">
                {tiles.length === 0
                  ? $t('cloud.browser.empty')
                  : $t('cloud.browser.noMatches')}
              </div>
            {/each}
          </div>
        </div>

        <div class="cloud-modal-foot">
          <span class="quiet">{$t('cloud.browser.help')}</span>
          <button class="btn" on:click={closeCloudBrowser}>{$t('cloud.common.close')}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .preview-card { padding: 18px 20px; }
  .preview-card .export-row { padding: 0; }
  .preview-card h4 { margin: 16px 0 6px; }
  .preview-summary { margin: 14px 0 0; }
  .preview-warning { color: var(--danger, #e5484d); margin: 12px 0; }
  .preview-list { list-style: none; padding: 0; margin: 0 0 12px; max-height: 240px; overflow: auto; }
  .preview-list li { display: flex; justify-content: space-between; gap: 14px; padding: 7px 0; border-bottom: 1px solid var(--border); }
  .head {
    margin-bottom: 20px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.85rem;
  }
  .upload-progress {
    padding: 12px 22px;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .upload-progress-text {
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 0.84rem;
    color: var(--text-dim);
  }
  .upload-progress-text code {
    background: var(--bg);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.78rem;
  }
  .upload-bar {
    height: 6px;
    border-radius: 999px;
    background: var(--bg-active);
    overflow: hidden;
  }
  .upload-bar-fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 0.3s ease;
  }
  .provider-label {
    display: block;
    font-size: 0.85rem;
    color: var(--text-dim);
    font-weight: 600;
    margin: 18px 0 10px;
  }
  .own-app {
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
  }
  .own-app-required {
    margin: 0 0 10px;
  }
  .own-app-body {
    margin-top: 10px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .provider-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 10px;
    margin-bottom: 18px;
  }
  .provider-card {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 7px;
    padding: 16px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    color: var(--text-dim);
    cursor: pointer;
    transition: border-color 0.12s, background 0.12s, transform 0.12s;
    text-align: center;
  }
  .provider-card:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .provider-card.active {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .provider-icon {
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .provider-icon img {
    width: 34px;
    height: 34px;
    object-fit: contain;
  }
  .provider-name {
    font-weight: 600;
    font-size: 0.9rem;
    color: var(--text);
  }
  .provider-status {
    font-size: 0.72rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 130px;
  }
  .provider-card.active .provider-status {
    color: var(--accent);
  }
  .provider-status.is-connected,
  .provider-card.active .provider-status.is-connected {
    color: var(--success);
    font-weight: 600;
  }
  .prov-check {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--success);
    color: #0c0c0d;
    font-size: 0.72rem;
    font-weight: 800;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 12px;
    margin-top: 8px;
  }
  .acct {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 16px 18px;
    background: rgba(74, 222, 128, 0.05);
    border: 1px solid rgba(74, 222, 128, 0.3);
    border-radius: var(--radius-lg);
  }
  .acct-icon {
    width: 44px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 11px;
    color: var(--text-dim);
    flex-shrink: 0;
  }
  .acct-icon img {
    width: 26px;
    height: 26px;
    object-fit: contain;
  }
  .acct-info {
    flex: 1;
    min-width: 0;
  }
  .acct-title {
    font-weight: 600;
    font-size: 0.98rem;
  }
  .acct-sub {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--text-dim);
    font-size: 0.82rem;
    margin-top: 3px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .acct-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--success);
    box-shadow: 0 0 6px rgba(74, 222, 128, 0.6);
    flex-shrink: 0;
  }
  .auth-code {
    margin-top: 12px;
  }
  .auth-waiting {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
    background: var(--accent-soft);
    border: 1px solid var(--accent);
    border-radius: var(--radius);
  }
  .auth-waiting-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.88rem;
  }
  .cspin {
    width: 16px;
    height: 16px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: cspin 0.8s linear infinite;
    flex-shrink: 0;
  }
  @keyframes cspin {
    to { transform: rotate(360deg); }
  }
  .linkish {
    margin-top: 10px;
    border: none;
    background: transparent;
    color: var(--text-faint);
    font-size: 0.8rem;
    cursor: pointer;
    text-decoration: underline;
    padding: 2px 0;
  }
  .linkish:hover {
    color: var(--text-dim);
  }
  .auth-code code {
    background: var(--bg);
    padding: 1px 5px;
    border-radius: 4px;
  }
  .section {
    margin: 22px 0 10px;
  }
  /* Cloud snapshot browser overlay (same pattern as the auto-scan modal) */
  .cloud-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.62);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 80;
    padding: 32px;
  }
  .cloud-modal {
    width: min(760px, 100%);
    height: min(78vh, 720px);
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  }
  .cloud-modal-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 20px 22px 14px;
    border-bottom: 1px solid var(--border);
  }
  .cloud-modal-head h2 {
    font-size: 1.2rem;
  }
  .cloud-modal-sub {
    font-size: 0.84rem;
    color: var(--text-faint);
    margin-top: 3px;
  }
  .cloud-head-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .cloud-loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: var(--text-dim);
  }
  .cloud-empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    color: var(--text-dim);
    text-align: center;
    padding: 20px;
  }
  .cloud-empty-icon {
    font-size: 2.2rem;
    opacity: 0.6;
  }
  .cloud-toolbar {
    display: flex;
    gap: 12px;
    padding: 14px 22px 10px;
    align-items: center;
    flex-wrap: wrap;
  }
  .cloud-search {
    flex: 1;
    min-width: 200px;
    padding: 9px 13px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .cloud-tabs {
    display: flex;
    gap: 6px;
  }
  .cloud-tabs button {
    padding: 7px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-dim);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .cloud-tabs button:hover {
    background: var(--bg-hover);
  }
  .cloud-tabs button.active {
    background: var(--bg-active);
    color: var(--text);
    border-color: var(--border-strong);
  }
  .cloud-tabs .count {
    color: var(--text-faint);
    font-size: 0.75rem;
    margin-left: 2px;
  }
  /* Cover-art tile grid (same look as the auto-scan modal) */
  .cloud-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 16px;
  }
  .cloud-grid-empty {
    grid-column: 1 / -1;
    text-align: center;
    color: var(--text-faint);
    padding: 50px 20px;
  }
  .cover-tile {
    cursor: pointer;
    outline: none;
  }
  .cover-art {
    position: relative;
    aspect-ratio: 600 / 900;
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg-active);
    border: 2px solid transparent;
    transition: transform 0.12s, border-color 0.12s, box-shadow 0.12s;
  }
  .cover-tile:hover .cover-art {
    transform: translateY(-2px);
    box-shadow: 0 8px 22px rgba(0, 0, 0, 0.45);
  }
  .cover-art img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 2;
  }
  .cover-fallback {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 14px;
    text-align: center;
    background: linear-gradient(160deg, rgba(138, 99, 244, 0.22), rgba(138, 99, 244, 0.04));
  }
  .cover-emoji {
    font-size: 2.2rem;
  }
  .cover-fallback-name {
    font-weight: 700;
    font-size: 0.9rem;
    color: var(--text);
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .cover-type {
    position: absolute;
    top: 8px;
    right: 8px;
    z-index: 3;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.68rem;
    font-weight: 600;
    background: rgba(0, 0, 0, 0.6);
    color: var(--text-dim);
    backdrop-filter: blur(2px);
  }
  .cover-type.in-cloud {
    color: var(--accent);
  }
  .cover-hover {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    padding: 12px;
    opacity: 0;
    background: linear-gradient(to top, rgba(0, 0, 0, 0.75), transparent 55%);
    transition: opacity 0.12s;
  }
  .cover-tile:hover .cover-hover {
    opacity: 1;
  }
  .cover-name {
    margin-top: 7px;
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--text-dim);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* Drill-in detail view */
  .detail-head {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 22px;
    border-bottom: 1px solid var(--border);
  }
  .detail-title {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .detail-title strong {
    font-size: 0.98rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cloud-modal-list {
    flex: 1;
    overflow-y: auto;
    padding: 4px 22px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .cloud-modal-foot {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 14px 22px;
    border-top: 1px solid var(--border);
  }
  .cloud-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .cloud-info {
    flex: 1;
  }
  .cloud-name {
    font-weight: 600;
    font-size: 0.9rem;
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .cloud-meta {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .export-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
  }
  .export-actions {
    display: flex;
    gap: 8px;
  }

  /* Export picker + import mode dialog */
  .export-toolbar {
    display: flex;
    gap: 8px;
    padding: 10px 20px 0;
  }
  /* Cover-tile grid — same look as the auto-scan modal. */
  .export-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 16px;
    padding: 4px;
  }
  .cover-tile {
    cursor: pointer;
  }
  .cover-art {
    position: relative;
    aspect-ratio: 600 / 900;
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg-active);
    border: 2px solid transparent;
    transition: transform 0.12s, border-color 0.12s, box-shadow 0.12s;
  }
  .cover-tile:hover .cover-art {
    transform: translateY(-2px);
    box-shadow: 0 8px 22px rgba(0, 0, 0, 0.45);
  }
  .cover-tile.sel .cover-art {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  .cover-art img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 2;
  }
  .cover-fallback {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 14px;
    text-align: center;
    background: linear-gradient(160deg, rgba(138, 99, 244, 0.22), rgba(138, 99, 244, 0.04));
  }
  .cover-emoji {
    font-size: 2.2rem;
  }
  .cover-fallback-name {
    font-weight: 700;
    font-size: 0.9rem;
    color: var(--text);
    line-height: 1.25;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .cover-check {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 3;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--accent);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    font-weight: 700;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
  .cover-type {
    position: absolute;
    top: 8px;
    right: 8px;
    z-index: 3;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.68rem;
    font-weight: 600;
    background: rgba(0, 0, 0, 0.6);
    color: var(--text-dim);
    backdrop-filter: blur(2px);
  }
  .cover-name {
    margin-top: 7px;
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--text-dim);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cover-tile.sel .cover-name {
    color: var(--text);
  }
  .export-foot-actions {
    display: flex;
    gap: 8px;
  }
  .import-modal {
    max-width: 560px;
    /* Two radio cards + a footer don't need the full browser-modal
       height — size to content so the dialog doesn't look hollow. */
    height: auto;
    max-height: min(78vh, 720px);
  }
  .import-modes {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px 20px;
  }
  .mode-option {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 14px;
    border: 1px solid var(--border);
    border-radius: 10px;
    cursor: pointer;
  }
  .mode-option input {
    margin-top: 3px;
  }
  .mode-option.selected {
    border-color: var(--accent);
    background: var(--bg-hover, rgba(128, 128, 128, 0.06));
  }
  .mode-option.danger-option.selected {
    border-color: var(--danger, #e5484d);
  }
  .mode-title {
    font-weight: 600;
    margin-bottom: 4px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .mode-option p {
    margin: 0;
    font-size: 0.82rem;
  }
</style>
