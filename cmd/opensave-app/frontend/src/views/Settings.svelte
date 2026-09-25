<script>
  import { onMount } from 'svelte';
  import { settings, toast, askConfirm, gameList, navigate, pairingRequests } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { locale, t } from '../lib/i18n.js';

  export let params = {};
  $: params;

  let tab = 'general';
  let draft = null;
  let busy = false;
  let pruning = false;

  // The running build, so the updates toggle can explain what it means for
  // THIS install: someone already on a beta is offered betas regardless, and
  // saying so is the difference between the checkbox looking broken and
  // looking deliberate.
  let appVersion = '';
  $: onPreRelease = appVersion.includes('-');
  onMount(async () => {
    try {
      appVersion = (await native.appInfo())?.version ?? '';
    } catch {
      appVersion = '';
    }
  });

  async function cleanUpSnapshots() {
    pruning = true;
    try {
      // Save the limit first so the cleanup uses it, then prune everything.
      await api.post('/api/settings', draft);
      const res = await api.post('/api/snapshots/prune', { applyDefaultToAll: true });
      const mb = (res.freedBytes / 1048576).toFixed(1);
      toast(
        res.removed > 0
          ? $t('settings.cleanup.removed', { count: res.removed, size: mb })
          : $t('settings.cleanup.none'),
        'success'
      );
    } catch (e) {
      toast($t('settings.error.cleanup'), 'error');
    } finally {
      pruning = false;
    }
  }

  $: if ($settings && !draft) {
    draft = structuredClone($settings);
    // older daemons may omit cloudSync from the settings payload
    draft.cloudSync ??= {
      enabled: true, provider: 'local', url: '', username: '', password: '', headers: '{}', folderId: ''
    };
    // Older daemons predate the separate manual-snapshot budget; 0 is the
    // "keep forever" default, so an omitted value behaves as it should.
    draft.defaultMaxManualSnapshots ??= 0;
    // Older daemons predate the update channel; stable is the default.
    draft.updateChannel ??= 'stable';
  }

  async function save() {
    busy = true;
    try {
      const updated = await api.post('/api/settings', draft);
      settings.set(updated);
      draft = structuredClone(updated);
      toast($t('settings.saved'), 'success');
    } catch (e) {
      toast($t('settings.error.save'), 'error');
    } finally {
      busy = false;
    }
  }

  // Path translations editor
  function addRule() {
    draft.pathTranslations = [...(draft.pathTranslations ?? []), { fromPattern: '', toPattern: '' }];
  }
  function removeRule(i) {
    draft.pathTranslations = draft.pathTranslations.filter((_, idx) => idx !== i);
  }

  // Custom scan paths
  async function addScanPath() {
    const dir = await native.selectDirectory($t('settings.scanner.addPicker'));
    if (dir) draft.customScanPaths = [...(draft.customScanPaths ?? []), dir];
  }
  function removeScanPath(i) {
    draft.customScanPaths = draft.customScanPaths.filter((_, idx) => idx !== i);
  }

  // Excluded folders — locations the auto-scan should skip entirely.
  async function addExcludePath() {
    const dir = await native.selectDirectory($t('settings.scanner.excludePicker'));
    if (dir) draft.excludePaths = [...(draft.excludePaths ?? []), dir];
  }
  function removeExcludePath(i) {
    draft.excludePaths = draft.excludePaths.filter((_, idx) => idx !== i);
  }

  // Reset tracking — untrack every game so the user can re-add them from the
  // right locations (e.g. after moving games between launchers or drives).
  // Non-destructive: this only clears the tracked list; snapshot backups on
  // disk are kept.
  let resetting = false;
  async function resetTracking() {
    const n = $gameList.length;
    if (n === 0) return;
    const ok = await askConfirm(
      $t('settings.reset.confirmMessage', { count: n }),
      { title: $t('settings.reset.confirmTitle'), confirmText: $t('settings.reset.confirmAction', { count: n }), danger: true }
    );
    if (!ok) return;
    resetting = true;
    try {
      const res = await api.post('/api/games/untrack-bulk', { all: true });
      toast($t('settings.reset.success', { count: res.untracked }), 'success');
    } catch (e) {
      toast($t('settings.error.reset'), 'error');
    } finally {
      resetting = false;
    }
  }

  async function pickBackupsDir() {
    const dir = await native.selectDirectory($t('settings.storage.backupsPicker'));
    if (dir) draft.backupsDir = dir;
  }

  async function pickSyncBackupsDir() {
    const dir = await native.selectDirectory($t('settings.storage.safetyPicker'));
    if (dir) draft.syncBackupsDir = dir;
  }

  // Relay hosting: LAN IPs / public IP to share with friends. Shown only on
  // request — these identify the machine, so they shouldn't appear on screen
  // just because the checkbox was ticked.
  let relayInfo = null;
  let relayInfoShown = false;
  let relayInfoLoading = false;

  async function toggleRelayInfo() {
    if (relayInfoShown) {
      relayInfoShown = false;
      return;
    }
    relayInfoLoading = true;
    try {
      // Re-fetch each time: the public IP changes, and a stale one sent to a
      // friend is worse than none.
      relayInfo = await api.get('/api/relay/ips');
      relayInfoShown = true;
    } catch (e) {
      toast($t('settings.error.relayInfo'), 'error');
    } finally {
      relayInfoLoading = false;
    }
  }

  // Turning hosting off should also hide addresses left on screen.
  $: if (draft && !draft.hostRelay && relayInfoShown) relayInfoShown = false;
</script>

<div class="head">
  <h2 class="page-title">{$t('settings.title')}</h2>
</div>

{#if !draft}
  <p class="quiet">{$t('settings.loading')}</p>
{:else}
  <div class="pill-tabs" style="margin-bottom: 18px;">
    <button class:active={tab === 'general'} on:click={() => (tab = 'general')}>{$t('settings.tabs.general')}</button>
    <button class:active={tab === 'sync'} on:click={() => (tab = 'sync')}>{$t('settings.tabs.sync')}</button>
    <button class:active={tab === 'storage'} on:click={() => (tab = 'storage')}>{$t('settings.tabs.storage')}</button>
    <button class:active={tab === 'advanced'} on:click={() => (tab = 'advanced')}>{$t('settings.tabs.advanced')}</button>
  </div>

  {#if tab === 'general'}
    <div class="card">
      <h3 class="section-title">🌐 {$t('settings.language.section')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-language">{$t('settings.language.label')}</label>
        <select id="s-language" bind:value={$locale}>
          <option value="en">{$t('settings.language.en')}</option>
          <option value="zh-CN">{$t('settings.language.zhCN')}</option>
        </select>
        <span class="hint">{$t('settings.language.hint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🖥️ {$t('settings.device.section')}</h3>
      <div class="field">
        <label for="s-name">{$t('settings.device.name')}</label>
        <input id="s-name" bind:value={draft.deviceName} />
      </div>
      <div class="field">
        <label for="s-type">{$t('settings.device.type')}</label>
        <select id="s-type" bind:value={draft.deviceType}>
          <option value="desktop">{$t('settings.device.types.desktop')}</option>
          <option value="deck">{$t('settings.device.types.deck')}</option>
          <option value="handheld">{$t('settings.device.types.handheld')}</option>
          <option value="mobile">{$t('settings.device.types.mobile')}</option>
        </select>
        <span class="hint">{$t('settings.device.typeHint')}</span>
      </div>
      <div class="field">
        <label for="s-node">{$t('settings.device.id')}</label>
        <input id="s-node" value={draft.nodeId ?? ''} readonly class="mono" />
        <span class="hint">{$t('settings.device.idHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🚀 {$t('settings.startup.section')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.startOnBoot} />
        {$t('settings.startup.enable')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {$t('settings.startup.hint')}
      </p>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🧪 {$t('settings.updates.section')}</h3>
      <label class="check">
        <input
          type="checkbox"
          checked={draft.updateChannel === 'beta'}
          on:change={(e) => (draft.updateChannel = e.currentTarget.checked ? 'beta' : 'stable')}
        />
        {$t('settings.updates.beta')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {#if onPreRelease}
          {$t('settings.updates.prereleaseBefore')} <strong>{appVersion}</strong>{$t('settings.updates.prereleaseAfter')}
        {:else}
          {$t('settings.updates.hint')}
        {/if}
      </p>
    </div>

  {:else if tab === 'sync'}
    <div class="card">
      <h3 class="section-title">🔄 {$t('settings.sync.section')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoSyncOnTrack} />
        {$t('settings.sync.onTrack')}
      </label>
      <label class="check" style="margin-top: 18px;">
        <input type="checkbox" bind:checked={draft.matchByAppId} />
        {$t('settings.sync.matchAppId')}
      </label>
      <span class="hint" style="margin-top: 6px;">{$t('settings.sync.matchAppIdHint')}</span>
      <div class="field" style="margin-top: 14px;">
        <label for="s-limit">{$t('settings.sync.bandwidth')}</label>
        <select id="s-limit" bind:value={draft.speedLimit}>
          <option value={0}>{$t('settings.sync.speeds.unlimited')}</option>
          <option value={100}>{$t('settings.sync.speeds.veryLow')}</option>
          <option value={500}>{$t('settings.sync.speeds.medium')}</option>
          <option value={1024}>{$t('settings.sync.speeds.high')}</option>
          <option value={5120}>{$t('settings.sync.speeds.veryHigh')}</option>
          <option value={10240}>{$t('settings.sync.speeds.ultra')}</option>
        </select>
        <span class="hint">{$t('settings.sync.bandwidthHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🌐 {$t('settings.relay.section')}</h3>
      <div class="field">
        <label for="s-relay-url">{$t('settings.relay.url')}</label>
        <input id="s-relay-url" bind:value={draft.relayUrl} placeholder="wss://relay.opensave.org" />
        <span class="hint">{$t('settings.relay.urlHint')}</span>
      </div>
      <label class="check">
        <input type="checkbox" bind:checked={draft.hostRelay} />
        {$t('settings.relay.host')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {$t('settings.relay.hostHint')}
      </p>
      {#if draft.hostRelay}
        <div class="field" style="margin-top: 12px;">
          <label for="s-relay-port">{$t('settings.relay.port')}</label>
          <input id="s-relay-port" type="number" bind:value={draft.relayPort} />
          <span class="hint">{$t('settings.relay.portHint')}</span>
        </div>
        <button class="btn small" on:click={toggleRelayInfo} disabled={relayInfoLoading}>
          {#if relayInfoLoading}
            {$t('settings.relay.lookingUp')}
          {:else if relayInfoShown}
            {$t('settings.relay.hideAddresses')}
          {:else}
            {$t('settings.relay.showAddresses')}
          {/if}
        </button>
        {#if relayInfoShown && relayInfo}
          <div class="share-banner">
            <div class="share-title">📡 {$t('settings.relay.shareTitle')}</div>
            <div class="share-row"><span>{$t('settings.relay.lanIps')}</span> {relayInfo.lanIps?.join(', ') || '—'}</div>
            <div class="share-row"><span>{$t('settings.relay.publicIp')}</span> {relayInfo.publicIp || $t('settings.relay.unavailable')}</div>
            <div class="share-row"><span>{$t('settings.relay.port')}</span> {relayInfo.relayPort}</div>
          </div>
        {/if}
      {/if}
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">☁️ {$t('settings.cloud.section')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.cloudSync.enabled} />
        {$t('settings.cloud.autoMirror')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {$t('settings.cloud.autoMirrorHint')}
      </p>
      <div class="field" style="margin-top: 14px;">
        <label for="s-driveid">{$t('settings.cloud.driveFolderId')}</label>
        <input id="s-driveid" bind:value={draft.cloudSync.folderId} placeholder={$t('settings.cloud.driveFolderPlaceholder')} />
        <span class="hint">
          {$t('settings.cloud.driveFolderHint')}
        </span>
      </div>
      <!-- The client-ID inputs that used to sit here have moved to Cloud
           Backup, beside the provider they belong to. Two screens writing one
           setting is a way to be told two different things; and the version
           there can also take the client SECRET, warns that changing an id
           signs you out, and gives OneDrive the portal link it needs — none of
           which fitted a row of three bare boxes. -->
      <div class="field" style="margin-bottom: 0;">
        <label for="s-oauth-moved">{$t('settings.cloud.ownApp')}</label>
        <span class="hint" id="s-oauth-moved">
          {$t('settings.cloud.ownAppHint')}
        </span>
      </div>
    </div>
  {:else if tab === 'storage'}
    <div class="card">
      <h3 class="section-title">🗄️ {$t('settings.storage.section')}</h3>
      <div class="field">
        <label for="s-backups">{$t('settings.storage.backups')}</label>
        <div class="path-row">
          <input id="s-backups" bind:value={draft.backupsDir} />
          <button class="btn" on:click={pickBackupsDir}>{$t('settings.browse')}</button>
        </div>
        <span class="hint">{$t('settings.storage.backupsHint')}</span>
      </div>
      <div class="field">
        <label for="s-sync-backups">{$t('settings.storage.safety')}</label>
        <div class="path-row">
          <input id="s-sync-backups" bind:value={draft.syncBackupsDir} placeholder={$t('settings.storage.safetyPlaceholder')} />
          <button class="btn" on:click={pickSyncBackupsDir}>{$t('settings.browse')}</button>
        </div>
        <span class="hint">{$t('settings.storage.safetyHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🧹 {$t('settings.retention.section')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoDeleteBackups} />
        {$t('settings.retention.autoDelete')}
      </label>
      {#if draft.autoDeleteBackups}
        <div class="field" style="margin-top: 10px;">
          <label for="s-days">{$t('settings.retention.period')}</label>
          <select id="s-days" bind:value={draft.autoDeleteDays}>
            <option value={7}>{$t('settings.retention.days', { count: 7 })}</option>
            <option value={14}>{$t('settings.retention.days', { count: 14 })}</option>
            <option value={30}>{$t('settings.retention.days', { count: 30 })}</option>
            <option value={60}>{$t('settings.retention.days', { count: 60 })}</option>
            <option value={90}>{$t('settings.retention.days', { count: 90 })}</option>
            <option value={180}>{$t('settings.retention.days', { count: 180 })}</option>
          </select>
        </div>
      {/if}
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">📸 {$t('settings.history.section')}</h3>
      <div class="field">
        <label for="s-max-snaps">{$t('settings.history.autoLimit')}</label>
        <input id="s-max-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxSnapshots} />
        <span class="hint">
          {$t('settings.history.autoHint')}
        </span>
      </div>
      <div class="field">
        <label for="s-max-manual-snaps">{$t('settings.history.manualLimit')}</label>
        <input id="s-max-manual-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxManualSnapshots} />
        <span class="hint">
          {$t('settings.history.manualHint')}
        </span>
      </div>
      <div class="field" style="margin-bottom: 0;">
        <div>
          <button class="btn small" disabled={pruning} on:click={cleanUpSnapshots}>
            {pruning ? $t('settings.cleanup.cleaning') : $t('settings.cleanup.action')}
          </button>
        </div>
        <span class="hint">
          {$t('settings.cleanup.hint')}
        </span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🔎 {$t('settings.scanner.section')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-scan-paths">{$t('settings.scanner.extraFolders')}</label>
        <span class="hint">{$t('settings.scanner.extraHint')}</span>
        {#each draft.customScanPaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small danger" on:click={() => removeScanPath(i)}>✕</button>
          </div>
        {/each}
        <button id="s-scan-paths" class="btn small" on:click={addScanPath}>{$t('settings.scanner.addFolder')}</button>
      </div>

      <div class="field" style="margin: 18px 0 0;">
        <label for="s-exclude-paths">{$t('settings.scanner.excludeFolders')}</label>
        <span class="hint">{$t('settings.scanner.excludeHint')}</span>
        {#each draft.excludePaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small danger" on:click={() => removeExcludePath(i)}>✕</button>
          </div>
        {/each}
        <button id="s-exclude-paths" class="btn small" on:click={addExcludePath}>{$t('settings.scanner.excludeFolder')}</button>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🧹 {$t('settings.reset.section')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">{$t('settings.reset.hint')}</span>
        <button
          class="btn small danger"
          style="margin-top: 12px; width: fit-content; align-self: flex-start;"
          on:click={resetTracking}
          disabled={resetting || $gameList.length === 0}
        >
          {#if resetting}{$t('settings.reset.working')}{:else if $gameList.length === 0}{$t('settings.reset.empty')}{:else}{$t('settings.reset.action', { count: $gameList.length })}{/if}
        </button>
      </div>
    </div>
  {:else if tab === 'advanced'}
    <div class="card">
      <h3 class="section-title">🧰 {$t('settings.advanced.section')}</h3>
      <p class="hint advanced-intro">{$t('settings.advanced.intro')}</p>
      <div class="advanced-links">
        <button class="advanced-link" on:click={() => navigate('devices')}>
          <span class="advanced-icon" aria-hidden="true">🖥️</span>
          <span class="advanced-copy">
            <strong>{$t('settings.advanced.deviceSync')}</strong>
            <small>{$t('settings.advanced.deviceSyncHint')}</small>
          </span>
          <span class="advanced-actions">
            {#if $pairingRequests.length > 0}
              <span class="advanced-badge">
                {$t('settings.advanced.pendingPairings', { count: $pairingRequests.length })}
              </span>
            {/if}
            <span class="advanced-open">{$t('settings.advanced.open')} →</span>
          </span>
        </button>
        <button class="advanced-link" on:click={() => navigate('changelog')}>
          <span class="advanced-icon" aria-hidden="true">📄</span>
          <span class="advanced-copy">
            <strong>{$t('settings.advanced.changelog')}</strong>
            <small>{$t('settings.advanced.changelogHint')}</small>
          </span>
          <span class="advanced-open">{$t('settings.advanced.open')} →</span>
        </button>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">⚙️ {$t('settings.network.section')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-port">{$t('settings.network.port')}</label>
        <input id="s-port" type="number" bind:value={draft.port} />
        <span class="hint">{$t('settings.network.portHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title">🔀 {$t('settings.paths.section')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">{$t('settings.paths.hint')}</span>
        {#each draft.pathTranslations ?? [] as rule, i}
          <div class="rule-row">
            <input placeholder={$t('settings.paths.from')} bind:value={rule.fromPattern} />
            <span class="arrow">→</span>
            <input placeholder={$t('settings.paths.to')} bind:value={rule.toPattern} />
            <button class="btn small danger" on:click={() => removeRule(i)}>✕</button>
          </div>
        {/each}
        <button id="s-rules" class="btn small" on:click={addRule}>{$t('settings.paths.add')}</button>
      </div>
    </div>
  {/if}

  <div class="save-bar">
    <button class="btn primary" disabled={busy} on:click={save}>{$t('settings.saveChanges')}</button>
  </div>
{/if}

<style>
  .head {
    margin-bottom: 18px;
  }
  .quiet {
    color: var(--text-faint);
  }
  /* .check and the checkbox itself are styled globally in app.css. */
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .rule-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .rule-row input {
    flex: 1;
    padding: 7px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
    outline: none;
  }
  .rule-path {
    flex: 1;
    font-size: 0.83rem;
    color: var(--text-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    color: var(--text-faint);
  }
  .section-title {
    font-size: 0.95rem;
    font-weight: 600;
    margin-bottom: 14px;
    padding-bottom: 10px;
    border-bottom: 1px solid var(--border);
    color: var(--text);
  }
  .mono {
    font-family: ui-monospace, 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  input[readonly] {
    opacity: 0.75;
    cursor: default;
  }
  .share-banner {
    margin-top: 12px;
    background: rgba(138, 99, 244, 0.06);
    border: 1px solid rgba(138, 99, 244, 0.28);
    border-radius: var(--radius);
    padding: 12px 14px;
    font-size: 0.82rem;
  }
  .share-title {
    font-weight: 600;
    margin-bottom: 6px;
  }
  .share-row {
    color: var(--text-dim);
    margin-top: 3px;
  }
  .share-row span {
    color: var(--text-faint);
    display: inline-block;
    width: 78px;
  }
  .save-bar {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }
  .advanced-intro {
    margin: -2px 0 14px;
  }
  .advanced-links {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 10px;
  }
  .advanced-link {
    min-width: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    padding: 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    cursor: pointer;
    text-align: left;
  }
  .advanced-link:hover,
  .advanced-link:focus-visible {
    border-color: var(--border-strong);
    background: var(--bg-hover);
  }
  .advanced-link:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .advanced-icon {
    width: 34px;
    height: 34px;
    display: grid;
    place-items: center;
    border-radius: 9px;
    background: var(--accent-soft);
  }
  .advanced-copy {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .advanced-copy strong {
    font-size: 0.9rem;
  }
  .advanced-copy small {
    color: var(--text-faint);
    line-height: 1.4;
  }
  .advanced-badge {
    padding: 2px 8px;
    border-radius: 999px;
    background: rgba(251, 191, 36, 0.13);
    color: var(--warn);
    font-size: 0.72rem;
    font-weight: 600;
    white-space: nowrap;
  }
  .advanced-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .advanced-open {
    color: var(--accent);
    font-size: 0.78rem;
    white-space: nowrap;
  }
</style>
