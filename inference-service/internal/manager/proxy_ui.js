(() => {
  'use strict';
  const prefix = window.__modelProxyPrefix;
  const parts = prefix.split('/').filter(Boolean);
  const namespace = 'inference:' + encodeURIComponent(parts[parts.length - 2]) + ':' + encodeURIComponent(parts[parts.length - 1]) + ':';

  // IndexedDB is scoped by origin, not by URL path. Namespace every database
  // operation before llama UI (including Dexie) is initialized.
  const factory = window.indexedDB;
  const open = factory.open.bind(factory);
  const remove = factory.deleteDatabase.bind(factory);
  factory.open = (name, ...args) => open(namespace + String(name), ...args);
  factory.deleteDatabase = (name, ...args) => remove(namespace + String(name), ...args);
  if (factory.databases) {
    const databases = factory.databases.bind(factory);
    factory.databases = async () => (await databases())
      .filter(db => db.name && db.name.startsWith(namespace))
      .map(db => ({ ...db, name: db.name.slice(namespace.length) }));
  }

  // The UI stores settings in localStorage; direct properties and methods must
  // use the same namespace and clear() must not remove another deployment.
  const storage = window.localStorage;
  const storageKeys = () => Array.from({ length: storage.length }, (_, i) => storage.key(i))
    .filter(key => key !== null && key.startsWith(namespace));
  const scopedStorage = new Proxy(storage, {
    get(target, prop) {
      if (prop === 'length') return storageKeys().length;
      if (prop === 'key') return i => { const k = storageKeys()[i]; return k === undefined ? null : k.slice(namespace.length); };
      if (prop === 'getItem') return key => storage.getItem(namespace + String(key));
      if (prop === 'setItem') return (key, value) => storage.setItem(namespace + String(key), value);
      if (prop === 'removeItem') return key => storage.removeItem(namespace + String(key));
      if (prop === 'clear') return () => storageKeys().forEach(key => storage.removeItem(key));
      if (typeof prop === 'string') return storage.getItem(namespace + prop) ?? undefined;
      return Reflect.get(target, prop, target);
    },
    set(_target, prop, value) { storage.setItem(namespace + String(prop), value); return true; },
    deleteProperty(_target, prop) { storage.removeItem(namespace + String(prop)); return true; }
  });
  Object.defineProperty(window, 'localStorage', { configurable: true, value: scopedStorage });

  // llama UI uses root-relative API URLs. Keep requests to the current origin
  // inside this deployment while leaving external URLs untouched.
  function scopedURL(input) {
    const url = new URL(input, location.href);
    if (url.origin === location.origin && !url.pathname.startsWith(prefix)) {
      url.pathname = prefix + url.pathname.replace(/^\//, '');
    }
    return url.href;
  }
  const fetchOriginal = window.fetch.bind(window);
  window.fetch = (input, init) => {
    if (input instanceof Request) return fetchOriginal(new Request(scopedURL(input.url), input), init);
    return fetchOriginal(scopedURL(input), init);
  };
  const xhrOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, ...rest) {
    return xhrOpen.call(this, method, scopedURL(url), ...rest);
  };
})();
