export function isAccountPath(raw:string|null|undefined):boolean{
 let path=raw??'';
 for(let i=0;i<3;i++){try{const next=decodeURIComponent(path);if(next===path)break;path=next;}catch{break;}}
 path=path.replaceAll('\\','/').toLowerCase();return path==='/account'||path.startsWith('/account/');
}
// Runs in <head> before any analytics. Bearers stay only in memory, never HTML,
// storage, history, query parameters or telemetry. Forms consume and delete this slot.
export const accountBootstrap=`(()=>{let p=location.pathname;for(let i=0;i<3;i++){try{let n=decodeURIComponent(p);if(n===p)break;p=n}catch{break}}p=p.replaceAll('\\\\','/').toLowerCase();if(p!=='/account'&&!p.startsWith('/account/'))return;let t='';try{let q=new URLSearchParams(location.hash.slice(1));if(q.getAll('token').length===1)t=q.get('token')||''}catch{}if(!/^v1\\.[A-Za-z0-9_-]{43}$/.test(t))t='';Object.defineProperty(window,'__dcfAccountEntry',{value:{token:t},configurable:true,writable:true});history.replaceState(history.state,'',location.pathname)})();`;
