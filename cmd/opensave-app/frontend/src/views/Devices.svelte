<script>
  import { peers, discoveredPeers, wanRoom, appUpdate, toast, askConfirm } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import InternetSync from './InternetSync.svelte';
  import { locale, t } from '../lib/i18n.js';

  export let params = {};

  let manualIp = '';
  let manualPort = 8383;
  let busy = false;
  // Which "add a device" method is shown. Deep-links can request the
  // internet tab; otherwise default to the local network.
  let connectTab = params.tab === 'internet' ? 'wan' : 'lan';

  $: pairedList = Object.values($peers).sort((a, b) => a.name.localeCompare(b.name));
  $: pairedIds = new Set(pairedList.map((p) => p.id));
  $: lanDiscovered = $discoveredPeers.filter((d) => !d.isWan && !pairedIds.has(d.id));

  async function run(fn, okMsg) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (okMsg) toast(okMsg, 'success');
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('devices.operationFailed') : e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const pairDiscovered = (d) =>
    run(() => api.post('/api/peers/pair', { address: d.address, port: d.port }), $t('devices.pairSentTo', { name: d.deviceName }));
  const pairManual = () =>
    run(() => api.post('/api/peers/pair', { address: manualIp, port: Number(manualPort) }), $t('devices.pairSent'));
  const unpair = async (peer) => {
    if (!(await askConfirm($t('devices.unpairConfirm', { name: peer.name }), { title: $t('devices.unpairTitle'), confirmText: $t('devices.unpair'), danger: true }))) return;
    run(() => api.del(`/api/peers/${peer.id}`), $t('devices.unpaired', { name: peer.name }));
  };

  // Peer-to-peer app update: pull the newer build the peer is running and
  // install it here — no manually copying the exe between machines.
  const updateFromPeer = async (peer) => {
    const ok = await askConfirm(
      $t('devices.updateConfirm', { name: peer.name, version: peer.appVersion }),
      { title: $t('devices.updateTitle'), confirmText: $t('devices.updateAction') }
    );
    if (!ok) return;
    const err = await native.installFromPeer(peer.id);
    if (err) toast($locale === 'zh-CN' ? $t('devices.operationFailed') : err, 'error');
  };

  const fmtTime = (time) => (time ? new Date(time).toLocaleString($locale) : $t('devices.never'));
</script>

<div class="head">
  <h2 class="page-title">{$t('devices.title')}</h2>
</div>

<h3 class="section">{$t('devices.pairedSection')}</h3>
{#if pairedList.length === 0}
  <div class="empty">
    <h3>{$t('devices.emptyTitle')}</h3>
    <p>{$t('devices.emptyHint')}</p>
  </div>
{:else}
  <div class="list">
    {#each pairedList as peer (peer.id)}
      <div class="card peer">
        <div class="peer-icon">{peer.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
        <div class="peer-info">
          <div class="peer-name">
            {peer.name}
            <span class="badge" class:online={peer.status === 'online'} class:offline={peer.status !== 'online'}>
              {$t(peer.status === 'online' ? 'devices.online' : 'devices.offline')}
            </span>
          </div>
          <div class="peer-meta">
            {peer.address === 'relay' ? $t('devices.internetRelay') : `🖧 ${peer.address}:${peer.port}`}
            · {$t('devices.lastSynced', { time: fmtTime(peer.lastSynced) })}
            {#if peer.appVersion}· GameSave Go {peer.appVersion}{/if}
          </div>
        </div>
        {#if peer.hasNewerBuild && peer.status === 'online'}
          <button class="btn small primary" disabled={busy || !!$appUpdate} on:click={() => updateFromPeer(peer)}>
            ⬆ {$t('devices.updateFromPeer')}
          </button>
        {/if}
        <button class="btn small danger" disabled={busy} on:click={() => unpair(peer)}>{$t('devices.unpair')}</button>
      </div>
    {/each}
  </div>
{/if}

<h3 class="section">{$t('devices.addSection')}</h3>
<div class="pill-tabs connect-tabs">
  <button class:active={connectTab === 'lan'} on:click={() => (connectTab = 'lan')}>🖧 {$t('devices.localTab')}</button>
  <button class:active={connectTab === 'wan'} on:click={() => (connectTab = 'wan')}>
    🌐 {$t('devices.internetTab')}
    {#if $wanRoom?.connected}<span class="tab-dot"></span>{/if}
  </button>
</div>

{#if connectTab === 'lan'}
  <p class="quiet lan-intro">
    {$t('devices.lanHint')}
  </p>
  <h4 class="subsection">{$t('devices.foundSection')}</h4>
  {#if lanDiscovered.length === 0}
    <p class="quiet">{$t('devices.noneFound')}</p>
  {:else}
    <div class="list">
      {#each lanDiscovered as d (d.id)}
        <div class="card peer">
          <div class="peer-icon">{d.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
          <div class="peer-info">
            <div class="peer-name">{d.deviceName}</div>
            <div class="peer-meta">{d.address}:{d.port}</div>
          </div>
          <button class="btn small primary" disabled={busy} on:click={() => pairDiscovered(d)}>{$t('devices.pair')}</button>
        </div>
      {/each}
    </div>
  {/if}

  <h4 class="subsection">{$t('devices.addByIp')}</h4>
  <div class="card manual">
    <input placeholder="192.168.1.42" bind:value={manualIp} />
    <input class="port" type="number" bind:value={manualPort} />
    <button class="btn primary" disabled={!manualIp || busy} on:click={pairManual}>{$t('devices.sendPair')}</button>
  </div>
{:else}
  <InternetSync />
{/if}

<style>
  .head {
    margin-bottom: 20px;
  }
  .section {
    margin: 22px 0 10px;
  }
  .subsection {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text-dim);
    margin: 18px 0 8px;
  }
  .connect-tabs {
    margin-bottom: 14px;
  }
  .connect-tabs button {
    display: inline-flex;
    align-items: center;
    gap: 7px;
  }
  .tab-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--success);
    box-shadow: 0 0 6px rgba(74, 222, 128, 0.7);
  }
  .lan-intro {
    margin-bottom: 4px;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .peer {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
  }
  .peer-icon {
    font-size: 1.3rem;
  }
  .peer-info {
    flex: 1;
    min-width: 0;
  }
  .peer-name {
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .peer-meta {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 2px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.88rem;
  }
  .manual {
    display: flex;
    gap: 10px;
    padding: 14px;
  }
  .manual input {
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
    flex: 1;
  }
  .manual .port {
    flex: 0 0 90px;
  }
</style>
