export const PRIMARY_NAV = [
  {
    id: 'home',
    labelKey: 'nav.games',
    icon: 'M3 10.5 L10 4 L17 10.5 M5 9 V16 H8.5 V12 H11.5 V16 H15 V9'
  },
  {
    id: 'cloud',
    labelKey: 'nav.cloud',
    icon: 'M6 14 a3.5 3.5 0 0 1 0 -7 a4.5 4.5 0 0 1 8.6 1.2 A3 3 0 0 1 14 14 z'
  },
  {
    id: 'activity',
    labelKey: 'nav.activity',
    icon: 'M3 10 h3 l2 -5 l3 10 l2 -5 h4'
  },
  {
    id: 'settings',
    labelKey: 'nav.settings',
    icon: 'M10 7 a3 3 0 1 0 0 6 a3 3 0 1 0 0 -6 M10 2.5 v2 M10 15.5 v2 M2.5 10 h2 M15.5 10 h2 M4.6 4.6 l1.4 1.4 M14 14 l1.4 1.4 M15.4 4.6 L14 6 M6 14 l-1.4 1.4'
  }
];

// These routes remain intact but live under Settings instead of competing
// with the four everyday Windows workflows in the primary navigation.
export const SETTINGS_OWNED_VIEWS = new Set(['settings', 'devices', 'changelog']);

export function primaryNavIsActive(currentView, itemId) {
  if (itemId === 'settings') return SETTINGS_OWNED_VIEWS.has(currentView);
  return currentView === itemId;
}
