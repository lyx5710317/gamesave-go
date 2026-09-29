<script>
  import { settings, wanRoom, peers, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { generateRoomCode } from '../lib/roomcode.js';
  import { locale, t } from '../lib/i18n.js';

  let codeDraft = '';
  let relayDraft = '';
  let busy = false;
  let health = null;

  // Seeded once, when the fields are still blank — which includes every time
  // this view is rebuilt after a tab switch, because the drafts are component
  // state and do not survive that.
  //
  // The room code comes from the live room in preference to the saved setting.
  // They normally agree, but the room is the thing actually joined, and it can
  // be changed from the CLI or another window; showing the stored code instead
  // would tell you that you are in a room you have already left.
  $: if ($settings && codeDraft === '' && relayDraft === '') {
    codeDraft = $wanRoom?.roomCode || ($settings.syncCode ?? '');
    relayDraft = $settings.relayUrl ?? '';
  }

  // Every save here has to put the server's answer back into the store.
  // Nothing else does: the store is filled by the `init` message at launch and
  // by the Settings view saving its own form, so a relay or room code changed
  // from this view stayed invisible to the rest of the app — and to this view
  // itself once a tab switch destroyed it and the fields above re-seeded from
  // the settings as they were before the change.
  async function saveSettings(patch) {
    settings.set(await api.post('/api/settings', patch));
  }
  $: pairedIds = new Set(Object.keys($peers));
  $: roomPeers = $wanRoom?.peers ?? [];

  function randomCode() {
    codeDraft = generateRoomCode();
  }

  async function run(fn, okMsg) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (okMsg) toast(okMsg, 'success');
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('internet.operationFailed') : e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // Each outcome gets its own toast — joining is announced as in-progress
  // (the status banner reports the eventual result), leaving as completed.
  const joinRoom = () =>
    run(async () => {
      const code = codeDraft.trim();
      if (!code) {
        if ($wanRoom?.enabled) {
          await saveSettings({ syncCode: '', relayUrl: relayDraft.trim() });
          toast($t('internet.leftRoom'), 'success');
        } else {
          toast($t('internet.enterCode'), 'error');
        }
        return;
      }
      const rejoining = code === ($wanRoom?.roomCode ?? '') && relayDraft.trim() === ($settings?.relayUrl ?? '');
      await saveSettings({ syncCode: code, relayUrl: relayDraft.trim() });
      // Saving identical settings doesn't re-dial, so force a fresh attempt.
      if (rejoining) await api.post('/api/relay/reconnect');
      toast(rejoining ? $t('internet.reconnectingRoom', { code }) : $t('internet.joiningRoom', { code }));
    });

  const retryConnect = () =>
    run(async () => {
      await api.post('/api/relay/reconnect');
      toast($t('internet.reconnecting'));
    });
  const leaveRoom = () => {
    codeDraft = '';
    return run(() => saveSettings({ syncCode: '' }), $t('internet.leftRoom'));
  };
  const pairWan = (p) =>
    run(() => api.post('/api/peers/pair', { peerId: p.id, address: 'relay' }), $t('devices.pairSentTo', { name: p.deviceName }));

  async function checkHealth() {
    health = null;
    try {
      health = await api.get('/api/relay/health');
    } catch (e) {
      toast($locale === 'zh-CN' ? $t('internet.operationFailed') : e.message, 'error');
    }
  }

  function copyCode() {
    navigator.clipboard?.writeText($wanRoom?.roomCode ?? codeDraft);
    toast($t('internet.codeCopied'));
  }
</script>

<p class="lead">
  {$t('internet.intro')}
</p>

<!-- Said here, at the point where somebody decides whether to use the public
     relay, rather than only in the documentation.

     The encryption is transport-level and ends AT the relay: there is no
     end-to-end layer, so the relay process handles save data in the clear.
     Calling that "an encrypted tunnel" and stopping was true enough to be
     misleading — it invites the reading that nobody in the middle can see the
     save, which is the opposite of the case. Whoever runs the relay is being
     trusted, and the person choosing is the one who should get to weigh it. -->
<p class="lead subtle">
  {$t('internet.trustWarning')}
</p>

<!-- Always-visible connection status: exactly one of four states. Keyed
     off the live wanRoom broadcast (settings only arrive at WS init and
     would go stale after a join). -->
{#if !$wanRoom?.enabled}
  <div class="status idle">
    <span class="status-dot gray"></span>
    <div class="status-text">
      <strong>{$t('internet.notInRoom')}</strong>
      <span>{$t('internet.notInRoomHint')}</span>
    </div>
  </div>
{:else if $wanRoom?.connected}
  <div class="status ok">
    <span class="status-dot green"></span>
    <div class="status-text">
      <strong>{$t('internet.inRoom', { code: $wanRoom.roomCode })}</strong>
      <span>{roomPeers.length === 0 ? $t('internet.waitingForPeer') : $t('internet.peerCount', { count: roomPeers.length })}</span>
    </div>
  </div>
{:else if $wanRoom?.state === 'connecting'}
  <div class="status wait">
    <span class="sspin"></span>
    <div class="status-text">
      <strong>{$t('internet.connecting')}</strong>
      <span>{$t('internet.connectingHint')}</span>
    </div>
  </div>
{:else}
  <div class="status err">
    <span class="status-dot red"></span>
    <div class="status-text">
      <strong>{$t('internet.disconnected')}</strong>
      <span>{$t('internet.disconnectedHint')}</span>
    </div>
    <button class="btn small" disabled={busy} on:click={retryConnect}>{$t('internet.retry')}</button>
  </div>
{/if}

<div class="card">
  <h3>{$t('internet.roomSection')}</h3>
  <div class="row">
    <div class="field grow">
      <label for="room-code">{$t('internet.roomCode')}</label>
      <div class="code-row">
        <input id="room-code" placeholder="e.g. k7m2-9xqp-4wnt" bind:value={codeDraft} />
        <button class="btn" on:click={randomCode}>🎲</button>
        {#if $wanRoom?.enabled}
          <button class="btn" on:click={copyCode}>{$t('internet.copy')}</button>
        {/if}
      </div>
    </div>
  </div>
  <div class="row">
    <div class="field grow">
      <label for="relay-url">{$t('internet.server')}</label>
      <!-- Pinned by the environment: shown, because it is the relay actually
           in use, but not editable, because typing here would be discarded. -->
      <input id="relay-url" bind:value={relayDraft} readonly={$settings?.relayUrlLocked} />
      {#if $settings?.relayUrlLocked}
        <span class="hint">
          {$t('internet.lockedBefore')} <code>OPENSAVE_RELAY_URL</code> {$t('internet.lockedAfter')}
        </span>
      {:else}
        <span class="hint">{$t('internet.selfHostHint')}</span>
      {/if}
    </div>
  </div>
  <div class="actions">
    <button class="btn" on:click={checkHealth}>{$t('internet.testRelay')}</button>
    {#if $wanRoom?.enabled}
      <button class="btn danger" disabled={busy} on:click={leaveRoom}>{$t('internet.leaveRoom')}</button>
    {/if}
    <button class="btn primary" disabled={busy} on:click={joinRoom}>
      {$wanRoom?.enabled ? $t('internet.update') : $t('internet.joinRoom')}
    </button>
  </div>
  {#if health}
    <div class="health" class:ok={health.reachable}>
      {#if health.reachable}
        ✓ {$t('internet.reachable', { clients: health.health?.clients ?? 0, rooms: health.health?.rooms ?? 0 })}
      {:else}
        ✕ {$t('internet.unreachable')}
      {/if}
    </div>
  {/if}
</div>

{#if $wanRoom?.enabled}
  <h3 class="section">{$t('internet.inThisRoom')}</h3>
  {#if roomPeers.length === 0}
    <p class="quiet">
      {$t('internet.noPeers')}
    </p>
  {:else}
    <div class="list">
      {#each roomPeers as p (p.id)}
        <div class="card peer">
          <div class="peer-icon">{p.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
          <div class="peer-info">
            <div class="peer-name">
              {p.deviceName}
              <span class="badge" class:online={p.online} class:offline={!p.online}>{p.online ? $t('devices.online') : $t('internet.away')}</span>
            </div>
          </div>
          {#if p.paired || pairedIds.has(p.id)}
            <span class="badge online">{$t('internet.paired')}</span>
          {:else}
            <button class="btn small primary" disabled={busy} on:click={() => pairWan(p)}>{$t('devices.pair')}</button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .lead {
    color: var(--text-dim);
    font-size: 0.9rem;
    max-width: 640px;
    margin-bottom: 20px;
  }
  /* The trust note under the lead. Quieter than the sentence above it, and
     deliberately not styled as a warning: nothing is wrong, it is a fact about
     the arrangement that the person choosing a relay should have. */
  .lead.subtle {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin-top: -12px;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 18px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--bg-raised);
    margin-bottom: 18px;
  }
  .status.ok {
    border-color: rgba(74, 222, 128, 0.4);
    background: rgba(74, 222, 128, 0.07);
  }
  .status.wait {
    border-color: rgba(138, 99, 244, 0.45);
    background: var(--accent-soft);
  }
  .status.err {
    border-color: rgba(217, 87, 87, 0.45);
    background: rgba(217, 87, 87, 0.08);
  }
  .status-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.9rem;
  }
  .status-text span {
    color: var(--text-dim);
    font-size: 0.82rem;
  }
  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .status-dot.green {
    background: var(--success);
    box-shadow: 0 0 8px rgba(74, 222, 128, 0.6);
  }
  .status-dot.gray {
    background: var(--text-faint);
  }
  .status-dot.red {
    background: var(--danger);
  }
  .sspin {
    width: 15px;
    height: 15px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: sspin 0.8s linear infinite;
    flex-shrink: 0;
  }
  @keyframes sspin {
    to { transform: rotate(360deg); }
  }
  .card h3 {
    margin-bottom: 14px;
  }
  .row {
    display: flex;
    gap: 12px;
  }
  .grow {
    flex: 1;
  }
  .code-row {
    display: flex;
    gap: 8px;
  }
  .code-row input {
    flex: 1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .health {
    margin-top: 12px;
    padding: 10px 14px;
    border-radius: var(--radius);
    background: rgba(217, 87, 87, 0.12);
    color: #f1a3a3;
    font-size: 0.85rem;
  }
  .health.ok {
    background: rgba(74, 222, 128, 0.1);
    color: var(--success);
  }
  .section {
    margin: 22px 0 10px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.88rem;
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
  }
  .peer-name {
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
