<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { gameList, navigate } from '../lib/stores.js';
  import { locale, t } from '../lib/i18n.js';
  import { filterUploadActivity, summarizeUploadActivity, uploadNextStep, uploadStatusText } from '../lib/uploadActivity.js';

  export let onActivityPage = false;

  const providerKeys = {
    jianguoyun: 'jianguoyun', google_drive: 'googleDrive', onedrive: 'oneDrive',
    dropbox: 'dropbox', local: 'local', webdav: 'webdav', webhook: 'webhook', baidu: 'baidu'
  };

  let uploads = [];
  let loadFailed = false;
  let refreshing = false;
  let disposed = false;
  let failedOnly = false;
  $: counts = summarizeUploadActivity(uploads);
  $: visibleUploads = filterUploadActivity(uploads, failedOnly);

  async function refresh() {
    if (refreshing) return;
    refreshing = true;
    try {
      const response = await api.get('/api/cloud/uploads');
      if (!Array.isArray(response.uploads)) throw new Error('invalid upload activity');
      if (!disposed) {
        uploads = response.uploads;
        loadFailed = false;
      }
    } catch {
      if (!disposed) loadFailed = true;
    } finally {
      refreshing = false;
    }
  }

  onMount(() => {
    refresh();
    const timer = setInterval(refresh, 3000);
    return () => { disposed = true; clearInterval(timer); };
  });

  function gameName(id) {
    return $gameList.find((game) => game.id === id)?.name || id || $t('cloud.activity.unknownGame');
  }

  function formatTime(value, language) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '' : date.toLocaleString(language);
  }
</script>

<div class="card activity-card">
  <div class="activity-head">
    <div>
      <h3>{$t('cloud.activity.title')}</h3>
      <p class="quiet">{$t('cloud.activity.hint')}</p>
    </div>
    <button class="btn small" disabled={refreshing} on:click={refresh}>{$t('cloud.activity.refresh')}</button>
  </div>
  {#if loadFailed}<p class="error" role="alert">{$t('cloud.activity.loadFailed')}</p>{/if}
  {#if uploads.length}
    <div class="activity-head summary">
      <p class="quiet">{$t('cloud.activity.summary', { running: counts.running, succeeded: counts.succeeded, failed: counts.failed })}</p>
      <label><input type="checkbox" bind:checked={failedOnly} /> {$t('cloud.activity.failuresOnly')}</label>
    </div>
    {#if !visibleUploads.length}<p class="quiet">{$t('cloud.activity.noFailures')}</p>{/if}
    <ul class="activity-list">
      {#each visibleUploads as record (record.id)}
        <li>
          <div class="activity-game">
            <strong>{gameName(record.gameId)}</strong>
            <div class="quiet">
              {Object.hasOwn(providerKeys, record.provider) ? $t(`cloud.providers.${providerKeys[record.provider]}`) : $t('cloud.activity.unknownProvider')}
              · {formatTime(record.finishedAt || record.startedAt, $locale)}
              {record.snapshotId ? ` · ${record.snapshotId}` : ''}
            </div>
          </div>
          <div class="activity-result">
            <div class:failed={record.status === 'failed'} class:running={record.status === 'running'}>
              {uploadStatusText(record, $t)}
            </div>
            {#if record.status === 'failed'}
              <p class="quiet next-step">{uploadNextStep(record, $t)}</p>
              {#if onActivityPage}<button class="btn small" on:click={() => navigate('cloud')}>{$t('cloud.activity.openCloud')}</button>{/if}
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {:else if !loadFailed}
    <p class="quiet">{$t('cloud.activity.empty')}</p>
  {/if}
</div>

<style>
  .activity-card { padding: 18px 20px; }
  .activity-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .activity-head h3 { margin: 0 0 5px; }
  .activity-head p { margin: 0; }
  .summary { margin-top: 12px; flex-wrap: wrap; }
  .summary label { font-size: 0.85rem; }
  .quiet { color: var(--text-faint); font-size: 0.85rem; }
  .error, .failed { color: var(--danger, #e5484d); }
  .running { color: var(--accent); }
  .activity-list { list-style: none; margin: 12px 0 0; padding: 0; max-height: 300px; overflow: auto; }
  .activity-list li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 14px; padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 0.87rem; }
  .activity-game { overflow-wrap: anywhere; }
  .activity-result { overflow-wrap: anywhere; }
  .next-step { margin: 4px 0 6px; }
  @media (max-width: 680px) { .activity-list li { grid-template-columns: 1fr; gap: 4px; } }
</style>
