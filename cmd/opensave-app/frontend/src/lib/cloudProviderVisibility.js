// Product visibility only. The provider implementations and existing saved
// connections remain intact so a temporary UI decision cannot erase data.
const temporarilyHidden = new Set(['baidu', 'onedrive', 'dropbox']);

export const isTemporarilyHiddenProvider = (provider) => temporarilyHidden.has(provider);

export const visibleCloudProviders = (providers) =>
  providers.filter((provider) => !isTemporarilyHiddenProvider(provider.id));
