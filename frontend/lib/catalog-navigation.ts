const contextKey = 'devcourse-catalog-context';
const restoreKey = 'devcourse-catalog-restore';
type CatalogContext = {url:string; scrollY:number};

export function catalogContext():CatalogContext|null {
  try {
    const value = JSON.parse(sessionStorage.getItem(contextKey)||'null');
    if (!value || typeof value.url !== 'string' || !Number.isFinite(value.scrollY) || value.scrollY < 0) return null;
    const url = new URL(value.url, location.origin);
    if (url.origin !== location.origin || !['/', '/courses'].includes(url.pathname)) return null;
    return {url:url.pathname+url.search, scrollY:value.scrollY};
  } catch {return null;}
}

export function rememberCatalog() {
  if (!['/', '/courses'].includes(location.pathname)) return;
  try {sessionStorage.setItem(contextKey, JSON.stringify({url:location.pathname+location.search, scrollY:window.scrollY}));} catch {}
}

export function requestCatalogRestore() {
  try {sessionStorage.setItem(restoreKey, 'true');} catch {}
}

export function takeCatalogRestore():CatalogContext|null {
  try {
    if (sessionStorage.getItem(restoreKey) !== 'true') return null;
    sessionStorage.removeItem(restoreKey);
    const context=catalogContext();
    return context?.url === location.pathname+location.search ? context : null;
  } catch {return null;}
}
