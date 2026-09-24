// Manual syncs can finish on one peer while another needs a folder chosen on
// that device. Surface the actionable peer instead of a generic success toast.
export function peerRequiringSavePath(response) {
  for (const [peerId, result] of Object.entries(response?.results ?? {})) {
    if (result?.status === 'path_mapping_required') return result.peerName || peerId;
  }
  return '';
}
