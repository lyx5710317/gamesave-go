<script>
  import { logEntries } from '../lib/stores.js';
  import { locale, t } from '../lib/i18n.js';
  import CloudUploadActivity from '../components/CloudUploadActivity.svelte';

  export let params = {};
  $: params;
  let currentTab = 'cloud';

  const colors = { info: 'var(--text-dim)', warn: 'var(--warn)', error: 'var(--danger)', success: 'var(--success)' };
  const levelKeys = {
    info: 'activity.levels.info',
    warn: 'activity.levels.warn',
    error: 'activity.levels.error',
    success: 'activity.levels.success'
  };
  const fmtTime = (timestamp, language) => new Date(timestamp).toLocaleTimeString(language);

  // Stick to the bottom as new entries arrive. Implemented as an action
  // (node is a local, uninstrumented reference): assigning scrollTop on a
  // bind:this variable would $$invalidate it and loop the reactive flush
  // forever, freezing the app.
  function autoscroll(node, _count) {
    const toBottom = () => queueMicrotask(() => (node.scrollTop = node.scrollHeight));
    toBottom();
    return { update: toBottom };
  }
</script>

<div class="head">
  <h2 class="page-title">{$t('activity.title')}</h2>
  <div class="activity-tabs" role="group" aria-label={$t('activity.title')}>
    <button class="btn small" class:primary={currentTab === 'cloud'} aria-pressed={currentTab === 'cloud'} on:click={() => currentTab = 'cloud'}>{$t('activity.cloud')}</button>
    <button class="btn small" class:primary={currentTab === 'log'} aria-pressed={currentTab === 'log'} on:click={() => currentTab = 'log'}>{$t('activity.log')}</button>
  </div>
</div>

{#if currentTab === 'cloud'}
  <CloudUploadActivity onActivityPage />
{:else}
<div class="card log" use:autoscroll={$logEntries.length}>
  {#if $logEntries.length === 0}
    <div class="empty"><h3>{$t('activity.empty.title')}</h3><p>{$t('activity.empty.body')}</p></div>
  {:else}
    {#each $logEntries as entry}
      <div class="line">
        <span class="time">{fmtTime(entry.timestamp, $locale)}</span>
        <span class="level" style="color: {colors[entry.level] ?? 'var(--text-dim)'}">
          {levelKeys[entry.level] ? $t(levelKeys[entry.level]) : entry.level}
        </span>
        <span class="msg">{entry.message}</span>
      </div>
    {/each}
  {/if}
</div>
{/if}

<style>
  .head {
    margin-bottom: 18px;
  }
  .activity-tabs { display: flex; gap: 8px; margin-top: 10px; }
  .log {
    height: calc(100vh - var(--titlebar-h) - var(--statusbar-h) - 172px);
    overflow-y: auto;
    font-family: 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.8rem;
    padding: 14px;
  }
  .line {
    display: flex;
    gap: 10px;
    padding: 2px 0;
    align-items: baseline;
  }
  .time {
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .level {
    width: 56px;
    flex-shrink: 0;
    font-weight: 600;
  }
  .msg {
    color: var(--text);
    word-break: break-word;
    user-select: text;
  }
</style>
