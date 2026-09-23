<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { gameList } from '../lib/stores.js';
  import { t } from '../lib/i18n.js';
  import { summarizeUploadActivity } from '../lib/uploadActivity.js';

  let uploads = [];
  let loadFailed = false;
  let refreshing = false;
  let disposed = false;
  $: counts = summarizeUploadActivity(uploads);

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

  function statusText(record) {
    if (record.status === 'running') return $t('cloud.activity.running');
    if (record.status === 'succeeded') return $t('cloud.activity.succeeded');
    if (record.failure === 'configuration') return $t('cloud.activity.configuration');
    if (record.failure === 'conflict') return $t('cloud.activity.conflict');
    return $t('cloud.activity.failed');
  }
</script>

<h3 class="section">{$t('cloud.activity.section')}</h3>
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
    <p class="quiet">{$t('cloud.activity.summary', { running: counts.running, succeeded: counts.succeeded, failed: counts.failed })}</p>
    <ul class="activity-list">
      {#each uploads as record (record.id)}
        <li>
          <span class="activity-game">{gameName(record.gameId)}{record.snapshotId ? ` · ${record.snapshotId}` : ''}</span>
          <span class:failed={record.status === 'failed'} class:running={record.status === 'running'}>
            {statusText(record)}
          </span>
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
  .quiet { color: var(--text-faint); font-size: 0.85rem; }
  .error, .failed { color: var(--danger, #e5484d); }
  .running { color: var(--accent); }
  .activity-list { list-style: none; margin: 12px 0 0; padding: 0; max-height: 300px; overflow: auto; }
  .activity-list li { display: flex; justify-content: space-between; gap: 14px; padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 0.87rem; }
  .activity-game { overflow-wrap: anywhere; }
</style>
