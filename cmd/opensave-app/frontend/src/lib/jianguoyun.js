export const JIANGUOYUN_BASE_URL = 'https://dav.jianguoyun.com/dav/';
export const JIANGUOYUN_REMOTE_FOLDER = 'GameSaveGo/';

// Provider changes must not carry a password (or URL) from another service.
// The actual remote folder is enforced by the backend, not user-editable.
export function selectCloudProvider(config, provider) {
  if (provider === config.provider) return config;
  return {
    ...config,
    provider,
    url: provider === 'jianguoyun' ? JIANGUOYUN_BASE_URL : '',
    username: '',
    password: '',
    passwordConfigured: false
  };
}

// A fresh, empty local-folder default opens the recommended domestic setup
// without changing the persisted provider until the user explicitly saves.
export function recommendJianguoyunForUnset(config) {
  if (config.provider === 'jianguoyun') return { ...config, url: JIANGUOYUN_BASE_URL, password: '' };
  return config.provider === 'local' && !config.url
    ? selectCloudProvider(config, 'jianguoyun')
    : config;
}
